package server

import (
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"lemwood_mirror/internal/auth"
	"lemwood_mirror/internal/bandwidth"
	"lemwood_mirror/internal/config"
	"lemwood_mirror/internal/db"
	"lemwood_mirror/internal/download_authz"
	"lemwood_mirror/internal/firewall"
	"lemwood_mirror/internal/netutil"
	"lemwood_mirror/internal/pow"
	"lemwood_mirror/internal/selfupdate"
	"lemwood_mirror/internal/stats"
	"lemwood_mirror/internal/traffic"
	"lemwood_mirror/internal/version"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ============================================================
// v2 响应信封类型
// ============================================================

type v2Meta struct {
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
	RequestID string `json:"request_id,omitempty"`
	Cached    bool   `json:"cached,omitempty"`
}

type v2ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type v2Envelope struct {
	Data  any          `json:"data"`
	Error *v2ErrorBody `json:"error"`
	Meta  v2Meta       `json:"meta"`
}

// ============================================================
// v2 辅助函数
// ============================================================

const (
	v2CacheControl  = "public, max-age=300"
	v2GzipThreshold = 1024
)

// generateRequestID 生成 16 字符十六进制请求 ID。
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

// computeETag 对数据计算弱 ETag（基于 SHA-256 前 16 字节）。
func computeETag(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf(`W/"%s"`, hex.EncodeToString(h[:16]))
}

// writeV2Success 写入成功信封，支持 ETag 条件请求和 gzip 压缩。
// 可选 statusCode 参数覆盖默认 200（如扫描接口传 202）。
func writeV2Success(w http.ResponseWriter, r *http.Request, data any, cached bool, statusCodes ...int) {
	status := http.StatusOK
	if len(statusCodes) > 0 {
		status = statusCodes[0]
	}

	// ETag 基于数据载荷计算（不含 timestamp/request_id 等每次变化的字段）
	dataBytes, err := json.Marshal(data)
	if err != nil {
		writeV2Error(w, r, http.StatusInternalServerError, "internal_error", "Failed to encode response", nil)
		return
	}
	etag := computeETag(dataBytes)

	// ETag 条件请求（仅对 200 响应有意义）
	if status == http.StatusOK {
		if inm := r.Header.Get("If-None-Match"); inm != "" {
			for _, v := range strings.Split(inm, ",") {
				if strings.TrimSpace(v) == etag {
					w.Header().Set("ETag", etag)
					if w.Header().Get("Cache-Control") == "" {
						w.Header().Set("Cache-Control", v2CacheControl)
					}
					w.WriteHeader(http.StatusNotModified)
					return
				}
			}
		}
	}

	meta := v2Meta{
		Version:   "v2",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RequestID: generateRequestID(),
		Cached:    cached,
	}
	envelope := v2Envelope{Data: data, Error: nil, Meta: meta}

	body, err := json.Marshal(envelope)
	if err != nil {
		writeV2Error(w, r, http.StatusInternalServerError, "internal_error", "Failed to encode response", nil)
		return
	}

	w.Header().Set("ETag", etag)
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", v2CacheControl)
	}
	w.Header().Set("Content-Type", "application/json")

	// gzip 压缩（仅对足够大的响应）
	if acceptsGzip(r) && len(body) > v2GzipThreshold {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		w.WriteHeader(status)
		gz.Write(body)
		return
	}

	w.WriteHeader(status)
	w.Write(body)
}

// writeV2Error 写入错误信封，data 为 null。
func writeV2Error(w http.ResponseWriter, r *http.Request, statusCode int, code, message string, details map[string]any) {
	meta := v2Meta{
		Version:   "v2",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RequestID: generateRequestID(),
	}
	envelope := v2Envelope{
		Data:  nil,
		Error: &v2ErrorBody{Code: code, Message: message, Details: details},
		Meta:  meta,
	}
	body, _ := json.Marshal(envelope)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(body)
}

// acceptsGzip 检查客户端是否接受 gzip 压缩。
func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

// markNoStore 标记响应不可缓存。挑战/授权/令牌等一次性凭证响应必须 no-store，
// 否则浏览器缓存会导致重试拿到已消费的挑战（401 challenge not found）。
func markNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
}

// ============================================================
// v2 Admin 中间件（返回信封格式错误）
// ============================================================

// v2AdminSwitchMiddleware 检查 admin 是否启用，未启用时返回 v2 错误信封。
func (s *State) v2AdminSwitchMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.Conf().AdminEnabled {
			writeV2Error(w, r, http.StatusForbidden, "admin_disabled", "Admin console is disabled", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// v2AdminMiddleware 验证 token，无效时返回 v2 错误信封。
func (s *State) v2AdminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			if cookie, err := r.Cookie("admin_token"); err == nil {
				token = cookie.Value
			}
		}
		// 兼容 "Bearer <token>" 格式
		token = strings.TrimPrefix(token, "Bearer ")
		if token == "" || !auth.ValidateToken(token) {
			writeV2Error(w, r, http.StatusUnauthorized, "unauthorized", "Unauthorized", nil)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// ============================================================
// v2 公共查询 Handler
// ============================================================

// handleV2Status 返回所有启动器状态（信封包裹）。
func (s *State) handleV2Status(w http.ResponseWriter, r *http.Request) {
	result := s.getLauncherStatusData(r)
	writeV2Success(w, r, result, false)
}

// handleV2LauncherStatus 返回单个启动器版本数组（信封包裹）。
func (s *State) handleV2LauncherStatus(w http.ResponseWriter, r *http.Request) {
	launcher := strings.TrimPrefix(r.URL.Path, "/api/v2/launchers/")

	s.mu.RLock()
	versions, ok := s.index[launcher]
	versionsCopy := make(map[string]string)
	if ok {
		for k, v := range versions {
			versionsCopy[k] = v
		}
	}
	infoCacheCopy := make(map[string]map[string]interface{})
	for k, v := range s.infoCache {
		infoCacheCopy[k] = v
	}
	s.mu.RUnlock()

	if !ok {
		writeV2Error(w, r, http.StatusNotFound, "not_found", fmt.Sprintf("Launcher %q not found", launcher), nil)
		return
	}

	list := buildVersionList(versionsCopy, infoCacheCopy, s)

	writeV2Success(w, r, list, false)
}

// handleV2LatestAll 返回所有启动器最新版本 map（信封包裹）。
func (s *State) handleV2LatestAll(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	latestCopy := make(map[string]string, len(s.latest))
	for k, v := range s.latest {
		latestCopy[k] = v
	}
	s.mu.RUnlock()

	// 保留 X-Latest-Versions 头以兼容
	if b, err := json.Marshal(latestCopy); err == nil {
		w.Header().Set("X-Latest-Versions", string(b))
	}
	writeV2Success(w, r, latestCopy, false)
}

// handleV2LatestLauncher 返回指定启动器最新版本号（信封包裹）。
func (s *State) handleV2LatestLauncher(w http.ResponseWriter, r *http.Request) {
	launcher := strings.TrimPrefix(r.URL.Path, "/api/v2/latest/")
	s.mu.RLock()
	defer s.mu.RUnlock()
	if val, ok := s.latest[launcher]; ok {
		// 保留 X-Latest-Version 头以兼容旧客户端
		w.Header().Set("X-Latest-Version", val)
		writeV2Success(w, r, map[string]string{"version": val}, false)
	} else {
		writeV2Error(w, r, http.StatusNotFound, "not_found", fmt.Sprintf("Launcher %q not found", launcher), nil)
	}
}

// handleV2Stats 返回站点统计信息（信封包裹）。
func (s *State) handleV2Stats(w http.ResponseWriter, r *http.Request) {
	data, err := stats.GetStats(s.BasePath)
	if err != nil {
		log.Printf("获取统计数据失败: %v", err)
		writeV2Error(w, r, http.StatusInternalServerError, "internal_error", "Failed to fetch stats", nil)
		return
	}
	writeV2Success(w, r, data, true)
}

// handleV2Bandwidth returns the in-memory rolling bandwidth status for downloads.
func (s *State) handleV2Bandwidth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	// 带宽是实时内存状态，禁用缓存（CDN/浏览器），否则前端轮询会一直拿到旧值。
	markNoStore(w)
	writeV2Success(w, r, s.bandwidth.Snapshot(), false)
}

// handleV2PowConfig 返回 PoW 公开参数（算法/迭代数/难度），供客户端预知求解成本，信封包裹。
func (s *State) handleV2PowConfig(w http.ResponseWriter, r *http.Request) {
	mgr := s.POW()
	if mgr == nil {
		writeV2Success(w, r, map[string]any{"enabled": false}, false)
		return
	}
	cfg := mgr.PublicConfig()
	markNoStore(w)
	writeV2Success(w, r, map[string]any{
		"enabled":    true,
		"algorithm":  cfg.Algorithm,
		"cost":       cfg.Cost,
		"key_length": cfg.KeyLength,
		"difficulty": cfg.Difficulty,
	}, false)
}

// handleV2Auth2FAStatus 返回 2FA 状态（信封包裹）。
func (s *State) handleV2Auth2FAStatus(w http.ResponseWriter, r *http.Request) {
	writeV2Success(w, r, map[string]bool{
		"enabled": s.Conf().TwoFactorEnabled,
	}, false)
}

// ============================================================
// v2 认证 Handler
// ============================================================

// loginOutcome 表示登录结果，Token 非空表示成功。
type loginOutcome struct {
	Token      string
	StatusCode int
	Code       string
	Message    string
}

// processLogin 执行登录核心逻辑，供 v2 调用（v1 handleLogin 保持独立不变）。
func (s *State) processLogin(r *http.Request) loginOutcome {
	ip := netutil.ExtractClientIP(r)

	// 检查锁定状态
	s.loginAttemptsMu.Lock()
	s.sweepLoginStateLocked(time.Now())
	if unlockTime, locked := s.loginLocks[ip]; locked {
		if time.Now().Before(unlockTime) {
			s.loginAttemptsMu.Unlock()
			diff := time.Until(unlockTime).Round(time.Second)
			return loginOutcome{
				StatusCode: http.StatusForbidden,
				Code:       "account_locked",
				Message:    fmt.Sprintf("账号已被锁定，请在 %v 后重试", diff),
			}
		}
		delete(s.loginLocks, ip)
		delete(s.loginAttempts, ip)
	}
	s.loginAttemptsMu.Unlock()

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		OTPCode  string `json:"otp_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return loginOutcome{StatusCode: http.StatusBadRequest, Code: "bad_request", Message: "Bad Request"}
	}
	cfg := s.Conf()

	if cfg.AdminUser == "" || cfg.AdminPassword == "" {
		return loginOutcome{StatusCode: http.StatusInternalServerError, Code: "admin_not_configured", Message: "Admin account not configured"}
	}

	// 验证用户名密码
	if req.Username != cfg.AdminUser || !auth.CheckPasswordHash(req.Password, cfg.AdminPassword) {
		s.loginAttemptsMu.Lock()
		s.loginAttempts[ip]++
		attempts := s.loginAttempts[ip]
		if attempts >= cfg.AdminMaxRetries {
			lockUntil := time.Now().Add(time.Duration(cfg.AdminLockDuration) * time.Minute)
			s.loginLocks[ip] = lockUntil
			log.Printf("IP %s 登录失败次数达到上限 (%d)，已锁定至 %v", ip, attempts, lockUntil.Format("2006-01-02 15:04:05"))
			s.loginAttemptsMu.Unlock()
			return loginOutcome{
				StatusCode: http.StatusForbidden,
				Code:       "account_locked",
				Message:    fmt.Sprintf("登录失败次数过多，账号已锁定 %d 小时", cfg.AdminLockDuration/60),
			}
		}
		log.Printf("IP %s 登录失败 (%d/%d)", ip, attempts, cfg.AdminMaxRetries)
		s.loginAttemptsMu.Unlock()
		return loginOutcome{StatusCode: http.StatusUnauthorized, Code: "invalid_credentials", Message: "Invalid credentials"}
	}

	// 验证 2FA
	if cfg.TwoFactorEnabled {
		if req.OTPCode == "" {
			return loginOutcome{StatusCode: http.StatusUnauthorized, Code: "otp_required", Message: "需要验证码"}
		}
		if !auth.ValidateTOTP(req.OTPCode, cfg.TwoFactorSecret) {
			s.loginAttemptsMu.Lock()
			s.loginAttempts[ip]++
			attempts := s.loginAttempts[ip]
			if attempts >= cfg.AdminMaxRetries {
				lockUntil := time.Now().Add(time.Duration(cfg.AdminLockDuration) * time.Minute)
				s.loginLocks[ip] = lockUntil
				log.Printf("IP %s 2FA 验证失败次数达到上限 (%d)，已锁定至 %v", ip, attempts, lockUntil.Format("2006-01-02 15:04:05"))
				s.loginAttemptsMu.Unlock()
				return loginOutcome{
					StatusCode: http.StatusForbidden,
					Code:       "account_locked",
					Message:    fmt.Sprintf("登录失败次数过多，账号已锁定 %d 小时", cfg.AdminLockDuration/60),
				}
			}
			log.Printf("IP %s 2FA 验证失败 (%d/%d)", ip, attempts, cfg.AdminMaxRetries)
			s.loginAttemptsMu.Unlock()
			return loginOutcome{StatusCode: http.StatusUnauthorized, Code: "invalid_otp", Message: "验证码错误"}
		}
	}

	// 登录成功，重置计数
	s.loginAttemptsMu.Lock()
	delete(s.loginAttempts, ip)
	delete(s.loginLocks, ip)
	s.loginAttemptsMu.Unlock()

	token, err := auth.GenerateToken()
	if err != nil {
		return loginOutcome{StatusCode: http.StatusInternalServerError, Code: "token_generation_failed", Message: "Failed to generate token"}
	}
	return loginOutcome{Token: token}
}

// handleV2Logout revokes the current administrator session.
func (s *State) handleV2Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	value := r.Header.Get("Authorization")
	// 兼容中间件同样接受裸 token（无 Bearer 前缀），一律撤销，避免"注销成功但 token 仍有效"
	if value != "" {
		auth.RevokeToken(strings.TrimSpace(strings.TrimPrefix(value, "Bearer ")))
	}
	if cookie, err := r.Cookie("admin_token"); err == nil {
		auth.RevokeToken(cookie.Value)
		http.SetCookie(w, &http.Cookie{Name: "admin_token", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
	}
	writeV2Success(w, r, map[string]bool{"revoked": true}, false)
}

// handleV2Login 管理员登录（信封包裹）。
func (s *State) handleV2Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	outcome := s.processLogin(r)
	if outcome.Token != "" {
		writeV2Success(w, r, map[string]string{"token": outcome.Token}, false)
		return
	}
	writeV2Error(w, r, outcome.StatusCode, outcome.Code, outcome.Message, nil)
}

// ============================================================
// v2 下载 Handler
// ============================================================

// handleV2DownloadPrepare 准备下载（无 PoW，CLI/API 直发授权），信封包裹。
// 替代旧内存 token：现签发 DB 授权（download_authorizations），客户端可见 token 仍为 43 字符 base64url。
func (s *State) handleV2DownloadPrepare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req downloadPrepareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeV2Error(w, r, http.StatusBadRequest, "bad_request", "Bad Request", nil)
		return
	}

	if req.FilePath == "" {
		writeV2Error(w, r, http.StatusBadRequest, "missing_required_parameters", "Missing required parameters", nil)
		return
	}

	_, info, validationErr := s.validateDownloadFile(req.FilePath)
	if validationErr != nil {
		writeV2Error(w, r, validationErr.StatusCode, validationErr.Code, validationErr.Message, nil)
		return
	}

	clientIP := netutil.ExtractClientIP(r)
	resp, err := s.issueAuthz(req.FilePath, req.ReturnURL, req.Source, "prepare", clientIP, "api", info.Size())
	if err != nil {
		writeV2Error(w, r, http.StatusInternalServerError, "token_generation_failed", "Failed to generate download token", nil)
		return
	}

	markNoStore(w)
	writeV2Success(w, r, resp, false)
}

// handleV2DownloadLanding 获取下载引导信息（Peek 授权，不消费），信封包裹。
func (s *State) handleV2DownloadLanding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		writeV2Error(w, r, http.StatusBadRequest, "missing_token", "Missing token", nil)
		return
	}

	auth, valid := s.AUTHZ().Peek(token)
	if !valid {
		writeV2Error(w, r, http.StatusForbidden, "expired_token", "Download token is invalid or expired", nil)
		return
	}

	markNoStore(w)
	url := buildDownloadURL(token, auth.FilePath)
	writeV2Success(w, r, map[string]interface{}{
		"download_url": url,
		"return_url":   auth.ReturnURL,
		"source":       auth.Source,
		"file_name":    filepath.Base(auth.FilePath),
		"file_path":    auth.FilePath,
		"flow":         auth.Flow,
	}, false)
}

// handleV2DownloadChallenge 创建 PoW 挑战（浏览器验证页用），信封包裹。
// file_path 绑定进挑战 data 并由签名保护，授权时必须一致。
func (s *State) handleV2DownloadChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	mgr := s.POW()
	if mgr == nil {
		writeV2Error(w, r, http.StatusNotImplemented, "pow_disabled", "PoW verification is disabled", nil)
		return
	}

	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		writeV2Error(w, r, http.StatusBadRequest, "missing_required_parameters", "Missing file_path", nil)
		return
	}

	// 校验文件存在且路径合法，避免为不存在的资产签发挑战
	if _, _, validationErr := s.validateDownloadFile(filePath); validationErr != nil {
		writeV2Error(w, r, validationErr.StatusCode, validationErr.Code, validationErr.Message, nil)
		return
	}

	clientIP := netutil.ExtractClientIP(r)
	challenge, err := mgr.CreateChallenge(filePath, clientIP, "web")
	if err != nil {
		log.Printf("[PoW] 创建挑战失败: %v", err)
		writeV2Error(w, r, http.StatusServiceUnavailable, "pow_busy", "Failed to create challenge", nil)
		return
	}

	markNoStore(w)
	writeV2Success(w, r, challenge, false)
}

// handleV2DownloadAuthorize 提交 PoW 解并领取下载授权，信封包裹。
// 成功后返回 download_token + download_url（同 prepare 响应结构）。
func (s *State) handleV2DownloadAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	mgr := s.POW()
	if mgr == nil {
		writeV2Error(w, r, http.StatusNotImplemented, "pow_disabled", "PoW verification is disabled", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload pow.Payload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeV2Error(w, r, http.StatusBadRequest, "bad_request", "Invalid request body", nil)
		return
	}

	// 从挑战 data 中取绑定的 file_path（签名保护，不可被客户端篡改）
	filePath, _ := payload.Challenge.Parameters.Data["file_path"].(string)
	if filePath == "" {
		writeV2Error(w, r, http.StatusBadRequest, "missing_required_parameters", "Challenge is missing file binding", nil)
		return
	}

	clientIP := netutil.ExtractClientIP(r)
	if err := mgr.VerifyAndConsume(payload, filePath, clientIP); err != nil {
		log.Printf("[PoW] 授权失败: nonce=%s file=%s challenges=%d err=%v",
			payload.Challenge.Parameters.Nonce, filePath, mgr.ChallengeCount(), err)
		switch {
		case errors.Is(err, pow.ErrChallengeNotFound), errors.Is(err, pow.ErrChallengeExpired):
			writeV2Error(w, r, http.StatusUnauthorized, "challenge_required", "Challenge not found, expired or already consumed", nil)
		case errors.Is(err, pow.ErrChallengeConsumed):
			writeV2Error(w, r, http.StatusConflict, "challenge_in_progress", "Challenge is already being used", nil)
		case errors.Is(err, pow.ErrSignatureInvalid), errors.Is(err, pow.ErrClientIPMismatch):
			writeV2Error(w, r, http.StatusForbidden, "challenge_failed", "Challenge signature invalid", nil)
		case errors.Is(err, pow.ErrFileBindingMismatch):
			writeV2Error(w, r, http.StatusForbidden, "challenge_failed", "Challenge file binding mismatch", nil)
		case errors.Is(err, pow.ErrSolutionInvalid):
			writeV2Error(w, r, http.StatusForbidden, "challenge_failed", "PoW solution invalid", nil)
		case errors.Is(err, pow.ErrVerificationBusy):
			// 瞬时并发过载：让客户端稍后重试，而不是当成服务端故障
			w.Header().Set("Retry-After", "2")
			writeV2Error(w, r, http.StatusServiceUnavailable, "pow_busy", "Verification is busy, please retry shortly", nil)
		default:
			log.Printf("[PoW] 授权失败: %v", err)
			writeV2Error(w, r, http.StatusInternalServerError, "internal_error", "Authorization failed", nil)
		}
		return
	}

	// PoW 通过后再校验文件并签发 DB 授权
	_, info, validationErr := s.validateDownloadFile(filePath)
	if validationErr != nil {
		writeV2Error(w, r, validationErr.StatusCode, validationErr.Code, validationErr.Message, nil)
		return
	}

	clientIP = netutil.ExtractClientIP(r)
	resp, err := s.issueAuthz(filePath, "", "pow", "authorize", clientIP, "web", info.Size())
	if err != nil {
		writeV2Error(w, r, http.StatusInternalServerError, "token_generation_failed", "Failed to generate download token", nil)
		return
	}

	markNoStore(w)
	writeV2Success(w, r, resp, false)
}

// ============================================================
// v2 扫描 Handler
// ============================================================

// handleV2ScanAll 触发全量扫描（信封包裹）。
func (s *State) handleV2ScanAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	if s.scanAllFunc == nil {
		writeV2Error(w, r, http.StatusNotImplemented, "scan_unavailable", "Scan not available", nil)
		return
	}
	go s.scanAllFunc()
	writeV2Success(w, r, map[string]string{"status": "accepted", "message": "Scan triggered"}, false, http.StatusAccepted)
}

// handleV2ScanLauncher 触发单个启动器扫描（信封包裹）。
func (s *State) handleV2ScanLauncher(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	if s.scanLauncherFunc == nil {
		writeV2Error(w, r, http.StatusNotImplemented, "scan_unavailable", "Scan not available", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req LauncherScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeV2Error(w, r, http.StatusBadRequest, "bad_request", "Invalid request body", nil)
		return
	}
	if req.Launcher == "" {
		writeV2Error(w, r, http.StatusBadRequest, "missing_required_parameters", "launcher is required", nil)
		return
	}

	go s.scanLauncherFunc(req.Launcher)
	writeV2Success(w, r, LauncherScanResponse{Status: "accepted", Message: "扫描已触发"}, false, http.StatusAccepted)
}

// ============================================================
// v2 内部辅助（数据获取）
// ============================================================

// getLauncherStatusData 获取所有启动器状态数据，供 v2 信封包裹。
func (s *State) getLauncherStatusData(r *http.Request) map[string][]map[string]any {
	s.mu.RLock()
	indexCopy := make(map[string]map[string]string)
	for k, v := range s.index {
		indexCopy[k] = make(map[string]string)
		for vk, vv := range v {
			indexCopy[k][vk] = vv
		}
	}
	infoCacheCopy := make(map[string]map[string]interface{})
	for k, v := range s.infoCache {
		infoCacheCopy[k] = v
	}
	s.mu.RUnlock()

	result := make(map[string][]map[string]any)
	for launcher, versions := range indexCopy {
		list := buildVersionList(versions, infoCacheCopy, s)
		result[launcher] = list
	}

	return result
}

// buildVersionList 根据版本映射和缓存构建排序后的版本列表。
func buildVersionList(versions map[string]string, infoCache map[string]map[string]interface{}, s *State) []map[string]any {
	var list []map[string]any
	for v, p := range versions {
		info := map[string]any{
			"tag_name": v,
		}

		if fileInfo, ok := infoCache[p]; ok {
			for k, val := range fileInfo {
				if k != "is_latest" {
					info[k] = val
				}
			}
		} else {
			if content, err := os.ReadFile(p); err == nil {
				var fileInfo map[string]any
				if err := json.Unmarshal(content, &fileInfo); err == nil {
					infoCache[p] = fileInfo
					s.updateInfoCache(p, fileInfo)
					for k, val := range fileInfo {
						if k != "is_latest" {
							info[k] = val
						}
					}
				}
			}
		}

		list = append(list, info)
	}
	sort.Slice(list, func(i, j int) bool {
		v1, _ := list[i]["tag_name"].(string)
		v2, _ := list[j]["tag_name"].(string)
		return version.Compare(v1, v2) > 0
	})
	return list
}

// ============================================================
// v2 管理后台 Handler（补全：config/blacklist/files/self-update）
// ============================================================

// handleV2AdminConfig 管理后台配置读写（信封包裹）。
// 秘密字段（token/密码类）GET 一律掩码返回、POST 留空保持原值：
// 控制台不需要回显密钥明文，掩码可避免密钥经浏览器缓存/代理日志泄露。
func (s *State) handleV2AdminConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		cfgCopy := *s.Conf()
		maskConfigSecrets(&cfgCopy)
		markNoStore(w)
		writeV2Success(w, r, cfgCopy, false)
		return
	}

	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
		// 以当前运行配置为基底解码：管理端漏传的字段保持现值，
		// 避免空 JSON 结构体把未提交字段静默清零。
		oldCfg := s.Conf()
		newCfg := *oldCfg
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			writeV2Error(w, r, http.StatusBadRequest, "bad_request", "Bad Request", nil)
			return
		}

		// 秘密字段留空 = 保持现值（GET 掩码后前端必然回传空值）
		if err := keepConfigSecrets(&newCfg, oldCfg); err != nil {
			writeV2Error(w, r, http.StatusInternalServerError, "hash_failed", "Failed to hash password", nil)
			return
		}

		if err := config.NormalizeConfig(&newCfg); err != nil {
			writeV2Error(w, r, http.StatusBadRequest, "invalid_config", fmt.Sprintf("Invalid config: %v", err), nil)
			return
		}

		if err := newCfg.Save(s.ProjectRoot); err != nil {
			writeV2Error(w, r, http.StatusInternalServerError, "save_failed", "Failed to save config", nil)
			return
		}

		// PoW/授权/带宽参数变化时热重建对应管理器（旧 PoW 挑战作废属预期）。
		powChanged := oldCfg.PowEnabled != newCfg.PowEnabled ||
			oldCfg.PowHMACSecret != newCfg.PowHMACSecret ||
			oldCfg.PowCost != newCfg.PowCost ||
			oldCfg.PowKeyLength != newCfg.PowKeyLength ||
			oldCfg.PowDifficulty != newCfg.PowDifficulty ||
			oldCfg.PowChallengeTTL != newCfg.PowChallengeTTL
		authzTTLChanged := oldCfg.DownloadTokenTTL != newCfg.DownloadTokenTTL
		bandwidthChanged := oldCfg.BandwidthLimitMbps != newCfg.BandwidthLimitMbps

		s.mu.Lock()
		s.Config = &newCfg
		if powChanged {
			if s.powMgr != nil {
				s.powMgr.Close()
			}
			s.powMgr = nil
			if newCfg.PowEnabled {
				s.powMgr = pow.NewManager(pow.Config{
					Secret:     newCfg.PowHMACSecret,
					Cost:       newCfg.PowCost,
					KeyLength:  newCfg.PowKeyLength,
					Difficulty: newCfg.PowDifficulty,
					TTL:        parseDuration(newCfg.PowChallengeTTL, 2*time.Minute),
				})
			}
		}
		if authzTTLChanged {
			s.authzMgr = download_authz.NewManager(parseDuration(newCfg.DownloadTokenTTL, 5*time.Minute))
		}
		if bandwidthChanged {
			s.bandwidth = bandwidth.NewTracker(int64(newCfg.BandwidthLimitMbps))
		}
		manager := s.selfUpdate
		s.mu.Unlock()

		// 防火墙设置热更新（无需重启）
		firewall.UpdateSettings(firewall.Settings{
			Enabled:      newCfg.RateLimitEnabled,
			PerMinute:    newCfg.RateLimitPerMinute,
			BanThreshold: newCfg.RateLimitBanThreshold,
		}, newCfg.FirewallWhitelist)

		if manager != nil {
			manager.UpdateConfig(selfupdate.Config{
				Enabled:       newCfg.SelfUpdateEnabled,
				RepoURL:       newCfg.SelfUpdateRepoURL,
				Channel:       newCfg.SelfUpdateChannel,
				AutoRestart:   newCfg.SelfUpdateAutoRestart,
				ProxyURL:      newCfg.ProxyURL,
				AssetProxyURL: newCfg.AssetProxyURL,
			})
		}

		writeV2Success(w, r, map[string]string{"message": "Config updated"}, false)
		return
	}

	writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
}

// maskConfigSecrets 把配置中的秘密字段清空（GET 响应不回显密钥明文）。
// two_factor_secret 例外：管理端需要现有密钥生成验证器二维码。
func maskConfigSecrets(cfg *config.Config) {
	cfg.AdminPassword = ""
	cfg.GitHubToken = ""
	cfg.PowHMACSecret = ""
	cfg.MySQLPassword = ""
	cfg.PostgresPassword = ""
}

// keepConfigSecrets 用 oldCfg 回填 newCfg 中为空的秘密字段（POST 留空=保持不变）。
// AdminPassword 非空时视为新密码，做 bcrypt 哈希后写入。
func keepConfigSecrets(newCfg, oldCfg *config.Config) error {
	if newCfg.AdminPassword == "" {
		newCfg.AdminPassword = oldCfg.AdminPassword
	} else {
		hashed, err := auth.HashPassword(newCfg.AdminPassword)
		if err != nil {
			return err
		}
		newCfg.AdminPassword = hashed
	}
	if newCfg.GitHubToken == "" {
		newCfg.GitHubToken = oldCfg.GitHubToken
	}
	if newCfg.PowHMACSecret == "" {
		newCfg.PowHMACSecret = oldCfg.PowHMACSecret
	}
	if newCfg.MySQLPassword == "" {
		newCfg.MySQLPassword = oldCfg.MySQLPassword
	}
	if newCfg.PostgresPassword == "" {
		newCfg.PostgresPassword = oldCfg.PostgresPassword
	}
	return nil
}

// handleV2AdminBlacklist 黑名单管理（信封包裹）。
// GET 支持 page/page_size/source/q 分页过滤；不带 page 参数时保持旧版全量列表
// 响应（兼容外部脚本/旧客户端）。分页响应附带按来源统计，供前端统计卡使用。
func (s *State) handleV2AdminBlacklist(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("page") == "" {
			list, err := db.GetIPBlacklist()
			if err != nil {
				writeV2Error(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
				return
			}
			writeV2Success(w, r, list, false)
			return
		}

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if pageSize < 1 {
			pageSize = 20
		}
		if pageSize > 100 {
			pageSize = 100
		}
		source := r.URL.Query().Get("source")
		keyword := strings.TrimSpace(r.URL.Query().Get("q"))

		items, total, err := db.GetIPBlacklistPaged((page-1)*pageSize, pageSize, source, keyword)
		if err != nil {
			writeV2Error(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
			return
		}
		counts, err := db.GetIPBlacklistSourceCounts()
		if err != nil {
			writeV2Error(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
			return
		}
		// 前端统计卡语义：manual=手动，external=外部同步，local(+历史 auto)=自动封禁
		counts["auto"] = counts["local"] + counts["auto"]
		writeV2Success(w, r, map[string]any{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"stats":     counts,
		}, false)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		var req struct {
			IP     string `json:"ip"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeV2Error(w, r, http.StatusBadRequest, "bad_request", "Bad Request", nil)
			return
		}
		req.IP = strings.TrimSpace(req.IP)
		if !netutil.ValidEntry(req.IP) {
			writeV2Error(w, r, http.StatusBadRequest, "invalid_param", "ip 必须是合法的 IP 地址或 CIDR 网段（如 1.2.3.4、10.0.0.0/8）", nil)
			return
		}
		if err := db.AddIPToBlacklist(req.IP, req.Reason); err != nil {
			writeV2Error(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
			return
		}
		if err := firewall.RefreshBlacklist(); err != nil {
			log.Printf("[防火墙] 刷新网段黑名单失败: %v", err)
		}
		traffic.SyncBanRecord()
		writeV2Success(w, r, map[string]string{"message": "added"}, false, http.StatusCreated)
	case http.MethodDelete:
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			writeV2Error(w, r, http.StatusBadRequest, "missing_param", "Missing ip parameter", nil)
			return
		}
		if err := db.RemoveIPFromBlacklist(ip); err != nil {
			writeV2Error(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
			return
		}
		if err := firewall.RefreshBlacklist(); err != nil {
			log.Printf("[防火墙] 刷新网段黑名单失败: %v", err)
		}
		traffic.SyncBanRecord()
		writeV2Success(w, r, map[string]string{"message": "removed"}, false)
	default:
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
	}
}

// handleV2AdminFirewallStatus 防火墙运行状态（信封包裹）。
// 汇总频率限制设置、白名单/网段封禁条数与内存中的活跃窗口/违规计数。
func (s *State) handleV2AdminFirewallStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	markNoStore(w)
	writeV2Success(w, r, firewall.Snapshot(), false)
}

// handleV2AdminFiles 文件管理（信封包裹，下载除外）。
func (s *State) handleV2AdminFiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		path := r.URL.Query().Get("path")
		fullPath, ok := s.resolveStoragePath(path)
		if !ok {
			writeV2Error(w, r, http.StatusForbidden, "forbidden", "Forbidden", nil)
			return
		}

		entries, err := os.ReadDir(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				writeV2Error(w, r, http.StatusNotFound, "not_found", "Directory not found", nil)
				return
			}
			writeV2Error(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
			return
		}

		var result []map[string]interface{}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			result = append(result, map[string]interface{}{
				"name":     e.Name(),
				"is_dir":   e.IsDir(),
				"size":     info.Size(),
				"mod_time": info.ModTime(),
			})
		}
		writeV2Success(w, r, result, false)

	case http.MethodDelete:
		path := r.URL.Query().Get("path")
		if path == "" {
			writeV2Error(w, r, http.StatusBadRequest, "missing_param", "Missing path", nil)
			return
		}
		fullPath := filepath.Join(s.BasePath, path)
		if err := removePathUnderBase(s.BasePath, fullPath); err != nil {
			if errors.Is(err, errForbiddenPath) {
				writeV2Error(w, r, http.StatusForbidden, "forbidden", "Forbidden", nil)
				return
			}
			writeV2Error(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
			return
		}
		writeV2Success(w, r, map[string]string{"message": "deleted"}, false)

	case http.MethodPost:
		// 文件上传
		path := r.URL.Query().Get("path")
		if path == "" {
			writeV2Error(w, r, http.StatusBadRequest, "missing_param", "Missing path", nil)
			return
		}
		fullPath, ok := s.resolveStoragePath(path)
		if !ok {
			writeV2Error(w, r, http.StatusForbidden, "forbidden", "Forbidden", nil)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
		file, _, err := r.FormFile("file")
		if err != nil {
			writeV2Error(w, r, http.StatusBadRequest, "bad_request", "Failed to get file: "+err.Error(), nil)
			return
		}
		defer file.Close()

		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			writeV2Error(w, r, http.StatusInternalServerError, "internal_error", "Failed to create directory", nil)
			return
		}

		dst, err := os.Create(fullPath)
		if err != nil {
			writeV2Error(w, r, http.StatusInternalServerError, "internal_error", "Failed to create file: "+err.Error(), nil)
			return
		}
		defer dst.Close()

		if _, err := io.Copy(dst, file); err != nil {
			writeV2Error(w, r, http.StatusInternalServerError, "internal_error", "Failed to save file", nil)
			return
		}

		writeV2Success(w, r, map[string]string{"message": "File uploaded"}, false)

	default:
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
	}
}

// handleV2AdminFileDownload 文件下载（二进制流，不走信封）。
func (s *State) handleV2AdminFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeV2Error(w, r, http.StatusBadRequest, "missing_param", "Missing path", nil)
		return
	}
	fullPath, ok := s.resolveStoragePath(path)
	if !ok {
		writeV2Error(w, r, http.StatusForbidden, "forbidden", "Forbidden", nil)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		writeV2Error(w, r, http.StatusNotFound, "not_found", "File not found", nil)
		return
	}

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(fullPath)})
	if disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, fullPath)
}

// handleV2AdminSelfUpdateStatus 自更新状态（信封包裹）。
func (s *State) handleV2AdminSelfUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	if s.selfUpdate == nil {
		writeV2Error(w, r, http.StatusNotImplemented, "not_configured", "Self update not configured", nil)
		return
	}
	writeV2Success(w, r, s.selfUpdate.Status(), false)
}

// handleV2AdminSelfUpdateCheck 检查更新（信封包裹）。
func (s *State) handleV2AdminSelfUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	if s.selfUpdate == nil {
		writeV2Error(w, r, http.StatusNotImplemented, "not_configured", "Self update not configured", nil)
		return
	}
	status, err := s.selfUpdate.Check(r.Context())
	if err != nil {
		writeV2Error(w, r, http.StatusBadGateway, "check_failed", err.Error(), nil)
		return
	}
	writeV2Success(w, r, status, false)
}

// handleV2AdminSelfUpdateApply 应用更新（信封包裹）。
func (s *State) handleV2AdminSelfUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	if s.selfUpdate == nil || s.applySelfUpdate == nil {
		writeV2Error(w, r, http.StatusNotImplemented, "not_configured", "Self update apply is not available", nil)
		return
	}
	if err := s.applySelfUpdate(r.Context()); err != nil {
		status := s.selfUpdate.SetApplyError(err)
		writeV2Error(w, r, http.StatusBadGateway, "apply_failed", err.Error(), map[string]any{"status": status})
		return
	}
	writeV2Success(w, r, s.selfUpdate.Status(), false)
}

// handleV2AdminSelfUpdateRestart 重启进程（信封包裹）。
func (s *State) handleV2AdminSelfUpdateRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	if s.restartProcess == nil {
		writeV2Error(w, r, http.StatusNotImplemented, "not_configured", "Restart is not available", nil)
		return
	}
	if err := s.restartProcess(); err != nil {
		writeV2Error(w, r, http.StatusInternalServerError, "restart_failed", err.Error(), nil)
		return
	}
	writeV2Success(w, r, map[string]string{
		"status":  "accepted",
		"message": "重启请求已触发",
	}, false)
}

// handleV2SelfUpdateCheckEndpoint 触发自更新检查（信封包裹）。
func (s *State) handleV2SelfUpdateCheckEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	if s.selfUpdateCheckFunc == nil {
		writeV2Error(w, r, http.StatusNotImplemented, "not_configured", "Self update check not available", nil)
		return
	}
	go s.selfUpdateCheckFunc()
	writeV2Success(w, r, map[string]string{"status": "accepted", "message": "Self update check triggered"}, false, http.StatusAccepted)
}
