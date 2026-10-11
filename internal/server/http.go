package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"
)

type LauncherScanRequest struct {
	Launcher string `json:"launcher"`
}

type LauncherScanResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// StartHTTPWithScan creates the HTTP server and registers handlers.
// scanFunc, launcherScanFunc, and selfUpdateCheckFunc are stored on State
// and registered inside Routes with AdminMiddleware protection.
//
// 监听在返回前同步完成：成功后立即交出 *http.Server 并在后台开始 Serve，
// 保证调用方（信号处理）随时能对 srv 调用 Shutdown。
func StartHTTPWithScan(addr string, s *State, scanFunc func(), launcherScanFunc func(launcherName string), selfUpdateCheckFunc func(), selfUpdateApplyFunc func(ctx context.Context) error, restartFunc func() error) (*http.Server, error) {
	// Store callbacks on State so Routes can register them with auth middleware
	s.scanAllFunc = scanFunc
	s.scanLauncherFunc = launcherScanFunc
	s.selfUpdateCheckFunc = selfUpdateCheckFunc
	s.SetSelfUpdateActions(selfUpdateApplyFunc, restartFunc)

	mux := http.NewServeMux()
	s.Routes(mux)

	// 主题选择统一委托给 getFrontendThemeDir()，转换为绝对路径供 SPAFallbackMiddleware 使用
	staticDir := filepath.Join(s.ProjectRoot, s.getFrontendThemeDir())
	handler := SPAFallbackMiddleware(mux, staticDir)

	handler = s.SecurityMiddleware(handler)

	// 应用层带宽统计放最外层：SPA 回退/静态资产/安全中间件自身的错误响应
	// （429/403 等）全部计入出站字节，口径覆盖全站。
	handler = s.BandwidthMiddleware(handler)

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Printf("http 服务异常退出: %v", serveErr)
		}
	}()

	return srv, nil
}
