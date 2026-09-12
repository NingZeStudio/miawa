import api from '@/lib/axios'
import { removeStoredItem } from '@/lib/storage'
import type { LoginRequest, LoginResponse, TOTPStatus } from '@/types'

export async function login(data: LoginRequest): Promise<LoginResponse> {
  const response = await api.post<LoginResponse>('/auth/login', data)
  return response.data
}

export async function getTOTPStatus(): Promise<TOTPStatus> {
  const response = await api.get<TOTPStatus>('/auth/2fa/status')
  return response.data
}

export function logout(): void {
  removeStoredItem('admin_token')
}
