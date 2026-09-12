/**
 * ElementsPlus Admin - IframeBridge 客户端 SDK
 * 用于子页面/嵌入系统与宿主管理后台进行安全双向交互
 */

export interface MessagePayload {
  type: 'success' | 'warning' | 'info' | 'error'
  message: string
  duration?: number
}

export interface NotificationPayload {
  title: string
  message: string
  type?: 'success' | 'warning' | 'info' | 'error'
  duration?: number
}

export class IframeBridgeClient {
  private targetOrigin: string
  private listeners: Map<string, Array<(payload: any) => void>> = new Map()

  constructor(targetOrigin = '*') {
    this.targetOrigin = targetOrigin
    this.initListener()
  }

  private initListener() {
    window.addEventListener('message', (event) => {
      const data = event.data
      if (!data || typeof data !== 'object' || !data.type) return

      const handlers = this.listeners.get(data.type)
      if (handlers) {
        handlers.forEach(fn => fn(data.payload))
      }
    })

    if (typeof document !== 'undefined') {
      document.addEventListener('click', () => {
        this.send('SUB_APP_CLICK')
      })
    }
  }

  /**
   * 获取宿主当前 Token 及上下文环境
   */
  public getContext(callback: (context: { token: string; isDark: boolean; systemTime: number }) => void) {
    const unbind = this.on('RESPONSE_CONTEXT', (payload) => {
      unbind()
      callback(payload)
    })
    this.send('GET_CONTEXT')
  }

  /**
   * 向宿主发送底层消息
   */
  public send(type: string, payload?: any) {
    if (window.parent && window.parent !== window) {
      window.parent.postMessage(
        {
          type,
          payload,
          source: 'sub-app',
          timestamp: Date.now()
        },
        this.targetOrigin
      )
    } else {
      console.warn('[IframeBridgeClient] 当前不在 iframe 环境中运行')
    }
  }

  /**
   * 监听来自宿主的消息
   */
  public on(type: string, callback: (payload: any) => void) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, [])
    }
    this.listeners.get(type)!.push(callback)
    return () => {
      const list = this.listeners.get(type) || []
      const index = list.indexOf(callback)
      if (index > -1) list.splice(index, 1)
    }
  }

  /**
   * 调用宿主的 Element Plus Message 提示
   */
  public showMessage(message: string, type: 'success' | 'warning' | 'info' | 'error' = 'info', duration = 3000) {
    this.send('SHOW_MESSAGE', { message, type, duration })
  }

  /**
   * 调用宿主的 Element Plus Notification 通知
   */
  public showNotification(title: string, message: string, type: 'success' | 'warning' | 'info' | 'error' = 'info', duration = 4500) {
    this.send('SHOW_NOTIFICATION', { title, message, type, duration })
  }

  /**
   * 修改当前 Tab 标题
   */
  public setTabTitle(title: string, path?: string) {
    this.send('SET_TAB_TITLE', { title, path })
  }

  /**
   * 命令宿主路由跳转
   */
  public navigate(path: string) {
    this.send('NAVIGATE', { path })
  }

  /**
   * 命令宿主打开一个新标签页
   */
  public openTab(title: string, path: string, iframeUrl?: string) {
    this.send('OPEN_TAB', { title, path, iframeUrl })
  }

  /**
   * 请求关闭当前标签页
   */
  public closeCurrentTab(path?: string) {
    this.send('CLOSE_CURRENT_TAB', { path })
  }

  /**
   * 请求刷新当前 iframe
   */
  public refreshSelf() {
    this.send('REFRESH_CURRENT_IFRAME')
  }

  /**
   * 监听暗黑模式与主题变更
   */
  public onThemeChange(callback: (data: { isDark: boolean }) => void) {
    return this.on('THEME_CHANGED', callback)
  }
}

export const bridgeClient = new IframeBridgeClient()
