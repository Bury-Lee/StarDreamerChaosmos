// Package dialer 是全局拨号器:业务与微服务之间的中间层,也是"单体 ↔ 集群"的交界点。
// 传输基于 gRPC:本地走内存(bufconn),远程走地址(待接)。
//
// 约定:
//   - 提供方:把生成的 gRPC 服务端注册到 Dialer.Server();
//   - 调用方:Dial(服务键) 取一个 *grpc.ClientConn,再交给生成的 client;
//   - 拨号器不认任何服务签名,只做传输/连接(远程发现、LB、熔断待接)。
package dialer

import (
	"context"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// Dialer 是全局拨号器:内置一个 bufconn 上的本地 gRPC server(单体模式)。
type Dialer struct {
	mu        sync.RWMutex
	lis       *bufconn.Listener
	server    *grpc.Server
	conns     map[string]*grpc.ClientConn
	startOnce sync.Once
}

// New 创建拨号器;宿主应在建 Manager 之前把它预挂到 root。
// 注意:本地 server 此时尚未 Serve——必须先由各提供方注册服务,宿主再调 Start。
func New() *Dialer {
	return &Dialer{
		lis:    bufconn.Listen(1024 * 1024),
		server: grpc.NewServer(),
		conns:  make(map[string]*grpc.ClientConn),
	}
}

// Server 返回本地 gRPC server:提供方用它注册生成的服务端。
func (d *Dialer) Server() *grpc.Server { return d.server }

// Start 启动本地 server(幂等)。必须等所有提供方都注册完服务后由宿主调用,
// 因为 gRPC 不允许在 Serve 之后再 RegisterService。
func (d *Dialer) Start() {
	d.startOnce.Do(func() {
		go func() { _ = d.server.Serve(d.lis) }()
	})
}

// Dial 取服务连接(按服务键缓存)。当前:本地 bufconn;
// TODO:远程地址表待接。
func (d *Dialer) Dial(ctx context.Context, service string) (*grpc.ClientConn, error) {
	d.mu.RLock()
	if cc, ok := d.conns[service]; ok {
		d.mu.RUnlock()
		return cc, nil
	}
	d.mu.RUnlock()

	cc, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return d.lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dialer: 拨号 %q 失败: %w", service, err)
	}
	d.mu.Lock()
	d.conns[service] = cc
	d.mu.Unlock()
	return cc, nil
}

// ConnCount 返回已缓存的连接数(诊断用)。
func (d *Dialer) ConnCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.conns)
}

// Close 关闭全部连接与本地 server。
func (d *Dialer) Close() error {
	d.mu.Lock()
	for _, cc := range d.conns {
		_ = cc.Close()
	}
	d.conns = map[string]*grpc.ClientConn{}
	d.mu.Unlock()
	d.server.GracefulStop()
	return nil
}
