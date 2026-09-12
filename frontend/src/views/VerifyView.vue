<script setup>
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  PhCheckCircle as CheckCircle,
  PhHeart as Heart,
  PhCircleNotch as Loader2,
  PhArrowClockwise as RefreshCw,
  PhShieldCheck as ShieldCheck,
  PhUsers as Users,
  PhXCircle as XCircle
} from '@phosphor-icons/vue'
import { getPowConfig, createDownloadChallenge, authorizeDownload } from '@/services/api'
import { base64urlDecode, base64urlEncode, leadingZeroBits } from '@/lib/pow'
import { globalConfig } from '@/lib/globalConfig'
import Button from '@/components/ui/Button.vue'
import Card from '@/components/ui/Card.vue'
import CardContent from '@/components/ui/CardContent.vue'
import CardDescription from '@/components/ui/CardDescription.vue'
import CardFooter from '@/components/ui/CardFooter.vue'
import CardHeader from '@/components/ui/CardHeader.vue'
import CardTitle from '@/components/ui/CardTitle.vue'

const route = useRoute()
const router = useRouter()

const filePath = ref('')
const isLoading = ref(true)
const progress = ref(0)
const statusText = ref('正在获取验证挑战…')
const errorMessage = ref('')
const verifyStatus = ref('pending') // pending | error

let cancelled = false

// ---- PBKDF2 求解（编解码与零位统计见 lib/pow.js，与后端 internal/pow 协议一致） ----

// 迭代 counter，直到 PBKDF2 派生密钥的前导零位数满足难度要求。
const solve = async (params, onProgress) => {
  const salt = base64urlDecode(params.salt)
  const total = Math.min(Math.pow(2, params.difficulty + 4), 4000000)
  for (let counter = 0; counter < total; counter++) {
    if (cancelled) return null
    const pw = new TextEncoder().encode(String(counter))
    const km = await crypto.subtle.importKey('raw', pw, { name: 'PBKDF2' }, false, ['deriveBits'])
    const dk = new Uint8Array(
      await crypto.subtle.deriveBits(
        { name: 'PBKDF2', salt: salt, iterations: params.cost, hash: 'SHA-256' },
        km,
        params.keyLength * 8
      )
    )
    if (leadingZeroBits(dk) >= params.difficulty) {
      return { counter: counter, derivedKey: base64urlEncode(dk) }
    }
    if (counter % 64 === 0) {
      onProgress(Math.min(99, (counter / total) * 100))
      await new Promise((r) => setTimeout(r, 0))
    }
  }
  return null
}

// ---- 能力检测：Web Crypto 仅在 HTTPS 安全上下文可用，老内核可能缺失 subtle ----

const isPowSupported = () => {
  try {
    return (
      window.isSecureContext === true &&
      typeof window.crypto !== 'undefined' &&
      !!window.crypto.subtle &&
      typeof TextEncoder === 'function' &&
      typeof atob === 'function' &&
      typeof btoa === 'function'
    )
  } catch {
    return false
  }
}

// ---- 主流程 ----

const init = async () => {
  cancelled = false
  isLoading.value = true
  progress.value = 0
  errorMessage.value = ''
  verifyStatus.value = 'pending'
  filePath.value = route.query.file || ''

  if (!filePath.value) {
    errorMessage.value = '缺少文件参数，请从来源页面重新发起下载'
    verifyStatus.value = 'error'
    isLoading.value = false
    return
  }

  if (!isPowSupported()) {
    errorMessage.value =
      '当前浏览器不支持 Web Crypto（需要较新的内核与 HTTPS 环境），无法自动完成验证。可升级浏览器，或使用下方备用下载入口。'
    verifyStatus.value = 'error'
    isLoading.value = false
    return
  }

  try {
    // 1. 查询 PoW 配置；未启用则直接走原始下载路径
    const powRes = await getPowConfig()
    if (!powRes.data || !powRes.data.enabled) {
      window.location.href = `/download/${filePath.value}`
      return
    }

    // 2. 创建挑战
    statusText.value = '正在获取验证挑战…'
    const chRes = await createDownloadChallenge(filePath.value)
    const challenge = chRes.data
    if (!challenge || !challenge.parameters) {
      throw new Error('获取挑战失败')
    }

    // 3. 浏览器求解 PoW
    statusText.value = '正在后台完成计算，请稍候…'
    const solution = await solve(challenge.parameters, (pct) => {
      progress.value = pct
    })
    if (!solution) {
      throw new Error('未能在限定迭代内求出解，请刷新重试')
    }

    // 4. 提交授权
    statusText.value = '正在领取下载凭证…'
    const authRes = await authorizeDownload(challenge, solution)
    const token = authRes.data.download_token
    if (token) {
      // 显示“验证成功”，短暂停留再跳转，避免用户误以为验证未完成就出现下载页
      verifyStatus.value = 'success'
      statusText.value = '验证通过，即将开始下载…'
      progress.value = 100
      await new Promise((r) => setTimeout(r, 500))
      const returnUrl = route.query.return_url
      router.push(`/download-started?token=${token}${returnUrl ? `&return_url=${encodeURIComponent(returnUrl)}` : ''}`)
      return
    }
    throw new Error('授权失败')
  } catch (error) {
    console.error('PoW verify error:', error)
    errorMessage.value = error.response?.data?.error?.message || error.message || '验证失败，请重试'
    verifyStatus.value = 'error'
    isLoading.value = false
  }
}

const retry = () => {
  init()
}

const directDownload = () => {
  if (filePath.value) {
    window.location.href = `/download/${filePath.value}`
  }
}

watch(
  () => route.query.file,
  (newFile, oldFile) => {
    if (newFile && newFile !== oldFile) {
      init()
    }
  }
)

onMounted(() => {
  init()
})

onUnmounted(() => {
  cancelled = true
})
</script>

<template>
  <div class="flex min-h-[calc(100vh-10rem)] flex-col items-center justify-center gap-4 py-8 supports-[height:100dvh]:min-h-[calc(100dvh-10rem)]">
    <Card class="w-full max-w-lg">
      <CardHeader class="items-center text-center">
        <div class="mb-2 rounded-full bg-primary/10 p-3 text-primary">
          <ShieldCheck weight="duotone" class="h-8 w-8" />
        </div>
        <CardTitle class="text-2xl">安全验证</CardTitle>
        <CardDescription>正在确认你是真实访客，无需任何操作</CardDescription>
      </CardHeader>

      <CardContent class="space-y-6">
        <div
          v-if="isLoading && verifyStatus !== 'error'"
          class="flex flex-col items-center justify-center gap-4 rounded-lg border border-dashed px-6 py-12 text-muted-foreground"
        >
          <Loader2 weight="duotone" class="h-8 w-8 animate-spin text-primary" />
          <span class="text-sm">{{ statusText }}</span>
          <div class="h-2 w-full max-w-xs overflow-hidden rounded-full bg-muted">
            <div
              class="h-full rounded-full bg-primary transition-[width] duration-200"
              :style="{ width: progress + '%' }"
            ></div>
          </div>
          <span v-if="progress > 0" class="text-xs">{{ progress.toFixed(0) }}%</span>
        </div>

        <div
          v-else-if="verifyStatus === 'success'"
          class="flex flex-col items-center justify-center gap-4 rounded-lg border border-emerald-500/20 bg-emerald-500/10 px-6 py-12 text-muted-foreground"
        >
          <CheckCircle weight="duotone" class="h-10 w-10 text-emerald-500" />
          <span class="font-medium text-foreground">验证成功</span>
          <span class="text-sm">{{ statusText }}</span>
        </div>

        <div v-else-if="verifyStatus === 'error'" class="space-y-5">
          <div class="rounded-lg border border-destructive/20 bg-destructive/10 p-4 text-center">
            <XCircle weight="duotone" class="mx-auto mb-3 h-12 w-12 text-destructive" />
            <p class="font-medium text-foreground">验证失败</p>
            <p class="mt-1 text-sm text-muted-foreground">{{ errorMessage }}</p>
          </div>
          <Button class="w-full" @click="retry">
            <RefreshCw weight="duotone" class="mr-2 h-4 w-4" />
            重新验证
          </Button>
          <Button v-if="filePath" variant="outline" class="w-full" @click="directDownload">
            验证遇到问题？直接下载
          </Button>
        </div>
      </CardContent>

      <CardFooter v-if="filePath" class="border-t text-xs text-muted-foreground">
        <span class="break-all">目标文件：{{ filePath.split('/').pop() }}</span>
      </CardFooter>
    </Card>

    <!-- 赞助请求：收款码在关于页 -->
    <Card class="w-full max-w-lg">
      <CardContent class="flex items-center justify-between gap-3 p-5">
        <div class="min-w-0">
          <p class="text-sm font-semibold text-foreground">喜欢本站？请考虑赞助支持</p>
          <p class="mt-0.5 text-xs leading-relaxed text-muted-foreground">
            您的支持会用于服务器、带宽与镜像存储等基础设施支出，收款码在关于页
          </p>
        </div>
        <RouterLink
          to="/about"
          class="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:opacity-90"
        >
          <Heart weight="duotone" class="h-4 w-4" />
          查看收款码
        </RouterLink>
      </CardContent>
    </Card>

    <!-- 官方用户群 -->
    <a
      :href="globalConfig.links.qqGroup"
      target="_blank"
      rel="noopener noreferrer"
      class="inline-flex items-center gap-2 rounded-lg border px-5 py-2.5 text-sm font-medium text-foreground hover:bg-muted/60"
    >
      <Users weight="duotone" class="h-4 w-4 text-primary" />
      进入官方用户群
      <span class="text-xs font-normal text-muted-foreground">柠泽资源站用户群</span>
    </a>
  </div>
</template>
