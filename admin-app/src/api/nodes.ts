import api from '@/lib/axios'

/** 子节点路由（主服聚合视角） */
export interface MirrorNode {
  name: string
  url: string
}

/** 子节点运行状态（轮询缓存） */
export interface NodeStatus {
  name: string
  url: string
  online: boolean
  error?: string
  version?: string
  node_name?: string
  uptime_seconds: number
  active_downloads: number
  current_bandwidth_mbps: number
  bandwidth_limit_mbps: number
  total_downloads: number
  disk?: { total: number; free: number; used?: number } | null
  launchers: { name: string; versions: number; latest?: string }[]
  last_poll: string
}

/** 子节点清单 + 最近轮询缓存 */
export async function getNodes(): Promise<{ nodes: MirrorNode[]; status: NodeStatus[] }> {
  const response = await api.get('/admin/nodes')
  return response.data
}

/** 保存子节点清单（服务端做 URL 校验/去重清洗并持久化） */
export async function saveNodes(nodes: MirrorNode[]): Promise<void> {
  await api.post('/admin/nodes', { nodes })
}

/** 立即重新轮询全部子节点 */
export async function refreshNodes(): Promise<NodeStatus[]> {
  const response = await api.post('/admin/nodes/refresh')
  return response.data.status
}
