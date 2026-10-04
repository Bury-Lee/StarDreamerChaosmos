package user

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	userv1 "StarDreamerChaosmos/api/gen/user/v1"
	"StarDreamerChaosmos/common/cap"
	"StarDreamerChaosmos/common/jwts"
)

// Server 是用户服务端实现:实现 gRPC 生成的服务端接口 + GORM + JWT。
// 同时对外提供"鉴权能力函数指针"(见 auth.go 契约,注册在组件层的 cap.Provider)。
type Server struct {
	userv1.UnimplementedUserServiceServer
	db  *gorm.DB
	tok jwts.Config
	c   *cache // Redis 鉴权缓存:黑名单 + 角色(由 User 组件持有)
}

// ---------- gRPC:注册 / 登录 / 刷新 / 登出 ----------

func (s *Server) Register(ctx context.Context, req *userv1.RegisterRequest) (*userv1.RegisterResponse, error) {
	if req.GetUsername() == "" || len(req.GetPassword()) < 6 {
		return nil, status.Error(codes.InvalidArgument, "用户名不能为空,密码至少 6 位")
	}
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&UserModel{}).Where("user_name = ?", req.GetUsername()).Count(&cnt).Error; err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if cnt > 0 {
		return nil, status.Error(codes.AlreadyExists, "用户名已存在")
	}
	h, err := HashPassword(req.GetPassword())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	u := UserModel{UserName: req.GetUsername(), Password: h, Role: UserRole}
	if err := s.db.WithContext(ctx).Create(&u).Error; err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &userv1.RegisterResponse{UserId: int64(u.ID)}, nil
}

func (s *Server) Login(ctx context.Context, req *userv1.LoginRequest) (*userv1.LoginResponse, error) {
	if req.GetUsername() == "" || req.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "参数不合法")
	}
	var u UserModel
	if err := s.db.WithContext(ctx).Where("user_name = ?", req.GetUsername()).First(&u).Error; err != nil {
		return nil, status.Error(codes.Unauthenticated, "用户名或密码错误")
	}
	if !CheckPassword(req.GetPassword(), u.Password) {
		return nil, status.Error(codes.Unauthenticated, "用户名或密码错误")
	}
	if u.Role == BlackRole {
		return nil, status.Error(codes.Unauthenticated, "账号已被封禁")
	}
	return s.issue(u)
}

// Refresh 用 refresh 令牌换一对新的 access/refresh。
func (s *Server) Refresh(ctx context.Context, req *userv1.RefreshRequest) (*userv1.LoginResponse, error) {
	rc, err := jwts.ParseRefresh(s.tok, req.GetRefreshToken())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "刷新令牌无效")
	}
	if _, revoked := s.c.Revoked(req.GetRefreshToken()); revoked {
		return nil, status.Error(codes.Unauthenticated, "登录凭证无效")
	}
	var u UserModel
	if err := s.db.WithContext(ctx).First(&u, rc.ID).Error; err != nil {
		return nil, status.Error(codes.Unauthenticated, "用户不存在或已失效")
	}
	if u.Role == BlackRole {
		return nil, status.Error(codes.Unauthenticated, "账号已被封禁")
	}
	return s.issue(u)
}

func (s *Server) Logout(_ context.Context, req *userv1.LogoutRequest) (*userv1.LogoutResponse, error) {
	tok := req.GetToken()
	if tok == "" {
		return nil, status.Error(codes.InvalidArgument, "参数不合法")
	}
	if _, revoked := s.c.Revoked(tok); revoked { // 已登记过 → 视为已登出
		return nil, status.Error(codes.Unauthenticated, "登录凭证无效")
	}
	s.c.Revoke(tok, UserBlackType)
	return &userv1.LogoutResponse{Ok: true}, nil
}

// issue 为已通过校验的用户签发 access + refresh。
func (s *Server) issue(u UserModel) (*userv1.LoginResponse, error) {
	access, refresh, err := jwts.Issue(s.tok, jwts.Claims{
		UserID:   uint64(u.ID),
		Username: u.UserName,
		Role:     int8(u.Role),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &userv1.LoginResponse{UserId: int64(u.ID), Token: access, RefreshToken: refresh}, nil
}

// ---------- 鉴权能力(供 cap.Provider 注册,函数指针契约见 common/cap) ----------

// ParseAccess 仅解析并校验 access 令牌(签名/过期),不查库。
func (s *Server) ParseAccess(token string) (*cap.Claims, error) {
	c, err := jwts.ParseAccess(s.tok, token)
	if err != nil {
		return nil, err
	}
	return &cap.Claims{UserID: c.UserID, Username: c.Username, Role: int8(c.Role)}, nil
}

// RequireLogin 解析 + 黑名单 + 加载用户 + 封禁校验;返回以库中最新角色为准的声明。
func (s *Server) RequireLogin(token string) (*cap.Claims, error) {
	if _, revoked := s.c.Revoked(token); revoked {
		return nil, errors.New("登录凭证无效")
	}
	c, err := s.ParseAccess(token)
	if err != nil {
		return nil, err
	}
	role, ok := s.roleOf(c.UserID)
	if !ok {
		return nil, errors.New("用户不存在或已失效")
	}
	if role == int8(BlackRole) {
		return nil, errors.New("账号已被封禁")
	}
	return &cap.Claims{UserID: c.UserID, Username: c.Username, Role: role}, nil
}

// RequireRoles 在 RequireLogin 基础上,要求用户角色落在 allow 掩码内
// (自定义要求等级/类型;掩码为位图,便于将来按等级位图判断)。
func (s *Server) RequireRoles(token string, allow cap.RoleMask) (*cap.Claims, error) {
	c, err := s.RequireLogin(token)
	if err != nil {
		return nil, err
	}
	if !allow.Has(c.Role) {
		return nil, errors.New("权限不足")
	}
	return c, nil
}

// IsRevoked 令牌是否已被拉黑。
func (s *Server) IsRevoked(token string) bool {
	_, ok := s.c.Revoked(token)
	return ok
}

// RoleOf 查用户当前角色。
func (s *Server) RoleOf(userID uint64) (int8, bool) { return s.roleOf(userID) }

// roleOf 角色:Redis 缓存优先,未命中查库并回填。
func (s *Server) roleOf(userID uint64) (int8, bool) {
	if r, ok := s.c.Role(userID); ok {
		return r, true
	}
	var u UserModel
	if err := s.db.First(&u, userID).Error; err != nil {
		return 0, false
	}
	s.c.SetRole(userID, int8(u.Role))
	return int8(u.Role), true
}

// 编译期:必须实现生成的 gRPC 服务端接口。
var _ userv1.UserServiceServer = (*Server)(nil)
