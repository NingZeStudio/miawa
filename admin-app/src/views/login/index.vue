<template>
  <div class="min-h-screen flex items-center justify-center bg-zinc-50 dark:bg-zinc-950 p-4 select-none">
    <div class="w-full max-w-sm">
      <!-- 品牌标识 -->
      <div class="text-center mb-8">
        <div class="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-zinc-900 dark:bg-zinc-100 text-zinc-100 dark:text-zinc-900 shadow-sm mb-4">
          <component
            :is="LayersIcon"
            class="w-6 h-6"
          />
        </div>
        <h1 class="text-xl font-bold tracking-tight text-zinc-900 dark:text-zinc-100">
          Lemwood Mirror
        </h1>
        <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
          管理控制台身份认证
        </p>
      </div>

      <!-- 登录卡片 -->
      <div class="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-xl p-6 shadow-sm">
        <el-form
          ref="formRef"
          :model="loginForm"
          :rules="rules"
          label-position="top"
          @keyup.enter="handleLogin"
        >
          <el-form-item
            label="管理员账号"
            prop="username"
          >
            <el-input
              v-model="loginForm.username"
              placeholder="请输入管理员用户名"
              size="large"
              clearable
            >
              <template #prefix>
                <component
                  :is="UserIcon"
                  class="w-4 h-4 text-zinc-400"
                />
              </template>
            </el-input>
          </el-form-item>

          <el-form-item
            label="登录密码"
            prop="password"
          >
            <el-input
              v-model="loginForm.password"
              type="password"
              placeholder="请输入登录密码"
              size="large"
              show-password
            >
              <template #prefix>
                <component
                  :is="LockIcon"
                  class="w-4 h-4 text-zinc-400"
                />
              </template>
            </el-input>
          </el-form-item>

          <el-form-item
            v-if="totpEnabled"
            label="二次验证码 (TOTP)"
            prop="otp_code"
          >
            <el-input
              v-model="loginForm.otp_code"
              placeholder="请输入 6 位动态验证码"
              size="large"
              maxlength="6"
              clearable
            >
              <template #prefix>
                <component
                  :is="ShieldCheckIcon"
                  class="w-4 h-4 text-zinc-400"
                />
              </template>
            </el-input>
          </el-form-item>

          <el-button
            type="primary"
            class="w-full mt-2 !h-10 !text-sm !font-medium"
            :loading="loading"
            @click="handleLogin"
          >
            立即登录
          </el-button>
        </el-form>
      </div>

      <!-- 底部版权与前台链接 -->
      <div class="text-center mt-6 text-xs text-zinc-400 space-x-3">
        <span>Lemwood Mirror</span>
        <span>•</span>
        <a
          href="/"
          target="_blank"
          class="hover:text-zinc-600 dark:hover:text-zinc-200 transition-colors"
        >
          前往服务首页
        </a>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { useUserStore } from '@/store/modules/user'
import { login, getTOTPStatus } from '@/api/auth'
import {
  Layers,
  User,
  Lock,
  ShieldCheck
} from 'lucide-vue-next'

const LayersIcon = Layers
const UserIcon = User
const LockIcon = Lock
const ShieldCheckIcon = ShieldCheck

const router = useRouter()
const userStore = useUserStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const totpEnabled = ref(false)

const loginForm = ref({
  username: '',
  password: '',
  otp_code: ''
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入管理员账号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
  otp_code: [
    { required: true, message: '请输入 6 位动态验证码', trigger: 'blur' },
    { pattern: /^\d{6}$/, message: '验证码必须为 6 位数字', trigger: 'blur' }
  ]
}

onMounted(async () => {
  if (userStore.isAuthenticated) {
    router.replace('/dashboard')
    return
  }

  try {
    const status = await getTOTPStatus()
    totpEnabled.value = status.enabled
  } catch {
    // 忽略未登录时的 TOTP 状态获取失败
  }
})

const handleLogin = async () => {
  if (!formRef.value) return

  await formRef.value.validate(async (valid) => {
    if (!valid) return

    loading.value = true
    try {
      const res = await login({
        username: loginForm.value.username,
        password: loginForm.value.password,
        otp_code: totpEnabled.value ? loginForm.value.otp_code : undefined
      })

      userStore.setToken(res.token)
      userStore.updateUserInfo({
        username: loginForm.value.username,
        displayName: loginForm.value.username
      })

      ElMessage.success('登录成功')
      router.push('/dashboard')
    } catch (error: any) {
      const msg = error.response?.data?.error?.message || error.message || '登录失败，请检查账号密码或验证码'
      ElMessage.error(msg)
    } finally {
      loading.value = false
    }
  })
}
</script>
