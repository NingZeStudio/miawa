package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// sha256Cache 以 路径|大小|mtime 为键缓存文件哈希，避免重复全量读盘。
// 镜像产物文件集有限，全局缓存即可；超限时整体重置防膨胀。
var sha256Cache = struct {
	sync.Mutex
	entries map[string]string
}{entries: make(map[string]string)}

// sumSHA256Cached 计算文件 SHA-256（hex），带 (path, size, mtime) 缓存。
// 文件内容变更会改变 size/mtime，自动视为新键重新计算。
func sumSHA256Cached(path string, info os.FileInfo) (string, error) {
	key := fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())
	sha256Cache.Lock()
	if h, ok := sha256Cache.entries[key]; ok {
		sha256Cache.Unlock()
		return h, nil
	}
	sha256Cache.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(hasher.Sum(nil))

	sha256Cache.Lock()
	if len(sha256Cache.entries) >= 1024 {
		sha256Cache.entries = make(map[string]string)
	}
	sha256Cache.entries[key] = sum
	sha256Cache.Unlock()
	return sum, nil
}

// handleV2FileIntegrity 返回单个文件的 SHA-256 完整性信息（信封包裹）。
// GET /api/v2/files/integrity?file_path=launcher/v1/file.txt
// 纯新增端点：不改动既有 v2 响应结构，旧客户端不受影响。
func (s *State) handleV2FileIntegrity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		writeV2Error(w, r, http.StatusBadRequest, "missing_required_parameters", "Missing file_path", nil)
		return
	}
	cleanPath, info, validationErr := s.validateDownloadFile(filePath)
	if validationErr != nil {
		writeV2Error(w, r, validationErr.StatusCode, validationErr.Code, validationErr.Message, nil)
		return
	}
	sum, err := sumSHA256Cached(cleanPath, info)
	if err != nil {
		log.Printf("[Integrity] 计算哈希失败: %v", err)
		writeV2Error(w, r, http.StatusInternalServerError, "hash_failed", "Failed to compute file hash", nil)
		return
	}
	markNoStore(w)
	writeV2Success(w, r, map[string]interface{}{
		"file_path": filePath,
		"file_name": filepath.Base(filePath),
		"size":      info.Size(),
		"algorithm": "sha256",
		"sha256":    sum,
	}, false)
}
