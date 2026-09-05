package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"lemwood_mirror/internal/bandwidth"
	"lemwood_mirror/internal/blacklist"
	"lemwood_mirror/internal/config"
	"lemwood_mirror/internal/db"
	"lemwood_mirror/internal/download_authz"
	"lemwood_mirror/internal/firewall"
	"lemwood_mirror/internal/geoip"
	"lemwood_mirror/internal/netutil"
	"lemwood_mirror/internal/pow"
	"lemwood_mirror/internal/selfupdate"
	"lemwood_mirror/internal/stats"
	"lemwood_mirror/internal/traffic"
	"lemwood_mirror/internal/version"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var errForbiddenPath = errors.New("forbidden")

type State struct {
	BasePath    string
	ProjectRoot string
	Config      *config.Config
	// 缓存状态：map[launcher]map[version]infoPath
	mu        sync.RWMutex
	index     map[string]map[string]string
	latest    map[string]string
	infoCache map[string]map[string]interface{} // 缓存 index.json 文件内容

	// 登录限制
	loginAttempts   map[string]int       // IP -> 失败次数
	loginLocks      map[string]time.Time // IP -> 解锁时间
	loginAttemptsMu sync.Mutex

	// 验证码（已移除极验）→ PoW 挑战 + DB 授权
	powMgr          *pow.Manager
	authzMgr        *download_authz.Manager
	bandwidth       *bandwidth.Tracker
	selfUpdate      *selfupdate.Manager
	applySelfUpdate func(ctx context.Context) error
	restartProcess  func() error

	// 扫描回调（在 Routes 中使用）
	scanAllFunc         func()
	scanLauncherFunc    func(launcherName string)
	selfUpdateCheckFunc func()
}

func NewState(base string, projectRoot string, cfg *config.Config) *State {
	s := &State{
		BasePath:    base,
		ProjectRoot: projectRoot,
		Config:      cfg,
		index:       make(map[string]map[string]string),
		latest:      make(map[string]string),
		infoCache:   make(map[string]map[string]interface{}),

		loginAttempts: make(map[string]int),
		loginLocks:    make(map[string]time.Time),
	}

	if cfg.PowEnabled {
		s.powMgr = pow.NewManager(pow.Config{
			Secret:     cfg.PowHMACSecret,
			Cost:       cfg.PowCost,
			KeyLength:  cfg.PowKeyLength,
			Difficulty: cfg.PowDifficulty,
			TTL:        parseDuration(cfg.PowChallengeTTL, 2*time.Minute),
		})
	}
	s.authzMgr = download_authz.NewManager(parseDuration(cfg.DownloadTokenTTL, 5*time.Minute))
	s.bandwidth = bandwidth.NewTracker(int64(cfg.BandwidthLimitMbps))

	return s
}

// parseDuration 解析 Go duration 字符串，失败或非正回退到 fallback。
func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

// maxLoginStateEntries 登录失败状态 map 的容量上限；超过时触发清扫，
// 防止（转发头被无条件信任的异常部署下）海量伪造 IP 使内存无界增长。
const maxLoginStateEntries = 4096

// sweepLoginStateLocked 清理已过期的登录锁定，避免 map 只增不减。
// 调用方必须持有 loginAttemptsMu。
func (s *State) sweepLoginStateLocked(now time.Time) {
	if len(s.loginAttempts) < maxLoginStateEntries && len(s.loginLocks) < maxLoginStateEntries {
		return
	}
	for ip, unlockAt := range s.loginLocks {
		if now.After(unlockAt) {
			delete(s.loginLocks, ip)
			delete(s.loginAttempts, ip)
		}
	}
	if len(s.loginAttempts) >= maxLoginStateEntries {
		s.loginAttempts = make(map[string]int)
	}
}

// Close 释放 State 持有的后台资源（PoW 清理协程等），供优雅停机调用。
func (s *State) Close() {
	if mgr := s.POW(); mgr != nil {
		mgr.Close()
	}
}

// Conf 返回当前配置快照。后台保存会整体替换 s.Config，
// 读侧一律经此获取，避免与保存路径产生数据竞争。
func (s *State) Conf() *config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Config
}

// POW 返回当前 PoW 管理器；nil 表示未启用。管理器在配置保存时可被整体替换。
func (s *State) POW() *pow.Manager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.powMgr
}

// AUTHZ 返回当前下载授权管理器。管理器在配置保存时可被整体替换。
func (s *State) AUTHZ() *download_authz.Manager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authzMgr
}

// pathRelWithin 判断 target 是否位于 base 目录内（含 base 本身）。
func pathRelWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// resolveStoragePath 把面向用户的相对路径安全解析为真实绝对路径：
//   - 文本级穿越（../）直接拒绝；
//   - 已存在部分经 EvalSymlinks 解析后必须仍落在 BasePath 内（防符号链接逃逸）；
//   - 目标不存在（如上传新文件）时按最近存在祖先解析校验后拼接。
//
// 返回值为 (解析后的绝对路径, 是否合法)。
func (s *State) resolveStoragePath(rel string) (string, bool) {
	absBase, err := filepath.Abs(s.BasePath)
	if err != nil {
		return "", false
	}
	baseReal, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", false
	}
	clean := filepath.Clean(filepath.Join(absBase, rel))
	if !pathRelWithin(absBase, clean) {
		return "", false
	}
	if _, err := os.Lstat(clean); err == nil {
		real, err := filepath.EvalSymlinks(clean)
		if err != nil || !pathRelWithin(baseReal, real) {
			return "", false
		}
		return real, true
	}
	// 目标不存在：找最近的存在祖先并校验其真实位置
	cur := filepath.Dir(clean)
	for cur != absBase {
		realDir, err := filepath.EvalSymlinks(cur)
		if err == nil {
			if !pathRelWithin(baseReal, realDir) {
				return "", false
			}
			suffix, err := filepath.Rel(cur, clean)
			if err != nil {
				return "", false
			}
			return filepath.Join(realDir, suffix), true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	suffix, err := filepath.Rel(absBase, clean)
	if err != nil {
		return "", false
	}
	return filepath.Join(baseReal, suffix), true
}

func (s *State) SetSelfUpdateManager(manager *selfupdate.Manager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selfUpdate = manager
}

func (s *State) SetSelfUpdateActions(apply func(ctx context.Context) error, restart func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applySelfUpdate = apply
	s.restartProcess = restart
}

func (s *State) updateInfoCache(path string, info map[string]any) {
	s.mu.Lock()
	s.infoCache[path] = info
	s.mu.Unlock()
}

func (s *State) UpdateIndex(launcher string, version string, infoPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index[launcher] == nil {
		s.index[launcher] = make(map[string]string)
	}
	s.index[launcher][version] = infoPath

	if content, err := os.ReadFile(infoPath); err == nil {
		var info map[string]interface{}
		if err := json.Unmarshal(content, &info); err == nil {
			s.infoCache[infoPath] = info
		}
	}

	s.latest[launcher] = s.pickLatest(s.index[launcher])
	log.Printf("更新启动器 %s 索引: 版本=%s, 最新版本=%s", launcher, version, s.latest[launcher])
}

// GetLatestVersion 获取启动器的最新版本号
func (s *State) GetLatestVersion(launcher string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest[launcher]
}

func (s *State) RemoveVersion(launcher string, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index[launcher] == nil {
		return
	}
	delete(s.index[launcher], version)
	s.latest[launcher] = s.pickLatest(s.index[launcher])
}

func (s *State) TrimLauncherVersions(launcher string, keep int) error {
	keep = config.NormalizeMaxVersions(keep)

	s.mu.RLock()
	launcherVersions := s.index[launcher]
	if len(launcherVersions) == 0 {
		s.mu.RUnlock()
		return nil
	}

	versions := make([]string, 0, len(launcherVersions))
	infoPaths := make(map[string]string, len(launcherVersions))
	for version, infoPath := range launcherVersions {
		versions = append(versions, version)
		infoPaths[version] = infoPath
	}
	s.mu.RUnlock()

	if len(versions) <= keep {
		return nil
	}

	sort.Slice(versions, func(i, j int) bool {
		return version.Compare(versions[i], versions[j]) > 0
	})

	var deleted []string
	for _, version := range versions[keep:] {
		infoPath := infoPaths[version]
		if infoPath == "" {
			continue
		}

		versionDir := filepath.Dir(infoPath)
		if err := removePathUnderBase(s.BasePath, versionDir); err != nil {
			return fmt.Errorf("删除版本 %s 目录失败: %w", version, err)
		}

		s.mu.Lock()
		if currentVersions := s.index[launcher]; currentVersions != nil {
			delete(currentVersions, version)
			if len(currentVersions) == 0 {
				delete(s.index, launcher)
				delete(s.latest, launcher)
			} else {
				s.latest[launcher] = s.pickLatest(currentVersions)
			}
		}
		delete(s.infoCache, infoPath)
		s.mu.Unlock()

		deleted = append(deleted, version)
	}

	if len(deleted) > 0 {
		log.Printf("%s: 已清理旧版本 %s", launcher, strings.Join(deleted, ", "))
	}

	return nil
}

// ClearLatestFlags 清除指定启动器所有版本的 is_latest 标记
func (s *State) ClearLatestFlags(launcher string) error {
	s.mu.RLock()
	versions, exists := s.index[launcher]
	s.mu.RUnlock()

	if !exists {
		return nil // 启动器不存在，无需清除
	}

	for _, infoPath := range versions {
		// 检查缓存中的 is_latest 字段，如果为 true 才处理
		s.mu.RLock()
		info, exists := s.infoCache[infoPath]
		s.mu.RUnlock()

		// 如果缓存存在且 is_latest 为 true，或者缓存不存在（需要读取文件），则处理
		if !exists || (exists && info["is_latest"] == true) {
			if err := s.clearLatestFlag(infoPath); err != nil {
				log.Printf("清除 %s 的 latest 标记失败: %v", infoPath, err)
				// 继续处理其他文件，不返回错误
			}
		}
	}

	return nil
}

// clearLatestFlag 清除单个 index.json 文件的 is_latest 标记
func (s *State) clearLatestFlag(infoPath string) error {
	s.mu.RLock()
	info, exists := s.infoCache[infoPath]
	s.mu.RUnlock()

	// 如果缓存不存在，读取文件
	if !exists {
		content, err := os.ReadFile(infoPath)
		if err != nil {
			return fmt.Errorf("读取文件失败: %w", err)
		}

		var fileInfo map[string]interface{}
		if err := json.Unmarshal(content, &fileInfo); err != nil {
			return fmt.Errorf("解析 JSON 失败: %w", err)
		}

		info = fileInfo
	}

	// 如果存在 is_latest 字段且为 true，则将其设置为 false。
	// 注意：info 可能与 infoCache 中的共享对象同源（来自其他写入方的浅拷贝），
	// 必须先深拷贝再修改，避免与公开 API 的读取方产生并发读写。
	if isLatest, exists := info["is_latest"]; exists && isLatest == true {
		infoCopy := make(map[string]interface{}, len(info))
		for k, v := range info {
			infoCopy[k] = v
		}
		infoCopy["is_latest"] = false
		info = infoCopy

		// 重新写入文件
		newContent, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化 JSON 失败: %w", err)
		}

		if err := os.WriteFile(infoPath, newContent, 0o644); err != nil {
			return fmt.Errorf("写入文件失败: %w", err)
		}

		// 更新缓存
		s.mu.Lock()
		s.infoCache[infoPath] = info
		s.mu.Unlock()

		log.Printf("已清除 %s 的 latest 标记", infoPath)
	}

	return nil
}

func (s *State) AdminSwitchMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.Conf().AdminEnabled {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "Admin console is disabled", http.StatusForbidden)
			} else {
				http.Error(w, "Admin console is disabled by administrator", http.StatusForbidden)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

// getFrontendThemeDir 返回前端主题目录（相对路径）。
// v1 已移除，始终使用 web/default 主题。
func (s *State) getFrontendThemeDir() string {
	return filepath.Join("web", "default")
}

func (s *State) Routes(mux *http.ServeMux) {
	// 静态 UI — 使用 v2 主题目录
	staticDir := s.getFrontendThemeDir()
	adminStaticDir := filepath.Join("web", "admin")

	// 统一静态资源服务函数
	serveStatic := func(w http.ResponseWriter, r *http.Request, baseDir string, prefix string) {
		path := r.URL.Path
		if containsDotDot(path) {
			http.NotFound(w, r)
			return
		}

		relPath := strings.TrimPrefix(path, prefix)
		if relPath == "" || strings.HasSuffix(relPath, "/") {
			http.NotFound(w, r)
			return
		}

		fullPath := filepath.Join(baseDir, relPath)
		cleanPath := filepath.Clean(fullPath)

		// 验证路径安全性和文件类型
		absBase, _ := filepath.Abs(baseDir)
		absPath, _ := filepath.Abs(cleanPath)
		if !strings.HasPrefix(absPath, absBase) {
			log.Printf("安全警告：拦截到来自 %s 的路径逃逸尝试，请求路径：%s", r.RemoteAddr, path)
			http.NotFound(w, r)
			return
		}

		info, err := os.Stat(cleanPath)
		if err != nil || info.IsDir() {
			// 禁止访问目录
			http.NotFound(w, r)
			return
		}

		http.ServeFile(w, r, cleanPath)
	}

	// 静态资源处理器 - /dist/ 和 /assets/
	mux.HandleFunc("/dist/", func(w http.ResponseWriter, r *http.Request) {
		serveStatic(w, r, staticDir, "/dist/")
	})

	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		// assets 通常在 dist/assets 下
		serveStatic(w, r, filepath.Join(staticDir, "assets"), "/assets/")
	})

	// 根路径处理器 - 处理静态文件和 SPA fallback
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 根路径和 index.html
		if path == "/" || path == "/index.html" {
			indexPath := filepath.Join(staticDir, "index.html")
			http.ServeFile(w, r, indexPath)
			return
		}

		// 检查是否是静态文件
		relPath := strings.TrimPrefix(path, "/")
		if relPath != "" {
			fullPath := filepath.Join(staticDir, relPath)
			cleanPath := filepath.Clean(fullPath)

			// 安全检查
			absBase, _ := filepath.Abs(staticDir)
			absPath, _ := filepath.Abs(cleanPath)
			if strings.HasPrefix(absPath, absBase) {
				if info, err := os.Stat(cleanPath); err == nil && !info.IsDir() {
					http.ServeFile(w, r, cleanPath)
					return
				}
			}
		}

		// SPA fallback: 其他所有情况返回 index.html
		indexPath := filepath.Join(staticDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}

		http.NotFound(w, r)
	})

	// 下载 - PoW/授权安全处理器
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		cfg := s.Conf()
		path := r.URL.Path
		if containsDotDot(path) {
			http.NotFound(w, r)
			return
		}

		relPath := strings.TrimPrefix(path, "/download/")
		if relPath == "" || strings.HasSuffix(relPath, "/") {
			// 禁止直接访问 /download/ 根目录或任何子目录列表
			http.NotFound(w, r)
			return
		}

		// token 从 query 或 Authorization: Bearer 提取（不再支持路径内嵌 token）
		token := r.URL.Query().Get("token")
		if token == "" {
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		// 无 token 且 PoW 开启：浏览器走 PoW 验证页，非浏览器返回 JSON 引导走 API PoW 链路。
		// PoW 关闭时无 token 直接放行（无门控，等同旧 captcha_enabled=false 行为）。
		if token == "" && cfg.PowEnabled {
			if isBrowserRequest(r) {
				http.Redirect(w, r, "/verify?file="+url.QueryEscape(relPath), http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":              "verification_required",
				"message":            "Download requires PoW authorization",
				"pow":                true,
				"challenge_endpoint": "/api/v2/downloads/challenge",
			})
			return
		}

		// 路径与文件校验必须先于授权消费：任何校验失败都不应烧掉一次性 PoW 授权。
		fullPath := filepath.Join(s.BasePath, relPath)
		cleanPath := filepath.Clean(fullPath)
		if !pathWithinBase(s.BasePath, cleanPath) {
			log.Printf("安全警告：拦截到来自 %s 的路径逃逸尝试，请求路径：%s", r.RemoteAddr, path)
			http.NotFound(w, r)
			return
		}
		resolvedPath, err := filepath.EvalSymlinks(cleanPath)
		if err != nil || !pathWithinBase(s.BasePath, resolvedPath) {
			http.NotFound(w, r)
			return
		}
		cleanPath = resolvedPath

		// 检查是否为目录
		info, err := os.Stat(cleanPath)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
				return
			}
			log.Printf("访问文件出错：%s, %v", path, err)
			http.NotFound(w, r)
			return
		}
		if info.IsDir() {
			// 禁止目录列表访问
			http.NotFound(w, r)
			return
		}

		switch r.Method {
		case http.MethodHead:
			// HEAD 不传输内容：不消费授权、不做流量预留/记账，
			// 避免客户端探测请求白白烧掉一次性 PoW 授权。
			http.ServeFile(w, r, cleanPath)
			return
		case http.MethodGet:
		default:
			// method 校验在授权消费之前，避免 405 请求浪费 token
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		clientIP := netutil.ExtractClientIP(r)

		// 授权处理：先 Peek 做绑定校验（不消耗），全部校验通过后再原子消费，
		// 避免 404/405/超配额/路径不匹配请求烧掉一次性授权。
		// Peek 失败时尝试并行分段复用：下载器（IDM/aria2/浏览器分段/断点续传）
		// 对同一 URL 携带 Range 头并发建连，只有第一条连接能消费授权；已消费
		// token 在 TTL 内且 Range 请求 + 文件 + 客户端 IP 均匹配时放行后续
		// 连接（不再重复消费）。无 Range 的重放不受复用保护，仍然拒绝。
		var auth db.DownloadAuthorization
		reused := false
		if token != "" {
			peeked, ok := s.AUTHZ().Peek(token)
			if !ok {
				var reuseAuth db.DownloadAuthorization
				reuseOK := false
				if r.Header.Get("Range") != "" {
					reuseAuth, reuseOK = s.AUTHZ().PeekReuse(token)
				}
				if !reuseOK {
					if isBrowserRequest(r) && cfg.PowEnabled {
						http.Redirect(w, r, "/verify?file="+url.QueryEscape(relPath), http.StatusFound)
						return
					}
					writeJSONError(w, http.StatusForbidden, "invalid_token", "Download token is invalid or expired")
					return
				}
				if reuseAuth.FilePath != relPath {
					writeJSONError(w, http.StatusForbidden, "token_mismatch", "Download token does not match requested file")
					return
				}
				if reuseAuth.ClientIP != "" && reuseAuth.ClientIP != clientIP {
					writeJSONError(w, http.StatusForbidden, "client_ip_mismatch", "Download token is bound to another client IP")
					return
				}
				auth = reuseAuth
				reused = true
			} else {
				if peeked.FilePath != relPath {
					writeJSONError(w, http.StatusForbidden, "token_mismatch", "Download token does not match requested file")
					return
				}
			}
		}

		estimatedBytes := traffic.EstimateTransferBytes(info.Size(), r.Header.Get("Range"))
		allowed, _, projectedBytes, reason := traffic.ReserveTraffic(clientIP, estimatedBytes)
		if !allowed {
			message := reason
			if message == "" {
				message = "已超过当日下载流量限制"
			}
			if cfg.AppealContact != "" {
				message = fmt.Sprintf("%s，如有误封请联系 %s", message, cfg.AppealContact)
			}
			http.Error(w, message, http.StatusForbidden)
			log.Printf("[防刷墙] 拒绝下载请求: ip=%s path=%s projected=%.2fGB reason=%s", clientIP, relPath, traffic.ToGB(projectedBytes), reason)
			return
		}

		// 首次使用的 token 在此原子消费；并行分段复用（reused）不再消费。
		if token != "" && !reused {
			consumed, ok := s.AUTHZ().Consume(token)
			if !ok {
				// 授权并发失效：回滚刚预留的流量（按实际 0 字节结账）
				traffic.FinalizeTraffic(clientIP, estimatedBytes, 0)
				if isBrowserRequest(r) && cfg.PowEnabled {
					http.Redirect(w, r, "/verify?file="+url.QueryEscape(relPath), http.StatusFound)
					return
				}
				writeJSONError(w, http.StatusForbidden, "invalid_token", "Download token is invalid or expired")
				return
			}
			auth = consumed
		}

		counter := &traffic.CountingWriter{}
		defer func() {
			if banned, reason, trafficGB, err := traffic.FinalizeTraffic(clientIP, estimatedBytes, counter.Total); err != nil {
				log.Printf("[防刷墙] 记录流量失败: %v", err)
			} else if banned {
				log.Printf("[防刷墙] IP %s 因 %s 被封禁，当日流量: %.2fGB", clientIP, reason, trafficGB)
			}
		}()

		s.bandwidth.StartDownload()
		defer s.bandwidth.FinishDownload()
		countingWriter := &responseWriterCounter{
			ResponseWriter: w,
			counter:        counter,
			recordBytes:    s.bandwidth.RecordBytes,
		}
		// token-based URL，禁止中间缓存共享。
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// 强制附件下载：无扩展名文件（如 mirror-linux-amd64）浏览器需要 attachment 才弹下载
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(relPath)})
		if disposition != "" {
			w.Header().Set("Content-Disposition", disposition)
		}

		http.ServeFile(countingWriter, r, cleanPath)

		// 事件状态表记录（流量与下载次数的统一来源）：
		// served 口径 = 实际写出字节（含中止的部分传输，防刷墙用）；
		// completed 口径 = 完整传输（写出达到预估且状态 200/206，展示用）。
		// 注：multi-range 请求 EstimateTransferBytes 返回整个文件大小，completed 多为 false，属预期。
		// 必须先写事件再 FinalizeTraffic，防刷墙 CheckAndBan 才能读到本次字节。
		completed := counter.Total >= estimatedBytes && isSuccessfulDownloadStatus(countingWriter.statusCode)
		if counter.Total > 0 {
			parts := strings.Split(filepath.ToSlash(relPath), "/")
			var launcher, version, fileName string
			if len(parts) >= 2 {
				launcher = parts[0]
				version = parts[1]
			}
			fileName = filepath.Base(relPath)
			// 地区归属：入库前本地解析（内嵌 ip2region，微秒级，不影响传输速率）。
			// 解析失败或内网/保留地址留空，聚合展示时归入「其他」。
			country, _, _, ok := geoip.Lookup(clientIP)
			if !ok {
				country = ""
			}
			if err := db.RecordDownloadEvent(db.DownloadEvent{
				AuthorizationID: auth.AuthorizationID,
				FilePath:        relPath,
				FileName:        fileName,
				Launcher:        launcher,
				Version:         version,
				ClientIP:        clientIP,
				Country:         country,
				BytesServed:     counter.Total,
				Completed:       completed,
				StatusCode:      countingWriter.statusCode,
			}); err != nil {
				log.Printf("[流量统计] 记录下载事件失败: %v", err)
			} else {
				stats.InvalidateSnapshot()
			}
		}
	})

	// ============================================================
	// v2 API 端点 (/api/v2/) — 唯一 API 版本，始终注册
	// ============================================================
	// 公共查询
	mux.HandleFunc("/api/v2/launchers", s.handleV2Status)
	mux.HandleFunc("/api/v2/launchers/", s.handleV2LauncherStatus)
	mux.HandleFunc("/api/v2/latest", s.handleV2LatestAll)
	mux.HandleFunc("/api/v2/latest/", s.handleV2LatestLauncher)
	mux.HandleFunc("/api/v2/stats", s.handleV2Stats)
	mux.HandleFunc("/api/v2/bandwidth", s.handleV2Bandwidth)
	mux.HandleFunc("/api/v2/auth/2fa/status", s.handleV2Auth2FAStatus)

	// 下载：prepare（CLI/API 直发授权）+ PoW 挑战/授权（替代极验浏览器验证）+ landing
	// PoW 端点始终注册：管理后台热更新 pow_enabled 后无需重启即可生效。
	mux.HandleFunc("/api/v2/downloads/prepare", s.handleV2DownloadPrepare)
	mux.HandleFunc("/api/v2/downloads/landing", s.handleV2DownloadLanding)
	mux.HandleFunc("/api/v2/downloads/challenge", s.handleV2DownloadChallenge)
	mux.HandleFunc("/api/v2/downloads/authorize", s.handleV2DownloadAuthorize)
	mux.HandleFunc("/api/v2/pow/config", s.handleV2PowConfig)

	// 认证 + 扫描（v2 admin 中间件，返回信封格式错误）
	mux.Handle("/api/v2/auth/login", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.handleV2Login)))
	mux.Handle("/api/v2/auth/logout", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.handleV2Logout)))
	mux.Handle("/api/v2/admin/scans", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2ScanAll))))
	mux.Handle("/api/v2/admin/scans/launcher", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2ScanLauncher))))

	// 管理后台（v2 admin 中间件）
	mux.Handle("/api/v2/admin/config", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2AdminConfig))))
	mux.Handle("/api/v2/admin/blacklist", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2AdminBlacklist))))
	mux.Handle("/api/v2/admin/firewall/status", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2AdminFirewallStatus))))
	mux.Handle("/api/v2/admin/files", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2AdminFiles))))
	mux.Handle("/api/v2/admin/files/download", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2AdminFileDownload))))
	mux.Handle("/api/v2/admin/self-update/status", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2AdminSelfUpdateStatus))))
	mux.Handle("/api/v2/admin/self-update/check", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2AdminSelfUpdateCheck))))
	mux.Handle("/api/v2/admin/self-update/apply", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2AdminSelfUpdateApply))))
	mux.Handle("/api/v2/admin/self-update/restart", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2AdminSelfUpdateRestart))))
	mux.Handle("/api/v2/admin/self-update", s.v2AdminSwitchMiddleware(http.HandlerFunc(s.v2AdminMiddleware(s.handleV2SelfUpdateCheckEndpoint))))

	// Admin UI
	mux.Handle("/admin", s.AdminSwitchMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})))
	mux.Handle("/admin/", s.AdminSwitchMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if containsDotDot(path) {
			http.NotFound(w, r)
			return
		}
		relPath := strings.TrimPrefix(path, "/admin/")

		if relPath == "" || relPath == "index.html" {
			http.ServeFile(w, r, filepath.Join(adminStaticDir, "index.html"))
			return
		}

		fullPath := filepath.Join(adminStaticDir, relPath)
		cleanPath := filepath.Clean(fullPath)
		// 路径安全验证：防止路径穿越
		absBase, _ := filepath.Abs(adminStaticDir)
		absPath, _ := filepath.Abs(cleanPath)
		if !strings.HasPrefix(absPath, absBase) {
			http.NotFound(w, r)
			return
		}

		if info, err := os.Stat(cleanPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, cleanPath)
			return
		}

		// Fallback to index.html for SPA-like behavior in admin
		http.ServeFile(w, r, filepath.Join(adminStaticDir, "index.html"))
	})))
}

func removePathUnderBase(basePath string, targetPath string) error {
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("解析基础路径失败: %w", err)
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("解析目标路径失败: %w", err)
	}

	baseResolved, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return fmt.Errorf("解析基础路径符号链接失败: %w", err)
	}
	targetResolved, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		return fmt.Errorf("解析目标路径符号链接失败: %w", err)
	}

	if !pathWithinBase(baseResolved, targetResolved) {
		return errForbiddenPath
	}

	if err := os.RemoveAll(targetResolved); err != nil {
		return err
	}

	return nil
}

// containsDotDot 检查路径是否包含 ".." 元素
func containsDotDot(v string) bool {
	if !strings.Contains(v, "..") {
		return false
	}
	for _, ent := range strings.FieldsFunc(v, func(r rune) bool { return r == '/' || r == '\\' }) {
		if ent == ".." {
			return true
		}
	}
	return false
}

// SecurityMiddleware 站点级防护：黑名单（精确 IP 查库 + CIDR 内存匹配 + 外部同步）、
// 请求频率限制、路径遍历拦截与安全响应头。白名单 IP 豁免外部黑名单与频率限制，
// 但不豁免本地黑名单（管理员手动封禁始终生效）。
func (s *State) SecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := netutil.ExtractClientIP(r)
		appeal := s.Config.AppealContact
		if appeal != "" {
			appeal = fmt.Sprintf("如有误封，请联系 %s", appeal)
		}

		// 本地黑名单（精确 IP：管理员手动 + 防刷墙/防火墙自动封禁）
		if banned, createdAt, _ := db.GetIPBlacklistInfo(ip); banned {
			log.Printf("[防火墙] 拒绝来自黑名单 IP 的访问: %s，封禁时间: %s", ip, createdAt)
			message := fmt.Sprintf("Access Denied: Your IP %s was banned at %s.", ip, createdAt)
			if appeal != "" {
				message = fmt.Sprintf("%s %s", message, appeal)
			}
			http.Error(w, message, http.StatusForbidden)
			return
		}

		// 网段封禁（CIDR 条目，内存匹配）
		if firewall.MatchBlacklistCIDR(ip) {
			log.Printf("[防火墙] 拒绝来自黑名单网段的访问: %s", ip)
			message := fmt.Sprintf("Access Denied: Your IP %s is in a banned network range.", ip)
			if appeal != "" {
				message = fmt.Sprintf("%s %s", message, appeal)
			}
			http.Error(w, message, http.StatusForbidden)
			return
		}

		// 外部黑名单（白名单豁免）
		if !firewall.Whitelisted(ip) && blacklist.IsExternalBlacklisted(ip) {
			log.Printf("[防火墙] 拒绝来自外部黑名单 IP 的访问: %s", ip)
			message := fmt.Sprintf("Access Denied: Your IP %s is in the external blacklist.", ip)
			if appeal != "" {
				message = fmt.Sprintf("%s %s", message, appeal)
			}
			http.Error(w, message, http.StatusForbidden)
			return
		}

		// 请求频率限制（白名单豁免；违规累计达阈值由 firewall 自动封禁）
		if !firewall.Whitelisted(ip) {
			if decision := firewall.Allow(ip); !decision.Allowed {
				log.Printf("[防火墙] IP %s 请求频率超限，拒绝访问（Retry-After %s）", ip, decision.RetryAfter)
				if decision.Banned {
					log.Printf("[防火墙] IP %s %s", ip, decision.Reason)
				}
				retrySeconds := int(decision.RetryAfter.Seconds())
				if retrySeconds < 1 {
					retrySeconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retrySeconds))
				http.Error(w, "Too Many Requests: 请求频率超过限制，请稍后重试", http.StatusTooManyRequests)
				return
			}
		}

		// 记录访问（仅在通过防火墙检查后）
		stats.RecordVisit(r)

		path := r.URL.Path
		// 拦截路径遍历尝试
		if containsDotDot(path) {
			log.Printf("安全警告：拦截到来自 %s 的路径遍历尝试，请求路径：%s", r.RemoteAddr, path)
			http.NotFound(w, r)
			return
		}

		// 安全响应头
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// Public API: callers may access it from any origin.
		// Admin 控制台同源访问，不暴露跨域授权（避免放大 CSRF/凭据面）。
		if !strings.HasPrefix(r.URL.Path, "/api/v2/admin") && !strings.HasPrefix(r.URL.Path, "/admin") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Expose-Headers", "X-Latest-Version, X-Latest-Versions")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (s *State) InitFromDisk() error {
	base := s.BasePath
	return filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) != "index.json" {
			return nil
		}
		rel, err := filepath.Rel(base, filepath.Dir(path))
		if err != nil {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) < 2 {
			return nil
		}
		launcher := parts[0]
		version := parts[1]
		s.UpdateIndex(launcher, version, path)
		return nil
	})
}

func (s *State) FixAssetURLs() error {
	cfg := s.Conf()
	if cfg.DownloadUrlBase == "" {
		return nil
	}

	baseURL := cfg.DownloadUrlBase
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("解析 download_url_base 失败: %w", err)
	}
	targetDomain := parsedURL.Host
	targetScheme := parsedURL.Scheme

	// 第一阶段：只持有读锁，复制文件路径列表
	s.mu.RLock()
	paths := make([]string, 0, len(s.infoCache))
	for path := range s.infoCache {
		paths = append(paths, path)
	}
	s.mu.RUnlock()

	// 第二阶段：无锁状态下进行文件 IO 和处理
	fixedCount := 0
	for _, infoPath := range paths {
		content, err := os.ReadFile(infoPath)
		if err != nil {
			continue
		}

		var info map[string]interface{}
		if err := json.Unmarshal(content, &info); err != nil {
			continue
		}

		assets, ok := info["assets"].([]interface{})
		if !ok {
			continue
		}

		changed := false
		for _, asset := range assets {
			assetMap, ok := asset.(map[string]interface{})
			if !ok {
				continue
			}

			assetURL, ok := assetMap["url"].(string)
			if !ok {
				continue
			}

			parsedAssetURL, err := url.Parse(assetURL)
			if err != nil {
				continue
			}

			if parsedAssetURL.Host != targetDomain {
				// 用 EscapedPath 保留合法转义（url.Path 会把 %xx 解码，可能生成含空格/# 的坏链接）
				newURL := fmt.Sprintf("%s://%s%s", targetScheme, targetDomain, parsedAssetURL.EscapedPath())
				assetMap["url"] = newURL
				changed = true
			}
		}

		if changed {
			newContent, err := json.MarshalIndent(info, "", "  ")
			if err != nil {
				continue
			}

			if err := os.WriteFile(infoPath, newContent, 0o644); err != nil {
				log.Printf("修复 %s 的 URL 失败: %v", infoPath, err)
				continue
			}

			// 第三阶段：最小化持有写锁，仅更新缓存
			s.mu.Lock()
			s.infoCache[infoPath] = info
			s.mu.Unlock()

			fixedCount++
		}
	}

	if fixedCount > 0 {
		log.Printf("[URL 统一性检查] 修复了 %d 个 index.json 文件中的下载链接", fixedCount)
	}

	return nil
}

// pickLatest 选择最新版本
func (s *State) pickLatest(versions map[string]string) string {
	if len(versions) == 0 {
		return ""
	}

	var latestFlagged []string
	for v, infoPath := range versions {
		info, exists := s.infoCache[infoPath]
		if !exists {
			continue
		}

		if isLatest, ok := info["is_latest"].(bool); ok && isLatest {
			latestFlagged = append(latestFlagged, v)
		}
	}

	if len(latestFlagged) > 0 {
		latest := latestFlagged[0]
		for _, v := range latestFlagged[1:] {
			if version.Compare(v, latest) > 0 {
				latest = v
			}
		}
		return latest
	}

	var stableVersions []string
	var unstableVersions []string

	for v := range versions {
		if version.IsStable(v) {
			stableVersions = append(stableVersions, v)
		} else {
			unstableVersions = append(unstableVersions, v)
		}
	}

	if len(stableVersions) > 0 {
		latest := stableVersions[0]
		for _, v := range stableVersions[1:] {
			if version.Compare(v, latest) > 0 {
				latest = v
			}
		}
		return latest
	}

	if len(unstableVersions) > 0 {
		latest := unstableVersions[0]
		for _, v := range unstableVersions[1:] {
			if version.Compare(v, latest) > 0 {
				latest = v
			}
		}
		return latest
	}

	return ""
}

type downloadValidationError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *downloadValidationError) Error() string {
	return e.Message
}

type downloadPrepareRequest struct {
	FilePath  string `json:"file_path"`
	ReturnURL string `json:"return_url"`
	Source    string `json:"source"`
}

type downloadTokenResponse struct {
	DownloadToken string `json:"download_token"`
	DownloadURL   string `json:"download_url"`
	LandingURL    string `json:"landing_url"`
}

func writeJSONError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}

func (s *State) validateDownloadFile(filePath string) (string, os.FileInfo, *downloadValidationError) {
	fullPath := filepath.Join(s.BasePath, filePath)
	cleanPath := filepath.Clean(fullPath)
	if !pathWithinBase(s.BasePath, cleanPath) {
		return "", nil, &downloadValidationError{
			StatusCode: http.StatusForbidden,
			Code:       "invalid_path",
			Message:    "Invalid file path",
		}
	}

	info, err := os.Stat(cleanPath)
	if err != nil || info.IsDir() {
		return "", nil, &downloadValidationError{
			StatusCode: http.StatusNotFound,
			Code:       "file_not_found",
			Message:    "File not found",
		}
	}

	return cleanPath, info, nil
}

func pathWithinBase(basePath, targetPath string) bool {
	base, err := filepath.Abs(basePath)
	if err != nil {
		return false
	}
	target, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	baseResolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		baseResolved = base
	}
	targetResolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		targetResolved = target
	}
	rel, err := filepath.Rel(baseResolved, targetResolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return rel != "."
}

// issueAuthz 签发一条 DB 授权并返回响应。明文 token 仅在响应中返回一次。
// 调用方需提供 clientIP（用于额度/流量归因）、sourceKind（web|api）与 maxBytes（文件大小，用于流量上限）。
func (s *State) issueAuthz(filePath, returnURL, source, flow, clientIP, sourceKind string, maxBytes int64) (downloadTokenResponse, error) {
	token, _, err := s.AUTHZ().Issue(download_authz.IssueRequest{
		FilePath:   filePath,
		ReturnURL:  returnURL,
		Source:     source,
		Flow:       flow,
		ClientIP:   clientIP,
		SourceKind: sourceKind,
		MaxBytes:   maxBytes,
		RequestID:  generateRequestID(),
	})
	if err != nil {
		return downloadTokenResponse{}, err
	}
	downloadURL := buildDownloadURL(token, filePath)
	return downloadTokenResponse{
		DownloadToken: token,
		DownloadURL:   downloadURL,
		LandingURL:    fmt.Sprintf("/api/v2/downloads/landing?token=%s", url.QueryEscape(token)),
	}, nil
}

func buildDownloadURL(token, filePath string) string {
	parts := strings.Split(filepath.ToSlash(filePath), "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return fmt.Sprintf("/download/%s?token=%s", strings.Join(escaped, "/"), url.QueryEscape(token))
}

func isBrowserRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	userAgent := r.Header.Get("User-Agent")

	if strings.Contains(accept, "text/html") {
		return true
	}

	if strings.Contains(userAgent, "Mozilla") ||
		strings.Contains(userAgent, "Chrome") ||
		strings.Contains(userAgent, "Safari") ||
		strings.Contains(userAgent, "Edge") ||
		strings.Contains(userAgent, "Firefox") {
		if !strings.Contains(accept, "application/json") && accept != "" {
			return true
		}
	}

	return false
}

// responseWriterCounter 包装 http.ResponseWriter 以统计实际写入的字节数
type responseWriterCounter struct {
	http.ResponseWriter
	counter     *traffic.CountingWriter
	statusCode  int
	wroteHeader bool
	recordBytes func(int64)
}

func (rw *responseWriterCounter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriterCounter) Write(p []byte) (int, error) {
	if !rw.wroteHeader {
		rw.statusCode = http.StatusOK
		rw.wroteHeader = true
	}
	n, err := rw.ResponseWriter.Write(p)
	rw.counter.Total += int64(n)
	if n > 0 && rw.recordBytes != nil {
		rw.recordBytes(int64(n))
	}
	return n, err
}

// ReadFrom 把底层 ResponseWriter 的 io.ReaderFrom 能力透传给 http.ServeFile，
// 恢复 Linux sendfile 零拷贝路径（包装器默认会禁用它，大文件走用户态拷贝）。
func (rw *responseWriterCounter) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := rw.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(r)
		rw.counter.Total += n
		if n > 0 && rw.recordBytes != nil {
			rw.recordBytes(n)
		}
		return n, err
	}
	return io.Copy(struct{ io.Writer }{rw}, r)
}

func isSuccessfulDownloadStatus(statusCode int) bool {
	return statusCode == http.StatusOK || statusCode == http.StatusPartialContent
}
