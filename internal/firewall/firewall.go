// Package firewall 提供站点级防火墙能力：
//   - 网段（CIDR）黑名单匹配：ip_blacklist 表中以 CIDR 形式存在的封禁条目
//     （精确 IP 仍由 SecurityMiddleware 直接查 DB，网段条目在此内存匹配）；
//   - IP/网段白名单：命中白名单的客户端豁免请求频率限制、外部黑名单与
//     流量防刷墙自动封禁（管理员手动封禁仍然生效）；
//   - 请求频率限制：单 IP 每分钟请求数上限，超限记违规，违规累计达到阈值
//     自动写入黑名单（ban_type=rate_limit）。
package firewall

import (
	"fmt"
	"lemwood_mirror/internal/db"
	"lemwood_mirror/internal/netutil"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Settings 控制请求频率限制行为。
type Settings struct {
	Enabled      bool `json:"enabled"`       // 是否启用频率限制
	PerMinute    int  `json:"per_minute"`    // 单 IP 每分钟最大请求数
	BanThreshold int  `json:"ban_threshold"` // 违规累计达到该值自动封禁（1 分钟窗口内连续超限才计违规）
}

// BanFunc 自动封禁回调：由调用方注入（写 DB 黑名单 + 同步封禁记录文件），
// firewall 包自身不依赖 traffic 包以避免 import 环。
type BanFunc func(ip, reason string) error

// Decision 是一次频率限制检查的结果。
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration // 拒绝时距当前窗口重置的剩余时间
	Banned     bool          // 本次检查触发了自动封禁
	Reason     string        // 触发封禁时的原因
}

const (
	strikeTTL     = 30 * time.Minute // 违规计数保留时长，超时归零
	maxTrackedIPs = 100000           // 内存保护：窗口数上限，超出时立即清理
	cleanupPeriod = time.Minute      // 后台清理周期
)

// entrySet 是一组精确 IP + CIDR 网段的集合，支持并发读取。
type entrySet struct {
	exact map[string]struct{}
	cidrs []*net.IPNet
}

func (e *entrySet) match(ip net.IP) bool {
	if ip == nil || e == nil {
		return false
	}
	if _, ok := e.exact[ip.String()]; ok {
		return true
	}
	for _, n := range e.cidrs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func buildEntrySet(entries []string) *entrySet {
	set := &entrySet{exact: make(map[string]struct{})}
	for _, e := range entries {
		exact, cidr, ok := netutil.ParseEntry(e)
		switch {
		case cidr != nil:
			set.cidrs = append(set.cidrs, cidr)
		case ok:
			set.exact[exact.String()] = struct{}{}
		}
	}
	return set
}

type window struct {
	start time.Time
	count int
}

type strike struct {
	count int
	last  time.Time
}

type Manager struct {
	mu        sync.RWMutex
	settings  Settings
	whitelist *entrySet
	cidrBans  *entrySet // 黑名单中的 CIDR 条目（精确 IP 由 DB 查询承载）

	windows map[string]*window
	strikes map[string]*strike

	banFunc BanFunc
	now     func() time.Time

	ctx    chan struct{}
	closed sync.Once
}

var defaultManager *Manager

// Init 初始化全局防火墙并启动后台清理。可在运行时重复调用以整体重置（测试用）。
func Init(settings Settings, whitelist []string, banFunc BanFunc) {
	Close()
	ctx := make(chan struct{})
	m := &Manager{
		settings:  normalizeSettings(settings),
		whitelist: buildEntrySet(whitelist),
		cidrBans:  &entrySet{exact: make(map[string]struct{})},
		windows:   make(map[string]*window),
		strikes:   make(map[string]*strike),
		banFunc:   banFunc,
		now:       time.Now,
		ctx:       ctx,
	}
	go m.cleanupWorker()
	defaultManager = m
}

// Close 停止后台清理（幂等，可安全重复调用）。
func Close() {
	if defaultManager != nil {
		defaultManager.shutdown()
	}
	defaultManager = nil
}

func (m *Manager) shutdown() {
	if m == nil {
		return
	}
	m.closed.Do(func() {
		close(m.ctx)
	})
}

// UpdateSettings 热更新频率限制参数与白名单（管理后台保存配置时调用）。
func UpdateSettings(settings Settings, whitelist []string) {
	m := defaultManager
	if m == nil {
		Init(settings, whitelist, nil)
		return
	}
	m.mu.Lock()
	m.settings = normalizeSettings(settings)
	m.whitelist = buildEntrySet(whitelist)
	m.mu.Unlock()
}

func normalizeSettings(s Settings) Settings {
	if s.PerMinute <= 0 {
		s.PerMinute = 300
	}
	if s.BanThreshold <= 0 {
		s.BanThreshold = 3
	}
	return s
}

// RefreshBlacklist 从 DB 黑名单重建内存 CIDR 集合（精确 IP 不在此维护）。
// 所有黑名单变更路径（管理员增删、自动封禁、外部同步）都应调用。
func RefreshBlacklist() error {
	m := defaultManager
	if m == nil {
		return nil
	}
	blacklist, err := db.GetIPBlacklist()
	if err != nil {
		return fmt.Errorf("读取黑名单失败: %w", err)
	}
	var cidrEntries []string
	for _, item := range blacklist {
		entry := item["ip"]
		if strings.Contains(entry, "/") {
			cidrEntries = append(cidrEntries, entry)
		}
	}
	m.mu.Lock()
	m.cidrBans = buildEntrySet(cidrEntries)
	m.mu.Unlock()
	return nil
}

// Whitelisted 报告 IP 是否命中白名单（豁免频率限制、外部黑名单与流量自动封禁）。
func Whitelisted(ip string) bool {
	m := defaultManager
	if m == nil || ip == "" {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.whitelist.match(parsed)
}

// MatchBlacklistCIDR 报告 IP 是否命中黑名单中的 CIDR 网段条目。
func MatchBlacklistCIDR(ip string) bool {
	m := defaultManager
	if m == nil || ip == "" {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cidrBans.match(parsed)
}

// Allow 对单次请求做频率限制检查。未启用、未命中白名单之外的 IP 计入窗口；
// 违规累计达到阈值时通过 Init 注入的 BanFunc 自动封禁。
func Allow(ip string) Decision {
	m := defaultManager
	if m == nil || ip == "" {
		return Decision{Allowed: true}
	}
	m.mu.Lock()
	if !m.settings.Enabled {
		m.mu.Unlock()
		return Decision{Allowed: true}
	}
	if m.whitelist.match(net.ParseIP(ip)) {
		m.mu.Unlock()
		return Decision{Allowed: true}
	}

	now := m.now()
	w := m.windows[ip]
	// 内存保护：窗口数达到上限时立即清理过期窗口（正常由后台清理兜底）
	if w == nil && len(m.windows) >= maxTrackedIPs {
		for trackedIP, tracked := range m.windows {
			if now.Sub(tracked.start) >= time.Minute {
				delete(m.windows, trackedIP)
			}
		}
		w = m.windows[ip]
	}
	if w == nil || now.Sub(w.start) >= time.Minute {
		w = &window{start: now}
		m.windows[ip] = w
	}
	w.count++
	if w.count <= m.settings.PerMinute {
		m.mu.Unlock()
		return Decision{Allowed: true}
	}

	// 超限：记一次违规并拒绝
	retryAfter := time.Minute - now.Sub(w.start)
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	s := m.strikes[ip]
	if s == nil || now.Sub(s.last) > strikeTTL {
		s = &strike{}
		m.strikes[ip] = s
	}
	s.count++
	s.last = now

	banned := false
	reason := ""
	if s.count >= m.settings.BanThreshold {
		banned = true
		reason = fmt.Sprintf("请求频率超过限制（每分钟超过 %d 次请求）", m.settings.PerMinute)
		delete(m.strikes, ip) // 已封禁，避免重复触发
	}
	m.mu.Unlock()

	if banned && m.banFunc != nil {
		if err := m.banFunc(ip, reason); err != nil {
			log.Printf("[防火墙] 自动封禁 IP %s 失败: %v", ip, err)
			return Decision{Allowed: false, RetryAfter: retryAfter}
		}
		log.Printf("[防火墙] IP %s 因频繁超限已被自动封禁（%s），如有误封请联系管理员", ip, reason)
	}
	return Decision{Allowed: false, RetryAfter: retryAfter, Banned: banned, Reason: reason}
}

// GetSettings 返回当前频率限制设置（供状态展示）。
func GetSettings() Settings {
	m := defaultManager
	if m == nil {
		return Settings{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

// StatusSnapshot 是防火墙运行状态的只读快照（管理端展示用）。
type StatusSnapshot struct {
	Settings       Settings `json:"settings"`
	WhitelistCount int      `json:"whitelist_count"` // 白名单条目数（IP + CIDR）
	CIDRBanCount   int      `json:"cidr_ban_count"`  // 黑名单中的网段条目数
	TrackedIPs     int      `json:"tracked_ips"`     // 近 2 分钟内有请求窗口的 IP 数
	ActiveStrikes  int      `json:"active_strikes"`  // 30 分钟内有违规记录的 IP 数
}

// Snapshot 返回防火墙当前运行状态。未 Init 时返回零值快照。
func Snapshot() StatusSnapshot {
	m := defaultManager
	if m == nil {
		return StatusSnapshot{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return StatusSnapshot{
		Settings:       m.settings,
		WhitelistCount: entrySetSize(m.whitelist),
		CIDRBanCount:   entrySetSize(m.cidrBans),
		TrackedIPs:     len(m.windows),
		ActiveStrikes:  len(m.strikes),
	}
}

func entrySetSize(e *entrySet) int {
	if e == nil {
		return 0
	}
	return len(e.exact) + len(e.cidrs)
}

func (m *Manager) cleanupWorker() {
	ticker := time.NewTicker(cleanupPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx:
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *Manager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for ip, w := range m.windows {
		if now.Sub(w.start) >= 2*time.Minute {
			delete(m.windows, ip)
		}
	}
	for ip, s := range m.strikes {
		if now.Sub(s.last) > strikeTTL {
			delete(m.strikes, ip)
		}
	}
}
