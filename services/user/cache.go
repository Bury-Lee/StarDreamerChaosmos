package user

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"StarDreamerChaosmos/common/jwts"
)

// 令牌黑名单 + 角色缓存:全部落 Redis(参考 GoBlog 的做法,不落本地)。
// 归属 User 组件。
//
//   - 黑名单:key=token_black_<token>,value=int(BlackType),TTL=令牌剩余有效期;
//     登出/封禁时写入,鉴权时查询。TTL 到期自动清理。
//   - 角色缓存:key=user_role_<id>,value=int(role),TTL 固定;
//     受保护请求据此避免每请求查库;封禁/改角色时 InvalidateRole 即时失效。
const (
	blacklistKeyPrefix = "token_black_"
	roleCacheKeyPrefix = "user_role_"
	roleCacheTTL       = 10 * time.Minute
)

// BlackType 是令牌进入黑名单的原因。
type BlackType int8

const (
	UserBlackType   BlackType = 1 // 用户手动登出
	AdminBlackType  BlackType = 2 // 管理员拉黑
	DeviceBlackType BlackType = 3 // 多设备登录被挤下线
)

func (b BlackType) String() string {
	switch b {
	case UserBlackType:
		return "已下线"
	case AdminBlackType:
		return "已被管理员拉黑"
	case DeviceBlackType:
		return "因多设备登录,已下线"
	default:
		return "未知"
	}
}

// cache 是基于 Redis 的鉴权缓存(黑名单 + 角色)。rdb 为 nil 时全部退化为 no-op。
type cache struct {
	rdb *redis.Client
	tok jwts.Config
}

// ---------- 令牌黑名单 ----------

// Revoke 把令牌加入黑名单;TTL 取该令牌剩余有效期(过期/无法解析则不写)。
func (c *cache) Revoke(token string, t BlackType) {
	if c == nil || c.rdb == nil || token == "" {
		return
	}
	remain := c.remaining(token)
	if remain <= 0 {
		return
	}
	_ = c.rdb.Set(context.Background(), blacklistKeyPrefix+token, int(t), remain).Err()
}

// Revoked 报告令牌是否在黑名单;返回黑名单类型。
func (c *cache) Revoked(token string) (BlackType, bool) {
	if c == nil || c.rdb == nil || token == "" {
		return 0, false
	}
	v, err := c.rdb.Get(context.Background(), blacklistKeyPrefix+token).Int()
	if err != nil { // redis.Nil / Redis 不可用 → 视为不在黑名单
		return 0, false
	}
	bt := BlackType(v)
	return bt, bt != 0
}

// remaining 解析令牌剩余有效期(access 优先,其次 refresh)。
func (c *cache) remaining(token string) time.Duration {
	if ac, err := jwts.ParseAccess(c.tok, token); err == nil && ac.ExpiresAt != nil {
		return time.Until(ac.ExpiresAt.Time)
	}
	if rc, err := jwts.ParseRefresh(c.tok, token); err == nil && rc.ExpiresAt != nil {
		return time.Until(rc.ExpiresAt.Time)
	}
	return 0
}

// ---------- 角色缓存 ----------

// Role 读角色缓存;ok=false 表示未命中或 Redis 不可用。
func (c *cache) Role(userID uint64) (int8, bool) {
	if c == nil || c.rdb == nil {
		return 0, false
	}
	v, err := c.rdb.Get(context.Background(), fmt.Sprintf("%s%d", roleCacheKeyPrefix, userID)).Int()
	if err != nil {
		return 0, false
	}
	return int8(v), true
}

// SetRole 写角色缓存。
func (c *cache) SetRole(userID uint64, role int8) {
	if c == nil || c.rdb == nil {
		return
	}
	_ = c.rdb.Set(context.Background(), fmt.Sprintf("%s%d", roleCacheKeyPrefix, userID), int(role), roleCacheTTL).Err()
}

// InvalidateRole 清除角色缓存(封禁/改角色后调用,令其即时生效)。
func (c *cache) InvalidateRole(userID uint64) {
	if c == nil || c.rdb == nil {
		return
	}
	_ = c.rdb.Del(context.Background(), fmt.Sprintf("%s%d", roleCacheKeyPrefix, userID)).Err()
}
