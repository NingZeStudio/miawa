# lemwood-mirror 前端（frontend/）记忆

柠泽资源站前端。Vue 3（`<script setup>` SFC）+ Vite 5 + Vue Router 4（history 模式）+ Tailwind 3 + Radix Vue。后端是仓库根目录的 Go 单体（本前端是其一叶），仓库整体 AGENTS.md 在根目录，含后端/构建/部署全貌，读本文件前先看根 AGENTS.md。

## 构建与命令

- **环境限制（Termux / Android）**：系统 `/tmp` 目录为只读，不可写入文件。如需临时文件操作，必须使用 Termux 下的 `/data/data/com.termux/files/usr/tmp/` 或 `<项目根>/tmp/` 目录。在智能体工具中执行命令时若因动态链接器隔离报错，需在宿主免沙盒环境下执行。
- **包管理器**：前端自身约定用 **npm**（`package.json` 的 `_packageManager: npm`、README 命令都是 npm）：`npm install`、`npm run dev`、`npm run build`、`npm run typecheck`、`npm run test`、`npm run preview`。git 只跟踪 `pnpm-lock.yaml`（`package-lock.json` 被 .gitignore，禁止提交），CI 与 Docker 统一用 pnpm 10.12.4。
- **质量检查顺序**：修改代码后必须严格按顺序运行验证：`npm run typecheck`（`vue-tsc --noEmit`）→ `npm run test`（vitest run，tests/ 目录 5 套测试共 25 个用例全部通过）→ `npm run build`。
- **聚焦单测**：运行指定测试文件使用 `npm run test -- tests/<name>.test.js`（例如 `npm run test -- tests/pow.test.js`）。
- **`npm run build` 输出到仓库根 `../web/default/`**（`vite.config.js` 的 `outDir: '../web/default'` + `emptyOutDir`）。该目录被 git 跟踪且内嵌进 Go 二进制（启动时释放）。改完前端代码后必须重新构建，否则线上不生效；构建会重写 git 跟踪的产物文件。

## 目录与入口

- `src/main.js` — 入口；`src/App.vue` — 根组件；`src/router/index.js` — 路由（history 模式，后端必须有 SPA 回退）。
- `src/lib/globalConfig.ts` — **站点/启动器/API 配置中心**：`api.baseUrl`、`api.endpoints`、`launchers`（启动器显示名与 logo）、`storage.keys`。改端点/文案/新增启动器优先改这里。`src/lib/launcher-info.ts` 据此提供显示名查询。`site.version` 取自构建期注入的 `__APP_VERSION__`（vite.config/vitest.config 从 package.json 读，单一版本源）；`site.url` 取 `VITE_SITE_URL`（index.html 的 og:url/twitter:url 同源注入）。
- `src/lib/` 其他模块：`pow.js`（PoW base64url 编解码 + leadingZeroBits，VerifyView 引用）、`format.js`（formatSize + compareVersionDesc 数值语义版本降序，VersionList/FilesView 共用）、`returnTarget.js`（外部跳转域名白名单）、`composables/useSeoMeta.js`（title/description/og/twitter meta 统一写入，各视图接入）。
- `src/services/api.js` — axios 单例 + **v2 信封解包拦截器**：响应含 `data`/`meta` 且 `error === null` 时才把 `response.data.data` 提升为 `response.data`（业务错误信封原样保留）。因此导出的 API 函数返回的是**内层数据而非 axios 响应**，别重复 `.data.data`。
- `src/views/` 页面 + `src/components/`（含 `layout/`、`ui/`，ui 是 Radix Vue + Tailwind 的 shadcn 风格组件）。
- `public/geo/china.json` — 中国省级行政区 GeoJSON 地图数据（供统计页全国分布图渲染）。
- `src/lib/chinaRegion.js` — 访问/下载分布地域归一化工具：将后端统计混杂的省级简称（如“广东”）与地级市（如“广州市”）映射归并至 34 个一级行政区 GeoJSON 全称（台湾省视同省份，海外/未命中返回 null 不上图）。
- `src/style.css` + `tailwind.config.js` — 全局样式/主题。

## 代码约定

- 语言混杂：`main.js`/`api.js`/`router/index.js`/大部分 `.vue` 用 JS，`globalConfig.ts`/`launcher-info.ts` 是 TS。改 TS 注意 `tsconfig.json` 的 `@/*` 别名与 `strict`。
- API 端点统一在 `globalConfig.api.endpoints` 声明（`/pow/config`、`/downloads/challenge`、`/downloads/authorize`、`/downloads/prepare`、`/downloads/landing`），不要散落硬编码路径。
- 环境变量：`.env.production` 与 `.env.development` 均为 `VITE_API_BASE_URL=/api/v2` + `VITE_SITE_URL`（dev 是 localhost:5173）；`globalConfig.api.baseUrl` 用 `import.meta.env.VITE_API_BASE_URL || '/api/v2'`。`vite.config.js` 配置了 dev/preview 代理：`/api/v1`、`/api/v2`、`/download` 三条精确前缀 → `VITE_PROXY_TARGET`（默认 `https://miawa.cn`，changeOrigin），dev 下 API 请求走线上站点联调，无跨域问题；如需指向本地后端设置 `VITE_PROXY_TARGET` 即可，不要改代理 key 为宽泛的 `/api`（会劫持 `/apidocs` SPA 路由）。

## 下载/PoW 链路（与后端 internal/pow + download_events 配套）

- 浏览器直连下载：`VersionList.vue`/`FilesView.vue` 的 `handleDownload` 在 `powConfig.enabled` 时路由到 `/verify?file=...`（`VerifyView.vue`），否则走 `prepareDownload`（CLI/API 路径，无 PoW）。
- `VerifyView.vue`：Web Crypto PBKDF2-SHA256 求解（Web Crypto API `importKey`/`deriveBits`），`derivedKey` 用**无填充** base64url 编码（与后端 `base64.RawURLEncoding` 约定一致），成功后跳 `/download-started?token=...`。
- `DownloadStartedView.vue`：最终落点。landing 返回单个同源 `download_url`（`/download/...?token=...`）；触发自动下载前先对路径发 **HEAD 请求**探测可达性（5s 超时 AbortController），失败则不自动跳转、仅保留手动按钮。**HEAD 探测不带 token**（后端对 HEAD 分支不校验、不记账、不写事件）；不要改成带 token 的 GET/Range 探测。「返回上一页/前往网站」的外部跳转经 `lib/returnTarget.js` 白名单校验（site.url + 友链域名），防开放重定向（2026-09-05 加）。
- `globalConfig.download.sourceLabels` 用于 `prepareDownload` 的 `source` 上报（home/files/verify）。

## 样式与设计规范

- **配色与质感**：严格遵循低饱和度纯色优先准则。主色统一采用 Zinc 系列等低饱和度冷灰，禁止使用 `#8b5cf6` 等高饱和紫蓝渐变或蓝粉/红橘高饱和过渡；界面尽可能减少 Emoji 的使用，保持工程化稳重风格。
- **主题色与深浅色**：多主题色配置已被完全移除，全站统一基于 `.dark` class 实现深浅色切换。显示模式三态持久化在 `displayMode` key（`light` / `dark` / `system`）；实际生效的布尔深色值同步维护在 `darkMode` key（`vueuse-color-scheme`，供 ECharts 等组件感知）。
- **轮询节流与防火墙协同**：`StatsView.vue` 的实时带宽（`/api/v2/bandwidth`）采用 5 秒轮询间隔（5000ms），防止多开页面触发站点级每分钟 300 次的 IP 频率限制；并通过监听 `visibilitychange` 事件在页面不可见（标签页切出/最小化）时主动暂停定时器，重回前台时立即触发一次刷新并恢复轮询。

## 设计体系（2026-09-02 起与 LogShare.CN 同步）

- **来源**：工作室另一项目 `~/Project/LogShare-Web-UI`（NingZeStudio）。设计体系整体迁移自该项目，改样式前应对照其实现保持两边同步（同工作室视觉一致性约定）。
- **字体**：自托管 HarmonyOS Sans SC（界面，2 个 woff2 子集）+ SauceCode Mono（等宽，4 个字重），位于 `src/assets/fonts/`，`@font-face` 定义在 `src/style.css`，经 `--font-sans`/`--font-mono` 变量接入 Tailwind `fontFamily`。
- **圆角刻度**：LogShare 的 7 档刻度（`--radius-sm` 0.25rem 到 `--radius-3xl` 1.5rem），映射在 `tailwind.config.js` 的 `borderRadius`（`rounded-sm`=sm … `rounded-2xl`=3xl，`rounded-lg`=xl 即 0.75rem）。与旧 shadcn 刻度不同，`rounded-lg` 现在更大。
- **回弹缓动**：`cubic-bezier(0.34, 1.7, 0.64, 1)`，Tailwind `ease-bounce-soft` + `style.css` 里 @layer utilities 对 `transition-*` 类的整体覆盖，全站过渡自动回弹。
- **图标**：Phosphor（`@phosphor-icons/vue`），**全站已无 lucide**（依赖已移除），模板里统一 `weight="duotone"`；`style.css` 有 `svg[viewBox='0 0 256 256'] { scale: 1.2 }` 视窗补偿。常用映射：Download→PhDownloadSimple、Home→PhHouse、Loader2(转圈)→PhCircleNotch、History→PhClockCounterClockwise、TrendingUp→PhTrendUp、Activity→PhPulse、Server→PhNetwork、Layers→PhStack、ExternalLink→PhArrowSquareOut、Link2→PhLinkSimpleHorizontal、ArrowUpToLine→PhArrowLineUp；新图标先在 `node_modules/@phosphor-icons/vue/dist/icons/` 确认导出名。
- **阴影**：Tailwind `shadow-*` 已覆盖为 LogShare 规格（固定 rgba，不用 hsl 变量）。
- **布局**：单列（无 Sidebar）。顶栏在 `layouts/DefaultLayout.vue`：滚动 >8px 变毛玻璃胶囊（h-14→h-12、rounded-full、backdrop-blur），右侧是**显示模式三态胶囊**（浅色/深色/跟随系统，高亮胶囊 translateX 平移，每格 30px）。移动菜单在 `components/layout/MobileNav.vue`：Teleport 到 body + 动画汉堡（三条线合并为 X）+ 主题色圆点区块，菜单位置随顶栏吸附状态微调（scrolled ? top-[72px] : top-[64px]）。页脚 `components/layout/Footer.vue` 为三栏（品牌简介+联系方式+版权备案 | 友情链接，友链来自 `friendLinksConfig`）。
- **组件**：`ui/Button.vue` 尺寸对齐 LogShare AppButton（sm=h-7/default=h-9/lg=h-11/icon=h-8），新增 `soft`/`soft-destructive`/`muted` 变体；`ui/AppDialog.vue` 是 Teleport 弹窗（宽度档 sm/md/lg/xl/2xl，遮罩点击关闭，内置细滚动条样式）；`lib/toast.js` + `ui/ToastHost.vue` 是无依赖 toast（`toast.success/error/dismiss`，自动 1.5s 消失）。公告弹窗 `AnnouncementDialog.vue` 已改用 AppDialog。
- **页面过渡**：`App.vue` 的 `RouterView` 带 `<Transition name="page">`（淡入+上滑 0.18s），路由组件必须有单根元素，`key` 是 `route.path`。
- **深浅色**：`.dark` class 切换，**主题色选择已砍掉**（2026-09-02 全量迁移对齐 LogShare，`data-theme-color`/`globalConfig.theme`/`theme-color` key 均已删除，老用户 localStorage 里的残留值被忽略）。显示模式三态存 `displayMode` key（light/dark/system）；`darkMode` key（'vueuse-color-scheme'）仍同步写实际生效值，供 StatsView 的 `useDark()` 读图表配色，两 key 并存勿删其一。`onMounted` 里注册系统深浅色监听。
- **已删除**：`components/layout/Sidebar.vue`、`components/ui/sheet/`、`lucide-vue-next` 依赖、顶栏 logo 图片（2026-09-02 起顶栏仅站名文字）。若需要恢复从 git 历史找。

## FilesView 双布局（2026-09-02 重设计）

- 桌面（`sm+`）列表行 + 移动（`<sm`）卡片式两套渲染，靠 `hidden sm:block` / `sm:hidden` 切换，数据源同为 `currentItems`。移动端文件名用 `break-words` 完整换行（不 truncate）+ 通栏大下载按钮，解决长文件名下载痛点。
- 文件类型彩色底片由 `fileMeta(name)` 返回 `{icon, chip}`（apk 绿 / 压缩包琥珀 / exe·msi·dmg 蓝 / jar 橙 / sig 紫罗兰 / rpm 红 / deb 紫 / hap 青 / 其他灰）；启动器行用 `launcherLogo(id)` 真实 logo。改类型配色只动 `fileMeta`。

## API 文档页（2026-09-02 起）

- `views/ApiDocsView.vue` 是完整文档页（替代旧"编写中"占位）：排版对齐 LogShare 的 ApiDocsView——Tab 导航（概述/API 端点/限制说明）+ method 徽标 + 参数表 + 深色代码块（`bg-slate-950`），**无 SDK 章节**（用户约定）。
- 端点数据以 `v2.go` 实际路由为准硬编码在组件里（12 个公开端点，不含 admin）；基础 URL 取 `globalConfig.site.url`。**`openapi.yaml` 已过时**（仍含 captcha/verify、缺 bandwidth/files/pow），改 API 时除 openapi 外要同步改此页。
- 赞助列表 `lib/sponsorConfig.js` 与 LogShare `data/sponsors.ts` **人工保持同步**（同一工作室共享赞助者名单），LogShare 新增记录时要搬过来。

## 文案规范（2026-09-02 全站润色）

- 口径：社区向、自然口吻、去翻译腔；空态给"原因 + 建议动作"两层信息；副标题 ≤ 一句话。
- 关键文案锚点：首页副标"实时同步上游发布…"、验证页"安全验证/正在确认你是真实访客…"、下载完成页"一切就绪。部分浏览器（尤其 Android）不会自动弹出下载…"、关于页"公益镜像服务"定位、"所有捐助将全额用于服务器运营，账目公开透明"。
- 改文案时优先改 globalConfig（site.description 等单点），页面内硬编码文案随页面维护； announcementConfig 的公告是一次性内容，改完要换 `id` 才会重新弹出。
