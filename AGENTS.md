# lemwood-mirror 项目记忆

面向 Minecraft 启动器分发场景的 GitHub Release 镜像服务（Go 单体 + 内嵌前端）。入口 `cmd/mirror/main.go`，后端全部在 `internal/`。

## 构建与测试

- Go 1.25。测试/构建：`go test -count=1 ./...`、`go vet ./...`。CI 用 `go test ./... -count=1 -timeout 120s`。
- Termux 环境 go 不在默认 PATH，位于 `/data/data/com.termux/files/usr/lib/go/bin`（apt 的 golang 包），用前先 `export PATH=$PREFIX/lib/go/bin:$PATH`。
- 构建：`go build -o mirror ./cmd/mirror`；开发运行：`go run ./cmd/mirror`。
- 纯 Go 构建，`CGO_ENABLED=0`：SQLite 驱动是 `modernc.org/sqlite`（pure-Go），**不需要 cgo**，不要为 SQLite 切换 `mattn/go-sqlite3`。
- 版本号靠 `-ldflags "-s -w -X main.Version=..."` 注入：CI tag 构建注入 `github.ref_name`；Docker 需 `--build-arg VERSION=x.y.z`（Dockerfile ARG VERSION）；dev 构建为 `"dev"`，自更新检查时视为"有更新"。
- **构建产物不入库（2026-08-15 起）**：`mirror-linux-amd64` 曾误入库（66MB）已 `git rm --cached` 移除，.gitignore 含 `mirror-linux-amd64`。本地 `go build` 的二进制只用于本地验证/部署，**不要 `git add` 任何二进制**；正式产物由 CI 按 tag 构建发布。
- 前端构建：`cd frontend && pnpm build`，产物输出到 `web/default/`（git 跟踪的内嵌资源，改完 frontend/src 必须重新构建否则线上不生效）。
- 管理端构建：`cd admin-app && pnpm build`，产物输出到 `web/admin/`（同样被 git 跟踪，会被构建重写 hash 文件名）。
- CI（`.github/workflows/build.yml`）先单独执行 Go test/vet 和前端检查/构建，再矩阵构建 windows/linux × amd64/arm64/x86；**仅 tag 推送时由单独 job 发 Release**（softprops/action-gh-release）。pnpm 版本固定 10.12.4。

## 后端架构（internal/ 包职责）

- `cmd/mirror/main.go` — 唯一入口。启动顺序：释放内嵌前端 → 加载配置 → InitDB → 流量 tracker → stats 写池（4 worker + 1000 缓冲）→ server.State → scanner（cron）→ selfupdate manager → cron 调度。`Scanner.scanLauncher` 是同步主循环，`ScanAll` 用 `scanMu.TryLock` 防重入。
- `internal/server/` — HTTP 路由 + SPA 托管 + 下载处理器。`server.go`（41KB，单体）是改动热点；`v2.go` 是公开 API v2 handler；`spafallback.go` SPA 回退；`http.go`/`utils.go` 小工具。**下载处理器是流量计数的唯一计数点**（见下"流量统计双口径"）。
- `internal/db/` — 数据库抽象（SQLite/MySQL/PostgreSQL），见下"数据库"。
- `internal/config/` — 配置加载/保存/迁移，见下"配置"。
- `internal/stats/` — 访问/下载统计，异步写池 + 快照预热。
- `internal/traffic/` — 单 IP 每日流量上限（防刷墙），预检 + 实际传输记账。
- `internal/github/` — go-github v50 封装，见下"GitHub 客户端"。
- `internal/selfupdate/` — GitHub Release 自更新，见下"自更新"。
- `internal/geoip/` — ip2region v4+v6 xdb 内嵌，本地查属地。
- `internal/version/` — 自研 SemVer-like 比较，被 launcher 索引和 selfupdate 共用。
- `internal/downloader/` — 版本索引生成与资产下载。
- `internal/download_authz/` — DB 授权状态表（`download_authorizations`）：43 字符 base64url token，DB 存 sha256 hash；Issue/Peek/Consume（atomic 单次消费）。见下"PoW 下载验证"。
- `internal/auth/` — 管理员认证 + TOTP，`CleanupTokens()` 后台清理。
- `internal/assets/` — 启动时把内嵌前端释放到项目根目录。
- `internal/firewall/` — 站点级防火墙（见下「防火墙」）。
- `internal/blacklist/` / `internal/netutil/`（客户端 IP 解析 + `ParseEntry`/`ValidEntry` IP/CIDR 解析）/ `internal/storage/` — 各自独立小包。

## 数据库

- 全局 `db.DB *sql.DB` + `db.isMySQL` / `db.isPostgres` 标志。**SQLite 默认**（`<storage>/stats.db`），MySQL 可选（`mysql_host` 非空即启用），PostgreSQL 可选（`database_mode=pgsql`，或 `auto` 且 `postgres_host` 非空）。
- SQLite：`SetMaxOpenConns(1)`（单写连接，避免 SQLite 锁冲突），开 WAL + `busy_timeout=10000`。MySQL：连接池 25/10，`ConnMaxLifetime=1h`。PG：pgx v5（`rebind()` 把 `?` 转 `$n`），连接池 2/1。**注意**：单连接使 SQLite 的 WAL"读不阻塞写"失效，热路径 DB 操作全部串行——已知取舍。
- **迁移系统**在 `internal/db/migrations.go`：`CurrentSchemaVersion` 常量（当前 = 6；v5 加 visit_count/event_count 聚合列，v6 加 aggregate_key 聚合键 + 唯一索引）。新增迁移 = 往 `migrations` 切片末尾追加一项 + 递增该常量。每个 `Up` **必须幂等且三方言兼容**（MySQL 用 INSERT IGNORE、PG 用 ON CONFLICT DO NOTHING、SQLite 用 INSERT OR IGNORE）。`system_info` 表的 `schema_version` 是提交点，每个迁移成功后立即写入。MySQL 用 `ON DUPLICATE KEY UPDATE`，PG 用 `ON CONFLICT ("key") DO UPDATE`，SQLite 用 `INSERT OR REPLACE` 写 schema_version。v6 的回填合并+建唯一索引必须在同一事务内（崩溃留下重复 key 会让 CREATE UNIQUE INDEX 永久失败无法自愈）。
- `mysql_migration: true` 且存在 `stats.db` → 一次性 SQLite→MySQL 迁移，旧库改名为 `.bak`。
- 表清单：`visits, downloads, ip_blacklist, ip_daily_traffic, daily_traffic, daily_completed_traffic, download_authorizations, download_events, system_info`。建表在 `createTables()`，按 `isMySQL` 分支用各自方言。
- 注：repo 镜像功能已移除，`repo_*` 表不再迁移；`launcher.mode` 的 `clone`/`all` 已废弃（Git 镜像功能已移除），仅为兼容旧配置保留，`ShouldSyncRelease` 对 `release`/`all` 返回 true、对 `clone` 返回 false。

## 配置

- `config.yaml` 和旧 `config.json` **都在 .gitignore（含 secrets，绝不提交）**。仓库里 `internal/config/default.yaml` 是提交的嵌入式默认模板（经 `embedded.go` 内嵌）。
- `LoadConfig` 行为：无 yaml 但存在旧 `config.json` → 自动迁移到 yaml 并删除旧 json；两者都没有 → 释放内嵌 `default.yaml` 写盘。
- **后台保存会从 `defaultConfigTemplate`（text/template）整体重写 config.yaml**，不要指望手写在 yaml 里加的自定义注释能在后台保存后保留。
- `GITHUB_TOKEN` 环境变量覆盖 yaml 里的 `github_token`，**但绝不写回 cfg.GitHubToken**（2026-09-05 起）：读取点统一用 `cfg.EffectiveGitHubToken()`；此前 NormalizeConfig 直接注入会导致 LoadConfig"自动补齐"把 env token 持久化到 config.yaml（密钥落盘缺陷，已修）。
- **admin config API 秘密字段**（admin_password/github_token/pow_hmac_secret/mysql_password/postgres_password）：GET 一律掩码返回（two_factor_secret 例外，管理端 enroll 需要回显），POST 留空=保持原值（`maskConfigSecrets`/`keepConfigSecrets`）。
- `NormalizeConfig` 不变量：`traffic_limit_gb < 0` → 5；`max_versions ≤ 0` → 3；`admin_enabled` 但 user/password 空 → 自动禁用后台；`check_cron` 空 → `*/10 * * * *`。
- 密码用 bcrypt 哈希：`htpasswd -bnBC 14 "" <password> | tr -d ':\n'`。

## 本地数据库环境（2026-09-01 搭建，供迁移集成测试）

- 已安装 PostgreSQL 18.6 与 MySQL 8.4.11（Ubuntu apt）。**端口按仓库集成测试约定**：PG = 127.0.0.1:55432，MySQL = 127.0.0.1:33306（PG 改 `port` 于 `/etc/postgresql/18/main/postgresql.conf`；MySQL 改 `/etc/mysql/mysql.conf.d/mysqld.cnf` 追加 `port = 33306` 与 `mysql_native_password=ON`，8.4 默认禁用该插件而 go-sql-driver DSN 未带 allowPublicKeyRetrieval，必须开）。
- 凭据：PG 用户 `lemwood`（superuser，pg_hba 对 127.0.0.1/::1 为 trust，免密）；MySQL 用户 `lemwood`/`testpass`（mysql_native_password，GRANT ALL ON *.*，仅回环）。
- 集成测试：`LEMWOOD_MIGRATION_INTEGRATION=1 go test ./internal/db/`（默认 skip）。覆盖：MySQL→PG 与 SQLite→PG 内置清洗迁移、CIDR 黑名单三方言读写、SQLite→MySQL 一次性迁移携带 CIDR。PG 目标库测试内自动 drop/recreate（`resetPostgresTestDB`），可重复跑；清洗迁移有一次性完成标记，换库才能重测。
- 已建库：`lemwood_builtin_test`、`lemwood_builtin_sqlite_test`、`lemwood_fw_test`（PG+MySQL 同名）、`lemwood_source`（MySQL）。
- 部署实测结论（2026-09-01，SQLite→PG→MySQL 三阶段切换）：`ip_blacklist` 的 CIDR 条目在三方言下均按 TEXT 精确存取，无需 schema 变更；迁移后防刷墙流量状态（download_events 当日 bytes）与统计口径完整保留；管理端 CIDR 增删 + 内存网段匹配在三种后端下均即时生效。

## 防火墙（2026-09 起）

- **黑名单分页（2026-09-05）**：`GET /api/v2/admin/blacklist` 带 `page/page_size/source/q` 时走服务端分页（`db.GetIPBlacklistPaged`，响应含 items/total/stats，stats 里 `auto = local + 历史 auto` 供前端统计卡）；不带 `page` 保持旧版全量列表（兼容外部脚本）。`GET /api/v2/admin/firewall/status` 返回 `firewall.Snapshot()`（设置 + 白名单/网段封禁/活跃窗口/违规 IP 计数）。管理端 BlacklistPage 全部改服务端分页 + 顶部防火墙状态卡。
- **下载多段复用（2026-09-05）**：`/download/{path}` 对已消费 token 允许"并行分段复用"（`download_authz.PeekReuse`）：仅当请求带 `Range` 头、同 file_path、同 client_ip、consumed_at 在 `download_token_ttl` 窗口内时放行（不再二次消费授权）。无 Range 的重放仍 403。多线程下载器（IDM/aria2/浏览器分段）与断点续传由此可用。

- `internal/firewall/` 单例管理器，`Init(Settings, whitelist, banFunc)` 在 main.go 启动时调用；`UpdateSettings` 支持管理后台保存配置后热更新（无需重启）。**依赖方向**：firewall → db/netutil；config → netutil（白名单校验）。**config 绝不能 import firewall**（db 依赖 config，会成环），所以 IP/CIDR 解析函数放在 `netutil.ParseEntry/ValidEntry` 三方共用。
- **三层拦截**（`SecurityMiddleware`，现为 `(s *State)` 方法，读取 `Config.AppealContact` 替代了旧硬编码 QQ 群号）：① 本地黑名单精确 IP 查 DB；② CIDR 网段封禁内存匹配（`RefreshBlacklist` 从 `ip_blacklist` 表读取含 `/` 的条目构建，管理员增删/自动封禁/外部同步后都要调用刷新）；③ 外部黑名单。频率限制（429 + Retry-After）在黑名单检查之后。
- **请求频率限制**：固定 1 分钟窗口（`Allow`），超限记违规，违规 30 分钟 TTL，累计达 `rate_limit_ban_threshold` 自动写黑名单（source=local，ban_type=rate_limit）。封禁动作通过 `Init` 注入的 `BanFunc` 回调执行（写 DB + `traffic.SyncBanRecord`），firewall 包不反向依赖 traffic。未 Init 或 settings.Enabled=false 时全部放行——**这是 server 包测试不受防火墙影响的关键**（firewall 是包级单例，测试用完要 `Close()` 或重新 `Init` 复位）。
- **白名单语义**：豁免频率限制、外部黑名单、流量预检与自动封禁（traffic `ReserveTraffic`/`CheckAndBan` 开头有豁免分支）；**不豁免本地黑名单**（管理员手动封禁始终生效）。
- 配置项：`rate_limit_enabled`(true)/`rate_limit_per_minute`(300)/`rate_limit_ban_threshold`(3)/`firewall_whitelist`([]string, IP 或 CIDR，NormalizeConfig 校验合法性)。`handleV2AdminConfig` POST 对这 4 个字段做**缺键保留原值**（管理端表单只提交注册过的字段，不做保护会在 UI 保存时清空白名单）；黑名单 POST 用 `netutil.ValidEntry` 校验 IP/CIDR，非法返回 400。
- 管理端 UI：ConfigPage「防火墙设置」卡片；BlacklistPage 添加表单支持 CIDR（前端轻校验 + 后端权威校验），「自动封禁」分类已修正为匹配 source `local`（旧代码只匹配不存在的 `auto` 值，自动封禁记录一直显示"未知"）。

## 流量统计双口径（2026-08 起）

- `daily_traffic` = served（服务器写出字节，含中止传输），防刷墙专用。
- `daily_completed_traffic` = 完整传输字节（`counter.Total >= EstimateTransferBytes(...)` 且状态 200/206），stats 展示口径。
- 唯一计数点：`internal/server/server.go` 下载处理器（`responseWriterCounter` 包装 ResponseWriter，ReadFrom 直通保留 sendfile；FinalizeTraffic 已 defer 化回收 pending 预留）。事件落库：`db.RecordDownloadEvent`（`download_events` 表）；防刷墙读 `GetDailyServedByIPFromEventsToday`。**`RecordTraffic`/`RecordCompletedTraffic`/`stats.RecordDownload` 已冻结只供测试，勿在请求路径调用。**
- stats 展示口径 = 冻结基线 SUM + `download_events` SUM（合并后统一排序 DailyStats）；防刷墙取 `download_events` 按 IP 当日 served。
- `stats.InitWritePool(4, 1000)`：访问/下载记录异步落库，**不要在请求路径里同步写 DB 统计**。`/api/v2/stats` 走 `RefreshSnapshot` 预热（启动 + 每 10m 刷新）的快照，避免每次跑聚合查询。

## PoW 下载验证 + 状态表授权/流量（2026-08-12 起，替代极验）

- **背景**：移除极验（`internal/captcha` 已删）与内存 `download_token`（`internal/download_token` 已删）。改为 PoW 自动验证客户端正规性 + DB 状态表承载授权与流量。参考 PoW实现.md 与 MapleMirror（AGPL，仅参考数据模型形状/流程，**从零实现不复制源码**）。单节点、改 v2（不新建 v3）。前端已迁移（2026-08-12）。
- **PoW（`internal/pow`）**：ALTCHA 风格 PBKDF2-SHA256（自实现，无 altcha 依赖）。挑战**内存态**三态 `open→issuing→consumed`（不落库），TTL 默认 2m；挑战绑定 file_path + client_ip（VerifyAndConsume 校验跨 IP 转用返回 ErrClientIPMismatch）。验证并发上限 4，占满返回 ErrVerificationBusy → HTTP 503 pow_busy。`pow.Solve(p, max)` 供测试/CLI。`servePowPage`（server.go）是浏览器直连验证页：Web Crypto PBKDF2 求解 → POST authorize → 跳转。
- **授权（`internal/download_authz` + `download_authorizations` 表）**：token = 32 字节无填充 base64url（43 字符），DB 只存 `token_hash=sha256(token)` hex。`Issue/Peek/Consume`（`ConsumeDownloadAuthorization` atomic：`issued 且未过期 → consumed`，单次防重放）。
- **事件/流量（`download_events` 表）**：每次下载一行（`authorization_id/file/launcher/version/client_ip/country/bytes_served/completed/status_code/date`）。**全切口径**：served=bytes_served（含中止），completed=bytes_served WHERE completed=1。防刷墙按 IP 当日 served 读 `download_events`（`GetDailyServedByIPFromEventsToday`，注入 `traffic.InitTracker`）；`daily_traffic`/`daily_completed_traffic` **冻结为只读历史基线**（不再写入，schema v4 迁移从 `downloads` 回填事件行 bytes=0）。
- **v2 端点**：`POST /api/v2/downloads/prepare`（CLI/API 直发授权，无 PoW，保留）、`GET /landing`（Peek）、`GET /downloads/challenge` + `POST /downloads/authorize`（浏览器 PoW，替代极验 verify）、`GET /api/v2/pow/config`。`/download/{path}`：token 取 `?token=` 或 `Authorization: Bearer`；处理顺序：路径/文件校验→method→Peek(绑定校验)→ReserveTraffic→Consume→ServeFile→RecordDownloadEvent→FinalizeTraffic(defer)；任何失败都不烧授权；HEAD 直通不消费授权不计流量。无 token+浏览器→PoW 验证页，无 token+非浏览器→403 `verification_required`。
- **stats**：`applyDownloadAndTrafficStats`（stats.go）合并：下载次数/top 从 `download_events`（含回填）；流量字节 = 冻结基线 SUM + 事件 SUM（按日 union）；visits/geo 不变（仍读 `visits`）。**注意（2026-08-14 修复）**：每日 `DailyStats[].DownloadCount` 与 `TrafficBytes` 一并合并事件口径，`queryDailyStats` 里的 `downloads` 表查询只作兜底，否则迁移后当日下载计数恒为 0（`downloads` 不再写入）。
- **config**：删 `captcha_*`；加 `pow_enabled`(默认 true)/`pow_algorithm`("PBKDF2-SHA256")/`pow_cost`(500)/`pow_key_length`(32)/`pow_difficulty`(14)/`pow_challenge_ttl`("2m")/`pow_hmac_secret`(空→`pow.NewManager` 创建时随机生成 32 字节——空密钥的 HMAC 任何人可算，配合内存中未过期 nonce 可伪造 difficulty=0 挑战绕过 PoW，2026-09-05 修复)/`download_token_ttl`("5m")。
- **前端**（`frontend/src`）：`globalConfig.api.endpoints` 用 `powConfig:'/pow/config'` + `downloadChallenge:'/downloads/challenge'` + `downloadAuthorize:'/downloads/authorize'`；`api.js` 移除 `getCaptchaConfig`/`verifyDownload`，新增 `getPowConfig`/`createDownloadChallenge`/`authorizeDownload`。`VersionList.vue`/`FilesView.vue` 的 `handleDownload`：`powConfig.enabled` 时路由 `/verify?file=...`（VerifyView 现在是 PoW 求解页：Web Crypto PBKDF2 → challenge → solve → authorize → `/download-started?token=`），否则 `prepareDownload`（CLI/API 路径无 PoW）。`DownloadStartedView` 不变（landing 契约未变）。构建：`cd frontend && pnpm build` → `web/default/`（git 跟踪，改完必须重新构建）。
- **PoW derivedKey 编码坑**：客户端（浏览器/CLI）`derivedKey` 用**无填充** base64url，服务端 `verifySolution` 须用 `base64.RawURLEncoding.DecodeString(strings.TrimRight(derivedKey,"="))` 解码后与重算字节做 `ConstantTimeCompare`，不要用带填充的 `URLEncoding.EncodeToString` 直接比字符串。
- **DATETIME 读取坑**：modernc.org/sqlite 把 DATETIME 列读回时解析为 time.Time 再以 RFC3339 回传给 `Scan(&string)`。`download_authz.isExpired` 用 `parseAuthzTime` 多布局兼容（AuthzTimeFormat + RFC3339*）。DB 内 SQL `expires_at > ?` 字符串比较仍按 "2006-01-02 15:04:05" 文本字典序工作。

## GitHub 客户端

- `internal/github/client.go`，go-github v50。`BackoffIfRateLimited`：当 `resp.Rate.Remaining == 0` 时 sleep 到 reset+2s（上限 15 分钟）。
- `GetReleaseAssetDigests`：go-github v50 的 ReleaseAsset 无 Digest 字段，用本地结构体直接解析 release JSON 获取资产 SHA-256（供自更新校验）。
- Token 可选：无 token 60 req/h，有 token 5000/h。`GITHUB_TOKEN` env 优先于配置。
- 代理：`proxy_url` 非空 → 显式 `http.ProxyURL`；为空 → 回退 `http.DefaultTransport`（即尊重 `HTTP_PROXY`/`HTTPS_PROXY` 环境变量）。运行时切代理用 `SetProxy`（atomic.Pointer 安全并发）。
- `ListReleasesByPolicy` 分页拉取并按 `include_prerelease` 过滤；`GetLatestRelease` API 不返回 pre-release，含 pre-release 时走 `ListReleases` 取首个。

## GeoIP

- `internal/geoip/`：ip2region v4 + v6 xdb 通过 `//go:embed` 内嵌（合计约 48MB，是二进制体积主因），本地查属地**不联网**。`sync.Once` 懒初始化。

## 客户端 IP 解析

- `netutil.ExtractClientIP`：仅当 RemoteAddr 是 loopback/private 才信任转发头；XFF **从右往左**取第一个非可信地址（客户端可注入的最左值绝不采用），全内网链回退 RemoteAddr；XFF 缺失时才看 `X-Real-IP`。改这里要同步更新 `client_ip_test.go` 的用例语义。

## 版本比较

- `internal/version/version.go` 自研 SemVer-like `Compare`，被 launcher 索引排序和 selfupdate 更新检查**共用**。pre-release 后缀排名低于同核心版本，pre-release 标识符按 SemVer §11 比较（数字段按数值：beta.9 < beta.10）。`IsParseable(""/"dev"/"latest")` = false → 自更新检查时视为"有更新"。

## 自更新

- 二进制路径在 main.go `resolveSelfBinaryPath()`（`os.Executable()+EvalSymlinks`）解析一次并传给 NewManager/doRestart。
- **容器内自更新未被禁用**（曾计划检测 `/.dockerenv`，未实现——2026-08-27 决定不做 Docker 适配）。
- 无状态持久化文件（`selfupdate_state.json` 未实现），重启对账靠内存 status + 新进程 Check。
- 重启用 `syscall.Exec`（unix，`restart_unix.go`）原地替换进程；Windows 走 `restart_other.go`（os.StartProcess）。`SetOnRestart` 在 main.go 注入闭包，重启前先优雅关 HTTP + 刷统计写池。Windows 替换运行中的 exe 用 rename-aside（旧文件改名 `.old` 再落新文件）。
- **下载链接内置（2026-08-14 重构）**：`Apply()` 用 `buildUpdateCandidates` 按 CI 资产命名规则（`mirror-{goos}-{label}{ext}`）构造候选（先裸二进制，404 回退 `.tar.gz`/`.zip`）。**完整性校验（2026-08-27）**：先用 `gh.GetReleaseAssetDigests` 从 GitHub API 取资产 SHA-256 摘要比对（API 失败降级为仅魔数校验并告警；有摘要则强校验不匹配即中止）。
- 频道：`notify` / `release` / `preview`。
- 自动更新默认开启：`DefaultConfig` 为 `SelfUpdateEnabled=true`/`Channel="release"`/`AutoRestart=true`/`CheckCron="0 */6 * * *"`。

## 2026-08-27 全面修复清单（review 后落地）

- **进程稳定性**：cron 用 `cron.New(cron.WithChain(cron.Recover(...)))` + `safeGo` 包装所有后台 goroutine（panic 不再杀进程）；SIGTERM 优雅关闭可达化（`http.go` 改 net.Listen 先交出 srv 再后台 Serve，main 等 srvCh ≤3s 后 Shutdown）；停机/自动重启前调用 `s.Close()`（PoW 管理器）+ 刷统计写池。
- **并发安全**：`clearLatestFlag` 内层 map 拷贝后写（与公开 API 读共享 map 的 fatal crash）；stats 快照 GetStats 返回副本（Disk/DroppedRecords 不再写共享对象）。
- **授权消费顺序重排（H3）**：`/download/{path}` 现为 路径校验→method 校验→Peek(绑定校验)→ReserveTraffic→Consume→ServeFile；任何校验失败不再烧掉一次性 PoW 授权；HEAD 不消费授权不计流量；Consume 失败按 0 字节回滚预留。
- **IP 伪造防线**：XFF 取右值（见上节）；登录失败 map 超 4096 触发清扫。
- **路径安全**：admin 文件接口（GET/POST/download）改 `safeJoinUnderBase`（Rel 语义，修 HasPrefix 弱前缀兄弟目录逃逸）；Content-Disposition 用 mime.FormatMediaType 转义。
- **DoS 加固**：prepare/login/blacklist POST 加 MaxBytesReader(1MB)；`download_authorizations` 每小时 cron 清理（标记过期 + 删 7 天前历史行）；外部黑名单同步兼容 IPv6（单冒号才切端口/注释）。
- **PG 支持**：createTables PG 分支不再提前 return，runMigrations 三方言跑通（v2-v4 补 PG 分支）；PG 数据迁移触发条件统一 `usePostgres`；cmd/db-migrate 运行时探测补齐 visit_count/event_count/aggregate_key 列。
- **config**:后台保存以当前配置为基底解码（漏传字段不再清零）；Save 改 temp+rename 原子写。
- **杂项**：MySQL Last30Visits 改 SUM(visit_count)+UTC_TIMESTAMP；GetDailyEventStats 阈值统一 UTC；downloader 字节数不符拒绝上线、index.json 在资产齐全后才写、公网 IP 查询走 https、FormatDownloadURL 剥离 serverAddress 自带端口避免双端口；tar.gz 超限返回真错误；version pre-release 数字段按数值比较；根目录杂物 package-lock.json 已删（项目用 pnpm）。admin CORS 收紧（/api/v2/admin 与 /admin 不再带 ACAO:*）。

## 嵌入前端释放

- `embedded_files.go`：`//go:embed all:web/default all:web/admin`。启动时 `assets.SyncEmbedded` 把内嵌前端释放到项目根 `web/default`、`web/admin`，**每次启动都释放**（内容相同的文件跳过写入减 IO，无 manifest 短路，确保前端随二进制即时生效）。`deprecatedBundles` 列表里的历史遗留目录（如 `web/default_v2`）会被整体删除。
