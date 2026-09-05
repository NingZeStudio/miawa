import api from '@/lib/axios'
import type { FirewallStatus } from '@/types'

/** 防火墙运行状态（频率限制设置 + 白名单/网段封禁/活跃窗口计数） */
export async function getFirewallStatus(): Promise<FirewallStatus> {
  const response = await api.get<FirewallStatus>('/admin/firewall/status')
  return response.data
}
