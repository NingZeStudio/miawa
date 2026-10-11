package stats

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"lemwood_mirror/internal/db"
	"lemwood_mirror/internal/geoip"
	"lemwood_mirror/internal/netutil"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type IPInfo struct {
	Status  string `json:"status"`
	Country string `json:"country"`
	Region  string `json:"regionName"`
	City    string `json:"city"`
	Query   string `json:"query"`
	Expires time.Time
}

type writeTask struct {
	query string
	args  []interface{}
}

type ipInfoTask struct {
	ip       string
	callback func(info *IPInfo)
}

var (
	ipCache = make(map[string]*IPInfo)
	ipMutex sync.RWMutex

	lastSnapshot     *StatsData
	lastSnapshotTime time.Time
	snapshotMu       sync.RWMutex
	refreshInFlight  sync.Mutex

	writeQueue   chan *writeTask
	ipInfoQueue  chan *ipInfoTask
	workerWg     sync.WaitGroup
	workerCtx    context.Context
	workerCancel context.CancelFunc

	droppedCount int64
)

const (
	defaultWorkers   = 4
	defaultQueueSize = 1000
	maxIPCacheSize   = 50000
	cacheTTL         = 5 * time.Minute
	snapshotTTL      = 15 * time.Minute
)

func scanCount(label, query string) int64 {
	var n int64
	if err := db.DB.QueryRow(db.Rebind(query)).Scan(&n); err != nil {
		log.Printf("[Stats] %s 查询失败: %v", label, err)
		return 0
	}
	return n
}

func InitWritePool(workers int, queueSize int) {
	if workers <= 0 {
		workers = defaultWorkers
	}
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}

	workerCtx, workerCancel = context.WithCancel(context.Background())
	writeQueue = make(chan *writeTask, queueSize)
	ipInfoQueue = make(chan *ipInfoTask, queueSize)

	for i := 0; i < workers; i++ {
		workerWg.Add(1)
		go writeWorker()
	}

	workerWg.Add(1)
	go ipInfoWorker()

	// 启动 IP 流量记录定期清理 goroutine（每小时清理 24 小时前的 IP 级流量数据）
	workerWg.Add(1)
	go trafficCleanupWorker()

	log.Printf("[Stats] 写入工作池已初始化: %d workers, queue size: %d", workers, queueSize)
}

// trafficCleanupWorker 每小时清理一次 ip_daily_traffic 中
// 超过 24 小时的记录。IP 级数据仅保留当日用于防刷墙，历史流量已聚合到无 IP 的 daily_traffic 表。
func trafficCleanupWorker() {
	defer workerWg.Done()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// 启动时先清理一次
	if deleted, err := db.CleanupOldTrafficRecords(); err != nil {
		log.Printf("[Stats] 清理过期 IP 流量记录失败: %v", err)
	} else if deleted > 0 {
		log.Printf("[Stats] 清理了 %d 条过期 IP 流量记录", deleted)
	}

	for {
		select {
		case <-ticker.C:
			if deleted, err := db.CleanupOldTrafficRecords(); err != nil {
				log.Printf("[Stats] 清理过期 IP 流量记录失败: %v", err)
			} else if deleted > 0 {
				log.Printf("[Stats] 清理了 %d 条过期 IP 流量记录", deleted)
			}
		case <-workerCtx.Done():
			return
		}
	}
}

func CloseWritePool() {
	if writeQueue == nil && ipInfoQueue == nil {
		return
	}
	// 先取消 ctx：让 ipInfoWorker / trafficCleanupWorker 能立即停止 HTTP 限流与远程请求，
	// 进入"仅排空队列"模式，从而在 close 之前快速 drain ipInfoQueue 剩余任务。
	if workerCancel != nil {
		workerCancel()
	}
	// 再关闭队列：writeWorker 不会感知 ctx，会继续把 writeQueue 剩余任务写入库后退出。
	if writeQueue != nil {
		close(writeQueue)
	}
	if ipInfoQueue != nil {
		close(ipInfoQueue)
	}

	done := make(chan struct{})
	go func() {
		workerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		log.Printf("[Stats] 关闭写入池超时，可能仍有未落盘记录")
	}
}

// InvalidateSnapshot forces the next stats request to refresh from the DB.
func InvalidateSnapshot() {
	snapshotMu.Lock()
	lastSnapshot = nil
	lastSnapshotTime = time.Time{}
	snapshotMu.Unlock()
	// 必须同时删除 DB 快照行：loadSnapshot 在内存未命中时会读到表里的旧快照，
	// 若只清内存，下次查询会端出最长 snapshotTTL（15min）前的陈旧数据，
	// 表现为「下载后统计面板长时间不更新」。删行后下次查询走 cold start 同步重算。
	if db.DB != nil {
		if _, err := db.DB.Exec(`DELETE FROM stats_snapshot WHERE id = 1`); err != nil {
			log.Printf("[Stats] 删除统计快照失败: %v", err)
		}
	}
}

func DroppedCount() int64 {
	return atomic.LoadInt64(&droppedCount)
}

func writeWorker() {
	defer workerWg.Done()
	for task := range writeQueue {
		if _, err := db.DB.Exec(db.Rebind(task.query), task.args...); err != nil {
			log.Printf("数据库写入失败: %v", err)
		}
	}
}

func ipInfoWorker() {
	defer workerWg.Done()
	if err := geoip.Init(); err != nil {
		log.Printf("[Stats] GeoIP 初始化失败: %v", err)
	}

	for task := range ipInfoQueue {
		// 关闭中：直接排空队列，不发起本地查询。
		if workerCtx.Err() != nil {
			continue
		}

		info := fetchIPInfo(task.ip)
		if info != nil {
			ipMutex.Lock()
			if len(ipCache) >= maxIPCacheSize {
				evictIPCache()
			}
			ipCache[task.ip] = info
			ipMutex.Unlock()
		}
		if task.callback != nil {
			task.callback(info)
		}
	}
}

func evictIPCache() {
	now := time.Now()
	for k, v := range ipCache {
		if now.After(v.Expires) {
			delete(ipCache, k)
		}
	}
	if len(ipCache) >= maxIPCacheSize {
		oldest := ""
		oldestTime := time.Now()
		for k, v := range ipCache {
			if v.Expires.Before(oldestTime) {
				oldestTime = v.Expires
				oldest = k
			}
		}
		if oldest != "" {
			delete(ipCache, oldest)
		}
	}
}

func isPrivateIP(ipStr string) bool {
	if ipStr == "localhost" {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func fetchIPInfo(ip string) *IPInfo {
	if isPrivateIP(ip) {
		return &IPInfo{
			Status:  "success",
			Country: "Local",
			Region:  "Local",
			City:    "Local",
			Expires: time.Now().Add(24 * time.Hour),
		}
	}

	country, region, city, ok := geoip.Lookup(ip)
	if !ok {
		return nil
	}
	return &IPInfo{
		Status:  "success",
		Country: country,
		Region:  region,
		City:    city,
		Expires: time.Now().Add(24 * time.Hour),
	}
}

func getIPInfoAsync(ip string, callback func(info *IPInfo)) {
	ipMutex.RLock()
	if info, ok := ipCache[ip]; ok {
		if time.Now().Before(info.Expires) {
			ipMutex.RUnlock()
			if callback != nil {
				callback(info)
			}
			return
		}
	}
	ipMutex.RUnlock()

	if ipInfoQueue == nil {
		if callback != nil {
			callback(nil)
		}
		return
	}

	defer func() {
		if r := recover(); r != nil {
			if callback != nil {
				callback(nil)
			}
		}
	}()
	select {
	case ipInfoQueue <- &ipInfoTask{ip: ip, callback: callback}:
	default:
		if callback != nil {
			callback(nil)
		}
	}
}

func enqueueWrite(task *writeTask) {
	if writeQueue == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			atomic.AddInt64(&droppedCount, 1)
		}
	}()
	select {
	case writeQueue <- task:
	default:
		atomic.AddInt64(&droppedCount, 1)
		log.Printf("写入队列已满，丢弃记录 (总丢弃: %d)", atomic.LoadInt64(&droppedCount))
	}
}

func RecordVisit(r *http.Request) {
	path := r.URL.Path

	if strings.HasPrefix(path, "/dist/") ||
		strings.HasPrefix(path, "/assets/") ||
		strings.HasPrefix(path, "/api/") ||
		path == "/favicon.svg" {
		return
	}

	if writeQueue == nil {
		return
	}

	// 解析 IP 属地后只存储地域信息，不存储 IP 本身
	ip := netutil.ExtractClientIP(r)
	getIPInfoAsync(ip, func(info *IPInfo) {
		country, region, city := "", "", ""
		if info != nil {
			country = info.Country
			region = info.Region
			city = info.City
		}

		date := time.Now().UTC().Format("2006-01-02")
		key := db.VisitAggregateKey(date, country, region, city)
		query := `INSERT INTO visits (ip, path, user_agent, referer, country, region, city, visit_count, aggregate_key, created_at)
			VALUES ('', '', '', '', ?, ?, ?, ?, ?, ?)`
		if db.IsMySQL() {
			query += ` ON DUPLICATE KEY UPDATE visit_count=visit_count+VALUES(visit_count)`
		} else if db.IsPostgres() {
			query += ` ON CONFLICT (aggregate_key) DO UPDATE SET visit_count=visits.visit_count+EXCLUDED.visit_count`
		} else {
			query += ` ON CONFLICT(aggregate_key) DO UPDATE SET visit_count=visit_count+excluded.visit_count`
		}
		enqueueWrite(&writeTask{
			query: query,
			args:  []interface{}{country, region, city, 1, key, time.Now().UTC().Format("2006-01-02 00:00:00")},
		})
	})
}

// RecordDownload 写入 downloads 表（旧口径）。
//
// Deprecated: 下载统计唯一口径已迁移至 download_events（db.RecordDownloadEvent），
// downloads 表冻结为只读兜底；生产路径不应再调用本函数。
func RecordDownload(r *http.Request, fileName, launcher, version string) {
	ip := netutil.ExtractClientIP(r)

	if writeQueue == nil {
		return
	}

	// 解析 IP 属地后只存储国家，不存储 IP 本身
	getIPInfoAsync(ip, func(info *IPInfo) {
		country := ""
		if info != nil {
			country = info.Country
		}

		enqueueWrite(&writeTask{
			query: `INSERT INTO downloads (file_name, launcher, version, ip, country) VALUES (?, ?, ?, ?, ?)`,
			args:  []interface{}{fileName, launcher, version, "", country},
		})
	})
}

type StatsData struct {
	TotalVisits      int64 `json:"total_visits"`
	TotalDownloads   int64 `json:"total_downloads"`
	TotalDays        int64 `json:"total_days"`
	Last30Visits     int64 `json:"last_30_visits"`
	Last30Downloads  int64 `json:"last_30_downloads"`
	TotalTrafficBytes  int64 `json:"total_traffic_bytes"`  // 总流量（served 口径：实际送出的全部字节）
	Last30TrafficBytes int64 `json:"last_30_traffic_bytes"` // 近 30 日流量（served 口径）
	// TotalServedBytes/Last30ServedBytes 兼容保留：与对应 traffic 字段同口径同值
	TotalServedBytes   int64          `json:"total_served_bytes"`
	Last30ServedBytes  int64          `json:"last_30_served_bytes"`
	Disk               *DiskInfo      `json:"disk"`
	TopDownloads       []DownloadRank `json:"top_downloads"`
	GeoDistribution    []GeoStat      `json:"geo_distribution"`
	DailyStats         []DailyStat    `json:"daily_stats"`
	DroppedRecords     int64          `json:"dropped_records"`
}

type DownloadRank struct {
	Launcher string `json:"launcher"`
	Count    int64  `json:"count"`
}

// GeoStat 保持旧版 API 响应形状不变：country 字段在国内条目上承载省份名（台湾视同省份），
// 海外与未知来源分别聚合为「海外」「其他」两个合成条目。
type GeoStat struct {
	Country string `json:"country"`
	Count   int64  `json:"count"`
}

type DailyStat struct {
	Date          string `json:"date"`
	VisitCount    int64  `json:"visit_count"`
	DownloadCount int64  `json:"download_count"`
	TrafficBytes  int64  `json:"traffic_bytes"`
}

// cloneStatsData 返回快照的深拷贝（切片字段独立），供 GetStats 在副本上
// 填充 Disk/DroppedRecords，避免并发请求改写共享缓存对象。
func cloneStatsData(src *StatsData) *StatsData {
	if src == nil {
		return nil
	}
	dst := *src
	dst.TopDownloads = append([]DownloadRank(nil), src.TopDownloads...)
	dst.GeoDistribution = append([]GeoStat(nil), src.GeoDistribution...)
	dst.DailyStats = append([]DailyStat(nil), src.DailyStats...)
	return &dst
}

func GetStats(storagePath string) (*StatsData, error) {
	snapshot, updatedAt := loadSnapshot()

	if snapshot != nil {
		age := time.Since(updatedAt)
		snapshot = cloneStatsData(snapshot)
		if age < snapshotTTL {
			return decorateSnapshot(snapshot, storagePath), nil
		}
		// Stale: serve old data, refresh in background
		go RefreshSnapshot()
		return decorateSnapshot(snapshot, storagePath), nil
	}

	// Cold start: no snapshot yet, compute synchronously
	if err := RefreshSnapshot(); err != nil {
		return &StatsData{
			TopDownloads:    []DownloadRank{},
			GeoDistribution: []GeoStat{},
			DailyStats:      []DailyStat{},
			DroppedRecords:  DroppedCount(),
		}, err
	}

	snapshot, _ = loadSnapshot()
	snapshot = cloneStatsData(snapshot)
	if snapshot == nil {
		snapshot = &StatsData{
			TopDownloads:    []DownloadRank{},
			GeoDistribution: []GeoStat{},
			DailyStats:      []DailyStat{},
		}
	}
	return decorateSnapshot(snapshot, storagePath), nil
}

// decorateSnapshot 返回带实时 Disk/DroppedRecords 的快照副本。
// 必须拷贝而非直接改写共享的缓存对象：并发请求同时读改同一 *StatsData 会构成 data race。
func decorateSnapshot(src *StatsData, storagePath string) *StatsData {
	dst := *src
	if storagePath != "" {
		if diskInfo, err := GetDiskUsage(storagePath); err == nil {
			dst.Disk = diskInfo
		}
	}
	dst.DroppedRecords = DroppedCount()
	return &dst
}

func loadSnapshot() (*StatsData, time.Time) {
	snapshotMu.RLock()
	if lastSnapshot != nil {
		cached := lastSnapshot
		updated := lastSnapshotTime
		snapshotMu.RUnlock()
		return cached, updated
	}
	snapshotMu.RUnlock()

	if db.DB == nil {
		return nil, time.Time{}
	}

	var dataJSON string
	var updatedAt time.Time
	var err error
	if db.IsMySQL() {
		err = db.DB.QueryRow("SELECT data, updated_at FROM stats_snapshot WHERE id = 1").Scan(&dataJSON, &updatedAt)
	} else if db.IsPostgres() {
		err = db.DB.QueryRow("SELECT data, updated_at FROM stats_snapshot WHERE id = $1", 1).Scan(&dataJSON, &updatedAt)
	} else {
		var raw interface{}
		err = db.DB.QueryRow("SELECT data, updated_at FROM stats_snapshot WHERE id = 1").Scan(&dataJSON, &raw)
		if err == nil {
			switch v := raw.(type) {
			case time.Time:
				updatedAt = v
			case string:
				updatedAt, _ = time.Parse("2006-01-02 15:04:05", v)
			case []byte:
				updatedAt, _ = time.Parse("2006-01-02 15:04:05", string(v))
			}
		}
	}
	if err != nil {
		return nil, time.Time{}
	}

	var data StatsData
	if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
		log.Printf("[Stats] 快照 JSON 解析失败: %v", err)
		return nil, time.Time{}
	}

	snapshotMu.Lock()
	lastSnapshot = &data
	lastSnapshotTime = updatedAt
	snapshotMu.Unlock()

	return &data, updatedAt
}

func computeStatsData() *StatsData {
	data := &StatsData{
		TopDownloads:    []DownloadRank{},
		GeoDistribution: []GeoStat{},
		DailyStats:      []DailyStat{},
		DroppedRecords:  DroppedCount(),
	}

	if db.DB == nil {
		return data
	}

	if db.IsMySQL() || db.IsPostgres() {
		var wg sync.WaitGroup
		last30VisitsQuery := "SELECT COALESCE(SUM(visit_count), 0) FROM visits WHERE created_at > DATE_SUB(UTC_TIMESTAMP(), INTERVAL 30 DAY)"
		if db.IsPostgres() {
			last30VisitsQuery = "SELECT COALESCE(SUM(visit_count), 0) FROM visits WHERE created_at > (NOW() AT TIME ZONE 'UTC') - INTERVAL '30 days'"
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			data.TotalVisits = scanCount("total_visits", "SELECT COALESCE(SUM(visit_count), 0) FROM visits")
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			data.Last30Visits = scanCount("last_30_visits", last30VisitsQuery)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			computeTotalDays(data)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			queryTopDownloads(data)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			queryGeoDistribution(data)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			queryDailyStats(data)
		}()

		wg.Wait()

		// 下载次数与流量 = 冻结基线（daily_traffic/daily_completed_traffic）+ 事件表（download_events）。
		// 下载次数/top 完全来自事件表（含历史回填）；流量字节 = 基线 SUM + 事件 SUM。
		applyDownloadAndTrafficStats(data)

		return data
	}

	data.TotalVisits = scanCount("total_visits", "SELECT COALESCE(SUM(visit_count), 0) FROM visits")
	data.Last30Visits = scanCount("last_30_visits", "SELECT COALESCE(SUM(visit_count), 0) FROM visits WHERE created_at > datetime('now', '-30 days')")

	computeTotalDays(data)
	queryTopDownloads(data)
	queryGeoDistribution(data)
	queryDailyStats(data)

	// 下载次数与流量 = 冻结基线 + 事件表（见 applyDownloadAndTrafficStats）
	applyDownloadAndTrafficStats(data)

	return data
}

// mergeDailyTraffic 将每日流量 map 合并到 DailyStats 中对应的日期。
func mergeDailyTraffic(data *StatsData, trafficMap map[string]int64) {
	for date, bytes := range trafficMap {
		findOrCreateDailyStat(data, date).TrafficBytes = bytes
	}
}

// mergeDailyDownloads 用事件表的按日下载计数覆盖 DailyStats 的每日下载次数。
// 事件表是下载次数的唯一口径（downloads 表已冻结不再写入），故每次有事件计数即覆盖。
func mergeDailyDownloads(data *StatsData, countMap map[string]int64) {
	for date, count := range countMap {
		findOrCreateDailyStat(data, date).DownloadCount = count
	}
	sort.Slice(data.DailyStats, func(i, j int) bool { return data.DailyStats[i].Date > data.DailyStats[j].Date })
}

func findOrCreateDailyStat(data *StatsData, date string) *DailyStat {
	for i := range data.DailyStats {
		if data.DailyStats[i].Date == date {
			return &data.DailyStats[i]
		}
	}
	data.DailyStats = append(data.DailyStats, DailyStat{Date: date})
	return &data.DailyStats[len(data.DailyStats)-1]
}

// applyDownloadAndTrafficStats 计算下载次数与流量字节：
//   - 下载次数/top 完全来自 download_events（含历史回填）；
//   - 流量字节 = 冻结基线（daily_traffic）SUM + 事件表全量 SUM，served 口径。
//
// 历史字节无法逐事件回填（基线是按日聚合），故基线表冻结为只读历史总量，新数据由事件表承载。
func applyDownloadAndTrafficStats(data *StatsData) {
	// 下载次数（事件表，含历史回填）
	if v, err := db.GetTotalDownloadsFromEvents(); err == nil {
		data.TotalDownloads = v
	}

	// 每日事件聚合（served/count）
	evtStats, _ := db.GetDailyEventStats(30)
	var last30EvtCount, last30EvtServed int64
	evtCount := map[string]int64{}
	evtServed := map[string]int64{}
	for _, s := range evtStats {
		last30EvtCount += s.Count
		last30EvtServed += s.Served
		evtCount[s.Date] = s.Count
		evtServed[s.Date] = s.Served
	}
	data.Last30Downloads = last30EvtCount
	mergeDailyDownloads(data, evtCount)

	// 流量总量 = 冻结基线（daily_traffic）+ 事件表全量，served 口径。
	// 不再使用 completed 口径合计：多段复用连接（IDM/aria2/浏览器分段）的
	// completed 几乎恒为 false（multi-range 的预估字节取整个文件大小），
	// completed 合计会严重低估实际送出流量。
	totalBaseServed, _ := db.GetTotalTraffic()
	totalEvtServed, _ := db.GetTotalServedFromEvents()
	data.TotalTrafficBytes = totalBaseServed + totalEvtServed
	data.TotalServedBytes = data.TotalTrafficBytes // 兼容保留：与总流量同口径同值

	// last30 每日：baseline served + events served
	baseServed, _ := db.GetDailyTrafficStats(30)
	var last30BaseServed int64
	dailyServedMap := map[string]int64{}
	for _, s := range baseServed {
		last30BaseServed += s.Bytes
		dailyServedMap[s.Date] += s.Bytes
	}
	for date, c := range evtServed {
		dailyServedMap[date] += c
	}
	data.Last30TrafficBytes = last30BaseServed + last30EvtServed
	data.Last30ServedBytes = data.Last30TrafficBytes

	// 排序必须放在所有合并之后：mergeDailyTraffic 可能追加基线中独有的旧日期，
	// 追加到尾部会破坏 mergeDailyDownloads 里已排好的顺序。
	mergeDailyTraffic(data, dailyServedMap)
	sort.Slice(data.DailyStats, func(i, j int) bool { return data.DailyStats[i].Date > data.DailyStats[j].Date })
}

func saveSnapshot(data *StatsData) error {
	if db.DB == nil {
		return nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化快照失败: %w", err)
	}
	var query string
	if db.IsMySQL() {
		query = "INSERT INTO stats_snapshot (id, data, updated_at) VALUES (1, ?, NOW()) ON DUPLICATE KEY UPDATE data = VALUES(data), updated_at = NOW()"
	} else if db.IsPostgres() {
		query = "INSERT INTO stats_snapshot (id, data, updated_at) VALUES (1, $1, CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, updated_at = CURRENT_TIMESTAMP"
	} else {
		query = "INSERT INTO stats_snapshot (id, data, updated_at) VALUES (1, ?, datetime('now')) ON CONFLICT(id) DO UPDATE SET data = excluded.data, updated_at = datetime('now')"
	}
	if _, err := db.DB.Exec(db.Rebind(query), string(b)); err != nil {
		return fmt.Errorf("保存快照失败: %w", err)
	}
	return nil
}

func RefreshSnapshot() error {
	refreshInFlight.Lock()
	defer refreshInFlight.Unlock()

	data := computeStatsData()
	if err := saveSnapshot(data); err != nil {
		return err
	}

	snapshotMu.Lock()
	lastSnapshot = data
	lastSnapshotTime = time.Now()
	snapshotMu.Unlock()

	return nil
}

// minVisitDateQuery 返回按数据库类型定制的"最早访问日期"查询，
// 结果为 'YYYY-MM-DD' 字符串，无记录时为 NULL。
// MySQL 必须用 DATE_FORMAT 强制返回字符串：DSN 开启 parseTime=True 后，
// DATE 类型结果会被驱动转成 time.Time，无法 Scan 进 string（旧实现的隐患）。
func minVisitDateQuery() string {
	if db.IsMySQL() {
		return "SELECT DATE_FORMAT(MIN(created_at), '%Y-%m-%d') FROM visits"
	}
	if db.IsPostgres() {
		return "SELECT TO_CHAR(MIN(created_at), 'YYYY-MM-DD') FROM visits"
	}
	return "SELECT date(MIN(created_at)) FROM visits"
}

// parseStatsTime 兼容解析历史数据里可能出现的多种时间格式。
// 无时区的格式按 UTC 解析（写入侧统一为 UTC，见 db.createTables）。
func parseStatsTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// daysSince 计算自 t 至今的天数，至少为 1（运行当天即算第 1 天），
// 容忍写入/读取时区差异导致的轻微"未来时间"。
func daysSince(t time.Time) int64 {
	days := int64(time.Since(t).Hours()/24) + 1
	if days < 1 {
		days = 1
	}
	return days
}

// computeTotalDays 计算站点运行天数：优先取 visits 表最早记录的日期，
// 无访问记录（或查询失败）时回退到 system_info.start_time（服务首次启动时间）。
// 只要能确定任一有效起始时间，TotalDays 至少为 1；两者都不可用时保持 0。
func computeTotalDays(data *StatsData) {
	var minDate sql.NullString
	if err := db.DB.QueryRow(minVisitDateQuery()).Scan(&minDate); err != nil {
		log.Printf("[Stats] min_visit_date 查询失败: %v", err)
	} else if minDate.Valid {
		if t, ok := parseStatsTime(minDate.String); ok {
			data.TotalDays = daysSince(t)
			return
		}
		log.Printf("[Stats] min_visit_date 解析失败: %q", minDate.String)
	}

	// 回退：系统首次启动时间
	keyRef := "key"
	if db.IsMySQL() {
		keyRef = "`key`"
	} else if db.IsPostgres() {
		keyRef = `"key"`
	}
	var startTimeStr string
	if err := db.DB.QueryRow(db.Rebind("SELECT value FROM system_info WHERE " + keyRef + " = 'start_time'")).Scan(&startTimeStr); err != nil {
		log.Printf("[Stats] system_info.start_time 查询失败: %v", err)
		return
	}
	if t, ok := parseStatsTime(startTimeStr); ok {
		data.TotalDays = daysSince(t)
	} else {
		log.Printf("[Stats] system_info.start_time 解析失败: %q", startTimeStr)
	}
}

func queryTopDownloads(data *StatsData) {
	ranks, err := db.GetTopDownloadsFromEvents(10)
	if err != nil {
		return
	}
	converted := make([]DownloadRank, 0, len(ranks))
	for _, r := range ranks {
		converted = append(converted, DownloadRank{Launcher: r.Launcher, Count: r.Count})
	}
	data.TopDownloads = converted
}

// queryGeoDistribution 统计国内省份访问分布：国内聚合 country 为 中国/China 以及
// 含「台湾」的记录（台湾视同国内省份），按 visits.region（ip2region 省份段）分组，
// 并归一为 34 个一级行政单位（直辖市/自治区/省/特别行政区，地级市回所属省），
// region 为空的台湾记录兜底并入归一后的「台湾」计数；海外国家合并为「海外」，
// Local/空白国家/无省份的未知来源合并为「其他」。响应字段沿用旧版 geo_distribution 形状。
func queryGeoDistribution(data *StatsData) {
	rows, err := db.DB.Query(db.Rebind(`
		SELECT region, COALESCE(SUM(visit_count), 0) as c
		FROM visits
		WHERE (country IN ('中国', 'China') OR country LIKE '%台湾%') AND region != ''
		GROUP BY region`))
	if err != nil {
		return
	}
	defer rows.Close()

	var geos []GeoStat
	counts := make(map[string]int64)
	var unmapped int64
	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			continue
		}
		// 归一为一级行政单位：地级市回所属省，全/简称统一为简称；归一不了的并入「其他」
		if n := geoip.NormalizeRegion(name); n != "" {
			counts[n] += count
		} else {
			unmapped += count
		}
	}
	scanSum := func(query string) int64 {
		var total int64
		if err := db.DB.QueryRow(db.Rebind(query)).Scan(&total); err != nil {
			return 0
		}
		return total
	}

	// region 为空的记录兜底：并入归一后的同名计数，避免出现重复条目
	if tw := scanSum(`
		SELECT COALESCE(SUM(visit_count), 0)
		FROM visits
		WHERE country LIKE '%台湾%' AND (region = '' OR region IS NULL)`); tw > 0 {
		counts["台湾"] += tw
	}
	for name, count := range counts {
		geos = append(geos, GeoStat{Country: name, Count: count})
	}
	if overseas := scanSum(`
		SELECT COALESCE(SUM(visit_count), 0)
		FROM visits
		WHERE country != '' AND country != 'Local' AND country != '中国' AND country != 'China'
		  AND country NOT LIKE '%台湾%'`); overseas > 0 {
		geos = append(geos, GeoStat{Country: "海外", Count: overseas})
	}
	if unknown := scanSum(`
		SELECT COALESCE(SUM(visit_count), 0)
		FROM visits
		WHERE country = '' OR country = 'Local' OR ((country = '中国' OR country = 'China') AND region = '')`); unknown+unmapped > 0 {
		geos = append(geos, GeoStat{Country: "其他", Count: unknown + unmapped})
	}

	// 计数相同时按名称升序，保证并列条目顺序确定（兼容旧口径的同时输出可复现）
	sort.Slice(geos, func(i, j int) bool {
		if geos[i].Count != geos[j].Count {
			return geos[i].Count > geos[j].Count
		}
		return geos[i].Country < geos[j].Country
	})
	data.GeoDistribution = geos
}

func dailyQueryFlavor() (visitQ, downloadQ string) {
	if db.IsMySQL() {
		visitQ = "SELECT DATE_FORMAT(created_at, '%Y-%m-%d') as d, COALESCE(SUM(visit_count), 0) FROM visits GROUP BY d ORDER BY d DESC LIMIT 30"
		downloadQ = "SELECT DATE_FORMAT(created_at, '%Y-%m-%d') as d, COUNT(*) FROM downloads GROUP BY d ORDER BY d DESC LIMIT 30"
	} else if db.IsPostgres() {
		visitQ = "SELECT TO_CHAR(created_at, 'YYYY-MM-DD') as d, COALESCE(SUM(visit_count), 0) FROM visits GROUP BY d ORDER BY d DESC LIMIT 30"
		downloadQ = "SELECT TO_CHAR(created_at, 'YYYY-MM-DD') as d, COUNT(*) FROM downloads GROUP BY d ORDER BY d DESC LIMIT 30"
	} else {
		visitQ = "SELECT date(created_at) as d, COALESCE(SUM(visit_count), 0) FROM visits GROUP BY d ORDER BY d DESC LIMIT 30"
		downloadQ = "SELECT date(created_at) as d, COUNT(*) FROM downloads GROUP BY d ORDER BY d DESC LIMIT 30"
	}
	return
}

func queryDailyStats(data *StatsData) {
	visitQ, downloadQ := dailyQueryFlavor()
	fillDailyStats(data, visitQ, downloadQ)
}

func fillDailyStats(data *StatsData, visitQ, downloadQ string) {
	dailyMap := make(map[string]*DailyStat)

	vRows, err := db.DB.Query(visitQ)
	if err == nil {
		defer vRows.Close()
		for vRows.Next() {
			var d string
			var c int64
			if err := vRows.Scan(&d, &c); err != nil {
				continue
			}
			if dailyMap[d] == nil {
				dailyMap[d] = &DailyStat{Date: d}
			}
			dailyMap[d].VisitCount = c
		}
	}

	dRows, err := db.DB.Query(downloadQ)
	if err == nil {
		defer dRows.Close()
		for dRows.Next() {
			var d string
			var c int64
			if err := dRows.Scan(&d, &c); err != nil {
				continue
			}
			if dailyMap[d] == nil {
				dailyMap[d] = &DailyStat{Date: d}
			}
			dailyMap[d].DownloadCount = c
		}
	}

	var daily []DailyStat
	for _, v := range dailyMap {
		daily = append(daily, *v)
	}
	sort.Slice(daily, func(i, j int) bool {
		return daily[i].Date > daily[j].Date
	})

	data.DailyStats = daily
}
