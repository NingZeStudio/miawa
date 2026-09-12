<template>
  <header class="navbar h-14 border-b border-zinc-200 dark:border-zinc-800 bg-white/80 dark:bg-zinc-950/80 backdrop-blur flex items-center justify-between px-4 z-20">
    <!-- 左侧：面包屑与状态 -->
    <div class="flex items-center gap-3">
      <el-breadcrumb
        separator="/"
        class="text-xs"
      >
        <el-breadcrumb-item :to="{ path: '/' }">
          首页
        </el-breadcrumb-item>
        <el-breadcrumb-item v-if="currentTitle !== '工作台'">
          {{ currentTitle }}
        </el-breadcrumb-item>
      </el-breadcrumb>
    </div>

    <!-- 右侧：操作区 -->
    <div class="flex items-center gap-2">
      <!-- 快捷搜索 (Cmd+K 风格) -->
      <button
        class="hidden sm:flex items-center gap-2 px-2.5 py-1 text-xs text-zinc-500 hover:text-zinc-800 dark:text-zinc-400 dark:hover:text-zinc-200 bg-zinc-100/80 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-md transition-colors mr-1"
        @click="openSearchDialog"
      >
        <component
          :is="SearchIcon"
          class="w-3.5 h-3.5"
        />
        <span>快速导航...</span>
        <kbd class="px-1 py-0.5 text-[10px] bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded font-mono">⌘K</kbd>
      </button>

      <!-- 前往前台 -->
      <el-tooltip
        content="前往前台服务"
        placement="bottom"
      >
        <a
          href="/"
          target="_blank"
          rel="noreferrer"
          class="flex items-center gap-1.5 px-2.5 py-1 text-xs text-zinc-600 dark:text-zinc-300 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-100 dark:hover:bg-zinc-800 rounded-md transition-colors no-underline"
        >
          <component
            :is="ExternalLinkIcon"
            class="w-3.5 h-3.5"
          />
          <span class="hidden sm:inline">前台首页</span>
        </a>
      </el-tooltip>

      <!-- 网页整体全屏 -->
      <el-tooltip
        content="浏览器全屏"
        placement="bottom"
      >
        <button
          class="p-2 rounded-md text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100 hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
          @click="toggleBrowserFullscreen"
        >
          <component
            :is="FullscreenIcon"
            class="w-4 h-4"
          />
        </button>
      </el-tooltip>

      <!-- 主题明暗切换 -->
      <el-tooltip
        :content="isDark ? '切换至浅色模式' : '切换至暗色模式'"
        placement="bottom"
      >
        <button
          class="p-2 rounded-md text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100 hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
          @click="appStore.toggleTheme"
        >
          <component
            :is="isDark ? SunIcon : MoonIcon"
            class="w-4 h-4"
          />
        </button>
      </el-tooltip>

      <!-- 用户信息下拉 -->
      <el-dropdown
        trigger="click"
        class="ml-2"
      >
        <div class="flex items-center gap-2 cursor-pointer select-none">
          <div class="w-7 h-7 rounded-full bg-zinc-800 dark:bg-zinc-200 text-zinc-100 dark:text-zinc-900 flex items-center justify-center text-xs font-semibold ring-1 ring-zinc-200 dark:ring-zinc-800">
            A
          </div>
          <div class="hidden md:flex flex-col text-left">
            <span class="text-xs font-medium text-zinc-900 dark:text-zinc-100 leading-none">
              {{ userInfo.displayName }}
            </span>
            <span class="text-[10px] text-zinc-500 dark:text-zinc-400 font-mono mt-0.5">
              {{ userInfo.role }}
            </span>
          </div>
        </div>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item @click="toggleWatermark">
              <span>{{ watermarkEnabled ? '隐藏安全水印' : '显示安全水印' }}</span>
            </el-dropdown-item>
            <el-dropdown-item
              divided
              @click="handleLogout"
            >
              <span class="text-red-500">安全退出</span>
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>

    <!-- 全局快捷搜索对话框 -->
    <el-dialog
      v-model="searchVisible"
      title="快速导航"
      width="480px"
      :show-close="false"
      class="zinc-dialog"
    >
      <el-input
        v-model="searchQuery"
        placeholder="输入功能名称或路由关键词..."
        clearable
        size="large"
      >
        <template #prefix>
          <component
            :is="SearchIcon"
            class="w-4 h-4 text-zinc-400"
          />
        </template>
      </el-input>

      <div class="mt-4 max-h-60 overflow-y-auto space-y-1">
        <div
          v-for="item in filteredRoutes"
          :key="item.path"
          class="flex items-center justify-between p-2 rounded-md hover:bg-zinc-100 dark:hover:bg-zinc-800 cursor-pointer transition-colors"
          @click="selectSearchItem(item.path)"
        >
          <div class="flex items-center gap-2">
            <span class="text-xs font-medium text-zinc-800 dark:text-zinc-200">{{ item.title }}</span>
          </div>
          <span class="text-[11px] text-zinc-400 font-mono">{{ item.path }}</span>
        </div>
        <div
          v-if="filteredRoutes.length === 0"
          class="text-center py-6 text-xs text-zinc-400"
        >
          未检索到相关路由
        </div>
      </div>
    </el-dialog>
  </header>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/store/modules/app'
import { useUserStore } from '@/store/modules/user'
import { useTabsStore } from '@/store/modules/tabs'
import { ElMessage } from 'element-plus'
import {
  Search,
  Expand,
  Sun,
  Moon,
  ExternalLink
} from 'lucide-vue-next'

const SearchIcon = Search
const FullscreenIcon = Expand
const SunIcon = Sun
const MoonIcon = Moon
const ExternalLinkIcon = ExternalLink

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const userStore = useUserStore()
const tabsStore = useTabsStore()

const isDark = computed(() => appStore.isDark)
const userInfo = computed(() => userStore.userInfo)
const watermarkEnabled = computed(() => appStore.watermarkEnabled)

const toggleWatermark = () => {
  appStore.toggleWatermark()
  ElMessage.info(appStore.watermarkEnabled ? '已开启全局防截屏水印' : '已停用全局水印')
}

const currentTitle = computed(() => {
  return (route.meta?.title as string) || '工作台'
})

const toggleBrowserFullscreen = () => {
  if (!document.fullscreenElement) {
    document.documentElement.requestFullscreen().catch(() => {})
  } else {
    document.exitFullscreen().catch(() => {})
  }
}

const handleLogout = () => {
  userStore.logout()
  tabsStore.closeAllTabs()
  ElMessage.success('已安全退出登录')
  router.push('/login')
}

// 快速搜索逻辑
const searchVisible = ref(false)
const searchQuery = ref('')

const allNavItems = [
  { title: '工作台概览', path: '/dashboard', isIframe: false },
  { title: '配置编辑', path: '/config', isIframe: false },
  { title: '文件管理', path: '/files', isIframe: false },
  { title: '黑名单管理', path: '/blacklist', isIframe: false },
]

const filteredRoutes = computed(() => {
  if (!searchQuery.value.trim()) return allNavItems
  const q = searchQuery.value.toLowerCase()
  return allNavItems.filter(i => i.title.toLowerCase().includes(q) || i.path.toLowerCase().includes(q))
})

const openSearchDialog = () => {
  searchQuery.value = ''
  searchVisible.value = true
}

const selectSearchItem = (path: string) => {
  searchVisible.value = false
  router.push(path)
}

// 监听键盘 ⌘K / Ctrl+K
const handleGlobalKeydown = (e: KeyboardEvent) => {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    searchVisible.value = true
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleGlobalKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
})
</script>
