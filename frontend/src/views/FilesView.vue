<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  PhAndroidLogo as AndroidLogo,
  PhArrowLineUp as ArrowUpToLine,
  PhCaretRight as ChevronRight,
  PhCopy as Copy,
  PhDownloadSimple as Download,
  PhFile as File,
  PhFileArchive as FileArchive,
  PhFolder as Folder,
  PhHardDrive as HardDrive,
  PhHouse as Home,
  PhJar as Jar,
  PhLinuxLogo as LinuxLogo,
  PhCube as Cube,
  PhPackage as Package,
  PhSignature as Signature,
  PhMagnifyingGlass as Search
} from '@phosphor-icons/vue'
import { useClipboard } from '@vueuse/core'
import { getStatus, getLatest, getPowConfig, prepareDownload } from '@/services/api'
import { getLauncherDisplayName } from '@/lib/launcher-info'
import { compareVersionDesc, formatSize } from '@/lib/format'
import { cn } from '@/lib/utils'
import { globalConfig } from '@/lib/globalConfig'
import { openBlankTab } from '@/lib/safeStorage'
import Badge from '@/components/ui/Badge.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'
import Skeleton from '@/components/ui/Skeleton.vue'

const props = defineProps({
  launcherName: String,
  versionName: String
})

const loading = ref(true)
const searchQuery = ref('')
const launchers = ref({})
const latestData = ref({})
const powConfig = ref({ enabled: false })
const { copy, copied } = useClipboard()

const route = useRoute()
const router = useRouter()
const currentPath = ref([])

// 浏览会话内缓存 /launchers + /latest + /pow/config：避免重复全量拉取
const dataCache = window.__FILES_DATA_CACHE__ || (window.__FILES_DATA_CACHE__ = { launchers: null, latest: null, pow: null })

const loadData = async () => {
  if (dataCache.launchers && dataCache.latest && dataCache.pow) {
    launchers.value = dataCache.launchers
    latestData.value = dataCache.latest
    powConfig.value = dataCache.pow
    loading.value = false
    return
  }
  loading.value = true
  try {
    const [statusRes, latestRes, powRes] = await Promise.all([
      getStatus(),
      getLatest(),
      getPowConfig().catch(() => ({ data: { enabled: false } }))
    ])

    const sortedLaunchers = {}
    if (statusRes.data && typeof statusRes.data === 'object') {
      Object.keys(statusRes.data)
        .sort()
        .forEach((key) => {
          sortedLaunchers[key] = (statusRes.data[key] || []).sort(compareVersionDesc)
        })
    }

    dataCache.launchers = sortedLaunchers
    dataCache.latest = latestRes.data || {}
    dataCache.pow = powRes.data || { enabled: false }
    launchers.value = sortedLaunchers
    latestData.value = latestRes.data || {}
    powConfig.value = powRes.data || { enabled: false }
  } catch (error) {
    console.error(error)
  } finally {
    loading.value = false
  }
}

// 文件类型 → 图标 + 彩色底片，让列表不再单调
const fileMeta = (filename) => {
  const ext = filename.split('.').pop()?.toLowerCase()
  if (['apk', 'apks', 'xapk'].includes(ext))
    return { icon: AndroidLogo, chip: 'bg-green-500/10 text-green-600 dark:text-green-400' }
  if (['zip', 'tar', 'gz', '7z', 'rar', 'xz'].includes(ext))
    return { icon: FileArchive, chip: 'bg-amber-500/10 text-amber-600 dark:text-amber-400' }
  if (['exe', 'msi', 'dmg', 'appimage'].includes(ext))
    return { icon: HardDrive, chip: 'bg-blue-500/10 text-blue-600 dark:text-blue-400' }
  if (ext === 'jar')
    return { icon: Jar, chip: 'bg-orange-500/10 text-orange-600 dark:text-orange-400' }
  if (ext === 'sig')
    return { icon: Signature, chip: 'bg-violet-500/10 text-violet-600 dark:text-violet-400' }
  if (ext === 'rpm')
    return { icon: LinuxLogo, chip: 'bg-red-500/10 text-red-600 dark:text-red-400' }
  if (ext === 'deb')
    return { icon: Package, chip: 'bg-purple-500/10 text-purple-600 dark:text-purple-400' }
  if (ext === 'hap')
    return { icon: Cube, chip: 'bg-cyan-500/10 text-cyan-600 dark:text-cyan-400' }
  return { icon: File, chip: 'bg-slate-500/10 text-slate-600 dark:text-slate-400' }
}

const launcherLogo = (id) => globalConfig.launchers[id]?.logoUrl || ''

const formatDate = (dateString) => {
  if (!dateString) return ''
  try {
    return new Date(dateString).toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    })
  } catch {
    return dateString
  }
}

const copyUrl = (url) => copy(url)

const handleDownload = async (item) => {
  const launcherName = currentPath.value[0]?.id
  const versionName = currentPath.value[1]?.id
  if (!launcherName || !versionName) {
    window.open(item.downloadUrl, '_blank')
    return
  }

  const filePath = `${launcherName}/${versionName}/${item.name}`
  const returnUrl = window.location.href
  const source = globalConfig.download.sourceLabels.files

  if (!powConfig.value.enabled) {
    // 点击同步栈内先开占位窗口，避免 await 后 window.open 被弹窗拦截
    const tab = openBlankTab()
    try {
      const response = await prepareDownload(filePath, returnUrl, source)
      const token = response.data.download_token
      if (token) {
        tab?.close()
        router.push(`/download-started?token=${token}`)
      } else {
        tab?.navigate(response.data.download_url || item.downloadUrl)
      }
    } catch (error) {
      console.error('Prepare download error:', error)
      tab?.navigate(item.downloadUrl)
    }
    return
  }

  router.push(`/verify?file=${encodeURIComponent(filePath)}&return_url=${encodeURIComponent(returnUrl)}&source=${encodeURIComponent(source)}`)
}

const findLauncherKey = (name) => {
  if (!name || !launchers.value) return null
  if (launchers.value[name]) return name
  const lower = String(name).toLowerCase()
  return Object.keys(launchers.value).find((k) => k.toLowerCase() === lower) || null
}

const navigateTo = (item, type) => {
  if (type === 'launcher') {
    router.push({ name: 'files-launcher', params: { launcherName: item.id } })
  } else if (type === 'version') {
    const launcherId = currentPath.value[0]?.id || props.launcherName
    if (launcherId) {
      router.push({
        name: 'files-version',
        params: { launcherName: launcherId, versionName: item.id }
      })
    }
  }
}

const navigateUp = () => {
  if (currentPath.value.length >= 2) {
    const launcherId = currentPath.value[0]?.id || props.launcherName
    router.push({
      name: 'files-launcher',
      params: { launcherName: launcherId }
    })
  } else {
    router.push({ name: 'files' })
  }
}

const navigateToBreadcrumb = (index) => {
  if (index === -1) {
    router.push({ name: 'files' })
  } else if (index === 0 && currentPath.value[0]) {
    router.push({
      name: 'files-launcher',
      params: { launcherName: currentPath.value[0].id }
    })
  } else if (index === 1 && currentPath.value[0] && currentPath.value[1]) {
    router.push({
      name: 'files-version',
      params: {
        launcherName: currentPath.value[0].id,
        versionName: currentPath.value[1].id
      }
    })
  }
}

const currentItems = computed(() => {
  const query = searchQuery.value.toLowerCase().trim()
  const depth = currentPath.value.length

  if (depth === 0) {
  	    return Object.keys(launchers.value)
  	      .filter((name) => name !== 'authlib-injector')
  	      .map((name) => ({
        id: name,
        name: getLauncherDisplayName(name),
        displayName: name,
        type: 'launcher',
        count: launchers.value[name].length,
        latest: latestData.value[name]
      }))
      .filter((l) => !query || l.name.toLowerCase().includes(query))
  }

  if (depth === 1) {
    const launcherName = currentPath.value[0].id
    const versions = launchers.value[launcherName] || []

    return versions
      .map((v) => ({
        id: v.tag_name || v.name,
        name: v.tag_name || v.name,
        type: 'version',
        date: v.published_at,
        isLatest: latestData.value[launcherName] === (v.tag_name || v.name),
        data: v,
        fileCount: v.assets?.length || 0
      }))
      .filter((v) => !query || v.name.toLowerCase().includes(query))
  }

  if (depth === 2) {
    const versionData = currentPath.value[1].data
    const launcherName = currentPath.value[0].id
    const versionName = currentPath.value[1].id

    return (versionData.assets || [])
      .map((asset) => ({
        id: asset.name,
        name: asset.name,
        type: 'file',
        size: asset.size,
        downloadUrl:
          asset.url && asset.url.startsWith('http')
            ? asset.url
            : `${globalConfig.download.baseUrl}/download/${launcherName}/${versionName}/${asset.name}`
      }))
      .filter((f) => !query || f.name.toLowerCase().includes(query))
  }

  return []
})

const syncPathFromRoute = () => {
  if (!props.launcherName) {
    currentPath.value = []
    applyFilesMeta()
    return
  }

  const launcherKey = findLauncherKey(props.launcherName)
  if (!launcherKey || !launchers.value[launcherKey]) {
    currentPath.value = []
    applyFilesMeta()
    return
  }

  const path = [
    {
      name: getLauncherDisplayName(launcherKey),
      id: launcherKey,
      type: 'launcher',
      displayName: launcherKey
    }
  ]

  if (props.versionName) {
    const versions = launchers.value[launcherKey] || []
    const versionData = versions.find((v) => (v.tag_name || v.name) === props.versionName)

    if (versionData) {
      path.push({
        name: props.versionName,
        id: props.versionName,
        type: 'version',
        data: versionData
      })
    }
  }

  currentPath.value = path
  applyFilesMeta()
}

watch([() => props.launcherName, () => props.versionName, () => launchers.value], () => {
  syncPathFromRoute()
})

onMounted(async () => {
  await loadData()
  syncPathFromRoute()
})

const nameFull = globalConfig.site.nameFull

const applyFilesMeta = () => {
  let title = '文件列表'
  let description = '浏览和下载 Minecraft 启动器版本文件'

  if (currentPath.value.length === 1) {
    const launcher = currentPath.value[0]
    title = `${launcher.name} - ${nameFull}`
    description = `浏览 ${launcher.name} 的所有版本`
  } else if (currentPath.value.length >= 2) {
    const launcher = currentPath.value[0]
    const version = currentPath.value[1]
    title = `${version.name} - ${launcher.name} - ${nameFull}`
    description = `下载 ${launcher.name} ${version.name} 版本的资源文件`
  }

  useSeoMeta({ title, description, fullTitle: true }, nameFull)()
}
</script>

<template>
  <div class="mx-auto w-full max-w-4xl min-w-0 space-y-4">
    <!-- 宽度与 API 文档页一致：max-w-4xl 居中 -->
    <div class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div class="space-y-1">
        <h1 class="text-3xl font-bold tracking-tight">文件浏览</h1>
        <p class="text-sm text-muted-foreground">按启动器、版本、文件逐层浏览，找到需要的安装包。</p>
      </div>
      <div class="relative w-full sm:w-64">
        <Search weight="duotone" class="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
        <Input v-model="searchQuery" type="search" placeholder="搜索启动器 / 版本 / 文件…" class="pl-9" />
      </div>
    </div>

    <div class="overflow-hidden rounded-lg border bg-card text-card-foreground shadow-sm">
      <div class="flex items-center gap-1 overflow-x-auto border-b px-3 py-2 text-sm text-muted-foreground">
        <Button variant="ghost" size="sm" class="h-7 shrink-0 whitespace-nowrap px-2 text-xs" @click="navigateToBreadcrumb(-1)">
          <Home weight="duotone" class="mr-1 h-3.5 w-3.5" />
          根目录
        </Button>
        <template v-for="(crumb, index) in currentPath" :key="crumb.id">
          <ChevronRight weight="duotone" class="h-3.5 w-3.5 shrink-0" />
          <Button variant="ghost" size="sm" class="h-7 shrink-0 whitespace-nowrap px-2 text-xs" @click="navigateToBreadcrumb(index)">
            {{ crumb.name }}
          </Button>
        </template>
        <span class="ml-auto flex shrink-0 items-center gap-2 whitespace-nowrap text-xs text-muted-foreground">
          <span v-if="copied" class="text-emerald-600 dark:text-emerald-400">链接已复制</span>
          <span>{{ currentItems.length }} 项</span>
        </span>
      </div>

      <div v-if="loading" class="divide-y">
        <div v-for="i in 8" :key="i" class="flex items-center gap-3 px-4 py-3">
          <Skeleton class="h-9 w-9 shrink-0 rounded-lg" />
          <div class="min-w-0 flex-1 space-y-1.5">
            <Skeleton class="h-4 w-3/5" />
            <Skeleton class="h-3 w-2/5" />
          </div>
          <Skeleton class="h-3 w-16 shrink-0" />
        </div>
      </div>

      <div v-else-if="!currentItems.length" class="flex flex-col items-center gap-3 px-4 py-16 text-muted-foreground">
        <Folder weight="duotone" class="h-10 w-10 opacity-40" />
        <p class="text-sm font-medium text-foreground">这里空空如也</p>
        <p class="text-xs">没有匹配的条目，换个关键词试试。</p>
      </div>

      <!-- 桌面端：列表行 -->
      <div v-else class="hidden divide-y sm:block">
        <button
          v-if="currentPath.length > 0"
          type="button"
          class="flex w-full items-center gap-3 px-4 py-3 text-left text-sm text-muted-foreground transition-colors hover:bg-accent"
          @click="navigateUp"
        >
          <ArrowUpToLine weight="duotone" class="h-4 w-4" />
          <span>返回上一级</span>
        </button>

        <div
          v-for="item in currentItems"
          :key="item.id"
          :class="cn(
            'flex items-center gap-3 px-4 py-2.5 transition-colors',
            item.type !== 'file' ? 'cursor-pointer hover:bg-accent/50' : 'hover:bg-muted/30'
          )"
          @click="item.type !== 'file' ? navigateTo(item, item.type) : null"
        >
          <!-- 启动器行显示真实 logo，版本/文件用彩色类型底片 -->
          <img
            v-if="item.type === 'launcher' && launcherLogo(item.id)"
            :src="launcherLogo(item.id)"
            alt=""
            class="h-9 w-9 shrink-0 rounded-lg border bg-background object-contain p-1"
          />
          <div
            v-else-if="item.type !== 'file'"
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"
          >
            <Folder weight="duotone" class="h-4 w-4" />
          </div>
          <div
            v-else
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
            :class="fileMeta(item.name).chip"
          >
            <component :is="fileMeta(item.name).icon" weight="duotone" class="h-4 w-4" />
          </div>

          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="truncate text-sm font-medium" :title="item.name">{{ item.name }}</span>
              <Badge v-if="item.isLatest" variant="success" class="text-[10px] leading-none">Latest</Badge>
            </div>
            <p v-if="item.type === 'launcher'" class="mt-0.5 text-xs text-muted-foreground">{{ item.count }} 个版本</p>
            <p v-else-if="item.type === 'version'" class="mt-0.5 text-xs text-muted-foreground">{{ formatDate(item.date) }} · {{ item.fileCount }} 个文件</p>
            <p v-else-if="item.size != null" class="mt-0.5 text-xs text-muted-foreground">{{ formatSize(item.size) }}</p>
          </div>

          <ChevronRight v-if="item.type !== 'file'" weight="duotone" class="h-4 w-4 shrink-0 text-muted-foreground" />

          <div v-if="item.type === 'file'" class="flex shrink-0 gap-0.5">
            <Button size="icon" variant="ghost" class="h-8 w-8" @click.stop="copyUrl(item.downloadUrl)">
              <Copy weight="duotone" class="h-4 w-4" />
              <span class="sr-only">复制链接</span>
            </Button>
            <Button size="icon" variant="soft" class="h-8 w-8" @click.stop="handleDownload(item)">
              <Download weight="duotone" class="h-4 w-4" />
              <span class="sr-only">下载</span>
            </Button>
          </div>
        </div>
      </div>

      <!-- 移动端：卡片式，长文件名完整换行，下载按钮加大 -->
      <div v-if="!loading && currentItems.length" class="divide-y sm:hidden">
        <button
          v-if="currentPath.length > 0"
          type="button"
          class="flex w-full items-center gap-3 px-4 py-3 text-left text-sm text-muted-foreground transition-colors hover:bg-accent"
          @click="navigateUp"
        >
          <ArrowUpToLine weight="duotone" class="h-4 w-4" />
          <span>返回上一级</span>
        </button>

        <div v-for="item in currentItems" :key="item.id" class="p-3">
          <!-- 启动器 / 版本：整卡可点 -->
          <button
            v-if="item.type !== 'file'"
            type="button"
            class="flex w-full items-start gap-3 rounded-lg border bg-background p-3 text-left transition-colors hover:bg-accent/50"
            @click="navigateTo(item, item.type)"
          >
            <img
              v-if="item.type === 'launcher' && launcherLogo(item.id)"
              :src="launcherLogo(item.id)"
              alt=""
              class="h-10 w-10 shrink-0 rounded-lg border bg-background object-contain p-1"
            />
            <div
              v-else
              class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"
            >
              <Folder weight="duotone" class="h-5 w-5" />
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="break-words text-sm font-medium leading-snug">{{ item.name }}</span>
                <Badge v-if="item.isLatest" variant="success" class="text-[10px] leading-none">Latest</Badge>
              </div>
              <p v-if="item.type === 'launcher'" class="mt-0.5 text-xs text-muted-foreground">{{ item.count }} 个版本</p>
              <p v-else class="mt-0.5 text-xs text-muted-foreground">{{ formatDate(item.date) }} · {{ item.fileCount }} 个文件</p>
            </div>
            <ChevronRight weight="duotone" class="mt-1 h-4 w-4 shrink-0 text-muted-foreground" />
          </button>

          <!-- 文件：文件名完整换行 + 大下载按钮 -->
          <div v-else class="space-y-2.5 rounded-lg border bg-background p-3">
            <div class="flex items-start gap-3">
              <div
                class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
                :class="fileMeta(item.name).chip"
              >
                <component :is="fileMeta(item.name).icon" weight="duotone" class="h-5 w-5" />
              </div>
              <div class="min-w-0 flex-1">
                <p class="break-words text-sm font-medium leading-snug">{{ item.name }}</p>
                <p v-if="item.size != null" class="mt-0.5 text-xs text-muted-foreground">{{ formatSize(item.size) }}</p>
              </div>
            </div>
            <div class="flex gap-2">
              <Button class="h-9 flex-1" @click="handleDownload(item)">
                <Download weight="duotone" class="mr-1.5 h-4 w-4" />
                下载
              </Button>
              <Button variant="outline" size="icon" class="h-9 w-9" @click="copyUrl(item.downloadUrl)">
                <Copy weight="duotone" class="h-4 w-4" />
                <span class="sr-only">复制链接</span>
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
