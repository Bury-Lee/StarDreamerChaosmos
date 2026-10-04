package user

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userv1 "StarDreamerChaosmos/api/gen/user/v1"
	"StarDreamerChaosmos/gateway"
)

// 业务函数统一流水线:参数校验 → (认证) → 拨号器(生成的 gRPC client) → 组装信封返回。
// 只面对框架无关的 gateway.Ctx。

func handleRegister(cli userv1.UserServiceClient) gateway.HandlerFunc {
	return func(c *gateway.Ctx) {
		var in struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.BindJSON(&in); err != nil {
			gateway.Fail(c, http.StatusBadRequest, "参数不合法")
			return
		}
		resp, err := cli.Register(c.Context(), &userv1.RegisterRequest{Username: in.Username, Password: in.Password})
		if err != nil {
			writeErr(c, err)
			return
		}
		gateway.OK(c, map[string]any{"userId": resp.GetUserId()})
	}
}

func handleLogin(cli userv1.UserServiceClient) gateway.HandlerFunc {
	return func(c *gateway.Ctx) {
		var in struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.BindJSON(&in); err != nil {
			gateway.Fail(c, http.StatusBadRequest, "参数不合法")
			return
		}
		resp, err := cli.Login(c.Context(), &userv1.LoginRequest{Username: in.Username, Password: in.Password})
		if err != nil {
			writeErr(c, err)
			return
		}
		gateway.OK(c, map[string]any{"userId": resp.GetUserId(), "token": resp.GetToken(), "refreshToken": resp.GetRefreshToken()})
	}
}

func handleRefresh(cli userv1.UserServiceClient) gateway.HandlerFunc {
	return func(c *gateway.Ctx) {
		var in struct {
			RefreshToken string `json:"refreshToken"`
		}
		if err := c.BindJSON(&in); err != nil {
			gateway.Fail(c, http.StatusBadRequest, "参数不合法")
			return
		}
		resp, err := cli.Refresh(c.Context(), &userv1.RefreshRequest{RefreshToken: in.RefreshToken})
		if err != nil {
			writeErr(c, err)
			return
		}
		gateway.OK(c, map[string]any{"userId": resp.GetUserId(), "token": resp.GetToken(), "refreshToken": resp.GetRefreshToken()})
	}
}

func handleLogout(cli userv1.UserServiceClient) gateway.HandlerFunc {
	return func(c *gateway.Ctx) {
		var in struct {
			Token string `json:"token"`
		}
		if err := c.BindJSON(&in); err != nil {
			gateway.Fail(c, http.StatusBadRequest, "参数不合法")
			return
		}
		if _, err := cli.Logout(c.Context(), &userv1.LogoutRequest{Token: in.Token}); err != nil {
			writeErr(c, err)
			return
		}
		gateway.OK(c, map[string]any{"logout": true})
	}
}

// writeErr 把 gRPC 状态码翻成统一 HTTP 状态码 + 中文提示。
func writeErr(c *gateway.Ctx, err error) {
	msg := status.Convert(err).Message()
	switch status.Code(err) {
	case codes.InvalidArgument:
		gateway.Fail(c, http.StatusBadRequest, msg)
	case codes.Unauthenticated:
		gateway.Fail(c, http.StatusUnauthorized, msg)
	case codes.AlreadyExists:
		gateway.Fail(c, http.StatusConflict, msg)
	default:
		gateway.Fail(c, http.StatusInternalServerError, msg)
	}
}
