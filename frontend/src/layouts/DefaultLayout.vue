<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import {
  PhSun as Sun,
  PhMoon as Moon,
  PhMonitor as Monitor
} from '@phosphor-icons/vue'
import Footer from '@/components/layout/Footer.vue'
import MobileNav from '@/components/layout/MobileNav.vue'
import { isNavigationActive, navigationLinks } from '@/lib/navigation'
import { globalConfig } from '@/lib/globalConfig'
import { setStoredItem, getStoredItem } from '@/lib/safeStorage'

const route = useRoute()

const { displayMode: displayModeKey, darkMode } = globalConfig.storage.keys

// 顶栏滚动后切换为毛玻璃胶囊（对齐 LogShare.CN 顶栏行为）
const isScrolled = ref(false)
const onWindowScroll = () => {
  isScrolled.value = window.scrollY > 8
}

// 显示模式：浅色 / 深色 / 跟随系统（顶栏三态胶囊切换）
const displayMode = ref('system')

const themeOptions = [
  { mode: 'light', icon: Sun, label: '浅色' },
  { mode: 'dark', icon: Moon, label: '深色' },
  { mode: 'system', icon: Monitor, label: '跟随系统' }
]

const applyDisplayMode = mode => {
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  const dark = mode === 'dark' || (mode === 'system' && prefersDark)
  document.documentElement.classList.toggle('dark', dark)
  setStoredItem(darkMode, dark ? 'dark' : 'light')
}

const setDisplayMode = mode => {
  displayMode.value = mode
  // 持久化三态值；'system' 表示跟随系统
  setStoredItem(displayModeKey, mode)
  applyDisplayMode(mode)
}

onMounted(() => {
  const stored = getStoredItem(displayModeKey)
  displayMode.value = stored === 'dark' || stored === 'light' ? stored : 'system'
  applyDisplayMode(displayMode.value)

  // 未显式设置显示模式时，跟随系统深浅色变化
  mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
  mediaQuery.addEventListener('change', onSystemColorSchemeChange)

  onWindowScroll()
  window.addEventListener('scroll', onWindowScroll, { passive: true })
})

let mediaQuery = null
const onSystemColorSchemeChange = () => {
  applyDisplayMode(displayMode.value)
}

onUnmounted(() => {
  window.removeEventListener('scroll', onWindowScroll)
  mediaQuery?.removeEventListener('change', onSystemColorSchemeChange)
})
</script>

<template>
  <div
    class="flex min-h-screen flex-col bg-background font-sans text-foreground antialiased transition-colors duration-500"
  >
    <header class="pointer-events-none sticky top-0 z-30 w-full">
      <div
        class="pointer-events-auto mx-auto transition-all duration-300 ease-out"
        :class="
          isScrolled
            ? 'mt-3 w-[calc(100%-2rem)] rounded-full border-border/60 bg-background/80 shadow-lg backdrop-blur-md'
            : 'mt-0 w-full rounded-none border border-transparent bg-background'
        "
      >
        <div
          class="flex items-center gap-3 px-4 transition-all duration-300"
          :class="isScrolled ? 'h-12' : 'h-14'"
        >
          <router-link to="/" class="flex shrink-0 items-center gap-2 font-semibold">
            <img
              src="@/assets/logo.svg"
              alt="Logo"
              class="h-7 w-7 dark:hidden"
            />
            <img
              src="@/assets/logo-dark.svg"
              alt="Logo"
              class="hidden h-7 w-7 dark:block"
            />
            <span class="inline">{{ globalConfig.site.name }}</span>
          </router-link>

          <nav class="ml-4 hidden items-center gap-1 md:flex">
            <router-link
              v-for="link in navigationLinks"
              :key="link.path"
              :to="link.path"
              class="rounded-md px-2.5 py-1.5 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground"
              :class="
                isNavigationActive(route.path, link)
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground'
              "
            >
              <component
                :is="link.icon"
                weight="duotone"
                class="mr-1 -mt-0.5 inline h-4 w-4"
              />
              {{ link.name }}
            </router-link>
          </nav>

          <div class="flex-1" />

          <!-- 主题三态切换：圆角容器 tab，高亮胶囊在选项间平移 -->
          <div
            class="relative flex items-center gap-0.5 rounded-full border border-border/60 bg-background/60 p-0.5"
            role="tablist"
            aria-label="显示模式"
          >
            <span
              aria-hidden="true"
              class="absolute left-0.5 top-1/2 h-7 w-7 -translate-y-1/2 rounded-full bg-muted-foreground/25 shadow-sm transition-transform duration-300 ease-out"
              :style="{
                transform: `translateX(${themeOptions.findIndex(o => o.mode === displayMode) * 30}px) translateY(-50%)`
              }"
            />
            <button
              v-for="option in themeOptions"
              :key="option.mode"
              role="tab"
              :aria-selected="displayMode === option.mode"
              :aria-label="option.label"
              :title="option.label"
              class="relative z-10 inline-flex h-7 w-7 items-center justify-center rounded-full transition-colors"
              :class="
                displayMode === option.mode
                  ? 'text-foreground'
                  : 'text-muted-foreground hover:text-accent-foreground'
              "
              @click="setDisplayMode(option.mode)"
            >
              <component :is="option.icon" weight="duotone" class="h-3.5 w-3.5" />
            </button>
          </div>

          <MobileNav :scrolled="isScrolled" />
        </div>
      </div>
    </header>

    <!-- [&>*]:min-w-0：flex 子项默认 min-width:auto，长内容（如文档代码块）会把页面撑出横向滚动 -->
    <main class="mx-auto flex w-[calc(100%-2rem)] max-w-6xl flex-1 flex-col gap-6 pt-6 lg:pt-8 pb-12 lg:pb-16 [&>*]:min-w-0">
      <slot />
    </main>
    <Footer />
  </div>
</template>
