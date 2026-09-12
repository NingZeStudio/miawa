<template>
  <div
    class="iframe-container relative w-full h-full overflow-hidden bg-background"
    :class="{ 'is-fullscreen fixed inset-0 z-50': isFullscreen }"
  >
    <!-- 全屏退出快捷浮动按钮 -->
    <div
      v-if="isFullscreen"
      class="absolute top-4 right-4 z-50 flex items-center gap-2 bg-zinc-900/90 text-zinc-100 dark:bg-zinc-100/90 dark:text-zinc-900 px-3 py-1.5 rounded-full shadow-lg text-xs cursor-pointer backdrop-blur hover:opacity-90 transition-opacity"
      @click="exitFullscreen"
    >
      <component
        :is="Minimize2Icon"
        class="w-3.5 h-3.5"
      />
      <span>退出全屏 (ESC)</span>
    </div>

    <!-- 遍历所有保活的 iframe 实例 -->
    <div
      v-for="item in iframeList"
      v-show="activeTabPath === item.id"
      :key="item.id"
      class="iframe-wrapper w-full h-full relative"
    >
      <!-- 加载遮罩与骨架动画 -->
      <div
        v-if="item.loading"
        class="absolute inset-0 z-10 flex flex-col items-center justify-center bg-background/80 backdrop-blur-sm transition-opacity duration-300"
      >
        <div class="flex flex-col items-center gap-3">
          <!-- Zinc 极简环形 Loader -->
          <div class="w-8 h-8 rounded-full border-2 border-zinc-200 dark:border-zinc-800 border-t-zinc-800 dark:border-t-zinc-200 animate-spin" />
          <div class="text-xs text-zinc-500 dark:text-zinc-400 font-medium">
            正在载入应用: {{ item.title }}...
          </div>
        </div>
      </div>

      <!-- 加载异常提示遮罩 -->
      <div
        v-if="item.hasError"
        class="absolute inset-0 z-20 flex flex-col items-center justify-center bg-background p-6"
      >
        <div class="max-w-md w-full border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 bg-card text-center space-y-4 shadow-sm">
          <div class="w-10 h-10 rounded-full bg-zinc-100 dark:bg-zinc-800 flex items-center justify-center mx-auto text-zinc-600 dark:text-zinc-300">
            <component
              :is="AlertTriangleIcon"
              class="w-5 h-5"
            />
          </div>
          <div class="space-y-1">
            <h4 class="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
              子页面载入受阻
            </h4>
            <p class="text-xs text-zinc-500 dark:text-zinc-400 leading-relaxed">
              {{ item.errorMessage || '该网站可能配置了 X-Frame-Options 限制或网络连接超时。' }}
            </p>
          </div>
          <div class="flex justify-center gap-3 pt-2">
            <button
              class="px-3 py-1.5 text-xs font-medium rounded-md border border-zinc-200 dark:border-zinc-800 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-zinc-700 dark:text-zinc-300 transition-colors"
              @click="retryLoad(item.id)"
            >
              重新载入
            </button>
            <a
              :href="item.url"
              target="_blank"
              rel="noopener noreferrer"
              class="px-3 py-1.5 text-xs font-medium rounded-md bg-zinc-900 dark:bg-zinc-100 text-zinc-100 dark:text-zinc-900 hover:opacity-90 transition-opacity"
            >
              在独立窗口打开
            </a>
          </div>
        </div>
      </div>

      <!-- 原生 iframe 节点 -->
      <iframe
        :id="'iframe-' + sanitizeId(item.id)"
        :key="item.refreshKey"
        :src="item.url"
        class="app-iframe w-full h-full border-0 block"
        allow="fullscreen; clipboard-read; clipboard-write"
        @load="handleIframeLoaded(item.id)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useTabsStore } from '@/store/modules/tabs'
import { useAppStore } from '@/store/modules/app'
import { Minimize2, AlertTriangle } from 'lucide-vue-next'

const Minimize2Icon = Minimize2
const AlertTriangleIcon = AlertTriangle

const tabsStore = useTabsStore()
const appStore = useAppStore()

const iframeList = computed(() => tabsStore.iframeList)
const activeTabPath = computed(() => tabsStore.activeTabPath)
const isFullscreen = computed(() => appStore.isIframeFullscreen)

const sanitizeId = (id: string) => {
  return id.replace(/[^a-zA-Z0-9_-]/g, '_')
}

const timeoutMap = new Map<string, number>()

const startTimeoutChecker = (id: string) => {
  if (timeoutMap.has(id)) {
    window.clearTimeout(timeoutMap.get(id))
  }
  const timer = window.setTimeout(() => {
    const item = tabsStore.iframeList.find(i => i.id === id)
    if (item && item.loading) {
      tabsStore.setIframeError(id, '子系统加载时间过长，可能由于网络缓慢或目标站点限制了跨域嵌入 (X-Frame-Options)。')
    }
  }, 10000)
  timeoutMap.set(id, timer)
}

const handleIframeLoaded = (id: string) => {
  if (timeoutMap.has(id)) {
    window.clearTimeout(timeoutMap.get(id))
    timeoutMap.delete(id)
  }
  tabsStore.setIframeLoaded(id)
}

const retryLoad = (id: string) => {
  const item = tabsStore.iframeList.find(i => i.id === id)
  if (item) {
    item.loading = true
    item.hasError = false
    item.refreshKey = Date.now()
    startTimeoutChecker(id)
  }
}

const exitFullscreen = () => {
  appStore.toggleIframeFullscreen(false)
}

// 键盘 ESC 退出 iframe 全屏
const handleKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && isFullscreen.value) {
    exitFullscreen()
  }
}

// 当宿主窗口失焦（用户点击了 iframe 内部）时，派发全局 click 事件收起可能悬浮的菜单
const handleWindowBlur = () => {
  window.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  window.addEventListener('blur', handleWindowBlur)
  // 为初始的 iframe 启动超时监控
  iframeList.value.forEach(item => {
    if (item.loading) {
      startTimeoutChecker(item.id)
    }
  })
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('blur', handleWindowBlur)
  timeoutMap.forEach(timer => window.clearTimeout(timer))
  timeoutMap.clear()
})
</script>

<style scoped>
.iframe-container {
  min-height: 100%;
}
.app-iframe {
  width: 100%;
  height: 100%;
}
</style>
