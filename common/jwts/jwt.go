// Package jwts 提供 JWT 的签发与本地校验(HS256)。
//
// 放在 common 模块的原因:JWT 是跨组件的公共能力,后期不同模块会往载荷里
// 追加自己的声明(claims),故统一收敛到 common,由各组件按需扩展载荷。
//
// 参考 GoBlog-StarDreamerCyberNook 的模式:
//   - access 令牌携带 用户ID/用户名/角色,热路径本地校验即可;
//   - refresh 令牌只携带 用户ID,刷新时再回查数据库;
//   - access / refresh 使用不同密钥,便于分别轮换。
//
// 本包无状态:密钥与有效期由调用方经 Config 传入(配置来自各组件)。
package jwts

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 是 access 令牌的业务声明。
// 其它模块如需追加载荷,可在此扩展(或自定义 Claims 后复用签发/校验逻辑)。
type Claims struct {
	UserID   uint64 `json:"ID"`   // 用户唯一标识
	Username string `json:"name"` // 用户名
	Role     int8   `json:"role"` // 角色(镜像 model.RoleType)
}

// AccessClaims 是 access 令牌的完整声明(业务 + 标准)。
type AccessClaims struct {
	Claims
	jwt.RegisteredClaims
}

// RefreshClaims 是 refresh 令牌的声明(仅用户ID + 标准)。
type RefreshClaims struct {
	ID uint64 `json:"id"`
	jwt.RegisteredClaims
}

// Config 是签发/校验 JWT 所需的密钥与有效期。
type Config struct {
	AccessSecret  string
	RefreshSecret string
	AccessExpire  time.Duration // access 有效期
	RefreshExpire time.Duration // refresh 有效期
	Issuer        string        // 签发者
}

var signingMethod = jwt.SigningMethodHS256

func newJTI() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// Issue 签发 access + refresh 令牌。
func Issue(cfg Config, c Claims) (access, refresh string, err error) {
	if access, err = signAccess(cfg, c); err != nil {
		return "", "", err
	}
	if refresh, err = signRefresh(cfg, c.UserID); err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

func signAccess(cfg Config, c Claims) (string, error) {
	claims := jwt.NewWithClaims(signingMethod, AccessClaims{
		Claims: c,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(cfg.AccessExpire)),
			ID:        newJTI(),
			Issuer:    cfg.Issuer,
		},
	})
	return claims.SignedString([]byte(cfg.AccessSecret))
}

func signRefresh(cfg Config, userID uint64) (string, error) {
	claims := jwt.NewWithClaims(signingMethod, RefreshClaims{
		ID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(cfg.RefreshExpire)),
			ID:        newJTI(),
			Issuer:    cfg.Issuer,
		},
	})
	return claims.SignedString([]byte(cfg.RefreshSecret))
}

// ParseAccess 本地校验 access 令牌。
func ParseAccess(cfg Config, tokenString string) (*AccessClaims, error) {
	if tokenString == "" {
		return nil, errors.New("请登录")
	}
	token, err := jwt.ParseWithClaims(tokenString, &AccessClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != signingMethod {
			return nil, fmt.Errorf("非法的算法: %v", t.Header["alg"])
		}
		return []byte(cfg.AccessSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, errors.New("无效的令牌")
	}
	return claims, nil
}

// ParseRefresh 本地校验 refresh 令牌。
func ParseRefresh(cfg Config, tokenString string) (*RefreshClaims, error) {
	if tokenString == "" {
		return nil, errors.New("请登录")
	}
	token, err := jwt.ParseWithClaims(tokenString, &RefreshClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != signingMethod {
			return nil, fmt.Errorf("非法的算法: %v", t.Header["alg"])
		}
		return []byte(cfg.RefreshSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*RefreshClaims)
	if !ok || !token.Valid {
		return nil, errors.New("无效的令牌")
	}
	return claims, nil
}
