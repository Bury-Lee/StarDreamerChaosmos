package user

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"StarDreamerChaosmos/common/jwts"
)

// UserConfig 用户组件的独立配置:通过配置组件的公有槽读取本段,整体反序列化后装配。
// 组件自持其基础资源(DB/Redis/邮箱等)与业务配置(Jwt)。
type UserConfig struct {
	DB    DB    `yaml:"db"`
	Jwt   Jwt   `yaml:"jwt"`
	Redis Redis `yaml:"redis"`
	Email Email `yaml:"email"`
}

// Redis 是组件自持的 Redis 连接配置(用于黑名单等缓存)。
type Redis struct {
	Addr     string `yaml:"addr"`     // 如 127.0.0.1:6379;为空则不用 Redis(仅本地缓存)
	Password string `yaml:"password"` // 密码,可空
	DB       int    `yaml:"db"`       // 库号
}

// decodeUserConfig 把配置段(map)反序列化为 UserConfig。
func decodeUserConfig(sec map[string]any) UserConfig {
	var uc UserConfig
	if sec == nil {
		return uc
	}
	b, err := yaml.Marshal(sec)
	if err != nil {
		return uc
	}
	_ = yaml.Unmarshal(b, &uc)
	return uc
}

// TokenConfig 把 Jwt 配置翻译为 jwts.Config(带缺省有效期)。
func (uc UserConfig) TokenConfig() jwts.Config {
	j := uc.Jwt
	if j.AccessExpire <= 0 {
		j.AccessExpire = 120 // 默认 2 小时
	}
	if j.RefreshExpire <= 0 {
		j.RefreshExpire = 168 // 默认 7 天
	}
	return jwts.Config{
		AccessSecret:  j.AccessTokenSecret,
		RefreshSecret: j.RefreshTokenSecret,
		AccessExpire:  time.Duration(j.AccessExpire) * time.Minute,
		RefreshExpire: time.Duration(j.RefreshExpire) * time.Hour,
		Issuer:        j.Issuer,
	}
}

// HashPassword / CheckPassword 使用 bcrypt 存/验密码(镜像参考项目 utils/hash)。
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type Jwt struct {
	AccessExpire       int    `yaml:"accessExpire"`       //过期时间,单位为分钟
	RefreshExpire      int    `yaml:"refreshExpire"`      //刷新令牌过期时间,单位为小时
	AccessTokenSecret  string `yaml:"accessTokenSecret"`  //JWT密钥
	RefreshTokenSecret string `yaml:"refreshTokenSecret"` //刷新令牌密钥
	Issuer             string `yaml:"issuer"`             //JWT签发者
}

type SqlName string

type DB struct {
	SqlName  SqlName `yaml:"sql_name"` // 模式 mysql pgsql sqlite
	DBName   string  `yaml:"db_name"`
	Host     string  `yaml:"host"`
	Port     int     `yaml:"port"`
	User     string  `yaml:"user"`
	Password string  `yaml:"password"`
}

const (
	DBMysqlMode  = "mysql"
	DBPgsqlMode  = "postgresql"
	DBSqliteMode = "sqlite"
)

func (db *DB) DSN() gorm.Dialector {
	switch db.SqlName {
	case DBMysqlMode:
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			db.User,
			db.Password,
			db.Host,
			db.Port,
			db.DBName,
		)
		return mysql.Open(dsn)
	case DBPgsqlMode:
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable",
			db.Host,
			db.User,
			db.Password,
			db.DBName,
			db.Port,
		)
		return postgres.Open(dsn)
	case DBSqliteMode:
		return sqlite.Open(db.DBName)
	default:
		logrus.Panicf("未配置数据库连接")
		return nil
	}
}

type Email struct { //邮箱
	Domain       string `yaml:"domain" json:"domain"`             // 邮箱域名
	Port         int    `yaml:"port" json:"port"`                 // 邮箱SMTP服务器端口
	SendEmail    string `yaml:"sendEmail" json:"sendEmail"`       // 发送邮箱
	AuthCode     string `yaml:"authCode" json:"authCode"`         // api代码一类的?
	SendNickname string `yaml:"sendNickname" json:"sendNickname"` // 发信人昵称
	SSL          bool   `yaml:"SSL" json:"SSL"`                   // 是否启用SSL
	TLS          bool   `yaml:"TLS" json:"TLS"`                   // 是否启用TLS
}
