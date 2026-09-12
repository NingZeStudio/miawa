import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { TabItem, IframeInstanceState } from '@/types'
import type { Router } from 'vue-router'

export const useTabsStore = defineStore('tabs', () => {
  let routerInstance: Router | null = null

  // 默认固定的首页 Tab
  const defaultTab: TabItem = {
    title: '工作台',
    path: '/dashboard',
    name: 'Dashboard',
    closable: false,
    isIframe: false,
    icon: 'LayoutDashboard'
  }

  const tabs = ref<TabItem[]>([defaultTab])
  const activeTabPath = ref<string>('/dashboard')

  // 保活的 iframe 实例池
  const iframeList = ref<IframeInstanceState[]>([])

  const isCurrentTabIframe = computed(() => {
    const current = tabs.value.find(t => t.path === activeTabPath.value)
    return !!current?.isIframe
  })

  const currentIframeInstance = computed(() => {
    return iframeList.value.find(item => item.id === activeTabPath.value)
  })

  const setRouter = (router: Router) => {
    routerInstance = router
  }

  /**
   * 添加或激活标签页
   */
  const addTab = (tab: TabItem) => {
    activeTabPath.value = tab.path

    const exists = tabs.value.some(t => t.path === tab.path)
    if (!exists) {
      tabs.value.push({
        ...tab,
        closable: tab.closable ?? true
      })
    }

    // 若为 iframe 路由，登记到常驻保活池
    if (tab.isIframe && tab.iframeUrl) {
      const existingIframe = iframeList.value.find(item => item.id === tab.path)
      if (!existingIframe) {
        iframeList.value.push({
          id: tab.path,
          title: tab.title,
          url: tab.iframeUrl,
          loaded: false,
          loading: true,
          hasError: false,
          refreshKey: Date.now()
        })
      }
    }
  }

  /**
   * 关闭单个标签
   */
  const closeTab = (targetPath: string) => {
    const index = tabs.value.findIndex(t => t.path === targetPath)
    if (index === -1) return

    const isCurrent = activeTabPath.value === targetPath

    // 若是 iframe，从保活池中销毁释放 DOM
    const iframeIdx = iframeList.value.findIndex(item => item.id === targetPath)
    if (iframeIdx > -1) {
      iframeList.value.splice(iframeIdx, 1)
    }

    // 移除 Tab
    tabs.value.splice(index, 1)

    // 如果关闭的是当前激活标签，则向前或向后激活
    if (isCurrent) {
      const nextTab = tabs.value[index] || tabs.value[index - 1] || defaultTab
      activeTabPath.value = nextTab.path
      if (routerInstance) {
        routerInstance.push(nextTab.path)
      }
    }
  }

  /**
   * 关闭其他标签
   */
  const closeOtherTabs = (currentPath: string) => {
    // 保留不可关闭的标签以及当前激活的标签
    tabs.value = tabs.value.filter(t => !t.closable || t.path === currentPath)
    // 销毁多余的 iframe
    iframeList.value = iframeList.value.filter(item => {
      return tabs.value.some(t => t.path === item.id)
    })
    activeTabPath.value = currentPath
    if (routerInstance) {
      routerInstance.push(currentPath)
    }
  }

  /**
   * 关闭所有可关闭的标签
   */
  const closeAllTabs = () => {
    tabs.value = tabs.value.filter(t => !t.closable)
    // 销毁所有非常驻 iframe
    iframeList.value = iframeList.value.filter(item => {
      return tabs.value.some(t => t.path === item.id)
    })
    const target = tabs.value[0] || defaultTab
    activeTabPath.value = target.path
    if (routerInstance) {
      routerInstance.push(target.path)
    }
  }

  /**
   * 刷新当前活跃的 iframe
   */
  const refreshCurrentIframe = () => {
    const current = currentIframeInstance.value
    if (current) {
      current.loading = true
      current.hasError = false
      current.refreshKey = Date.now()
    }
  }

  /**
   * 标记某个 iframe 加载完成
   */
  const setIframeLoaded = (id: string) => {
    const item = iframeList.value.find(i => i.id === id)
    if (item) {
      item.loaded = true
      item.loading = false
      item.hasError = false
    }
  }

  /**
   * 标记某个 iframe 加载失败
   */
  const setIframeError = (id: string, errorMsg?: string) => {
    const item = iframeList.value.find(i => i.id === id)
    if (item) {
      item.loading = false
      item.hasError = true
      item.errorMessage = errorMsg || '页面加载超时或目标拒绝嵌入'
    }
  }

  /**
   * 动态更新 Tab 标题
   */
  const updateTabTitle = (path: string, newTitle: string) => {
    const tab = tabs.value.find(t => t.path === path)
    if (tab) {
      tab.title = newTitle
    }
    const iframe = iframeList.value.find(i => i.id === path)
    if (iframe) {
      iframe.title = newTitle
    }
  }

  return {
    tabs,
    activeTabPath,
    iframeList,
    isCurrentTabIframe,
    currentIframeInstance,
    setRouter,
    addTab,
    closeTab,
    closeOtherTabs,
    closeAllTabs,
    refreshCurrentIframe,
    setIframeLoaded,
    setIframeError,
    updateTabTitle
  }
})
