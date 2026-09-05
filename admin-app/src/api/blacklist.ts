import api from '@/lib/axios'
import type { BlacklistItem, BlacklistPageData, AddBlacklistRequest } from '@/types'

export interface BlacklistQuery {
  page?: number
  pageSize?: number
  source?: string
  keyword?: string
}

/** 分页查询黑名单（page 缺省时后端回退旧版全量列表） */
export async function getBlacklistPage(query: BlacklistQuery): Promise<BlacklistPageData> {
  const response = await api.get<BlacklistPageData>('/admin/blacklist', {
    params: {
      page: query.page ?? 1,
      page_size: query.pageSize ?? 20,
      source: query.source && query.source !== 'all' ? query.source : undefined,
      q: query.keyword || undefined,
    },
  })
  return response.data
}

/** 旧版全量列表（兼容用途） */
export async function getBlacklist(): Promise<BlacklistItem[]> {
  const response = await api.get<BlacklistItem[]>('/admin/blacklist')
  return response.data
}

export async function addBlacklist(data: AddBlacklistRequest): Promise<void> {
  await api.post('/admin/blacklist', data)
}

export async function removeBlacklist(ip: string): Promise<void> {
  await api.delete('/admin/blacklist', {
    params: { ip },
  })
}
