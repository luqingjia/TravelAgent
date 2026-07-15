// Package main 提供 TravelAgent 可执行程序的最薄进程入口。
//
// 这里仅处理操作系统信号和最终退出码；配置、数据库、业务服务、Gin 路由等长期依赖都由
// internal/app 统一组装。这样 main 不会随着业务增长变成无法单元测试的“大杂烩”。
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/luqingjia/TravelAgent/internal/app"
)

// main 建立一个会被 Ctrl+C 或容器终止信号取消的根 context，然后把控制权交给 app。
func main() {
	// signal.NotifyContext 把 SIGINT/SIGTERM 转成 context 取消事件。HTTP Server 收到取消后会停止
	// 接收新请求，并在配置的 shutdown timeout 内等待正在处理的请求完成。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		// app.Run 已负责关闭自己拥有的资源；main 只记录最终错误并向操作系统返回非零退出码。
		slog.Error("travel-agent stopped", "error", err)
		os.Exit(1)
	}
}
