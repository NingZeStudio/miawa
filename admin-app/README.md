# ElementsPlus Admin for Lemwood Mirror

基于 [NingZeStudio/ElementsPlus-Admin-Template](https://github.com/NingZeStudio/ElementsPlus-Admin-Template) 构建的现代管理后台，为 Lemwood Mirror 启动器分发镜像服务提供可视化管理控制台。

## 技术栈与设计规范

- **核心框架**：Vue 3.5 + TypeScript 5.7 + Vite 6
- **UI 组件库**：Element Plus 2.9 (中文语言包)
- **状态管理**：Pinia 2.3
- **路由系统**：Vue Router 4.5
- **视觉风格**：
  - 采用 Zinc 低饱和度冷灰/中性灰配色，微阴影与精细边框（Shadcn-like 现代极简风格）。
  - 支持浅色 / 暗黑模式无缝一键切换。
  - 默认开启全屏安全防截屏水印（可通过右上角菜单切换显隐）。
  - 图标采用 Lucide 矢量图标库，严禁使用高饱和度红橙黄/蓝紫渐变，极简庄重。

## 目录结构

```text
admin-app/
├── src/
│   ├── api/            # 接口层 (auth, config, files, blacklist, firewall)
│   ├── assets/         # 样式表与 Zinc 主题覆盖 (Tailwind CSS, Element Plus vars)
│   ├── core/           # iframe Bridge 桥接通信核心 (支持扩展子应用)
│   ├── layout/         # 布局骨架 (Sidebar, Navbar, Tabs, IframeContainer)
│   ├── lib/            # 工具类 (axios 拦截器封装、安全 storage 降级、格式化工具)
│   ├── router/         # 路由表与全局鉴权守卫
│   ├── store/          # Pinia 状态树 (user, app, tabs)
│   ├── types/          # 全局与业务 TypeScript 类型定义
│   └── views/          # 视图组件
│       ├── login/      # 系统登录页 (支持用户名密码与 TOTP 动态码校验)
│       ├── dashboard/  # 控制台概览页 (运行指标、版本快照与快捷入口)
│       ├── config/     # 全配置可视化编辑 (基础、安全、启动器源、自更新等)
│       ├── files/      # 文件管理 (面包屑目录树、上传、下载、删除)
│       └── blacklist/  # 黑名单管理 (防火墙状态、分类统计、CIDR/IP 封禁与解封)
├── index.html
├── vite.config.ts      # 打包至 ../web/admin，base: /admin/
└── package.json
```

## 开发与构建

### 1. 安装依赖

推荐使用 pnpm：

```bash
pnpm install
```

### 2. 本地开发

启动本地 Vite 开发服务器（已预置反向代理转发 `/api` 到 `http://127.0.0.1:8080`）：

```bash
pnpm dev
```

### 3. 代码检查与格式化

```bash
pnpm typecheck   # 严格 TypeScript 类型检查
pnpm lint        # ESLint 自动检查与修复
```

### 4. 生产构建

构建产物将直接输出到父项目的 `web/admin/` 目录，供 Go 二进制文件通过 `//go:embed` 打包内嵌：

```bash
pnpm build
```

## 部署说明

管理端前端构建产物 `web/admin` 被 Git 跟踪并内嵌进 `cmd/mirror`。修改本目录源码后，必须执行 `pnpm build` 并重新编译 Go 服务，线上方可生效。
