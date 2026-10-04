package user

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"

	"GoTenon"
	userv1 "StarDreamerChaosmos/api/gen/user/v1"
	"StarDreamerChaosmos/common"
	"StarDreamerChaosmos/common/cap"
	"StarDreamerChaosmos/core/config"
	"StarDreamerChaosmos/dialer"
	"StarDreamerChaosmos/gateway"
)

// Plugin 是用户组件:开库 → 注册 gRPC 服务端到拨号器 → 用生成的 client 注册路由,
// 并通过"消息"对外提供鉴权能力函数指针(见 common/cap)。
// 自身资源与业务配置(DB / JWT 等)整体从 config 的 user 段读取。
type Plugin struct {
	mu     sync.Mutex
	dbPath string
	db     *gorm.DB

	caps *cap.Provider // 对外提供的能力表(鉴权等)
}

// NewPlugin 创建用户组件。
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "user" }
func (p *Plugin) Desc() map[string]string {
	return map[string]string{"provides": "user.v1.UserService + 鉴权能力", "inject": "config,dialer,gateway"}
}
func (p *Plugin) Inject() []string { return []string{"config", "dialer", "gateway"} }

func (p *Plugin) Status() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return map[string]any{"state": "ready", "db": p.dbPath}
}

func (p *Plugin) Register() error { return nil }

func (p *Plugin) Apply(ctx *GoTenon.GoTenonContext, _ any) error {
	// 1. 从 config 的 user 段读配置,整体反序列化为 UserConfig
	var uc UserConfig
	if cs := ctx.SlotOf(config.ServiceName); cs != nil && cs.Value != nil {
		if cfg, ok := cs.Value.(*config.Config); ok {
			uc = decodeUserConfig(cfg.Section("user"))
		}
	}
	// 2. 按「写库列表 + 读库列表」装配数据库(读写分离;读库为空时读也走写库)
	db, dbPath, err := openDB(uc)
	if err != nil {
		return err
	}

	// 3. 迁移:仅当本次启动带 initdb 指令(宿主写入启动选项)时才执行
	if s := ctx.SlotOf(common.BootService); s != nil {
		if b, ok := s.Value.(common.Boot); ok && b.InitDB {
			if err := db.AutoMigrate(&UserModel{}, &UserConfModel{}); err != nil {
				return fmt.Errorf("user: 迁移失败: %w", err)
			}
			fmt.Println("[user] 已执行数据库迁移")
		}
	}
	p.mu.Lock()
	p.dbPath = dbPath
	p.db = db
	p.mu.Unlock()

	d := ctx.SlotOf(dialer.ServiceName).Value.(*dialer.Dialer)

	// 4. 建 Redis 客户端(可选):黑名单缓存的共享后端;未配置则仅用本地缓存。
	var rdb *redis.Client
	if uc.Redis.Addr != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr:     uc.Redis.Addr,
			Password: uc.Redis.Password,
			DB:       uc.Redis.DB,
		})
	}

	// 5. 把生成的 gRPC 服务端注册到拨号器的本地 server
	tokCfg := uc.TokenConfig()
	srv := &Server{db: db, tok: tokCfg, c: &cache{rdb: rdb, tok: tokCfg}}
	userv1.RegisterUserServiceServer(d.Server(), srv)

	// 6. 经拨号器取连接 → 生成的 gRPC client
	cc, err := d.Dial(context.Background(), "user")
	if err != nil {
		return fmt.Errorf("user: 取连接失败: %w", err)
	}
	cli := userv1.NewUserServiceClient(cc)

	// 7. 用消息注册路由:投进网关信箱,网关收消息即注册;组件下线自动摘除
	gw := ctx.SlotOf(gateway.ServiceName).Value.(*gateway.Router)
	_ = gw.Submit(gateway.RouteMessage("user", "POST", "/api/user/register", handleRegister(cli)))
	_ = gw.Submit(gateway.RouteMessage("user", "POST", "/api/user/login", handleLogin(cli)))
	_ = gw.Submit(gateway.RouteMessage("user", "POST", "/api/user/refresh", handleRefresh(cli)))
	_ = gw.Submit(gateway.RouteMessage("user", "POST", "/api/user/logout", handleLogout(cli)))

	// 8. 通过消息对外提供鉴权能力函数指针(消费者按能力键请求;见 common/cap)
	//    应答经"请求内嵌回调"同步交回,故提供方无需持有总线。
	p.caps = cap.NewProvider()
	p.caps.Provide(cap.KeyAuthParse, cap.ParseFunc(srv.ParseAccess))
	p.caps.Provide(cap.KeyAuthRequireLogin, cap.RequireLoginFunc(srv.RequireLogin))
	p.caps.Provide(cap.KeyAuthRequireRoles, cap.RequireRolesFunc(srv.RequireRoles))
	p.caps.Provide(cap.KeyAuthIsRevoked, cap.IsRevokedFunc(srv.IsRevoked))
	p.caps.Provide(cap.KeyAuthRoleOf, cap.RoleOfFunc(srv.RoleOf))

	// 9. 卸载:关库、关 Redis
	ctx.Register(func() error {
		if rdb != nil {
			_ = rdb.Close()
		}
		sqlDB, err := db.DB()
		if err == nil {
			return sqlDB.Close()
		}
		return err
	})

	fmt.Printf("[user] 就绪(db=%s)\n", dbPath)
	return nil
}

func (p *Plugin) Start() error { return nil }
func (p *Plugin) Run() error   { return nil }
func (p *Plugin) End() error   { return nil }

// DealWithMessage 处理发往本组件的消息:优先分流"能力请求"(鉴权函数指针外借)。
func (p *Plugin) DealWithMessage(m GoTenon.Message) error {
	if p.caps != nil && p.caps.Handled(m) {
		return nil
	}
	return nil
}

func (p *Plugin) Function() map[string]any { return nil }
func (p *Plugin) ExecuteFunction(any)      {}

// openDB 按「写库列表 + 读库列表」装配数据库连接(读写分离):
//   - 至少一个写库;缺省为单个 sqlite(data/sdc.db);
//   - 读库非空时注册读写分离:读走 Replicas(随机负载均衡),写走 Sources;
//   - 读库为空时,读请求也落到写库。
//
// 返回主连接、主库名(用于展示)。
func openDB(uc UserConfig) (*gorm.DB, string, error) {
	writes := uc.DBWrite
	if len(writes) == 0 {
		writes = []DB{{SqlName: DBSqliteMode, DBName: filepath.Join("data", "sdc.db")}}
	}
	// 缺省字段 + 建父目录
	for i := range writes {
		if writes[i].SqlName == "" {
			writes[i].SqlName = DBSqliteMode
		}
		if writes[i].DBName == "" {
			writes[i].DBName = filepath.Join("data", "sdc.db")
		}
		if dir := filepath.Dir(writes[i].DBName); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0o755)
		}
	}

	// 所有库的驱动必须一致(参考做法)
	all := make([]DB, 0, len(writes)+len(uc.DBRead))
	all = append(all, writes...)
	all = append(all, uc.DBRead...)
	for i := 1; i < len(all); i++ {
		if all[i].SqlName != all[0].SqlName {
			return nil, "", fmt.Errorf("user: 数据库配置错误: 驱动不一致(%s != %s)", all[i].SqlName, all[0].SqlName)
		}
	}

	sources := make([]gorm.Dialector, len(writes))
	for i := range writes {
		sources[i] = writes[i].DSN()
	}
	db, err := gorm.Open(sources[0], &gorm.Config{})
	if err != nil {
		return nil, "", fmt.Errorf("user: 打开数据库失败: %w", err)
	}
	// 连接池(参考做法)
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	if len(uc.DBRead) > 0 {
		replicas := make([]gorm.Dialector, len(uc.DBRead))
		for i := range uc.DBRead {
			replicas[i] = uc.DBRead[i].DSN()
		}
		if err := db.Use(dbresolver.Register(dbresolver.Config{
			Sources:  sources,                   // 写库
			Replicas: replicas,                  // 读库
			Policy:   dbresolver.RandomPolicy{}, // 读库之间随机负载均衡
		})); err != nil {
			return nil, "", fmt.Errorf("user: 配置读写分离失败: %w", err)
		}
	}
	return db, writes[0].DBName, nil
}
