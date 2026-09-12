<template>
  <div class="p-6 space-y-6 max-w-7xl mx-auto">
    <!-- 头部栏 -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-zinc-200 dark:border-zinc-800">
      <div>
        <h2 class="text-xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 flex items-center gap-2">
          <component
            :is="ShieldAlertIcon"
            class="w-5 h-5 text-zinc-600 dark:text-zinc-400"
          />
          黑名单管理
        </h2>
        <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
          全站 IP 与 CIDR 网段封禁列表、自动限速违规记录与外部黑名单源同步监控
        </p>
      </div>

      <div class="flex items-center gap-2">
        <el-button
          size="default"
          @click="handleRefreshAll"
        >
          <template #icon>
            <component
              :is="RefreshCwIcon"
              class="w-4 h-4"
            />
          </template>
          刷新
        </el-button>
        <el-button
          type="primary"
          size="default"
          @click="openAddDialog"
        >
          <template #icon>
            <component
              :is="PlusIcon"
              class="w-4 h-4"
            />
          </template>
          添加封禁条目
        </el-button>
      </div>
    </div>

    <!-- 防火墙运行状态卡片 -->
    <div class="p-5 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="text-xs font-semibold text-zinc-700 dark:text-zinc-300 flex items-center gap-1.5 uppercase tracking-wider">
          <component
            :is="FlameIcon"
            class="w-3.5 h-3.5 text-zinc-500"
          />
          防火墙实时状态
        </h3>
        <span class="text-[11px] text-zinc-400">
          频率拦截: {{ fwStatus?.settings?.enabled ? `每分钟限额 ${fwStatus.settings.per_minute} 次 / 阈值 ${fwStatus.settings.ban_threshold} 次` : '已停用' }}
        </span>
      </div>

      <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs pt-1">
        <div class="p-3 rounded-lg bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800">
          <span class="text-zinc-400 block text-[11px]">白名单网段/IP</span>
          <span class="text-base font-bold text-zinc-900 dark:text-zinc-100 font-mono mt-0.5 block">
            {{ fwStatus?.whitelist_count ?? 0 }}
          </span>
        </div>
        <div class="p-3 rounded-lg bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800">
          <span class="text-zinc-400 block text-[11px]">CIDR 网段封禁</span>
          <span class="text-base font-bold text-zinc-900 dark:text-zinc-100 font-mono mt-0.5 block">
            {{ fwStatus?.cidr_ban_count ?? 0 }}
          </span>
        </div>
        <div class="p-3 rounded-lg bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800">
          <span class="text-zinc-400 block text-[11px]">活跃追踪客户端</span>
          <span class="text-base font-bold text-zinc-900 dark:text-zinc-100 font-mono mt-0.5 block">
            {{ fwStatus?.tracked_ips ?? 0 }}
          </span>
        </div>
        <div class="p-3 rounded-lg bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800">
          <span class="text-zinc-400 block text-[11px]">有效违规累计</span>
          <span class="text-base font-bold text-zinc-900 dark:text-zinc-100 font-mono mt-0.5 block">
            {{ fwStatus?.active_strikes ?? 0 }}
          </span>
        </div>
      </div>
    </div>

    <!-- 筛选过滤与搜索工具栏 -->
    <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3">
      <!-- 统计分类切换 -->
      <div class="flex items-center gap-1.5 overflow-x-auto text-xs pb-1 sm:pb-0">
        <button
          class="px-3 py-1.5 rounded-lg font-medium transition-colors flex items-center gap-1.5 shrink-0 border"
          :class="[
            filterSource === 'all'
              ? 'bg-zinc-900 dark:bg-zinc-100 text-zinc-100 dark:text-zinc-900 border-transparent shadow-sm'
              : 'bg-white dark:bg-zinc-900 text-zinc-600 dark:text-zinc-400 border-zinc-200 dark:border-zinc-800 hover:bg-zinc-100 dark:hover:bg-zinc-800'
          ]"
          @click="setFilter('all')"
        >
          <span>全部条目</span>
          <span class="text-[10px] font-mono opacity-80">({{ stats.all || 0 }})</span>
        </button>

        <button
          class="px-3 py-1.5 rounded-lg font-medium transition-colors flex items-center gap-1.5 shrink-0 border"
          :class="[
            filterSource === 'manual'
              ? 'bg-zinc-900 dark:bg-zinc-100 text-zinc-100 dark:text-zinc-900 border-transparent shadow-sm'
              : 'bg-white dark:bg-zinc-900 text-zinc-600 dark:text-zinc-400 border-zinc-200 dark:border-zinc-800 hover:bg-zinc-100 dark:hover:bg-zinc-800'
          ]"
          @click="setFilter('manual')"
        >
          <span>手动添加</span>
          <span class="text-[10px] font-mono opacity-80">({{ stats.manual || 0 }})</span>
        </button>

        <button
          class="px-3 py-1.5 rounded-lg font-medium transition-colors flex items-center gap-1.5 shrink-0 border"
          :class="[
            filterSource === 'local'
              ? 'bg-zinc-900 dark:bg-zinc-100 text-zinc-100 dark:text-zinc-900 border-transparent shadow-sm'
              : 'bg-white dark:bg-zinc-900 text-zinc-600 dark:text-zinc-400 border-zinc-200 dark:border-zinc-800 hover:bg-zinc-100 dark:hover:bg-zinc-800'
          ]"
          @click="setFilter('local')"
        >
          <span>自动封禁</span>
          <span class="text-[10px] font-mono opacity-80">({{ stats.auto ?? stats.local ?? 0 }})</span>
        </button>

        <button
          class="px-3 py-1.5 rounded-lg font-medium transition-colors flex items-center gap-1.5 shrink-0 border"
          :class="[
            filterSource === 'external'
              ? 'bg-zinc-900 dark:bg-zinc-100 text-zinc-100 dark:text-zinc-900 border-transparent shadow-sm'
              : 'bg-white dark:bg-zinc-900 text-zinc-600 dark:text-zinc-400 border-zinc-200 dark:border-zinc-800 hover:bg-zinc-100 dark:hover:bg-zinc-800'
          ]"
          @click="setFilter('external')"
        >
          <span>外部同步</span>
          <span class="text-[10px] font-mono opacity-80">({{ stats.external || 0 }})</span>
        </button>
      </div>

      <!-- 搜索输入框 -->
      <div class="w-full sm:w-64">
        <el-input
          v-model="searchInput"
          placeholder="搜索 IP、网段或原因..."
          clearable
          size="default"
        >
          <template #prefix>
            <component
              :is="SearchIcon"
              class="w-3.5 h-3.5 text-zinc-400"
            />
          </template>
        </el-input>
      </div>
    </div>

    <!-- 数据表格 -->
    <div class="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 overflow-hidden shadow-sm">
      <el-table
        v-loading="loading"
        :data="items"
        style="width: 100%"
        empty-text="暂无黑名单条目"
      >
        <el-table-column
          label="IP / CIDR 网段"
          min-width="180"
        >
          <template #default="{ row }">
            <span class="font-mono text-xs font-semibold text-zinc-900 dark:text-zinc-100 select-text">
              {{ row.ip }}
            </span>
          </template>
        </el-table-column>

        <el-table-column
          label="来源渠道"
          width="120"
        >
          <template #default="{ row }">
            <el-tag
              :type="getSourceTagType(row.source)"
              size="small"
              effect="plain"
            >
              {{ getSourceLabel(row.source) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column
          label="封禁类型"
          width="130"
        >
          <template #default="{ row }">
            <span class="text-xs text-zinc-500 font-mono">
              {{ row.ban_type || 'default' }}
            </span>
          </template>
        </el-table-column>

        <el-table-column
          label="封禁原因"
          min-width="220"
        >
          <template #default="{ row }">
            <span class="text-xs text-zinc-600 dark:text-zinc-300">
              {{ row.reason || '-' }}
            </span>
          </template>
        </el-table-column>

        <el-table-column
          label="录入时间"
          width="170"
        >
          <template #default="{ row }">
            <span class="text-xs text-zinc-400">
              {{ formatDate(row.created_at) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column
          label="操作"
          width="110"
          fixed="right"
        >
          <template #default="{ row }">
            <el-popconfirm
              title="确定解除对此条目的封禁吗？"
              confirm-button-text="解封"
              cancel-button-text="取消"
              @confirm="handleRemove(row.ip)"
            >
              <template #reference>
                <el-button
                  size="small"
                  type="danger"
                  text
                >
                  <template #icon>
                    <component
                      :is="UnlockIcon"
                      class="w-3.5 h-3.5"
                    />
                  </template>
                  解封
                </el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>

      <!-- 服务端分页条 -->
      <div class="p-4 border-t border-zinc-200 dark:border-zinc-800 flex items-center justify-between flex-wrap gap-3">
        <span class="text-xs text-zinc-400">
          共 {{ total }} 条记录
        </span>

        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="sizes, prev, pager, next"
          size="small"
          @size-change="loadData"
          @current-change="loadData"
        />
      </div>
    </div>

    <!-- 添加封禁条目模态框 -->
    <el-dialog
      v-model="addModalVisible"
      title="添加黑名单条目"
      width="460px"
      class="zinc-dialog"
    >
      <el-form
        ref="addFormRef"
        :model="addForm"
        :rules="addRules"
        label-position="top"
      >
        <el-form-item
          label="IP 地址或 CIDR 网段"
          prop="ip"
          extra="支持 IPv4、IPv6 单地址，或形如 192.168.1.0/24 的 CIDR 网段"
        >
          <el-input
            v-model="addForm.ip"
            placeholder="例如: 1.2.3.4 或 10.0.0.0/8"
            clearable
          />
        </el-form-item>

        <el-form-item
          label="封禁原因"
          prop="reason"
        >
          <el-input
            v-model="addForm.reason"
            type="textarea"
            :rows="3"
            placeholder="说明封禁背景或违规行为"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="flex items-center justify-end gap-2">
          <el-button @click="addModalVisible = false">
            取消
          </el-button>
          <el-button
            type="primary"
            :loading="adding"
            @click="handleSubmitAdd"
          >
            确认封禁
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { getBlacklistPage, addBlacklist, removeBlacklist } from '@/api/blacklist'
import { getFirewallStatus } from '@/api/firewall'
import type { BlacklistItem, BlacklistStats, FirewallStatus } from '@/types'
import dayjs from 'dayjs'
import {
  ShieldAlert,
  Plus,
  RefreshCw,
  Flame,
  Search,
  Unlock
} from 'lucide-vue-next'

const ShieldAlertIcon = ShieldAlert
const PlusIcon = Plus
const RefreshCwIcon = RefreshCw
const FlameIcon = Flame
const SearchIcon = Search
const UnlockIcon = Unlock

const loading = ref(false)
const adding = ref(false)
const items = ref<BlacklistItem[]>([])
const total = ref(0)
const stats = ref<BlacklistStats>({ all: 0, manual: 0, external: 0, local: 0, auto: 0 })
const fwStatus = ref<FirewallStatus | null>(null)

const page = ref(1)
const pageSize = ref(20)
const filterSource = ref('all')
const searchInput = ref('')
const keyword = ref('')

// IP/CIDR 正则与后端一致
const IP_OR_CIDR_RE = /^(?:\d{1,3}(?:\.\d{1,3}){3})(?:\/\d{1,2})?$|^[0-9A-Fa-f:]+(?:\/\d{1,3})?$/

const addModalVisible = ref(false)
const addFormRef = ref<FormInstance>()
const addForm = ref({
  ip: '',
  reason: ''
})

const addRules: FormRules = {
  ip: [
    { required: true, message: '请输入 IP 地址或 CIDR 网段', trigger: 'blur' },
    {
      validator: (_rule, value, callback) => {
        if (!value || IP_OR_CIDR_RE.test(value.trim())) {
          callback()
        } else {
          callback(new Error('IP 或 CIDR 网段格式不合法'))
        }
      },
      trigger: 'blur'
    }
  ],
  reason: [{ required: true, message: '请输入封禁原因', trigger: 'blur' }]
}

let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(searchInput, (val) => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    keyword.value = val.trim()
    page.value = 1
    loadData()
  }, 400)
})

const formatDate = (dateStr: string) => {
  return dayjs(dateStr).format('YYYY-MM-DD HH:mm')
}

const getSourceLabel = (source: string) => {
  switch (source) {
    case 'manual':
      return '手动添加'
    case 'local':
    case 'auto':
      return '自动封禁'
    case 'external':
      return '外部同步'
    default:
      return source || '未知'
  }
}

const getSourceTagType = (source: string): '' | 'info' | 'success' | 'warning' | 'danger' => {
  switch (source) {
    case 'manual':
      return 'info'
    case 'local':
    case 'auto':
      return 'danger'
    case 'external':
      return 'warning'
    default:
      return 'info'
  }
}

const setFilter = (src: string) => {
  filterSource.value = src
  page.value = 1
  loadData()
}

const loadData = async () => {
  loading.value = true
  try {
    const res = await getBlacklistPage({
      page: page.value,
      pageSize: pageSize.value,
      source: filterSource.value,
      keyword: keyword.value
    })
    items.value = res.items || []
    total.value = res.total
    stats.value = res.stats || { all: 0 }
  } catch (error: any) {
    ElMessage.error(error.message || '加载黑名单失败')
  } finally {
    loading.value = false
  }
}

const loadFwStatus = async () => {
  try {
    const status = await getFirewallStatus()
    fwStatus.value = status
  } catch {
    // 忽略
  }
}

const handleRefreshAll = () => {
  loadData()
  loadFwStatus()
}

const openAddDialog = () => {
  addForm.value = { ip: '', reason: '' }
  addModalVisible.value = true
}

const handleSubmitAdd = async () => {
  if (!addFormRef.value) return
  await addFormRef.value.validate(async (valid) => {
    if (!valid) return
    adding.value = true
    try {
      await addBlacklist({
        ip: addForm.value.ip.trim(),
        reason: addForm.value.reason.trim()
      })
      ElMessage.success('成功添加封禁条目')
      addModalVisible.value = false
      await loadData()
      await loadFwStatus()
    } catch (error: any) {
      ElMessage.error(error.message || '添加失败')
    } finally {
      adding.value = false
    }
  })
}

const handleRemove = async (ip: string) => {
  try {
    await removeBlacklist(ip)
    ElMessage.success(`已解除 ${ip} 的封禁`)
    await loadData()
    await loadFwStatus()
  } catch (error: any) {
    ElMessage.error(error.message || '解封失败')
  }
}

onMounted(() => {
  loadData()
  loadFwStatus()
})
</script>
