<script setup>
import { useRouter } from 'vue-router'
import {
  PhWarningCircle as WarningCircle,
  PhHouse as Home,
  PhFolder as Folder
} from '@phosphor-icons/vue'
import { globalConfig } from '@/lib/globalConfig'
import { useSeoMeta } from '@/composables/useSeoMeta'
import Button from '@/components/ui/Button.vue'
import Card from '@/components/ui/Card.vue'
import CardContent from '@/components/ui/CardContent.vue'
import CardHeader from '@/components/ui/CardHeader.vue'
import CardTitle from '@/components/ui/CardTitle.vue'
import CardDescription from '@/components/ui/CardDescription.vue'

const router = useRouter()

useSeoMeta(
  {
    title: '404 - 页面未找到',
    description: '抱歉，您访问的页面不存在或已被移除。'
  },
  globalConfig.site.nameFull
)()
</script>

<template>
  <div class="flex min-h-[calc(100vh-12rem)] flex-col items-center justify-center py-12 supports-[height:100dvh]:min-h-[calc(100dvh-12rem)]">
    <Card class="w-full max-w-md text-center shadow-sm">
      <CardHeader class="items-center pb-2">
        <div class="mb-2 flex h-14 w-14 items-center justify-center rounded-2xl bg-muted/60 text-muted-foreground">
          <WarningCircle weight="duotone" class="h-8 w-8" />
        </div>
        <CardTitle class="text-2xl font-bold tracking-tight">页面未找到</CardTitle>
        <CardDescription class="mt-1 text-sm text-muted-foreground">
          抱歉，您访问的路径不存在、已变更或已被移除。
        </CardDescription>
      </CardHeader>
      <CardContent class="pt-4 space-y-4">
        <div class="flex flex-col gap-2.5 sm:flex-row sm:justify-center">
          <Button variant="default" class="gap-2" @click="router.push('/')">
            <Home weight="duotone" class="h-4 w-4" />
            返回首页
          </Button>
          <Button variant="outline" class="gap-2" @click="router.push('/files')">
            <Folder weight="duotone" class="h-4 w-4" />
            文件浏览
          </Button>
        </div>
      </CardContent>
    </Card>
  </div>
</template>
