package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"lemwood_mirror/internal/bandwidth"
)

// TestBandwidthMiddlewareRecordsAllResponses 全站出站字节统一记账：
// 下载处理器、API、静态资源（ReadFrom/Write 两条路径）都经中间件计入 tracker。
func TestBandwidthMiddlewareRecordsAllResponses(t *testing.T) {
	s := &State{bandwidth: bandwidth.NewTracker(100)}
	payload := strings.Repeat("x", 300*1024) // 300KB：跨越 128KB 分块边界

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello api") // Write 路径
	})
	mux.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, strings.NewReader(payload)) // ReadFrom 分块路径
	})

	handler := s.BandwidthMiddleware(mux)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api", nil))
	if got := s.bandwidth.Snapshot().TotalBytesServed; got != int64(len("hello api")) {
		t.Fatalf("api bytes = %d, want %d", got, len("hello api"))
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/file", nil))
	if rec2.Body.Len() != len(payload) {
		t.Fatalf("body = %d bytes, want %d", rec2.Body.Len(), len(payload))
	}
	want := int64(len("hello api") + len(payload))
	if got := s.bandwidth.Snapshot().TotalBytesServed; got != want {
		t.Fatalf("total bytes = %d, want %d", got, want)
	}
}

// TestCountingResponseWriterReadFromChunked ReadFrom 必须分块记账而非
// 传输结束一次性记账：300KB 至少分 3 次喂给 record（128KB 块）。
// 这是修复「大文件传输期间带宽恒为 0、结束瞬间冲高」的关键属性。
func TestCountingResponseWriterReadFromChunked(t *testing.T) {
	var mu sync.Mutex
	var calls []int64
	cw := &countingResponseWriter{
		ResponseWriter: httptest.NewRecorder(),
		record: func(n int64) {
			mu.Lock()
			calls = append(calls, n)
			mu.Unlock()
		},
	}

	// 用不实现 io.WriterTo 的 reader（io.Copy 优先走 WriterTo 会绕过分块路径；
	// 生产上 http.ServeFile 的 *os.File 同样不实现 WriterTo，走 ReadFrom 分块）
	payload := bytes.Repeat([]byte("y"), 300*1024)
	src := struct{ io.Reader }{bytes.NewReader(payload)}
	n, err := io.Copy(cw, src)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("copied = %d, want %d", n, len(payload))
	}

	mu.Lock()
	defer mu.Unlock()
	var total int64
	for _, c := range calls {
		total += c
	}
	if total != int64(len(payload)) {
		t.Fatalf("recorded total = %d, want %d", total, len(payload))
	}
	// 300KB / 128KB 块 = 3 块；透传 sendfile 的旧实现只有 1 次一次性记账。
	if len(calls) < 3 {
		t.Fatalf("record calls = %d, want >= 3 (chunked recording)", len(calls))
	}
}
