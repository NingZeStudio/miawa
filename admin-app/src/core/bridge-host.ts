import { ElMessage, ElNotification } from 'element-plus'
import type { BridgeMessage } from '@/types'
import type { Router } from 'vue-router'
import type { useTabsStore } from '@/store/modules/tabs'

export interface BridgeHostOptions {
  allowedOrigins?: string[]
  router?: Router
  tabsStore?: ReturnType<typeof useTabsStore>
  onThemeChangeRequested?: (isDark: boolean) => void
}

export class BridgeHost {
  private allowedOrigins: string[] = ['*']
  private router?: Router
  private tabsStore?: ReturnType<typeof useTabsStore>
  private isDestroyed = false

  constructor(options: BridgeHostOptions = {}) {
    if (options.allowedOrigins && options.allowedOrigins.length > 0) {
      this.allowedOrigins = options.allowedOrigins
    }
    this.router = options.router
    this.tabsStore = options.tabsStore
    this.initListener()
  }

  public setRouter(router: Router) {
    this.router = router
  }

  public setTabsStore(tabsStore: ReturnType<typeof useTabsStore>) {
    this.tabsStore = tabsStore
  }

  private isOriginAllowed(origin: string): boolean {
    if (this.allowedOrigins.includes('*')) return true
    return this.allowedOrigins.includes(origin)
  }

  private initListener() {
    window.addEventListener('message', this.handleMessage)
  }

  private handleMessage = (event: MessageEvent) => {
    if (this.isDestroyed) return
    const data = event.data as BridgeMessage

    // 过滤非内部约定的结构
    if (!data || typeof data !== 'object' || !data.type) {
      return
    }

    if (!this.isOriginAllowed(event.origin)) {
      console.warn(`[IframeBridge] 收到来自未授权源的消息: ${event.origin}`, data)
      return
    }

    this.dispatchAction(data, event)
  }

  private dispatchAction(message: BridgeMessage, event: MessageEvent) {
    const { type, payload } = message

    switch (type) {
      case 'BRIDGE_PING':
        this.reply(event.source as Window, {
          type: 'BRIDGE_PONG',
          payload: { status: 'ok', serverTime: Date.now() }
        })
        break

      case 'SHOW_MESSAGE':
        if (payload && payload.message) {
          ElMessage({
            type: payload.type || 'info',
            message: payload.message,
            duration: payload.duration ?? 3000
          })
        }
        break

      case 'SHOW_NOTIFICATION':
        if (payload && payload.title) {
          ElNotification({
            title: payload.title,
            message: payload.message,
            type: payload.type || 'info',
            duration: payload.duration ?? 4500
          })
        }
        break

      case 'SET_TAB_TITLE':
        if (payload && payload.title && this.tabsStore) {
          const path = payload.path || this.tabsStore.activeTabPath
          this.tabsStore.updateTabTitle(path, payload.title)
        }
        break

      case 'NAVIGATE':
        if (payload && payload.path && this.router) {
          this.router.push(payload.path)
        }
        break

      case 'OPEN_TAB':
        if (payload && payload.path && this.tabsStore) {
          this.tabsStore.addTab({
            title: payload.title || '新标签页',
            path: payload.path,
            iframeUrl: payload.iframeUrl,
            isIframe: !!payload.iframeUrl,
            closable: true
          })
          if (this.router) {
            this.router.push(payload.path)
          }
        }
        break

      case 'CLOSE_CURRENT_TAB':
        if (this.tabsStore) {
          const path = payload?.path || this.tabsStore.activeTabPath
          this.tabsStore.closeTab(path)
        }
        break

      case 'REFRESH_CURRENT_IFRAME':
        if (this.tabsStore) {
          this.tabsStore.refreshCurrentIframe()
        }
        break

      case 'TOGGLE_FULLSCREEN':
        if (payload && typeof payload.fullscreen === 'boolean') {
          // 可由父级处理全屏切换
        }
        break

      case 'GET_CONTEXT': {
        const token = localStorage.getItem('access_token') || 'demo_zinc_token_2026'
        const isDark = document.documentElement.classList.contains('dark')
        this.reply(event.source as Window, {
          type: 'RESPONSE_CONTEXT',
          payload: {
            token,
            isDark,
            systemTime: Date.now()
          }
        })
        break
      }

      case 'SUB_APP_CLICK':
        // 子应用点击事件同步到宿主，触发宿主下拉菜单与右键菜单关闭
        window.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
        break

      default:
        // 可扩充自定义事件
        break
    }
  }

  public reply(targetWindow: Window | null, message: BridgeMessage, targetOrigin = '*') {
    if (!targetWindow) return
    targetWindow.postMessage(
      {
        ...message,
        source: 'host',
        timestamp: Date.now()
      },
      targetOrigin
    )
  }

  public broadcastToIframes(message: BridgeMessage) {
    const iframes = document.querySelectorAll<HTMLIFrameElement>('iframe.app-iframe')
    iframes.forEach((iframe) => {
      try {
        iframe.contentWindow?.postMessage(
          {
            ...message,
            source: 'host',
            timestamp: Date.now()
          },
          '*'
        )
      } catch (err) {
        console.error('[IframeBridge] 广播消息失败', err)
      }
    })
  }

  public destroy() {
    this.isDestroyed = true
    window.removeEventListener('message', this.handleMessage)
  }
}

export const bridgeHost = new BridgeHost()
