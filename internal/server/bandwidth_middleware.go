package server

import (
	"io"
	"net/http"
	"sync"
)

// bandwidthChunkSize 中间件分块复制的块大小：足够大以摊薄系统调用与内存分配开销，
// 足够小让带宽样本随传输推进实时更新（128KB ≈ 200Mbps 链路上约 5ms/块）。
const bandwidthChunkSize = 128 * 1024

var bandwidthBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, bandwidthChunkSize)
		return &b
	},
}

// BandwidthMiddleware 应用层出站带宽统计：包装 ResponseWriter，把全站
// 所有响应写出字节（SPA/静态资产/API/下载）实时喂给 bandwidth.Tracker。
//
// 取代旧的「仅下载处理器记账」方式。旧方式两个缺陷：
//  1. 范围只覆盖 /download/*，页面/资产/API 流量不计入，与真实出站水位有偏差；
//  2. responseWriterCounter.ReadFrom 透传底层 ReaderFrom（Linux sendfile 零拷贝），
//     整个传输结束后才一次性记账——大文件传输期间「当前带宽」恒为 0，
//     结束瞬间冲出尖峰，带宽分流（download_offload_mbps）的水位判断因此失真。
//
// 代价：有意放弃 sendfile 零拷贝，大文件走用户态分块复制（128KB/块）。
// 200Mbps ≈ 25MB/s 的链路上用户态拷贝 CPU 开销可忽略。
func (s *State) BandwidthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cw := &countingResponseWriter{ResponseWriter: w, record: s.bandwidth.RecordBytes}
		next.ServeHTTP(cw, r)
	})
}

// countingResponseWriter 统计写出字节的 ResponseWriter 包装器。
// ReadFrom 不透传底层 io.ReaderFrom（放弃 sendfile），改为分块复制，
// 每块经自身 Write 记账，带宽样本随传输实时推进。
type countingResponseWriter struct {
	http.ResponseWriter
	record func(int64)
}

func (w *countingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 && w.record != nil {
		w.record(int64(n))
	}
	return n, err
}

// ReadFrom 分块复制 r 到底层 ResponseWriter。
// 注意必须经 w.Write（而非 w.ResponseWriter.Write），确保每块都记账。
func (w *countingResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	bufp := bandwidthBufPool.Get().(*[]byte)
	buf := *bufp
	defer bandwidthBufPool.Put(bufp)

	var total int64
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			wn, werr := w.Write(buf[:n])
			total += int64(wn)
			if werr != nil {
				return total, werr
			}
			if wn < n {
				return total, io.ErrShortWrite
			}
		}
		if rerr == io.EOF {
			return total, nil
		}
		if rerr != nil {
			return total, rerr
		}
	}
}

// Flush 透传 http.Flusher，保持流式/进度推送类响应可用。
func (w *countingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
