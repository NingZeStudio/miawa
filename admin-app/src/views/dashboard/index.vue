<template>
  <div class="p-6 space-y-6 max-w-7xl mx-auto">
    <!-- 欢迎栏 -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-zinc-200 dark:border-zinc-800">
      <div>
        <h2 class="text-xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
          控制台概览
        </h2>
        <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
          Lemwood Mirror 启动器分发镜像服务运行状态与快捷导航
        </p>
      </div>
      <div class="flex items-center gap-2">
        <el-button
          :loading="loading"
          size="small"
          class="!border-zinc-200 dark:!border-zinc-800"
          @click="loadData"
        >
          <template #icon>
            <component
              :is="RefreshCwIcon"
              class="w-3.5 h-3.5"
            />
          </template>
          刷新状态
        </el-button>
        <el-button
          type="primary"
          size="small"
          @click="$router.push('/config')"
        >
          <template #icon>
            <component
              :is="Settings2Icon"
              class="w-3.5 h-3.5"
            />
          </template>
          配置中心
        </el-button>
      </div>
    </div>

    <!-- 核心指标卡片 -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      <div class="p-4 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
        <div class="flex items-center justify-between">
          <span class="text-xs text-zinc-500 dark:text-zinc-400 font-medium">系统版本</span>
          <component
            :is="ServerIcon"
            class="w-4 h-4 text-zinc-400"
          />
        </div>
        <div class="mt-3 flex items-baseline gap-2">
          <span class="text-lg font-bold text-zinc-900 dark:text-zinc-100 font-mono">
            {{ updateStatus?.current_version || 'dev' }}
          </span>
          <el-tag
            v-if="updateStatus?.has_update"
            size="small"
            type="success"
            effect="plain"
          >
            有更新
          </el-tag>
          <el-tag
            v-else
            size="small"
            type="info"
            effect="plain"
          >
            最新
          </el-tag>
        </div>
      </div>

      <div class="p-4 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
        <div class="flex items-center justify-between">
          <span class="text-xs text-zinc-500 dark:text-zinc-400 font-medium">请求频率限制</span>
          <component
            :is="ActivityIcon"
            class="w-4 h-4 text-zinc-400"
          />
        </div>
        <div class="mt-3 flex items-baseline gap-2">
          <span class="text-lg font-bold text-zinc-900 dark:text-zinc-100">
            {{ fwStatus?.settings?.enabled ? `${fwStatus.settings.per_minute} 次/分` : '未启用' }}
          </span>
          <el-tag
            :type="fwStatus?.settings?.enabled ? 'success' : 'info'"
            size="small"
            effect="plain"
          >
            {{ fwStatus?.settings?.enabled ? '防护中' : '关闭' }}
          </el-tag>
        </div>
      </div>

      <div class="p-4 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
        <div class="flex items-center justify-between">
          <span class="text-xs text-zinc-500 dark:text-zinc-400 font-medium">白名单 / 网段封禁</span>
          <component
            :is="ShieldCheckIcon"
            class="w-4 h-4 text-zinc-400"
          />
        </div>
        <div class="mt-3 flex items-baseline gap-2">
          <span class="text-lg font-bold text-zinc-900 dark:text-zinc-100 font-mono">
            {{ fwStatus?.whitelist_count ?? 0 }} / {{ fwStatus?.cidr_ban_count ?? 0 }}
          </span>
          <span class="text-xs text-zinc-400">个网段</span>
        </div>
      </div>

      <div class="p-4 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
        <div class="flex items-center justify-between">
          <span class="text-xs text-zinc-500 dark:text-zinc-400 font-medium">活跃追踪 / 违规次数</span>
          <component
            :is="ShieldAlertIcon"
            class="w-4 h-4 text-zinc-400"
          />
        </div>
        <div class="mt-3 flex items-baseline gap-2">
          <span class="text-lg font-bold text-zinc-900 dark:text-zinc-100 font-mono">
            {{ fwStatus?.tracked_ips ?? 0 }} / {{ fwStatus?.active_strikes ?? 0 }}
          </span>
          <span class="text-xs text-zinc-400">次违规</span>
        </div>
      </div>
    </div>

    <!-- 快捷功能入口导航 -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
      <div
        class="group p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 hover:border-zinc-400 dark:hover:border-zinc-600 transition-all cursor-pointer flex flex-col justify-between"
        @click="$router.push('/config')"
      >
        <div class="flex items-center gap-3">
          <div class="p-2.5 rounded-lg bg-zinc-100 dark:bg-zinc-800 text-zinc-800 dark:text-zinc-200">
            <component
              :is="Settings2Icon"
              class="w-5 h-5"
            />
          </div>
          <div>
            <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 group-hover:text-zinc-700 dark:group-hover:text-zinc-300">
              配置编辑
            </h3>
            <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-0.5">
              基础参数、PoW 门控、启动器源、自更新管理
            </p>
          </div>
        </div>
        <div class="mt-4 flex items-center justify-between text-xs text-zinc-400">
          <span>进入配置</span>
          <component
            :is="ArrowRightIcon"
            class="w-3.5 h-3.5 group-hover:translate-x-1 transition-transform"
          />
        </div>
      </div>

      <div
        class="group p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 hover:border-zinc-400 dark:hover:border-zinc-600 transition-all cursor-pointer flex flex-col justify-between"
        @click="$router.push('/files')"
      >
        <div class="flex items-center gap-3">
          <div class="p-2.5 rounded-lg bg-zinc-100 dark:bg-zinc-800 text-zinc-800 dark:text-zinc-200">
            <component
              :is="FolderIcon"
              class="w-5 h-5"
            />
          </div>
          <div>
            <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 group-hover:text-zinc-700 dark:group-hover:text-zinc-300">
              文件管理
            </h3>
            <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-0.5">
              文件树浏览、存储资源上传、下载与清理
            </p>
          </div>
        </div>
        <div class="mt-4 flex items-center justify-between text-xs text-zinc-400">
          <span>进入文件库</span>
          <component
            :is="ArrowRightIcon"
            class="w-3.5 h-3.5 group-hover:translate-x-1 transition-transform"
          />
        </div>
      </div>

      <div
        class="group p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 hover:border-zinc-400 dark:hover:border-zinc-600 transition-all cursor-pointer flex flex-col justify-between"
        @click="$router.push('/blacklist')"
      >
        <div class="flex items-center gap-3">
          <div class="p-2.5 rounded-lg bg-zinc-100 dark:bg-zinc-800 text-zinc-800 dark:text-zinc-200">
            <component
              :is="ShieldAlertIcon"
              class="w-5 h-5"
            />
          </div>
          <div>
            <h3 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 group-hover:text-zinc-700 dark:group-hover:text-zinc-300">
              黑名单管理
            </h3>
            <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-0.5">
              IP/CIDR 封禁、自动封禁监控、外部黑名单同步
            </p>
          </div>
        </div>
        <div class="mt-4 flex items-center justify-between text-xs text-zinc-400">
          <span>进入黑名单</span>
          <component
            :is="ArrowRightIcon"
            class="w-3.5 h-3.5 group-hover:translate-x-1 transition-transform"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getFirewallStatus } from '@/api/firewall'
import { getSelfUpdateStatus } from '@/api/config'
import type { FirewallStatus, SelfUpdateStatus } from '@/types'
import {
  Settings2,
  Folder,
  ShieldAlert,
  Server,
  Activity,
  ShieldCheck,
  RefreshCw,
  ArrowRight
} from 'lucide-vue-next'

const Settings2Icon = Settings2
const FolderIcon = Folder
const ShieldAlertIcon = ShieldAlert
const ServerIcon = Server
const ActivityIcon = Activity
const ShieldCheckIcon = ShieldCheck
const RefreshCwIcon = RefreshCw
const ArrowRightIcon = ArrowRight

const loading = ref(false)
const fwStatus = ref<FirewallStatus | null>(null)
const updateStatus = ref<SelfUpdateStatus | null>(null)

const loadData = async () => {
  loading.value = true
  try {
    const [fw, update] = await Promise.allSettled([
      getFirewallStatus(),
      getSelfUpdateStatus()
    ])
    if (fw.status === 'fulfilled') fwStatus.value = fw.value
    if (update.status === 'fulfilled') updateStatus.value = update.value
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadData()
})
</script>
