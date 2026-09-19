<template>
  <div class="p-6 space-y-6 max-w-7xl mx-auto">
    <!-- 头部栏 -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-zinc-200 dark:border-zinc-800">
      <div>
        <h2 class="text-xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 flex items-center gap-2">
          <component
            :is="NetworkIcon"
            class="w-5 h-5 text-zinc-600 dark:text-zinc-400"
          />
          子节点管理
        </h2>
        <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
          分流子节点清单与运行状态聚合（每 5 分钟自动轮询，保存后立即生效）
        </p>
      </div>

      <div class="flex items-center gap-2">
        <el-button
          size="default"
          :loading="refreshing"
          @click="handleRefresh"
        >
          <template #icon>
            <component
              :is="RefreshCwIcon"
              class="w-4 h-4"
            />
          </template>
          立即轮询
        </el-button>
      </div>
    </div>

    <!-- 节点状态 -->
    <el-card shadow="never">
      <template #header>
        <span class="text-sm font-medium">运行状态</span>
      </template>
      <el-empty
        v-if="!statuses.length"
        description="尚未配置子节点"
        :image-size="80"
      />
      <el-table
        v-else
        :data="statuses"
        size="default"
      >
        <el-table-column
          label="节点"
          min-width="140"
        >
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span
                class="inline-block h-2 w-2 rounded-full"
                :class="row.online ? 'bg-green-500' : 'bg-red-500'"
              />
              <span class="font-medium">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column
          prop="url"
          label="地址"
          min-width="200"
          show-overflow-tooltip
        />
        <el-table-column
          label="版本"
          width="90"
        >
          <template #default="{ row }">
            {{ row.online ? `v${row.version}` : '-' }}
          </template>
        </el-table-column>
        <el-table-column
          label="活跃下载"
          width="90"
        >
          <template #default="{ row }">
            {{ row.online ? row.active_downloads || 0 : '-' }}
          </template>
        </el-table-column>
        <el-table-column
          label="带宽"
          width="130"
        >
          <template #default="{ row }">
            <template v-if="row.online">
              {{ (row.current_bandwidth_mbps || 0).toFixed(1) }} / {{ row.bandwidth_limit_mbps ?? '-' }} Mbps
            </template>
            <template v-else>-</template>
          </template>
        </el-table-column>
        <el-table-column
          label="启动器"
          width="90"
        >
          <template #default="{ row }">
            {{ row.online ? (row.launchers || []).length : '-' }}
          </template>
        </el-table-column>
        <el-table-column
          label="磁盘可用"
          width="100"
        >
          <template #default="{ row }">
            <template v-if="row.online && row.disk">
              {{ formatBytes(row.disk.free) }}
            </template>
            <template v-else>-</template>
          </template>
        </el-table-column>
        <el-table-column
          label="最近轮询"
          width="110"
        >
          <template #default="{ row }">
            {{ formatTime(row.last_poll) }}
          </template>
        </el-table-column>
        <el-table-column
          label="备注"
          min-width="140"
        >
          <template #default="{ row }">
            <span
              v-if="!row.online"
              class="text-xs text-red-500"
            >{{ row.error || '无响应' }}</span>
            <span
              v-else
              class="text-xs text-zinc-400"
            >运行 {{ formatUptime(row.uptime_seconds) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 节点清单编辑 -->
    <el-card shadow="never">
      <template #header>
        <span class="text-sm font-medium">节点清单</span>
      </template>
      <div class="space-y-3">
        <div
          v-for="(node, index) in draftNodes"
          :key="index"
          class="flex flex-col sm:flex-row gap-3 items-start sm:items-center"
        >
          <el-input
            v-model="node.name"
            placeholder="节点名称（如 us1）"
            class="!w-full sm:!w-52"
            maxlength="32"
          />
          <el-input
            v-model="node.url"
            placeholder="基准地址（如 https://us1.miawa.cn）"
            class="!w-full sm:flex-1"
          />
          <el-button
            type="danger"
            plain
            @click="draftNodes.splice(index, 1)"
          >
            <template #icon>
              <component
                :is="TrashIcon"
                class="w-4 h-4"
              />
            </template>
            移除
          </el-button>
        </div>
        <el-button
          plain
          @click="draftNodes.push({ name: '', url: '' })"
        >
          <template #icon>
            <component
              :is="PlusIcon"
              class="w-4 h-4"
            />
          </template>
          添加节点
        </el-button>
        <div class="pt-2 border-t border-zinc-100 dark:border-zinc-800 flex justify-end">
          <el-button
            type="primary"
            :loading="saving"
            @click="handleSave"
          >
            保存清单
          </el-button>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Network, RefreshCw, Plus, Trash2 } from 'lucide-vue-next'
import { getNodes, saveNodes, refreshNodes } from '@/api/nodes'
import type { MirrorNode, NodeStatus } from '@/api/nodes'

const NetworkIcon = Network
const RefreshCwIcon = RefreshCw
const PlusIcon = Plus
const TrashIcon = Trash2

const statuses = ref<NodeStatus[]>([])
const draftNodes = ref<MirrorNode[]>([])
const refreshing = ref(false)
const saving = ref(false)

const formatBytes = (bytes?: number) => {
  if (!bytes) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

const formatTime = (ts?: string) => {
  if (!ts) return '-'
  try {
    return new Date(ts).toLocaleTimeString('zh-CN', { hour12: false })
  } catch {
    return '-'
  }
}

const formatUptime = (seconds?: number) => {
  if (!seconds || seconds < 60) return `${seconds || 0} 秒`
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return h > 0 ? `${h} 时 ${m} 分` : `${m} 分`
}

const loadNodes = async () => {
  try {
    const data = await getNodes()
    statuses.value = data.status || []
    draftNodes.value = (data.nodes || []).map((n) => ({ ...n }))
  } catch (e: any) {
    ElMessage.error(e.message || '加载子节点清单失败')
  }
}

const handleRefresh = async () => {
  refreshing.value = true
  try {
    statuses.value = await refreshNodes()
    ElMessage.success('已重新轮询全部子节点')
  } catch (e: any) {
    ElMessage.error(e.message || '轮询失败')
  } finally {
    refreshing.value = false
  }
}

const handleSave = async () => {
  saving.value = true
  try {
    await saveNodes(draftNodes.value.filter((n) => n.name || n.url))
    ElMessage.success('节点清单已保存')
    await loadNodes()
    statuses.value = await refreshNodes()
  } catch (e: any) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(loadNodes)
</script>
