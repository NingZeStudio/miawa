<template>
  <div class="p-6 space-y-6 max-w-5xl mx-auto pb-24">
    <!-- 头部标题 -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-zinc-200 dark:border-zinc-800">
      <div>
        <h2 class="text-xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 flex items-center gap-2">
          <component
            :is="Settings2Icon"
            class="w-5 h-5 text-zinc-600 dark:text-zinc-400"
          />
          配置编辑
        </h2>
        <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
          管理 Lemwood Mirror 服务的核心网络、安全、启动器源及自更新参数
        </p>
      </div>

      <div class="flex items-center gap-2">
        <el-button
          :loading="loading"
          size="default"
          @click="loadConfigData"
        >
          <template #icon>
            <component
              :is="RefreshCwIcon"
              class="w-4 h-4"
            />
          </template>
          重新加载
        </el-button>
        <el-button
          type="primary"
          size="default"
          :loading="saving"
          @click="handleSave"
        >
          <template #icon>
            <component
              :is="SaveIcon"
              class="w-4 h-4"
            />
          </template>
          保存全部配置
        </el-button>
      </div>
    </div>

    <!-- 加载中骨架屏 -->
    <div
      v-if="loading"
      class="py-16 text-center text-zinc-400"
    >
      <el-skeleton
        :rows="10"
        animated
      />
    </div>

    <el-form
      v-else
      ref="formRef"
      :model="formData"
      label-position="top"
      class="space-y-6"
    >
      <!-- 基础设置 -->
      <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4 pb-2 border-b border-zinc-100 dark:border-zinc-800 flex items-center gap-2">
          <component
            :is="SlidersIcon"
            class="w-4 h-4 text-zinc-500"
          />
          基础与网络服务
        </h3>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <el-form-item label="管理员重试限制 (次)">
            <el-input-number
              v-model="formData.admin_max_retries"
              :min="1"
              :max="100"
              class="!w-full"
            />
          </el-form-item>

          <el-form-item label="锁定时长 (分钟)">
            <el-input-number
              v-model="formData.admin_lock_duration"
              :min="1"
              :max="1440"
              class="!w-full"
            />
          </el-form-item>

          <el-form-item
            label="服务监听端口"
            required
          >
            <el-input-number
              v-model="formData.server_port"
              :min="1"
              :max="65535"
              class="!w-full"
            />
          </el-form-item>

          <el-form-item
            label="检查频率 (Cron 表达式)"
            required
            extra="5 段 cron 表达式（分钟粒度），例如: */10 * * * *"
          >
            <el-input
              v-model="formData.check_cron"
              placeholder="*/10 * * * *"
            />
          </el-form-item>

          <el-form-item
            label="文件存储根路径"
            required
          >
            <el-input
              v-model="formData.storage_path"
              placeholder="data"
            />
          </el-form-item>

          <el-form-item
            label="下载链接前缀"
            required
            extra="例如: https://mirror.example.com"
          >
            <el-input
              v-model="formData.download_url_base"
              placeholder="https://mirror.example.com"
            />
          </el-form-item>
        </div>
      </div>

      <!-- GitHub 配置 -->
      <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4 pb-2 border-b border-zinc-100 dark:border-zinc-800 flex items-center gap-2">
          <component
            :is="GithubIcon"
            class="w-4 h-4 text-zinc-500"
          />
          GitHub 认证与网络代理
        </h3>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <el-form-item
            label="GitHub 个人访问令牌 (Token)"
            class="md:col-span-2"
            extra="留空表示保持原值不变；用于 GitHub API 认证以提升速率限额"
          >
            <el-input
              v-model="formData.github_token"
              type="password"
              placeholder="ghp_xxxxxxxxxxxx (留空保持原值)"
              show-password
            />
          </el-form-item>

          <el-form-item
            label="API 代理 URL"
            extra="例如: http://127.0.0.1:7890"
          >
            <el-input
              v-model="formData.proxy_url"
              placeholder="http://127.0.0.1:7890"
            />
          </el-form-item>

          <el-form-item
            label="静态资产下载代理前缀"
            extra="用于加速 Release 文件直链，例如: https://ghproxy.com/"
          >
            <el-input
              v-model="formData.asset_proxy_url"
              placeholder="https://ghproxy.com/"
            />
          </el-form-item>
        </div>
      </div>

      <!-- 管理员设置 -->
      <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4 pb-2 border-b border-zinc-100 dark:border-zinc-800 flex items-center gap-2">
          <component
            :is="UserCheckIcon"
            class="w-4 h-4 text-zinc-500"
          />
          管理后台认证
        </h3>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <el-form-item
            label="启用管理后台"
            class="md:col-span-2"
          >
            <el-switch v-model="formData.admin_enabled" />
          </el-form-item>

          <el-form-item
            label="管理员用户名"
            required
          >
            <el-input
              v-model="formData.admin_user"
              placeholder="admin"
            />
          </el-form-item>

          <el-form-item
            label="新管理员密码"
            extra="留空表示保持当前原密码不变"
          >
            <el-input
              v-model="formData.admin_password"
              type="password"
              placeholder="输入新密码 (留空不修改)"
              show-password
            />
          </el-form-item>
        </div>
      </div>

      <!-- 二次验证 (TOTP) -->
      <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4 pb-2 border-b border-zinc-100 dark:border-zinc-800 flex items-center gap-2">
          <component
            :is="KeyRoundIcon"
            class="w-4 h-4 text-zinc-500"
          />
          二次验证 (TOTP 两步验证)
        </h3>

        <div class="space-y-4">
          <el-form-item
            label="启用两步验证"
            extra="使用微软身份验证器 (Microsoft Authenticator) 或谷歌身份验证器 (Google Authenticator)"
          >
            <el-switch
              v-model="formData.two_factor_enabled"
              @change="handleTOTPEnabledChange"
            />
          </el-form-item>

          <div
            v-if="formData.two_factor_enabled"
            class="p-4 rounded-lg bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 space-y-4"
          >
            <el-form-item label="验证器密钥 (Secret)">
              <div class="flex flex-col sm:flex-row gap-2 max-w-lg">
                <el-input
                  v-model="formData.two_factor_secret"
                  readonly
                  class="font-mono"
                />
                <el-button
                  type="default"
                  @click="handleGenerateTOTP"
                >
                  生成新密钥
                </el-button>
              </div>
            </el-form-item>

            <div
              v-if="formData.two_factor_secret"
              class="space-y-2"
            >
              <span class="text-xs font-medium text-zinc-700 dark:text-zinc-300">绑定二维码</span>
              <div class="p-2 inline-block bg-white rounded border border-zinc-200">
                <img
                  :src="getTOTPQRCodeUrl(formData.two_factor_secret)"
                  alt="TOTP 二维码"
                  class="w-36 h-36"
                >
              </div>
              <p class="text-xs text-zinc-500">
                请在手机验证器应用中扫描上方二维码，或手动输入密钥完成绑定。
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- 下载验证 (PoW) -->
      <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4 pb-2 border-b border-zinc-100 dark:border-zinc-800 flex items-center gap-2">
          <component
            :is="CpuIcon"
            class="w-4 h-4 text-zinc-500"
          />
          下载验证 (PoW 工作量证明)
        </h3>

        <div class="space-y-4">
          <el-form-item
            label="启用 PoW 下载验证"
            extra="在浏览器端通过轻量级 PBKDF2 工作量证明防止滥用爬取；关闭后无需门控验证"
          >
            <el-switch v-model="formData.pow_enabled" />
          </el-form-item>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <el-form-item
              label="PoW 难度 (前导零位数)"
              extra="难度越高计算耗时越长，推荐 6-14"
            >
              <el-input-number
                v-model="formData.pow_difficulty"
                :min="1"
                :max="30"
                class="!w-full"
              />
            </el-form-item>

            <el-form-item
              label="PBKDF2 迭代轮数"
              extra="单次哈希轮数，默认 500"
            >
              <el-input-number
                v-model="formData.pow_cost"
                :min="100"
                :max="100000"
                class="!w-full"
              />
            </el-form-item>

            <el-form-item
              label="挑战有效时间"
              extra="Go Duration 格式，例如 2m、10m"
            >
              <el-input
                v-model="formData.pow_challenge_ttl"
                placeholder="2m"
              />
            </el-form-item>

            <el-form-item
              label="下载令牌有效时间 (Token TTL)"
              extra="Go Duration 格式，也是分段并行下载的复用窗口，例如 5m、10m"
            >
              <el-input
                v-model="formData.download_token_ttl"
                placeholder="5m"
              />
            </el-form-item>
          </div>
        </div>
      </div>

      <!-- 防刷墙与申诉 -->
      <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4 pb-2 border-b border-zinc-100 dark:border-zinc-800 flex items-center gap-2">
          <component
            :is="ShieldIcon"
            class="w-4 h-4 text-zinc-500"
          />
          流量限额与误封申诉
        </h3>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <el-form-item
            label="单 IP 每日流量上限 (GB)"
            extra="0 表示不限制；超限自动加入当日封禁"
          >
            <el-input-number
              v-model="formData.traffic_limit_gb"
              :min="0"
              class="!w-full"
            />
          </el-form-item>

          <el-form-item
            label="外部黑名单订阅 URL"
            extra="扫描定时任务自动从该链接同步 IP 列表；留空表示停用"
          >
            <el-input
              v-model="formData.external_blacklist_url"
              placeholder="https://example.com/blacklist.txt"
            />
          </el-form-item>

          <el-form-item
            label="误封申诉联系渠道"
            class="md:col-span-2"
            extra="在客户端受到拦截或封禁时展示给用户的联系方式"
          >
            <el-input
              v-model="formData.appeal_contact"
              placeholder="例如：QQ 群 123456 或 admin@example.com"
            />
          </el-form-item>
        </div>
      </div>

      <!-- 防火墙设置 -->
      <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4 pb-2 border-b border-zinc-100 dark:border-zinc-800 flex items-center gap-2">
          <component
            :is="FlameIcon"
            class="w-4 h-4 text-zinc-500"
          />
          防火墙与频率拦截
        </h3>

        <div class="space-y-4">
          <el-form-item
            label="启用请求频率拦截"
            extra="对全站 HTTP 请求进行滑动窗口速率统计，超限返回 429 并计违规"
          >
            <el-switch v-model="formData.rate_limit_enabled" />
          </el-form-item>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <el-form-item label="单 IP 每分钟请求上限">
              <el-input-number
                v-model="formData.rate_limit_per_minute"
                :min="1"
                class="!w-full"
              />
            </el-form-item>

            <el-form-item label="自动封禁违规次数阈值">
              <el-input-number
                v-model="formData.rate_limit_ban_threshold"
                :min="1"
                class="!w-full"
              />
            </el-form-item>

            <el-form-item
              label="IP / CIDR 网段白名单"
              class="md:col-span-2"
              extra="支持输入单一 IP 或 CIDR 网段（如 192.168.1.1、10.0.0.0/8），回车即可录入。白名单豁免频率限制与自动封禁。"
            >
              <el-select
                v-model="formData.firewall_whitelist"
                multiple
                filterable
                allow-create
                default-first-option
                placeholder="输入 IP 或网段按回车添加"
                class="!w-full"
              />
            </el-form-item>
          </div>
        </div>
      </div>

      <!-- 高级设置 -->
      <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4 pb-2 border-b border-zinc-100 dark:border-zinc-800 flex items-center gap-2">
          <component
            :is="WrenchIcon"
            class="w-4 h-4 text-zinc-500"
          />
          高级下载与加速
        </h3>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <el-form-item label="最大并发下载数">
            <el-input-number
              v-model="formData.concurrent_downloads"
              :min="1"
              :max="32"
              class="!w-full"
            />
          </el-form-item>

          <el-form-item label="下载超时时间 (分钟)">
            <el-input-number
              v-model="formData.download_timeout_minutes"
              :min="1"
              :max="1440"
              class="!w-full"
            />
          </el-form-item>

          <el-form-item label="启用 Xget 镜像加速">
            <el-switch v-model="formData.xget_enabled" />
          </el-form-item>

          <el-form-item label="Xget 域名">
            <el-input
              v-model="formData.xget_domain"
              placeholder="mirror.example.com"
            />
          </el-form-item>
        </div>
      </div>

      <!-- 程序自更新 -->
      <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4 pb-2 border-b border-zinc-100 dark:border-zinc-800 flex items-center gap-2">
          <component
            :is="RefreshCwIcon"
            class="w-4 h-4 text-zinc-500"
          />
          系统程序自更新
        </h3>

        <div class="space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <el-form-item
              label="启用自更新检查"
              class="md:col-span-2"
            >
              <el-switch v-model="formData.self_update_enabled" />
            </el-form-item>

            <el-form-item label="更新频道">
              <el-select
                v-model="formData.self_update_channel"
                class="!w-full"
              >
                <el-option
                  label="仅提醒 (notify)"
                  value="notify"
                />
                <el-option
                  label="稳定版 (release)"
                  value="release"
                />
                <el-option
                  label="预览版 (preview)"
                  value="preview"
                />
              </el-select>
            </el-form-item>

            <el-form-item
              label="自动检查频率 (Cron)"
              extra="留空表示仅支持管理员手动检查"
            >
              <el-input
                v-model="formData.self_update_check_cron"
                placeholder="0 */6 * * *"
              />
            </el-form-item>

            <el-form-item label="更新完成后自动重启">
              <el-switch v-model="formData.self_update_auto_restart" />
            </el-form-item>
          </div>

          <!-- 自更新实时状态面板 -->
          <div
            v-if="updateStatus"
            class="p-4 rounded-lg bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 space-y-3"
          >
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
              <div>
                <span class="text-zinc-400 block">当前运行版本</span>
                <span class="font-mono font-semibold text-zinc-800 dark:text-zinc-200">{{ updateStatus.current_version || '-' }}</span>
              </div>
              <div>
                <span class="text-zinc-400 block">远端最新版本</span>
                <span class="font-mono font-semibold text-zinc-800 dark:text-zinc-200">{{ updateStatus.latest_version || '-' }}</span>
              </div>
              <div>
                <span class="text-zinc-400 block">最近检查时间</span>
                <span class="text-zinc-600 dark:text-zinc-300">{{ updateStatus.last_checked_at ? new Date(updateStatus.last_checked_at).toLocaleString() : '-' }}</span>
              </div>
              <div>
                <span class="text-zinc-400 block">更新就绪状态</span>
                <el-tag
                  v-if="updateStatus.pending_restart"
                  type="warning"
                  size="small"
                >
                  待重启生效
                </el-tag>
                <el-tag
                  v-else-if="updateStatus.has_update"
                  type="success"
                  size="small"
                >
                  发现新版本
                </el-tag>
                <el-tag
                  v-else
                  type="info"
                  size="small"
                >
                  已是最新
                </el-tag>
              </div>
            </div>

            <div
              v-if="updateStatus.last_check_error"
              class="text-xs text-red-500"
            >
              检查更新错误: {{ updateStatus.last_check_error }}
            </div>
            <div
              v-if="updateStatus.last_apply_error"
              class="text-xs text-red-500"
            >
              应用更新错误: {{ updateStatus.last_apply_error }}
            </div>
            <div
              v-if="updateStatus.last_apply_message"
              class="text-xs text-zinc-500"
            >
              更新提示: {{ updateStatus.last_apply_message }}
            </div>

            <div class="flex flex-wrap gap-2 pt-1">
              <el-button
                size="small"
                :loading="checkingUpdate"
                @click="handleCheckUpdate"
              >
                检查更新
              </el-button>
              <el-button
                type="primary"
                size="small"
                :loading="applyingUpdate"
                :disabled="!updateStatus.can_apply"
                @click="handleApplyUpdate"
              >
                下载并应用更新
              </el-button>
              <el-button
                type="danger"
                size="small"
                :loading="restarting"
                @click="handleRestart"
              >
                重启服务
              </el-button>
            </div>
          </div>
        </div>
      </div>

      <!-- 启动器配置 -->
      <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <div class="flex items-center justify-between pb-3 mb-4 border-b border-zinc-100 dark:border-zinc-800">
          <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 flex items-center gap-2">
            <component
              :is="RocketIcon"
              class="w-4 h-4 text-zinc-500"
            />
            启动器源配置 (Launchers)
          </h3>
          <el-button
            type="primary"
            size="small"
            @click="handleAddLauncher"
          >
            <template #icon>
              <component
                :is="PlusIcon"
                class="w-3.5 h-3.5"
              />
            </template>
            添加启动器
          </el-button>
        </div>

        <div
          v-if="formData.launchers.length === 0"
          class="text-center py-8 text-xs text-zinc-400"
        >
          暂无启动器配置，点击上方按钮新增
        </div>

        <div
          v-else
          class="space-y-4"
        >
          <div
            v-for="(launcher, index) in formData.launchers"
            :key="index"
            class="p-4 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-950/40 space-y-3"
          >
            <div class="flex items-center justify-between pb-2 border-b border-zinc-200 dark:border-zinc-800">
              <span class="text-xs font-semibold text-zinc-800 dark:text-zinc-200">
                #{{ index + 1 }} {{ launcher.name || '未命名启动器' }}
              </span>
              <div class="flex items-center gap-2">
                <el-button
                  size="small"
                  :loading="scanningLauncher === launcher.name"
                  :disabled="!launcher.name || scanningLauncher !== null"
                  @click="handleScanLauncher(launcher.name)"
                >
                  <template #icon>
                    <component
                      :is="RefreshCwIcon"
                      class="w-3 h-3"
                    />
                  </template>
                  扫描更新
                </el-button>
                <el-button
                  type="danger"
                  size="small"
                  text
                  @click="handleRemoveLauncher(index)"
                >
                  <template #icon>
                    <component
                      :is="Trash2Icon"
                      class="w-3.5 h-3.5"
                    />
                  </template>
                  删除
                </el-button>
              </div>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              <el-form-item
                label="标识名称"
                required
              >
                <el-input
                  v-model="launcher.name"
                  placeholder="例如: fcl, zl"
                />
              </el-form-item>

              <el-form-item
                label="GitHub 仓库或来源地址"
                required
              >
                <el-input
                  v-model="launcher.source_url"
                  placeholder="例如: https://github.com/owner/repo"
                />
              </el-form-item>

              <el-form-item
                label="最大保留版本数"
                extra="0 表示使用默认 3"
              >
                <el-input-number
                  v-model="launcher.max_versions"
                  :min="0"
                  :max="50"
                  class="!w-full"
                />
              </el-form-item>

              <el-form-item label="包含预发布版本 (Prerelease)">
                <el-switch v-model="launcher.include_prerelease" />
              </el-form-item>
            </div>
          </div>
        </div>
      </div>
    </el-form>

    <!-- 底部常驻悬浮保存条 -->
    <div class="fixed bottom-0 left-0 right-0 z-30 p-3 bg-white/90 dark:bg-zinc-950/90 backdrop-blur border-t border-zinc-200 dark:border-zinc-800 flex items-center justify-end px-6 gap-3">
      <span class="text-xs text-zinc-500">修改配置后需点击保存生效</span>
      <el-button
        type="primary"
        size="large"
        :loading="saving"
        @click="handleSave"
      >
        <template #icon>
          <component
            :is="SaveIcon"
            class="w-4 h-4"
          />
        </template>
        保存配置
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getConfig,
  updateConfig,
  triggerLauncherScan,
  getSelfUpdateStatus,
  checkSelfUpdate,
  applySelfUpdate,
  restartSelfUpdate
} from '@/api/config'
import type { Config, SelfUpdateStatus } from '@/types'
import { generateTOTPSecret, getTOTPQRCodeUrl } from '@/lib/utils'
import {
  Settings2,
  RefreshCw,
  Save,
  Sliders,
  Github,
  UserCheck,
  KeyRound,
  Cpu,
  Shield,
  Flame,
  Wrench,
  Rocket,
  Plus,
  Trash2
} from 'lucide-vue-next'

const Settings2Icon = Settings2
const RefreshCwIcon = RefreshCw
const SaveIcon = Save
const SlidersIcon = Sliders
const GithubIcon = Github
const UserCheckIcon = UserCheck
const KeyRoundIcon = KeyRound
const CpuIcon = Cpu
const ShieldIcon = Shield
const FlameIcon = Flame
const WrenchIcon = Wrench
const RocketIcon = Rocket
const PlusIcon = Plus
const Trash2Icon = Trash2

const loading = ref(false)
const saving = ref(false)
const checkingUpdate = ref(false)
const applyingUpdate = ref(false)
const restarting = ref(false)
const scanningLauncher = ref<string | null>(null)

const updateStatus = ref<SelfUpdateStatus | null>(null)

interface ConfigFormState extends Config {
  admin_password?: string
}

const formData = ref<ConfigFormState>({
  server_port: 8080,
  check_cron: '*/10 * * * *',
  storage_path: 'data',
  download_url_base: '',
  admin_user: 'admin',
  admin_enabled: true,
  admin_max_retries: 10,
  admin_lock_duration: 120,
  proxy_url: '',
  asset_proxy_url: '',
  github_token: '',
  admin_password: '',
  concurrent_downloads: 3,
  download_timeout_minutes: 30,
  xget_enabled: false,
  xget_domain: '',
  two_factor_enabled: false,
  two_factor_secret: '',
  pow_enabled: true,
  pow_difficulty: 6,
  pow_cost: 500,
  pow_challenge_ttl: '2m',
  download_token_ttl: '5m',
  traffic_limit_gb: 0,
  external_blacklist_url: '',
  appeal_contact: '',
  rate_limit_enabled: true,
  rate_limit_per_minute: 300,
  rate_limit_ban_threshold: 3,
  firewall_whitelist: [],
  self_update_enabled: true,
  self_update_channel: 'release',
  self_update_check_cron: '0 */6 * * *',
  self_update_auto_restart: true,
  launchers: []
})

const loadConfigData = async () => {
  loading.value = true
  try {
    const [cfg, selfStatus] = await Promise.allSettled([
      getConfig(),
      getSelfUpdateStatus()
    ])

    if (cfg.status === 'fulfilled') {
      formData.value = {
        ...formData.value,
        ...cfg.value,
        admin_password: '',
        github_token: '',
        firewall_whitelist: cfg.value.firewall_whitelist || [],
        launchers: cfg.value.launchers || []
      }
    }

    if (selfStatus.status === 'fulfilled') {
      updateStatus.value = selfStatus.value
    }
  } catch {
    ElMessage.error('加载系统配置失败')
  } finally {
    loading.value = false
  }
}

const handleSave = async () => {
  saving.value = true
  try {
    const payload: Record<string, any> = { ...formData.value }

    // 留空不更新的保密字段
    if (!payload.admin_password) {
      delete payload.admin_password
    }
    if (!payload.github_token) {
      delete payload.github_token
    }

    await updateConfig(payload)
    ElMessage.success('配置已保存成功，部分网络设置可能需要重启生效')
    await loadConfigData()
  } catch (error: any) {
    const msg = error.response?.data?.error?.message || error.message || '保存配置失败'
    ElMessage.error(msg)
  } finally {
    saving.value = false
  }
}

const handleGenerateTOTP = () => {
  formData.value.two_factor_secret = generateTOTPSecret()
}

const handleTOTPEnabledChange = (val: boolean | string | number) => {
  if (val && !formData.value.two_factor_secret) {
    handleGenerateTOTP()
  }
}

const handleAddLauncher = () => {
  formData.value.launchers.push({
    name: '',
    source_url: '',
    mode: 'release',
    max_versions: 3,
    include_prerelease: false
  })
}

const handleRemoveLauncher = (idx: number) => {
  formData.value.launchers.splice(idx, 1)
}

const handleScanLauncher = async (name: string) => {
  if (!name) return
  scanningLauncher.value = name
  try {
    await triggerLauncherScan(name)
    ElMessage.success(`已触发 ${name} 启动器更新扫描`)
  } catch (error: any) {
    ElMessage.error(error.message || `触发 ${name} 扫描失败`)
  } finally {
    scanningLauncher.value = null
  }
}

const handleCheckUpdate = async () => {
  checkingUpdate.value = true
  try {
    const res = await checkSelfUpdate()
    updateStatus.value = res
    if (res.has_update) {
      ElMessage.success(`发现新版本 ${res.latest_version}`)
    } else {
      ElMessage.info('当前已是最新版本')
    }
  } catch (error: any) {
    ElMessage.error(error.message || '检查更新失败')
  } finally {
    checkingUpdate.value = false
  }
}

const handleApplyUpdate = async () => {
  applyingUpdate.value = true
  try {
    const res = await applySelfUpdate()
    updateStatus.value = res
    if (res.pending_restart) {
      ElMessage.success('更新已成功下载，待重启生效')
    }
  } catch (error: any) {
    ElMessage.error(error.message || '应用更新失败')
  } finally {
    applyingUpdate.value = false
  }
}

const handleRestart = async () => {
  try {
    await ElMessageBox.confirm('重启服务将短暂中断下载连接，确定重启吗？', '系统警告', {
      type: 'warning',
      confirmButtonText: '确定重启',
      cancelButtonText: '取消'
    })
    restarting.value = true
    await restartSelfUpdate()
    ElMessage.success('已发出重启指令')
  } catch {
    // 取消重启
  } finally {
    restarting.value = false
  }
}

onMounted(() => {
  loadConfigData()
})
</script>
