package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"lemwood_mirror/internal/config"
	"lemwood_mirror/internal/db"
	"lemwood_mirror/internal/stats"
)

// 节点互联：主服（父）轮询子节点状态聚合展示，子节点向父供数（状态 + 黑名单）。
// 两侧互信凭 X-Node-Key 头（config.node_api_key / parent_node_key，恒时比较）。

// NodeStatus 是一个子节点的运行状态快照（主服聚合展示用）。
type NodeStatus struct {
	Name                 string               `json:"name"`
	URL                  string               `json:"url"`
	Online               bool                 `json:"online"`
	Error                string               `json:"error,omitempty"`
	Version              string               `json:"version,omitempty"`
	NodeName             string               `json:"node_name,omitempty"`
	UptimeSeconds        int64                `json:"uptime_seconds"`
	ActiveDownloads      int64                `json:"active_downloads"`
	CurrentBandwidthMbps float64              `json:"current_bandwidth_mbps"`
	BandwidthLimitMbps   int64                `json:"bandwidth_limit_mbps"`
	TotalDownloads       int64                `json:"total_downloads"`
	Disk                 *stats.DiskInfo      `json:"disk,omitempty"`
	Launchers            []NodeLauncherStatus `json:"launchers"`
	LastPoll             time.Time            `json:"last_poll"`
}

// NodeLauncherStatus 是子节点上报的单启动器概况。
type NodeLauncherStatus struct {
	Name     string `json:"name"`
	Versions int    `json:"versions"`
	Latest   string `json:"latest,omitempty"`
}

// nodeStart 记录进程启动时刻，用于状态上报的 uptime。
var nodeStart = time.Now()

// nodeKeyValid 校验 X-Node-Key 头与配置密钥恒时匹配。
func (s *State) nodeKeyValid(r *http.Request) bool {
	cfg := s.Conf()
	if cfg.NodeAPIKey == "" {
		return false
	}
	key := r.Header.Get("X-Node-Key")
	return subtle.ConstantTimeCompare([]byte(key), []byte(cfg.NodeAPIKey)) == 1
}

func writeV2NodeError(w http.ResponseWriter, r *http.Request, code string, message string) {
	writeV2Error(w, r, http.StatusForbidden, code, message, nil)
}

// handleV2NodeStatus 子节点自我状态上报（GET /api/v2/node/status，X-Node-Key 鉴权）。
// 父节点轮询本端点聚合到统计页与管理台。
func (s *State) handleV2NodeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	if !s.nodeKeyValid(r) {
		writeV2NodeError(w, r, "node_key_required", "Valid X-Node-Key header is required")
		return
	}

	cfg := s.Conf()
	bw := s.bandwidth.Snapshot()
	version := ""
	if mgr := s.selfUpdate; mgr != nil {
		version = mgr.Status().CurrentVersion
	}
	launchers := s.launcherBriefs()

	var disk *stats.DiskInfo
	var totalDownloads int64
	if data, err := stats.GetStats(s.BasePath); err == nil {
		disk = data.Disk
		totalDownloads = data.TotalDownloads
	}

	writeV2Success(w, r, map[string]interface{}{
		"node_name":              cfg.NodeName,
		"version":                version,
		"uptime_seconds":         int64(time.Since(nodeStart).Seconds()),
		"active_downloads":       bw.ActiveDownloads,
		"current_bandwidth_mbps": bw.CurrentBandwidthMbps,
		"bandwidth_limit_mbps":   bw.PeakBandwidthMbps,
		"total_downloads":        totalDownloads,
		"disk":                   disk,
		"launchers":              launchers,
	}, false)
}

// handleV2NodeBlacklist 父节点黑名单全量供给（GET /api/v2/node/blacklist，
// X-Node-Key 鉴权）。子节点定期拉取做全量对账（source=parent）。
func (s *State) handleV2NodeBlacklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	if !s.nodeKeyValid(r) {
		writeV2NodeError(w, r, "node_key_required", "Valid X-Node-Key header is required")
		return
	}
	list, err := db.GetIPBlacklist()
	if err != nil {
		writeV2Error(w, r, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	markNoStore(w)
	writeV2Success(w, r, list, false)
}

// launcherBriefs 汇总本机索引中每个启动器的版本数与最新版本。
func (s *State) launcherBriefs() []NodeLauncherStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	briefs := make([]NodeLauncherStatus, 0, len(s.index))
	for name, versions := range s.index {
		brief := NodeLauncherStatus{Name: name, Versions: len(versions), Latest: s.latest[name]}
		briefs = append(briefs, brief)
	}
	sort.Slice(briefs, func(i, j int) bool { return briefs[i].Name < briefs[j].Name })
	return briefs
}

// nodeStatusSnapshot 返回轮询缓存的副本（在线节点在前，各自按名称排序）。
func (s *State) nodeStatusSnapshot() []NodeStatus {
	s.nodeMu.RLock()
	defer s.nodeMu.RUnlock()
	list := make([]NodeStatus, 0, len(s.nodeStatuses))
	for _, st := range s.nodeStatuses {
		list = append(list, *st)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Online != list[j].Online {
			return list[i].Online
		}
		return list[i].Name < list[j].Name
	})
	return list
}

// PollNodes 轮询配置的所有子节点状态并刷新缓存。单个节点失败标记
// offline 并保留错误信息；节点清单逐次从配置读取，后台保存即时生效。
func (s *State) PollNodes(ctx context.Context) {
	cfg := s.Conf()
	if len(cfg.MirrorNodes) == 0 {
		return
	}
	for _, node := range cfg.MirrorNodes {
		status := s.pollNode(ctx, cfg.NodeAPIKey, node)
		s.nodeMu.Lock()
		if s.nodeStatuses == nil {
			s.nodeStatuses = make(map[string]*NodeStatus)
		}
		s.nodeStatuses[node.Name] = status
		s.nodeMu.Unlock()
	}
}

func (s *State) pollNode(ctx context.Context, apiKey string, node config.NodeRoute) *NodeStatus {
	status := &NodeStatus{Name: node.Name, URL: node.URL, LastPoll: time.Now()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, node.URL+"/api/v2/node/status", nil)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	req.Header.Set("X-Node-Key", apiKey)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		status.Error = fmt.Sprintf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return status
	}
	var payload struct {
		Data struct {
			NodeName             string               `json:"node_name"`
			Version              string               `json:"version"`
			UptimeSeconds        int64                `json:"uptime_seconds"`
			ActiveDownloads      int64                `json:"active_downloads"`
			CurrentBandwidthMbps float64              `json:"current_bandwidth_mbps"`
			BandwidthLimitMbps   int64                `json:"bandwidth_limit_mbps"`
			TotalDownloads       int64                `json:"total_downloads"`
			Disk                 *stats.DiskInfo      `json:"disk"`
			Launchers            []NodeLauncherStatus `json:"launchers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		status.Error = err.Error()
		return status
	}
	status.Online = true
	status.NodeName = payload.Data.NodeName
	status.Version = payload.Data.Version
	status.UptimeSeconds = payload.Data.UptimeSeconds
	status.ActiveDownloads = payload.Data.ActiveDownloads
	status.CurrentBandwidthMbps = payload.Data.CurrentBandwidthMbps
	status.BandwidthLimitMbps = payload.Data.BandwidthLimitMbps
	status.TotalDownloads = payload.Data.TotalDownloads
	status.Disk = payload.Data.Disk
	status.Launchers = payload.Data.Launchers
	return status
}

// handleV2AdminNodes 子节点清单管理（GET 返回配置 + 最近轮询缓存；POST 保存清单）。
func (s *State) handleV2AdminNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := s.Conf()
		nodes := make([]config.NodeRoute, len(cfg.MirrorNodes))
		copy(nodes, cfg.MirrorNodes)
		markNoStore(w)
		writeV2Success(w, r, map[string]interface{}{
			"nodes":  nodes,
			"status": s.nodeStatusSnapshot(),
		}, false)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var req struct {
			Nodes []config.NodeRoute `json:"nodes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeV2Error(w, r, http.StatusBadRequest, "bad_request", "Bad Request", nil)
			return
		}
		newCfg := *s.Conf()
		newCfg.MirrorNodes = req.Nodes
		if err := config.NormalizeConfig(&newCfg); err != nil {
			writeV2Error(w, r, http.StatusBadRequest, "invalid_nodes", err.Error(), nil)
			return
		}
		if err := newCfg.Save(s.ProjectRoot); err != nil {
			writeV2Error(w, r, http.StatusInternalServerError, "save_failed", err.Error(), nil)
			return
		}
		s.mu.Lock()
		s.Config = &newCfg
		s.mu.Unlock()
		writeV2Success(w, r, map[string]string{"message": "Nodes updated"}, false)
	default:
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
	}
}

// handleV2AdminNodesRefresh 立即重新轮询全部子节点（管理台手动刷新）。
func (s *State) handleV2AdminNodesRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeV2Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method Not Allowed", nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s.PollNodes(ctx)
	writeV2Success(w, r, map[string]interface{}{
		"status": s.nodeStatusSnapshot(),
	}, false)
}
