// 本节点身份：由后端 /api/v2/node/info 下发（子节点名称与父节点地址）。
// 模块加载即拉取；失败沿用 localStorage 缓存（节点角色变更几乎不会发生）。
import { ref } from 'vue'
import { getNodeInfo } from '@/services/api'

const nodeName = ref(localStorage.getItem('mirror_node_name') || '')
const parentNodeUrl = ref(localStorage.getItem('mirror_parent_url') || '')
let started = false

export async function loadNodeInfo() {
  if (started) return
  started = true
  try {
    const response = await getNodeInfo()
    nodeName.value = response.data?.node_name || ''
    parentNodeUrl.value = response.data?.parent_node_url || ''
    localStorage.setItem('mirror_node_name', nodeName.value)
    localStorage.setItem('mirror_parent_url', parentNodeUrl.value)
  } catch {
    // 拉取失败沿用缓存，不打断页面
  }
}

export { nodeName, parentNodeUrl }

export function useNodeInfo() {
  return { nodeName, parentNodeUrl }
}
