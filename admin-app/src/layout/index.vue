<template>
  <div class="layout-container flex h-screen w-screen overflow-hidden bg-background text-foreground select-none">
    <!-- 侧边栏 -->
    <Sidebar />

    <!-- 主体区域 -->
    <div class="flex-1 flex flex-col min-w-0 h-full overflow-hidden">
      <!-- 顶部导航栏 -->
      <Navbar />

      <!-- 多标签页栏 -->
      <Tabs />

      <!-- 主视图区 (普通路由与 iframe 容器无缝切换) -->
      <main class="flex-1 relative overflow-hidden bg-zinc-50 dark:bg-black select-text">
        <el-watermark
          :content="watermarkContent"
          :font="{ color: isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.05)', fontSize: 13 }"
          class="watermark-wrapper w-full h-full"
        >
          <!-- 普通 Vue 路由页面视图 -->
          <div
            v-show="!isCurrentIframe"
            class="view-container w-full h-full overflow-y-auto"
          >
            <router-view v-slot="{ Component, route: currentRoute }">
              <keep-alive>
                <component
                  :is="Component"
                  v-if="!currentRoute.meta?.isIframe"
                  :key="currentRoute.path"
                />
              </keep-alive>
            </router-view>
          </div>

          <!-- iframe 独立保活容器 -->
          <IframeContainer v-show="isCurrentIframe" />
        </el-watermark>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useTabsStore } from '@/store/modules/tabs'
import { useAppStore } from '@/store/modules/app'
import { useUserStore } from '@/store/modules/user'
import Sidebar from './components/Sidebar/index.vue'
import Navbar from './components/Navbar/index.vue'
import Tabs from './components/Tabs/index.vue'
import IframeContainer from './components/IframeContainer/index.vue'

const route = useRoute()
const tabsStore = useTabsStore()
const appStore = useAppStore()
const userStore = useUserStore()

// 直接依据当前实际路由 meta 判定，零时序差
const isCurrentIframe = computed(() => Boolean(route.meta?.isIframe))
const isDark = computed(() => appStore.isDark)

const watermarkContent = computed(() => {
  if (!appStore.watermarkEnabled) return ''
  return `${userStore.userInfo.displayName} ${userStore.userInfo.username}`
})

// 监听当前路由变动，自动同步到 Tab 状态中
watch(
  () => route.fullPath,
  () => {
    if (route.meta && route.meta.title) {
      tabsStore.addTab({
        title: route.meta.title,
        path: route.path,
        name: route.name as string,
        iframeUrl: route.meta.iframeUrl,
        isIframe: !!route.meta.isIframe,
        icon: route.meta.icon,
        closable: route.path !== '/dashboard'
      })
    }
  },
  { immediate: true }
)
</script>

<style scoped>
.watermark-wrapper {
  display: block;
  width: 100%;
  height: 100%;
}
:deep(.el-watermark) {
  width: 100% !important;
  height: 100% !important;
  display: block !important;
}
.view-container {
  height: 100%;
  width: 100%;
}
</style>
