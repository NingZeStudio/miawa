import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { UserInfo } from '@/types'
import { getStoredItem, setStoredItem, removeStoredItem } from '@/lib/storage'

export const useUserStore = defineStore('user', () => {
  const token = ref<string | null>(getStoredItem('admin_token'))

  const userInfo = ref<UserInfo>({
    username: 'admin',
    displayName: '系统管理员',
    role: '超级管理员',
    avatar: 'https://images.unsplash.com/photo-1534528741775-53994a69daeb?w=120&auto=format&fit=crop&q=80',
    token: token.value || ''
  })

  const isAuthenticated = computed(() => Boolean(token.value))

  const setToken = (newToken: string) => {
    token.value = newToken
    userInfo.value.token = newToken
    setStoredItem('admin_token', newToken)
  }

  const logout = () => {
    token.value = null
    userInfo.value.token = ''
    removeStoredItem('admin_token')
  }

  const updateUserInfo = (info: Partial<UserInfo>) => {
    userInfo.value = { ...userInfo.value, ...info }
  }

  return {
    token,
    userInfo,
    isAuthenticated,
    setToken,
    logout,
    updateUserInfo
  }
})
