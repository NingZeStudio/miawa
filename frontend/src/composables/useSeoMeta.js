// SEO meta 统一写入：title + description + og/twitter 标签。
// 各视图只需声明 { title, description }，不再各自手写 DOM 查询；
// fullTitle: true 表示 title 已含站名后缀，跳过拼接。
// 子节点部署时标题自动追加「 · <节点名> 子节点」（nodeInfo 启动时拉取）。
import { nodeName, parentNodeUrl } from '@/lib/nodeInfo'

const ensureMeta = (attr, key) => {
  const selector = `meta[${attr}="${key}"]`
  let el = document.querySelector(selector)
  if (!el) {
    el = document.createElement('meta')
    el.setAttribute(attr, key)
    document.head.appendChild(el)
  }
  return el
}

export function useSeoMeta({ title, description, image, fullTitle = false }, nameFull) {
  return () => {
    const nodeSuffix = parentNodeUrl.value && nodeName.value ? ` · ${nodeName.value} 子节点` : ''
    const finalTitle = fullTitle || !nameFull ? title + nodeSuffix : `${title} - ${nameFull}${nodeSuffix}`
    document.title = finalTitle

    ensureMeta('name', 'qq:share:title').setAttribute('content', finalTitle)

    if (description) {
      ensureMeta('name', 'description').setAttribute('content', description)
      ensureMeta('name', 'qq:share:description').setAttribute('content', description)
      ensureMeta('property', 'og:title').setAttribute('content', finalTitle)
      ensureMeta('property', 'og:description').setAttribute('content', description)
      ensureMeta('property', 'twitter:title').setAttribute('content', finalTitle)
      ensureMeta('property', 'twitter:description').setAttribute('content', description)
    }

    if (image) {
      ensureMeta('name', 'qq:share:image').setAttribute('content', image)
      ensureMeta('property', 'og:image').setAttribute('content', image)
      ensureMeta('property', 'twitter:image').setAttribute('content', image)
    }
  }
}
