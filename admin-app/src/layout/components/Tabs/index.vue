<template>
  <div class="tabs-bar h-10 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-950/40 flex items-center px-3 select-none justify-between">
    <!-- 标签横向滚动区 -->
    <div
      ref="scrollContainerRef"
      class="flex items-center gap-1.5 overflow-x-auto no-scrollbar py-1 flex-1 pr-2"
      @wheel.prevent="handleScroll"
    >
      <div
        v-for="tab in tabs"
        :key="tab.path"
        class="tab-item group flex items-center gap-1.5 px-3 py-1 text-xs font-medium rounded-md cursor-pointer transition-all whitespace-nowrap border"
        :class="[
          tab.path === activeTabPath
            ? 'bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 border-zinc-200 dark:border-zinc-800 shadow-sm'
            : 'text-zinc-500 hover:text-zinc-800 dark:text-zinc-400 dark:hover:text-zinc-200 border-transparent hover:bg-zinc-200/50 dark:hover:bg-zinc-800/40'
        ]"
        @click="handleTabClick(tab)"
        @contextmenu.prevent="openContextMenu($event, tab)"
      >
        <!-- iframe 标签特有微标指示 -->
        <span
          v-if="tab.isIframe"
          class="w-1.5 h-1.5 rounded-full bg-zinc-400 dark:bg-zinc-600 group-hover:bg-zinc-600 dark:group-hover:bg-zinc-300 transition-colors"
          title="iframe 独立保活容器"
        />

        <span class="truncate max-w-[160px]">{{ tab.title }}</span>

        <!-- 关闭按钮 -->
        <button
          v-if="tab.closable"
          class="w-4 h-4 rounded hover:bg-zinc-200 dark:hover:bg-zinc-800 flex items-center justify-center text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 transition-colors"
          @click.stop="closeTab(tab.path)"
        >
          <component
            :is="XIcon"
            class="w-3 h-3"
          />
        </button>
      </div>
    </div>

    <!-- 右侧快捷功能 -->
    <div class="flex items-center gap-1 pl-2 border-l border-zinc-200 dark:border-zinc-800">
      <el-tooltip
        content="刷新当前"
        placement="bottom"
      >
        <button
          class="p-1.5 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100 rounded hover:bg-zinc-200/60 dark:hover:bg-zinc-800 transition-colors"
          @click="refreshCurrent"
        >
          <component
            :is="RotateCwIcon"
            class="w-3.5 h-3.5"
          />
        </button>
      </el-tooltip>

      <el-dropdown
        trigger="click"
        @command="handleCommand"
      >
        <button class="p-1.5 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100 rounded hover:bg-zinc-200/60 dark:hover:bg-zinc-800 transition-colors">
          <component
            :is="ChevronDownIcon"
            class="w-3.5 h-3.5"
          />
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="refresh">
              刷新当前
            </el-dropdown-item>
            <el-dropdown-item
              v-if="isCurrentIframe"
              command="fullscreen"
            >
              当前应用全屏
            </el-dropdown-item>
            <el-dropdown-item
              v-if="isCurrentIframe"
              command="openNewTab"
            >
              新窗口弹出
            </el-dropdown-item>
            <el-dropdown-item
              divided
              command="closeOther"
            >
              关闭其他
            </el-dropdown-item>
            <el-dropdown-item command="closeAll">
              关闭所有
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>

    <!-- 右键菜单浮层 -->
    <div
      v-if="contextMenuVisible"
      ref="contextMenuRef"
      class="fixed z-50 min-w-[140px] bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-md shadow-lg py-1 text-xs text-zinc-700 dark:text-zinc-300"
      :style="{ top: contextMenuPosition.y + 'px', left: contextMenuPosition.x + 'px' }"
    >
      <div
        class="px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 cursor-pointer flex items-center gap-2"
        @click="handleContextAction('refresh')"
      >
        <component
          :is="RotateCwIcon"
          class="w-3.5 h-3.5 text-zinc-500"
        />
        <span>刷新</span>
      </div>
      <div
        v-if="selectedContextTab?.isIframe"
        class="px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 cursor-pointer flex items-center gap-2"
        @click="handleContextAction('fullscreen')"
      >
        <component
          :is="Maximize2Icon"
          class="w-3.5 h-3.5 text-zinc-500"
        />
        <span>应用全屏</span>
      </div>
      <div
        v-if="selectedContextTab?.isIframe"
        class="px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 cursor-pointer flex items-center gap-2"
        @click="handleContextAction('openExternal')"
      >
        <component
          :is="ExternalLinkIcon"
          class="w-3.5 h-3.5 text-zinc-500"
        />
        <span>独立窗口打开</span>
      </div>
      <div class="h-px bg-zinc-200 dark:bg-zinc-800 my-1" />
      <div
        v-if="selectedContextTab?.closable"
        class="px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 cursor-pointer flex items-center gap-2 text-red-600 dark:text-red-400"
        @click="handleContextAction('closeCurrent')"
      >
        <component
          :is="XIcon"
          class="w-3.5 h-3.5"
        />
        <span>关闭</span>
      </div>
      <div
        class="px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 cursor-pointer"
        @click="handleContextAction('closeOther')"
      >
        <span>关闭其他</span>
      </div>
      <div
        class="px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 cursor-pointer"
        @click="handleContextAction('closeAll')"
      >
        <span>关闭所有</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useTabsStore } from '@/store/modules/tabs'
import { useAppStore } from '@/store/modules/app'
import type { TabItem } from '@/types'
import {
  X,
  RotateCw,
  ChevronDown,
  Maximize2,
  ExternalLink
} from 'lucide-vue-next'

const XIcon = X
const RotateCwIcon = RotateCw
const ChevronDownIcon = ChevronDown
const Maximize2Icon = Maximize2
const ExternalLinkIcon = ExternalLink

const router = useRouter()
const tabsStore = useTabsStore()
const appStore = useAppStore()

const tabs = computed(() => tabsStore.tabs)
const activeTabPath = computed(() => tabsStore.activeTabPath)
const isCurrentIframe = computed(() => tabsStore.isCurrentTabIframe)

const scrollContainerRef = ref<HTMLElement | null>(null)
const contextMenuVisible = ref(false)
const contextMenuPosition = ref({ x: 0, y: 0 })
const selectedContextTab = ref<TabItem | null>(null)

// 鼠标滚轮横向滑动
const handleScroll = (e: WheelEvent) => {
  if (scrollContainerRef.value) {
    scrollContainerRef.value.scrollLeft += e.deltaY
  }
}

const handleTabClick = (tab: TabItem) => {
  if (tab.path === activeTabPath.value) return
  tabsStore.activeTabPath = tab.path
  router.push(tab.path)
}

const closeTab = (path: string) => {
  tabsStore.closeTab(path)
}

const refreshCurrent = () => {
  if (isCurrentIframe.value) {
    tabsStore.refreshCurrentIframe()
  } else {
    // 普通路由页面刷新
    const current = tabsStore.activeTabPath
    router.replace({ path: '/redirect' + current }).catch(() => {
      window.location.reload()
    })
  }
}

const openContextMenu = (e: MouseEvent, tab: TabItem) => {
  selectedContextTab.value = tab
  contextMenuPosition.value = { x: e.clientX, y: e.clientY }
  contextMenuVisible.value = true
}

const closeContextMenu = () => {
  contextMenuVisible.value = false
}

const handleContextAction = (action: string) => {
  if (!selectedContextTab.value) return
  const tab = selectedContextTab.value

  switch (action) {
    case 'refresh':
      if (tab.isIframe) {
        tabsStore.refreshCurrentIframe()
      } else {
        refreshCurrent()
      }
      break
    case 'fullscreen':
      appStore.toggleIframeFullscreen(true)
      break
    case 'openExternal':
      if (tab.iframeUrl) {
        window.open(tab.iframeUrl, '_blank')
      }
      break
    case 'closeCurrent':
      tabsStore.closeTab(tab.path)
      break
    case 'closeOther':
      tabsStore.closeOtherTabs(tab.path)
      break
    case 'closeAll':
      tabsStore.closeAllTabs()
      break
  }
  closeContextMenu()
}

const handleCommand = (cmd: string) => {
  const currentPath = activeTabPath.value
  switch (cmd) {
    case 'refresh':
      refreshCurrent()
      break
    case 'fullscreen':
      appStore.toggleIframeFullscreen(true)
      break
    case 'openNewTab':
      if (tabsStore.currentIframeInstance?.url) {
        window.open(tabsStore.currentIframeInstance.url, '_blank')
      }
      break
    case 'closeOther':
      tabsStore.closeOtherTabs(currentPath)
      break
    case 'closeAll':
      tabsStore.closeAllTabs()
      break
  }
}

onMounted(() => {
  document.addEventListener('click', closeContextMenu)
})

onUnmounted(() => {
  document.removeEventListener('click', closeContextMenu)
})
</script>

<style scoped>
.no-scrollbar::-webkit-scrollbar {
  display: none;
}
.no-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
</style>
