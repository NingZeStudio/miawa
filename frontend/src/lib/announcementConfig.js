import { globalConfig } from '@/lib/globalConfig'

export const announcementConfig = {
  id: '20260527_mirror_strategy',
  title: '镜像策略调整通知',
  content:
    '由于服务器存储空间有限，本站镜像策略由「保留全部历史版本」调整为「仅保留各启动器最近三个版本」。更早的版本将随清理逐步下架，如有需要请提前备份；确有特殊版本需求，欢迎联系我们协助获取。\n\n感谢您的理解与支持。',
  importantText:
    'AnyLyraLabs 官方 QQ 群已开放，诚邀乐于助人的朋友加入——帮萌新分析崩溃日志、解答日常疑问，都非常欢迎。',
  links: [
    {
      label: '加入 QQ 群',
      url: 'https://qm.qq.com/q/WMXCSUhU4O'
    },
    {
      label: '查看文件',
      url: '/files'
    },
    {
      label: '数据统计',
      url: '/stats'
    }
  ]
}

const KEYS = {
  shown: globalConfig.storage.keys.announcementShown,
  lastId: globalConfig.storage.keys.lastAnnouncementId
}

export function hasSeenAnnouncement() {
  return (
    localStorage.getItem(KEYS.lastId) === announcementConfig.id &&
    localStorage.getItem(KEYS.shown) === 'true'
  )
}

export function markAnnouncementAsSeen() {
  localStorage.setItem(KEYS.shown, 'true')
  localStorage.setItem(KEYS.lastId, announcementConfig.id)
}

export function resetAnnouncement() {
  localStorage.removeItem(KEYS.shown)
  localStorage.removeItem(KEYS.lastId)
}
