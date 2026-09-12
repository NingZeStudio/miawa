<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getDownloadLanding } from '@/services/api'
import { globalConfig } from '@/lib/globalConfig'
import { isAllowedExternalTarget, isSameOriginUrl } from '@/lib/returnTarget'
import { useSeoMeta } from '@/composables/useSeoMeta'

useSeoMeta(
  { title: '下载已开始' },
  globalConfig.site.nameFull
)()
import {
  PhDownloadSimple as Download,
  PhHouse as Home,
  PhArrowLeft as ArrowLeft,
  PhCircleNotch as Loader2,
  PhHeart as Heart,
  PhUsers as Users,
  PhXCircle as XCircle
} from '@phosphor-icons/vue'
import Button from '@/components/ui/Button.vue'
import Card from '@/components/ui/Card.vue'
import CardHeader from '@/components/ui/CardHeader.vue'
import CardTitle from '@/components/ui/CardTitle.vue'
import CardContent from '@/components/ui/CardContent.vue'
import CardDescription from '@/components/ui/CardDescription.vue'

const route = useRoute()
const router = useRouter()

const loading = ref(true)
const error = ref('')
const fileInfo = ref(null)
const downloadTriggered = ref(false)

const loadLandingInfo = async () => {
  const token = route.query.token
  if (!token) {
    error.value = '缺少下载凭证，请从来源页面重新发起下载'
    loading.value = false
    return
  }

  try {
    const response = await getDownloadLanding(token)
    fileInfo.value = response.data
    triggerDownload()
  } catch (err) {
    error.value = err.response?.data?.error?.message || err.message || '获取下载信息失败，凭证可能已过期'
  } finally {
    loading.value = false
  }
}

// HEAD 探测同源下载路径可达性（不带 token：后端对 HEAD 分支不校验、不记账），
// 探测通过才自动触发下载；失败则保留手动按钮引导，避免整页导航到异常流。
const PROBE_TIMEOUT_MS = 5000

const probeDownloadPath = async (downloadUrl) => {
  try {
    const path = new URL(downloadUrl, window.location.origin).pathname
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), PROBE_TIMEOUT_MS)
    try {
      const res = await fetch(path, { method: 'HEAD', signal: controller.signal })
      return res.ok
    } finally {
      clearTimeout(timer)
    }
  } catch {
    return false
  }
}

const triggerDownload = async () => {
  if (downloadTriggered.value || !fileInfo.value?.download_url) return
  downloadTriggered.value = true
  if (await probeDownloadPath(fileInfo.value.download_url)) {
    window.location.href = fileInfo.value.download_url
  }
}

// 跳转到目标地址：站外需命中白名单（防开放重定向），站内走 SPA 路由；非法或缺失回退首页
const navigateToTarget = (url) => {
  if (!url) {
    router.push('/')
    return
  }
  if (isAllowedExternalTarget(url)) {
    window.location.href = url
    return
  }
  if (isSameOriginUrl(url)) {
    try {
      const parsed = new URL(url, window.location.origin)
      router.push(parsed.pathname + parsed.search + parsed.hash)
      return
    } catch {
      // 容错继续
    }
  }
  if (typeof url === 'string' && url.startsWith('/') && !url.startsWith('//')) {
    router.push(url)
    return
  }
  router.push('/')
}

const goBack = () => {
  const returnUrl = route.query.return_url || fileInfo.value?.return_url
  if (returnUrl) {
    navigateToTarget(returnUrl)
    return
  }
  const referrer = document.referrer
  if (referrer && isAllowedExternalTarget(referrer)) {
    window.location.href = referrer
    return
  }
  router.push('/')
}

const goToWebsite = () => {
  const returnUrl = fileInfo.value?.return_url || route.query.return_url
  navigateToTarget(returnUrl)
}

onMounted(() => {
  loadLandingInfo()
})
</script>

<template>
  <div class="flex min-h-[calc(100vh-10rem)] flex-col items-center justify-center gap-4 py-8 supports-[height:100dvh]:min-h-[calc(100dvh-10rem)]">
    <Card class="w-full max-w-lg">
      <CardHeader class="items-center text-center">
        <div class="mb-2 rounded-full bg-primary/10 p-3 text-primary">
          <Download weight="duotone" class="h-8 w-8" />
        </div>
        <CardTitle class="text-2xl">下载已开始</CardTitle>
        <CardDescription v-if="fileInfo?.file_name">
          正在为您下载 {{ fileInfo.file_name }}
        </CardDescription>
        <CardDescription v-else>
          正在获取下载信息…
        </CardDescription>
      </CardHeader>

      <CardContent class="space-y-6">
        <div v-if="loading" class="flex flex-col items-center justify-center gap-3 py-8 text-muted-foreground">
          <Loader2 weight="duotone" class="h-8 w-8 animate-spin" />
          <span>正在准备下载...</span>
        </div>

        <div v-else-if="error" class="space-y-5">
          <div class="rounded-lg border border-destructive/20 bg-destructive/10 p-4 text-center">
            <XCircle weight="duotone" class="mx-auto mb-3 h-12 w-12 text-destructive" />
            <p class="font-medium text-foreground">下载失败</p>
            <p class="mt-1 text-sm text-muted-foreground">{{ error }}</p>
          </div>
          <Button class="w-full" @click="goBack">返回上一页</Button>
        </div>

        <div v-else class="space-y-5">
          <!-- 真实 <a> 用户手势触发下载：Chrome/Android 会拦截非手势的自动下载 -->
          <a
            :href="fileInfo.download_url"
            :download="fileInfo.file_name || undefined"
            class="flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-4 py-3 text-sm font-medium text-primary-foreground shadow-sm transition-opacity hover:opacity-90"
          >
            <Download weight="duotone" class="h-5 w-5" />
            开始下载
          </a>
          <p class="text-center text-sm text-muted-foreground">
            一切就绪。部分浏览器（尤其 Android）不会自动弹出下载，<span class="font-medium text-foreground">请点击上方按钮开始下载</span>。
          </p>

          <div class="grid gap-2 sm:grid-cols-2">
            <Button variant="outline" @click="goBack">
              <ArrowLeft weight="duotone" class="mr-2 h-4 w-4" />
              返回上一页
            </Button>
            <Button v-if="fileInfo.return_url" @click="goToWebsite">
              <Home weight="duotone" class="mr-2 h-4 w-4" />
              前往网站
            </Button>
            <Button v-else @click="router.push('/')">
              <Home weight="duotone" class="mr-2 h-4 w-4" />
              返回首页
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>

    <!-- 赞助请求：收款码在关于页 -->
    <Card class="w-full max-w-lg">
      <CardContent class="flex items-center justify-between gap-3 p-5">
        <div class="min-w-0">
          <p class="text-sm font-semibold text-foreground">喜欢这个项目？</p>
          <p class="mt-0.5 text-xs leading-relaxed text-muted-foreground">
            服务器与带宽都是持续开销，你的赞助能让它跑得更久。收款码见「关于」页。
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
      加入官方交流群
      <span class="text-xs font-normal text-muted-foreground">获取更新通知与在线帮助</span>
    </a>
  </div>
</template>
