# 柠泽资源站

[![CI](https://img.shields.io/github/actions/workflow/status/NingZeStudio/lemwood-mirror/build.yml?branch=main)](https://github.com/NingZeStudio/lemwood-mirror/actions)
[![Go Version](https://img.shields.io/badge/Go-1.25-blue)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

面向 Minecraft 启动器分发场景的 GitHub Release 镜像服务 — 自动同步、版本管理、下载加速、统计风控、PoW 反滥用，开箱即用。

![站点截图](screenshot.jpg)

## 目录

- [快速开始](#快速开始)
- [功能](#功能)
- [安装与构建](#安装与构建)
- [配置说明](#配置说明)
- [下载流程](#下载流程)
- [统计与风控](#统计与风控)
- [部署](#部署)
- [API](#api)
- [目录结构](#目录结构)
- [参与贡献](#参与贡献)
- [许可](#许可)

## 快速开始

```bash
# 1. 构建前端
cd frontend && pnpm install --frozen-lockfile && pnpm build
cd ../admin-app && pnpm install --frozen-lockfile && pnpm build

# 2. 构建并运行
go build -o mirror ./cmd/mirror
./mirror
```

打开 `http://localhost:8080`，看到启动器版本列表即运行成功。

> **前置依赖：** Go 1.25+、Node.js 18+（前端使用 pnpm）

## 功能

- **定时同步** — 按 Cron 表达式定时扫描上游 GitHub 仓库，自动同步 Release 资产
- **版本保留** — 每个启动器可独立配置保留版本数，自动清理旧版本
- **下载加速** — 支持 xget 代理、HTTP 代理，提升下载效率
- **PoW 下载验证** — 内置 ALTCHA 风格 PBKDF2-SHA256 工作量证明，自动验证客户端正规性，替代第三方人机验证
- **统计面板** — 访问量、下载量、热门资源、地区分布（ip2region 离线库）、每日趋势
- **流量控制** — 单 IP 每日流量上限 + 带宽限速，超限自动封禁
- **黑名单** — 本地 + 外部黑名单同步，公开封禁记录，IP 锁定/申诉流程
- **后台管理** — Web 管理面板，支持配置、黑名单、文件管理、TOTP 两步验证、自更新管理
- **多启动器** — 一个实例托管多个启动器的镜像，各自独立配置
- **数据库可选** — SQLite（默认）/ MySQL / PostgreSQL，支持 MySQL → PostgreSQL 一键迁移

## 安装与构建

### 前端

```bash
cd frontend && pnpm install --frozen-lockfile && pnpm build   # → web/default
cd ../admin-app && pnpm install --frozen-lockfile && pnpm build # → web/admin
```

### 后端

```bash
go build -o mirror ./cmd/mirror
```

Windows 下生成 `mirror.exe`。CI 自动构建 Windows/Linux 的 amd64/arm64/x86 包。

### 直接运行（开发）

```bash
go run ./cmd/mirror
```

启动时二进制会自动释放内嵌的前端（`web/default`）和后台（`web/admin`）到项目目录。每次启动都会重新释放，内容未变化的文件会跳过写入以减少 IO，确保二进制内嵌的前端版本总是即时生效。

> **注意**：`web/default`、`web/admin` 是构建产物且被 git 跟踪（发布流要求），修改前端源码后必须重新 `pnpm build` 并提交产物。

## 配置说明

运行前编辑根目录 `config.yaml`，以下为主要配置项（完整示例见仓库内 config.yaml）：

> **旧版升级**：从使用 `config.json` 的旧版本升级时，服务启动会自动迁移至 `config.yaml` 并删除旧文件，同时补全新增字段默认值。

### 服务与网络

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `server_address` | string | `""` | 绑定地址，留空监听所有网卡。**同时作为下载链接 fallback** |
| `server_port` | int | `8080` | 服务端口 |
| `download_url_base` | string | `""` | 对外下载链接基准地址（含协议头）。为空时回退到 `server_address` |
| `proxy_url` | string | `""` | HTTP 代理，用于扫描阶段下载 |
| `asset_proxy_url` | string | `""` | 资源下载地址前缀代理 |
| `xget_enabled` | bool | — | 启用 xget 代理加速 |
| `xget_domain` | string | — | xget 服务域名 |

### GitHub 与扫描

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `github_token` | string | `""` | GitHub Token（**强烈建议填写**，否则每小时仅 60 次 API 调用）。支持 `GITHUB_TOKEN` 环境变量覆盖 |
| `check_cron` | string | `"*/10 * * * *"` | 扫描 Cron 表达式（分钟粒度） |
| `download_timeout_minutes` | int | — | 单文件下载超时（分钟） |
| `concurrent_downloads` | int | `3` | 并发下载数 |

### PoW 下载验证

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `pow_enabled` | bool | — | 启用 PoW 验证（替代极验验证码） |
| `pow_algorithm` | string | `PBKDF2-SHA256` | 挑战算法 |
| `pow_cost` | int | `500` | PBKDF2 迭代成本 |
| `pow_key_length` | int | `32` | 派生密钥长度 |
| `pow_difficulty` | int | `6` | 前导零位数（难度） |
| `pow_challenge_ttl` | duration | `10m` | 挑战有效期 |
| `pow_hmac_secret` | string | `""` | 挑战 HMAC 密钥；留空启动随机生成（挑战内存态+短 TTL，重启即失效） |

### 下载令牌

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `download_token_ttl` | duration | `10m` | 下载授权令牌有效期 |

### 管理员与后台认证

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `admin_enabled` | bool | `true` | 启用后台管理（设为 false 禁用管理端及 `/api/v2/admin/*` 端点） |
| `admin_user` | string | `"admin"` | 管理员用户名，为空时自动禁用管理后台 |
| `admin_password` | string | `"$2a$04$..."` | bcrypt 哈希密码（默认初始账号 `admin`，密码 `admin123`） |
| `admin_max_retries` | int | `10` | 登录失败上限，超限锁定客户端 IP |
| `admin_lock_duration` | int | `120` | IP 锁定时间（分钟） |
| `two_factor_enabled` | bool | `false` | 启用 TOTP 两步验证（开启后登录须输入 6 位动态验证码） |
| `two_factor_secret` | string | `""` | TOTP 共享密钥（Base32 格式） |

#### 账号密码修改与 TOTP 配置途径

1. **后台可视化直接修改（推荐）**：
   - 登录后台（默认地址 `http://<ip>:8080/admin/`，初始账号 `admin`，密码 `admin123`）。
   - 前往 **「配置编辑」** 页面：
     - 在「管理后台设置」卡片中输入新账号或新密码（留空则不修改原密码），点击底部保存即时生效。
     - 在「二次验证 (TOTP)」卡片中开启开关，点击“生成新密钥”，使用手机身份验证器（Microsoft / Google Authenticator）扫描自动生成的二维码绑定，保存配置即可生效。
2. **通过配置文件修改**：
   - 在 `config.yaml` 中修改 `admin_user` 和 `admin_password`。密码采用 bcrypt 哈希存储，可通过以下命令生成：
     ```bash
     htpasswd -bnBC 14 "" <your-new-password> | tr -d ':\n'
     ```
   - 启用 TOTP 需配置 `two_factor_enabled: true` 并将 Base32 密钥写入 `two_factor_secret`。

### 流量控制与封禁

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `traffic_limit_gb` | int | — | 单 IP 每日下载流量上限（GB），`0` 禁用，负数自动修正为 `5` |
| `bandwidth_limit_mbps` | int | — | 单下载带宽上限（Mbps），`0` 不限制 |
| `ban_record_file` | string | `"banned_ips.json"` | 封禁记录文件（存于 `storage_path` 下） |
| `external_blacklist_url` | string | `""` | 外部黑名单同步地址（按行解析，跳过 `#` 注释） |
| `appeal_contact` | string | — | 封禁页显示的申诉联系方式 |
| `rate_limit_enabled` | bool | `true` | 请求频率限制开关（对全部 HTTP 请求生效，超限返回 429） |
| `rate_limit_per_minute` | int | `300` | 单 IP 每分钟最大请求数 |
| `rate_limit_ban_threshold` | int | `3` | 违规累计达到该值自动封禁（`ban_type=rate_limit`） |
| `firewall_whitelist` | string[] | `[]` | IP/网段白名单（支持 CIDR），豁免频率限制、外部黑名单与流量自动封禁 |

### 数据库

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `database_mode` | string | `auto` | `auto`/`mysql`/`postgres`；auto 时按连接配置自动选择 |
| `mysql_*` | — | — | MySQL 连接配置（host/port/user/password/database），`mysql_migration` 为迁移模式 |
| `postgres_*` | — | — | PostgreSQL 连接配置（host/port/user/password/database/sslmode/migration_batch/migration_delay） |

留空默认使用 SQLite，无需额外配置。MySQL 迁移到 PostgreSQL 使用独立工具：`go run ./cmd/db-migrate`。

### 自更新

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `self_update_enabled` | bool | `false` | 启用自更新 |
| `self_update_repo_url` | string | `""` | 更新源仓库（GitHub） |
| `self_update_channel` | string | `notify` | `notify` 仅通知 / `apply` 自动应用 |
| `self_update_check_cron` | string | `""` | 更新检查 Cron |
| `self_update_auto_restart` | bool | `false` | 应用更新后自动重启 |

### 启动器配置

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `name` | string | — | 启动器唯一标识，用于 API 路径和目录名 |
| `source_url` | string | — | GitHub 仓库地址（`https://github.com/<owner>/<repo>`） |
| `mode` | string | `"release"` | 同步模式：`release` 仅同步 Release；`clone` / `all` 已废弃（Git 镜像功能已移除），仅为兼容旧配置保留 |
| `include_prerelease` | bool | `false` | 包含预发布版本 |
| `max_versions` | int | `0` (=3) | 保留最大版本数，≤0 时自动修正为 3 |

## 下载流程

### PoW 验证开启（默认）

1. `GET /api/v2/pow/config` → 获取 PoW 参数
2. `POST /api/v2/downloads/challenge` → 客户端完成 PBKDF2-SHA256 计算，换取 `download_token`
3. 进入引导页 → `GET /api/v2/downloads/landing?token=...`
4. 触发真实下载 `/download/...`（校验 token 与文件绑定，防跨资产复用）

### 验证关闭（兼容旧流程）

1. `POST /api/v2/downloads/prepare` → 获取 `download_token`、`download_url`、`landing_url`
2. 进入引导页 → `GET /api/v2/downloads/landing?token=...`
3. 触发真实下载 `/download/...`

### 细节说明

- `download_token` 为 64 字符十六进制随机串，有效期默认 5 分钟（可配 `download_token_ttl`）
- `landing` 接口 Peek 模式（可多次调用），实际下载 Validate 模式（一次性消费）
- `landing_url` 支持 `return_url` 参数，实现下载后回跳
- 非浏览器请求在 PoW/验证码开启时返回 JSON 错误而非 HTML 验证页面

## 统计与风控

### 数据统计

- 访问记录：IP、路径、UA、Referer、地区（ip2region 离线地理位置库）
- 下载记录：启动器、版本、文件名、来源 IP（仅 200/206 计入）
- 聚合接口：总访问/下载量、近 30 天数据、Top 10 热门、Top 50 地区、每日趋势
- 异步写入（4 worker + 1000 缓冲队列），不阻塞请求
- 统计接口缓存：`Cache-Control: public, max-age=300`；有新下载/访问时快照即时失效重算，面板实时反映最新数据
- 带宽状态：实时速率（10 秒滚动窗口）+ 近 1 分钟下载连接数（60 秒滑动窗口）

### 流量限制

- 单 IP 每日下载流量上限（GB 级）
- 下载前按 `Range` 头预估做预检，超限直接拒绝
- 下载完成后按实际传输字节数精确记录
- 带宽限速（Mbps 级）逐连接控制
- 超限自动封禁，写入本地黑名单和封禁记录

## 部署

生产环境建议 Nginx 反代 + HTTPS：

```nginx
server {
    listen 443 ssl;
    server_name mirror.example.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Systemd 守护进程部署

创建 `/etc/systemd/system/lemwood-mirror.service`：

```ini
[Unit]
Description=Lemwood Mirror Service
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/lemwood-mirror
ExecStart=/opt/lemwood-mirror/mirror
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

启动与自启：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now lemwood-mirror
```

### 健康检查

- 首页 → 确认启动器版本列表正常展示
- `/api/v2/launchers` → 确认返回版本索引
- `/api/v2/latest` → 确认返回各启动器最新版本号
- `/api/v2/stats` → 确认统计接口正常
- 执行一次实际下载 → 确认链路可用

## API

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/v2/launchers` | GET | 版本索引 |
| `/api/v2/latest` | GET | 全部启动器最新版本号 |
| `/api/v2/latest/{launcher}` | GET | 指定启动器最新版本号 |
| `/api/v2/stats` | GET | 统计数据 |
| `/api/v2/pow/config` | GET | PoW 验证参数 |
| `/api/v2/downloads/challenge` | POST | 获取 PoW 挑战并换取下载令牌 |
| `/api/v2/downloads/prepare` | POST | 准备下载（PoW 关闭时） |
| `/api/v2/downloads/verify` | POST | 兼容旧验证流程 |
| `/api/v2/downloads/landing` | GET | 下载引导页 |
| `/api/v2/admin/scans` | POST | 手动触发全量扫描（需 Admin 登录） |
| `/api/v2/admin/scans/launcher` | POST | 手动触发指定启动器扫描（需 Admin 登录） |
| `/api/v2/admin/blacklist` | GET/POST | 黑名单查询/管理 |
| `/api/v2/admin/config` | GET/PUT | 配置读取/保存 |
| `/api/v2/admin/files` | GET | 文件列表 |
| `/api/v2/admin/files/download` | GET | 后台文件下载 |
| `/api/v2/admin/self-update/*` | GET/POST | 自更新检查/应用/重启/状态 |

> 完整文档见 [`API_DOCS.md`](API_DOCS.md)，管理后台 API 不在公开文档范围内。

## 目录结构

```
lemwood-mirror/
├── cmd/
│   ├── mirror/            # 程序入口
│   └── db-migrate/        # MySQL → PostgreSQL 数据迁移工具（独立 CLI）
├── internal/
│   ├── assets/            # Release 资产同步与清理
│   ├── auth/              # 管理员认证与 TOTP
│   ├── bandwidth/         # 带宽限速
│   ├── blacklist/         # 黑名单同步
│   ├── config/            # 配置加载与保存（YAML）
│   ├── db/                # 数据库抽象（SQLite/MySQL/PostgreSQL）
│   ├── download_authz/    # 下载授权与令牌校验
│   ├── downloader/        # 版本索引生成与资产下载
│   ├── geoip/             # ip2region 离线地理位置
│   ├── github/            # GitHub API 封装
│   ├── netutil/           # 客户端 IP 解析
│   ├── pow/               # PoW 工作量证明（PBKDF2-SHA256）
│   ├── selfupdate/        # 自更新检查与应用
│   ├── server/            # HTTP 路由、SPA 托管
│   ├── stats/             # 访问与下载统计
│   ├── storage/           # 文件存储抽象
│   ├── traffic/           # 流量限制
│   └── version/           # 版本信息
├── frontend/              # 用户站点前端源码（Vue3 + Vite）→ 构建至 web/default
├── admin-app/             # 后台管理前端源码（React + Vite）→ 构建至 web/admin
├── web/
│   ├── default/           # 用户前端产物（运行时由二进制释放）
│   └── admin/             # 后台前端产物（运行时由二进制释放）
├── download/              # Release 镜像文件存储（默认）
├── config.yaml            # 配置文件（YAML，带注释）
├── openapi.yaml           # 公共 API 描述
└── API_DOCS.md            # 公共 API 文档
```

## 参与贡献

欢迎提交 Issue 和 Pull Request。重大问题请先开 Issue 讨论方案。

## 许可

[MIT](LICENSE) © 2025 柠枺