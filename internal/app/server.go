package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
)

// NewServer 创建 HTTP 服务并挂到 fx 生命周期上。
//
// 直接依赖 *gin.Engine：fx 不做隐式接口转换，不能用 http.Handler 接收。
// OnStop 按注册顺序逆序执行：本 Hook 依赖连接池等资源，需最后注册（由 fx.Invoke 保证）。
func NewServer(lc fx.Lifecycle, cfg Config, engine *gin.Engine, log *slog.Logger) *http.Server {
	serverCfg := cfg.Server
	srv := &http.Server{
		Addr:              serverCfg.Addr,
		Handler:           engine,
		ReadHeaderTimeout: time.Duration(serverCfg.ReadHeaderTimeoutSec) * time.Second,
		ReadTimeout:       time.Duration(serverCfg.ReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(serverCfg.WriteTimeoutSec) * time.Second,
		IdleTimeout:       time.Duration(serverCfg.IdleTimeoutSec) * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			listener, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return fmt.Errorf("listen %s: %w", srv.Addr, err)
			}

			log.InfoContext(ctx, "http server listening", "addr", listener.Addr().String(), "env", cfg.App.Env)

			go func() {
				if serveErr := srv.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					log.ErrorContext(ctx, "http server exited", "err", serveErr)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			shutdownTimeout := time.Duration(serverCfg.ShutdownTimeoutSec) * time.Second
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()

			log.InfoContext(ctx, "http server shutting down", "timeout", shutdownTimeout.String())
			if err := srv.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("shutdown http server: %w", err)
			}
			log.InfoContext(ctx, "http server stopped")
			return nil
		},
	})

	return srv
}
