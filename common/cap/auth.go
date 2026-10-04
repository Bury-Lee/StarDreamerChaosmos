package cap

// auth.go —— 鉴权能力契约:能力键 + 函数指针类型 + 声明结构。
// 各组件"要鉴权"时只需依赖本文件的类型与键,不必 import 提供方(如 user)。

// 角色(镜像 user 的 RoleType;契约层用 int8)。
const (
	RoleAdmin    int8 = 1 // 管理员
	RoleSuperVip int8 = 2 // 超级会员
	RoleVip      int8 = 3 // 会员
	RoleUser     int8 = 4 // 普通用户
	RoleGuest    int8 = 5 // 访客
	RoleBlack    int8 = 6 // 封禁用户
)

// Claims 是鉴权返回的用户声明(与 access 令牌载荷同形)。
type Claims struct {
	UserID   uint64
	Username string
	Role     int8
}

// 鉴权能力键。
const (
	KeyAuthParse        = "auth.parse"        // token → claims(仅解析/校验签名与过期)
	KeyAuthRequireLogin = "auth.requireLogin" // token → claims(解析 + 黑名单 + 角色刷新 + 封禁校验)
	KeyAuthRequireRoles = "auth.requireRoles" // 在 requireLogin 基础上要求指定角色(自定义掩码)
	KeyAuthIsRevoked    = "auth.isRevoked"    // token 是否已被拉黑(登出/封禁)
	KeyAuthRoleOf       = "auth.roleOf"       // userID → 当前角色
)

// RoleMask 是角色的位图掩码:调用方可自定义"要求哪些角色/等级",
// 支持"满足其一/任意组合";将来按等级位图判断也复用此类型。
type RoleMask uint16

// MaskOf 由角色集合构造掩码。
func MaskOf(roles ...int8) RoleMask {
	var m RoleMask
	for _, r := range roles {
		m |= 1 << uint(r)
	}
	return m
}

// Has 报告掩码是否包含某角色。
func (m RoleMask) Has(role int8) bool { return m&(1<<uint(role)) != 0 }

// 常用掩码。
var (
	MaskAdmin = MaskOf(RoleAdmin)                        // 仅管理员
	MaskVip   = MaskOf(RoleAdmin, RoleSuperVip, RoleVip) // 会员及以上
)

// 函数指针类型(契约):参数只含 token / userID,不依赖 HTTP 框架。
type (
	ParseFunc        func(token string) (*Claims, error)
	RequireLoginFunc func(token string) (*Claims, error)
	RequireRolesFunc func(token string, allow RoleMask) (*Claims, error)
	IsRevokedFunc    func(token string) bool
	RoleOfFunc       func(userID uint64) (int8, bool)
)
