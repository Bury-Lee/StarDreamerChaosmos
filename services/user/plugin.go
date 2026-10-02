package user

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"GoTenon"
	userv1 "StarDreamerChaosmos/api/gen/user/v1"
	"StarDreamerChaosmos/core/config"
	"StarDreamerChaosmos/dialer"
	"StarDreamerChaosmos/gateway"
	"StarDreamerChaosmos/utils"
)

// Plugin 是用户组件:开库 → 注册 gRPC 服务端到拨号器 → 用生成的 client 注册路由。
// 自身设置(DB 路径)只从 config 的 user 段读取。
type Plugin struct {
	mu     sync.Mutex
	dbPath string
	db     *gorm.DB
}

// NewPlugin 创建用户组件。
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "user" }
func (p *Plugin) Desc() map[string]string {
	return map[string]string{"provides": "user.v1.UserService", "inject": "config,dialer,gateway"}
}
func (p *Plugin) Inject() []string { return []string{"config", "dialer", "gateway"} }

func (p *Plugin) Status() *map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return &map[string]any{"state": "ready", "db": p.dbPath}
}

func (p *Plugin) Register() error { return nil }

func (p *Plugin) Apply(ctx *GoTenon.GoTenonContext, _ any) error {
	// 1. 从 config 的 user 段读取 DB 路径
	dbPath := filepath.Join("data", "sdc.db")
	if cs := ctx.SlotOf(config.ServiceName); cs != nil && cs.Value != nil {
		if cfg, ok := cs.Value.(*config.Config); ok {
			sec, _ := cfg.Snapshot("user").(map[string]any)
			dbPath = utils.Str(sec, "db", dbPath)
		}
	}

	// 2. 开库 + 迁移
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("user: 打开数据库失败: %w", err)
	}
	if err := db.AutoMigrate(&User{}, &Session{}); err != nil {
		return fmt.Errorf("user: 迁移失败: %w", err)
	}
	p.mu.Lock()
	p.dbPath = dbPath
	p.db = db
	p.mu.Unlock()

	d := ctx.SlotOf(dialer.ServiceName).Value.(*dialer.Dialer)

	// 3. 把生成的 gRPC 服务端注册到拨号器的本地 server
	userv1.RegisterUserServiceServer(d.Server(), NewServer(db))

	// 4. 经拨号器取连接 → 生成的 gRPC client
	cc, err := d.Dial(context.Background(), "user")
	if err != nil {
		return fmt.Errorf("user: 取连接失败: %w", err)
	}
	cli := userv1.NewUserServiceClient(cc)

	// 5. 用消息注册路由:投进网关信箱,网关收消息即注册;组件下线自动摘除
	gw := ctx.SlotOf(gateway.ServiceName).Value.(*gateway.Router)
	_ = gw.Submit(gateway.RouteMessage("user", "POST", "/api/user/register", handleRegister(cli)))
	_ = gw.Submit(gateway.RouteMessage("user", "POST", "/api/user/login", handleLogin(cli)))
	_ = gw.Submit(gateway.RouteMessage("user", "POST", "/api/user/logout", handleLogout(cli)))

	// 6. 卸载关库
	ctx.Register(func() error {
		sqlDB, err := db.DB()
		if err == nil {
			return sqlDB.Close()
		}
		return err
	})

	fmt.Printf("[user] 就绪(db=%s)\n", dbPath)
	return nil
}

func (p *Plugin) Start() error                          { return nil }
func (p *Plugin) Run() error                            { return nil }
func (p *Plugin) End() error                            { return nil }
func (p *Plugin) DealWithMessage(GoTenon.Message) error { return nil }
func (p *Plugin) Function() map[string]any              { return nil }
func (p *Plugin) ExecuteFunction(any)                   {}
