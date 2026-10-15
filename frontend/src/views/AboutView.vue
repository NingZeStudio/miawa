<script setup>
import { computed } from 'vue'
import {
  PhCode as Code,
  PhArrowSquareOut as ExternalLink,
  PhGithubLogo as Github,
  PhHeart as Heart,
  PhStack as Layers,
  PhEnvelopeSimple as Mail,
  PhChatCircle as MessageCircle,
  PhHardDrive as Server,
  PhLightning as Zap
} from '@phosphor-icons/vue'
import Button from '@/components/ui/Button.vue'
import FriendLinks from '@/components/FriendLinks.vue'
import { globalConfig } from '@/lib/globalConfig'
import { useSeoMeta } from '@/composables/useSeoMeta'
import {
  sponsors,
  sponsorConfig,
  getTotalAmount,
  getSponsorCount,
  getPlatformIcon,
  getPlatformColor
} from '@/lib/sponsorConfig'

const totalAmount = computed(() => getTotalAmount())
const sponsorCount = computed(() => getSponsorCount())

const sortedSponsors = computed(() => {
  return [...sponsors].sort((a, b) => {
    if (a.pinned && !b.pinned) return -1
    if (!a.pinned && b.pinned) return 1
    return new Date(b.date).getTime() - new Date(a.date).getTime()
  })
})

useSeoMeta(
  {
    title: '关于',
    description: `了解${globalConfig.site.name}背后的团队、技术栈和项目故事`
  },
  globalConfig.site.nameFull
)()
</script>

<template>
  <div class="space-y-6">
    <div class="space-y-1">
      <h1 class="text-3xl font-bold tracking-tight">关于本站</h1>
      <p class="text-sm text-muted-foreground">一群 Minecraft 爱好者用业余时间维护的公益镜像服务。</p>
    </div>

    <div class="space-y-4">
      <div class="rounded-lg border bg-card p-5 shadow-sm">
        <div class="flex items-center gap-2 text-base font-semibold">
          <Layers weight="duotone" class="h-5 w-5 text-primary" />
          项目简介
        </div>
        <p class="mt-3 text-sm leading-relaxed text-muted-foreground">
          Lemwood Mirror 是面向 Minecraft Java 版社区的开源镜像服务，由 AnyLyraLabs 自托管运营。我们全自动追踪各启动器的
          GitHub Releases，第一时间同步最新版本——让网络不畅的地区，也能顺畅下载。
        </p>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <div class="rounded-lg border bg-card p-5 shadow-sm">
          <div class="flex items-center gap-2 text-base font-semibold">
            <Server weight="duotone" class="h-5 w-5 text-blue-500" />
            基础设施 & 后端
          </div>
          <div class="mt-3 space-y-3">
            <div>
              <h4 class="text-sm font-medium text-foreground">服务器 & 域名备案</h4>
              <p class="mt-0.5 text-xs text-muted-foreground">服务器与域名由 <span class="font-medium text-foreground">柠枺</span> 赞助运维。</p>
              <div class="mt-1 flex flex-col gap-1 text-xs text-muted-foreground">
                <span class="flex items-center gap-1"><Mail weight="duotone" class="h-3 w-3" /> {{ globalConfig.contact.email }}</span>
                <span>{{ globalConfig.contact.qq }}</span>
                <a :href="globalConfig.links.qqGroup" target="_blank" class="flex items-center gap-1 transition-colors hover:text-foreground">
                  <MessageCircle weight="duotone" class="h-3 w-3" /> QQ群：{{ globalConfig.contact.qqGroup }}
                </a>
              </div>
            </div>
            <div class="border-t border-dashed pt-3">
              <h4 class="text-sm font-medium text-foreground">技术栈</h4>
              <div class="mt-1 flex flex-wrap gap-1.5">
                <span class="rounded-md bg-blue-500/10 px-2 py-0.5 text-xs font-medium text-blue-500">Golang</span>
                <span class="rounded-md bg-cyan-500/10 px-2 py-0.5 text-xs font-medium text-cyan-500">Docker</span>
              </div>
            </div>
          </div>
        </div>

        <div class="rounded-lg border bg-card p-5 shadow-sm">
          <div class="flex items-center gap-2 text-base font-semibold">
            <Code weight="duotone" class="h-5 w-5 text-green-500" />
            前端开发 & 设计
          </div>
          <div class="mt-3 space-y-3">
            <div>
              <h4 class="text-sm font-medium text-foreground">核心开发</h4>
              <p class="mt-0.5 text-xs text-muted-foreground">界面设计与前端开发由 <span class="font-medium text-foreground">燕随YanSui</span> 完成。</p>
              <div class="mt-1 flex flex-col gap-1 text-xs text-muted-foreground">
                <span class="flex items-center gap-1"><Mail weight="duotone" class="h-3 w-3" /> lyl518@outlook.com</span>
                <span>Github：qitry</span>
              </div>
            </div>
            <div class="border-t border-dashed pt-3">
              <h4 class="text-sm font-medium text-foreground">技术栈</h4>
              <div class="mt-1 flex flex-wrap gap-1.5">
                <span class="rounded-md bg-green-500/10 px-2 py-0.5 text-xs font-medium text-green-500">Vue 3</span>
                <span class="rounded-md bg-purple-500/10 px-2 py-0.5 text-xs font-medium text-purple-500">Vite</span>
                <span class="rounded-md bg-slate-500/10 px-2 py-0.5 text-xs font-medium text-slate-500">Shadcn/Vue</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="space-y-4">
      <div class="rounded-lg border bg-card p-5 shadow-sm">
        <div class="flex items-center justify-between gap-4">
          <div class="space-y-2">
            <div class="inline-flex items-center gap-1.5 rounded-full border border-amber-500/20 bg-amber-500/10 px-3 py-1 text-xs font-medium text-amber-700 dark:text-amber-300">
              <Heart weight="duotone" class="h-3.5 w-3.5" />
              赞助支持
            </div>
            <h2 class="text-xl font-bold tracking-tight">{{ sponsorConfig.title }}</h2>
            <p class="max-w-lg text-sm leading-relaxed text-muted-foreground">{{ sponsorConfig.description }}</p>
          </div>
          <div class="shrink-0 rounded-xl border bg-background px-5 py-3 text-center shadow-sm">
            <p class="text-xs text-muted-foreground">累计赞助</p>
            <p class="text-2xl font-bold text-amber-600 dark:text-amber-400">¥{{ totalAmount }}</p>
            <p class="text-xs text-muted-foreground">{{ sponsorCount }} 位朋友</p>
          </div>
        </div>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <div class="rounded-lg border bg-card p-5 shadow-sm">
          <div class="flex items-center gap-2 text-sm font-semibold">
            <span class="rounded-md bg-blue-500/10 p-1.5">
              <Zap weight="duotone" class="h-4 w-4 text-blue-500" />
            </span>
            支付宝
          </div>
          <div class="mt-3 flex min-h-[200px] items-center justify-center overflow-hidden rounded-lg border bg-muted/40 p-4">
            <img :src="sponsorConfig.alipayQrCode" alt="支付宝赞助二维码" class="max-h-48 w-auto rounded object-contain shadow-sm" />
          </div>
          <p class="mt-2 text-center text-xs text-muted-foreground">扫码赞助</p>
        </div>

        <div class="rounded-lg border bg-card p-5 shadow-sm">
          <div class="flex items-center gap-2 text-sm font-semibold">
            <span class="rounded-md bg-green-500/10 p-1.5">
              <Zap weight="duotone" class="h-4 w-4 text-green-500" />
            </span>
            微信
          </div>
          <div class="mt-3 flex min-h-[200px] items-center justify-center overflow-hidden rounded-lg border bg-muted/40 p-4">
            <img :src="sponsorConfig.wechatQrCode" alt="微信赞助二维码" class="max-h-48 w-auto rounded object-contain shadow-sm" />
          </div>
          <p class="mt-2 text-center text-xs text-muted-foreground">扫码赞助</p>
        </div>
      </div>

      <div class="rounded-lg border bg-card shadow-sm">
        <div class="flex items-center justify-between border-b px-5 py-3">
          <div class="flex items-center gap-2 text-sm font-semibold">
            <span class="rounded-md bg-amber-500/10 p-1.5">
              <Heart weight="duotone" class="h-4 w-4 text-amber-500" />
            </span>
            赞助者列表
          </div>
          <div class="text-xs text-muted-foreground">
            <span class="font-medium text-foreground">{{ sponsorCount }}</span> 位 ·
            <span class="font-medium text-foreground">¥{{ totalAmount }}</span>
          </div>
        </div>

        <div v-if="sortedSponsors.length" class="divide-y">
          <div v-for="sponsor in sortedSponsors" :key="sponsor.id"
            class="flex items-center gap-3 px-5 py-3 transition-colors hover:bg-muted/30">
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-1.5">
                <span class="text-sm font-medium text-foreground">{{ sponsor.name }}</span>
                <span :class="['rounded-full px-2 py-0.5 text-[10px] font-medium', getPlatformColor(sponsor.platform)]">
                  {{ getPlatformIcon(sponsor.platform) }}
                </span>
                <span v-if="sponsor.pinned" class="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium text-amber-500">
                  置顶
                </span>
              </div>
              <p class="truncate text-xs text-muted-foreground">{{ sponsor.message || sponsor.date }}</p>
            </div>
            <div class="shrink-0 text-right">
              <span class="text-base font-bold text-red-500">¥{{ sponsor.amount }}</span>
              <p class="text-[10px] text-muted-foreground">{{ sponsor.date }}</p>
            </div>
          </div>
        </div>

        <div v-else class="py-10 text-center">
          <Heart weight="duotone" class="mx-auto mb-3 h-8 w-8 text-muted-foreground opacity-40" />
          <p class="text-sm text-muted-foreground">还没有人上榜，期待你的名字。</p>
        </div>
      </div>

      <div class="rounded-lg border border-amber-500/20 bg-amber-500/5 px-5 py-3 text-center text-xs text-foreground">
        <p class="font-medium">所有捐助将全额用于服务器运营，账目公开透明。</p>
      </div>
    </div>

    <FriendLinks />

    <div class="grid gap-4 sm:grid-cols-2">
      <div class="rounded-lg border bg-card p-5 shadow-sm transition-colors hover:border-primary/30">
        <h3 class="flex items-center gap-2 text-sm font-semibold">同门项目：LogShare.CN</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">
           Minecraft 日志分享与分析平台——mclo.gs 的增强替代，更快、更懂中文语境。
         </p>
         <Button variant="outline" size="sm" class="mt-3" as="a" :href="globalConfig.links.logshare" target="_blank">
          立即体验 <ExternalLink weight="duotone" class="ml-1.5 h-3 w-3" />
        </Button>
      </div>

      <div class="flex flex-col items-center justify-center rounded-lg border bg-card p-5 text-center shadow-sm">
        <Github weight="duotone" class="mb-3 h-8 w-8 text-foreground/70" />
        <h3 class="text-sm font-semibold">开源共建</h3>
        <p class="mt-1 text-xs text-muted-foreground">代码完全开源，欢迎 Star、Fork 与 Pull Request。</p>
        <Button size="sm" class="mt-3" as="a" :href="globalConfig.links.githubOrg" target="_blank">
          前往 GitHub 仓库
        </Button>
      </div>
    </div>
  </div>
</template>

