import { defineStore } from 'pinia'
import { ref } from 'vue'
import { bridgeHost } from '@/core/bridge-host'

export const useAppStore = defineStore('app', () => {
  const sidebarCollapsed = ref<boolean>(false)
  const isDark = ref<boolean>(
    localStorage.getItem('theme-mode') === 'dark' ||
    window.matchMedia('(prefers-color-scheme: dark)').matches
  )
  const isIframeFullscreen = ref<boolean>(false)

  // 响应并同步系统暗黑模式
  const applyTheme = (dark: boolean) => {
    isDark.value = dark
    if (dark) {
      document.documentElement.classList.add('dark')
      localStorage.setItem('theme-mode', 'dark')
    } else {
      document.documentElement.classList.remove('dark')
      localStorage.setItem('theme-mode', 'light')
    }

    // 广播通知所有已挂载的 iframe
    bridgeHost.broadcastToIframes({
      type: 'THEME_CHANGED',
      payload: { isDark: dark }
    })
  }

  const toggleTheme = () => {
    applyTheme(!isDark.value)
  }

  const toggleSidebar = () => {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  const toggleIframeFullscreen = (force?: boolean) => {
    if (typeof force === 'boolean') {
      isIframeFullscreen.value = force
    } else {
      isIframeFullscreen.value = !isIframeFullscreen.value
    }
  }

  const watermarkEnabled = ref<boolean>(true)

  const toggleWatermark = () => {
    watermarkEnabled.value = !watermarkEnabled.value
  }

  // 初始化应用主题
  applyTheme(isDark.value)

  return {
    sidebarCollapsed,
    isDark,
    isIframeFullscreen,
    watermarkEnabled,
    toggleTheme,
    applyTheme,
    toggleSidebar,
    toggleIframeFullscreen,
    toggleWatermark
  }
})
