package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"gorm.io/gorm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userv1 "StarDreamerChaosmos/api/gen/user/v1"
)

// Server 是用户服务端实现:实现 gRPC 生成的服务端接口 + GORM。
type Server struct {
	userv1.UnimplementedUserServiceServer
	db *gorm.DB
}

// NewServer 创建服务端。
func NewServer(db *gorm.DB) *Server { return &Server{db: db} }

func (s *Server) Register(ctx context.Context, req *userv1.RegisterRequest) (*userv1.RegisterResponse, error) {
	if req.GetUsername() == "" || len(req.GetPassword()) < 6 {
		return nil, status.Error(codes.InvalidArgument, "用户名不能为空,密码至少 6 位")
	}
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&User{}).Where("username = ?", req.GetUsername()).Count(&cnt).Error; err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if cnt > 0 {
		return nil, status.Error(codes.AlreadyExists, "用户名已存在")
	}
	salt := randHex(16)
	u := User{Username: req.GetUsername(), Salt: salt, PassHash: hash(req.GetPassword(), salt)}
	if err := s.db.WithContext(ctx).Create(&u).Error; err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &userv1.RegisterResponse{UserId: u.ID}, nil
}

func (s *Server) Login(ctx context.Context, req *userv1.LoginRequest) (*userv1.LoginResponse, error) {
	if req.GetUsername() == "" || req.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "参数不合法")
	}
	var u User
	if err := s.db.WithContext(ctx).Where("username = ?", req.GetUsername()).First(&u).Error; err != nil {
		return nil, status.Error(codes.Unauthenticated, "用户名或密码错误")
	}
	if hash(req.GetPassword(), u.Salt) != u.PassHash {
		return nil, status.Error(codes.Unauthenticated, "用户名或密码错误")
	}
	token := randHex(32)
	if err := s.db.WithContext(ctx).Create(&Session{Token: token, UserID: u.ID}).Error; err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &userv1.LoginResponse{UserId: u.ID, Token: token}, nil
}

func (s *Server) Logout(ctx context.Context, req *userv1.LogoutRequest) (*userv1.LogoutResponse, error) {
	if req.GetToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "参数不合法")
	}
	res := s.db.WithContext(ctx).Where("token = ?", req.GetToken()).Delete(&Session{})
	if res.Error != nil {
		return nil, status.Error(codes.Internal, res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return nil, status.Error(codes.Unauthenticated, "登录凭证无效")
	}
	return &userv1.LogoutResponse{Ok: true}, nil
}

// 编译期:必须实现生成的 gRPC 服务端接口。
var _ userv1.UserServiceServer = (*Server)(nil)

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func hash(pass, salt string) string {
	sum := sha256.Sum256([]byte(salt + ":" + pass))
	return hex.EncodeToString(sum[:])
}
