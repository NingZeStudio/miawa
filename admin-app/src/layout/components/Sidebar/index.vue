<template>
  <aside
    class="sidebar h-full flex flex-col border-r border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 transition-all duration-300 relative z-30"
    :class="[collapsed ? 'w-16' : 'w-64']"
  >
    <!-- 头部 Logo 与系统标名 -->
    <div class="h-14 flex items-center px-4 border-b border-zinc-200 dark:border-zinc-800 gap-3">
      <a
        href="/"
        target="_blank"
        rel="noreferrer"
        class="flex items-center gap-3 no-underline text-inherit overflow-hidden"
        title="前往前台首页"
      >
        <div class="w-8 h-8 rounded-md bg-zinc-900 dark:bg-zinc-100 flex items-center justify-center text-zinc-100 dark:text-zinc-900 font-bold shrink-0">
          <component
            :is="LayersIcon"
            class="w-4 h-4"
          />
        </div>
        <div
          v-show="!collapsed"
          class="flex flex-col truncate"
        >
          <span class="text-sm font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 truncate">
            Lemwood Mirror
          </span>
          <span class="text-[11px] text-zinc-500 dark:text-zinc-400 font-mono tracking-tighter">
            管理控制台
          </span>
        </div>
      </a>
    </div>

    <!-- 菜单导航列表 -->
    <div class="flex-1 overflow-y-auto py-3 px-2 space-y-1">
      <template
        v-for="item in menuList"
        :key="item.path"
      >
        <!-- 单级菜单项 -->
        <router-link
          v-if="!item.children || item.children.length === 0"
          :to="item.path"
          class="flex items-center gap-3 px-3 py-2 rounded-md text-xs font-medium transition-colors group relative"
          :class="[
            currentPath === item.path
              ? 'bg-zinc-100 dark:bg-zinc-800/80 text-zinc-900 dark:text-zinc-100 font-semibold'
              : 'text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100/60 dark:hover:bg-zinc-800/50 hover:text-zinc-900 dark:hover:text-zinc-200'
          ]"
          :title="collapsed ? item.title : ''"
        >
          <component
            :is="getIconComponent(item.icon)"
            class="w-4 h-4 shrink-0"
          />
          <span
            v-show="!collapsed"
            class="truncate flex-1"
          >{{ item.title }}</span>

          <span
            v-if="item.isIframe && !collapsed"
            class="text-[10px] px-1.5 py-0.5 rounded bg-zinc-200/70 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 font-mono scale-90"
          >
            iframe
          </span>
        </router-link>

        <!-- 多级折叠菜单组 -->
        <div
          v-else
          class="space-y-1"
        >
          <div
            class="flex items-center justify-between px-3 py-2 rounded-md text-xs font-medium text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100/50 dark:hover:bg-zinc-800/40 cursor-pointer"
            @click="toggleGroup(item.path)"
          >
            <div class="flex items-center gap-3">
              <component
                :is="getIconComponent(item.icon)"
                class="w-4 h-4 shrink-0"
              />
              <span v-show="!collapsed">{{ item.title }}</span>
            </div>
            <component
              :is="ChevronDownIcon"
              v-show="!collapsed"
              class="w-3.5 h-3.5 transition-transform duration-200"
              :class="{ '-rotate-90': !openedGroups.includes(item.path) }"
            />
          </div>

          <!-- 子菜单展开 -->
          <div
            v-show="!collapsed && openedGroups.includes(item.path)"
            class="pl-7 pr-1 space-y-1"
          >
            <router-link
              v-for="sub in item.children"
              :key="sub.path"
              :to="sub.path"
              class="flex items-center justify-between px-3 py-1.5 rounded-md text-xs font-medium transition-colors"
              :class="[
                currentPath === sub.path
                  ? 'bg-zinc-100 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100'
                  : 'text-zinc-500 dark:text-zinc-400 hover:bg-zinc-100/50 dark:hover:bg-zinc-800/40 hover:text-zinc-800 dark:hover:text-zinc-200'
              ]"
            >
              <div class="flex items-center gap-2 truncate">
                <span class="w-1.5 h-1.5 rounded-full bg-zinc-300 dark:bg-zinc-700" />
                <span class="truncate">{{ sub.title }}</span>
              </div>
            </router-link>
          </div>
        </div>
      </template>
    </div>

    <!-- 底部功能区与折叠切换条 -->
    <div class="border-t border-zinc-200 dark:border-zinc-800 p-2 space-y-1">
      <button
        class="w-full flex items-center gap-2 px-3 py-1.5 text-xs text-zinc-500 hover:text-red-600 dark:text-zinc-400 dark:hover:text-red-400 rounded hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
        :class="{ 'justify-center': collapsed }"
        title="退出登录"
        @click="handleLogout"
      >
        <component
          :is="LogOutIcon"
          class="w-4 h-4 shrink-0"
        />
        <span v-show="!collapsed">退出登录</span>
      </button>

      <button
        class="w-full flex items-center gap-2 px-3 py-1.5 text-xs text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100 rounded hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
        :class="{ 'justify-center': collapsed }"
        @click="appStore.toggleSidebar"
      >
        <component
          :is="collapsed ? PanelLeftOpenIcon : PanelLeftCloseIcon"
          class="w-4 h-4 shrink-0"
        />
        <span v-show="!collapsed">收起侧栏</span>
      </button>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/store/modules/app'
import { useUserStore } from '@/store/modules/user'
import { useTabsStore } from '@/store/modules/tabs'
import { ElMessage } from 'element-plus'
import type { MenuItemConfig } from '@/types'
import {
  Layers,
  LayoutDashboard,
  Settings2,
  Folder,
  ShieldAlert,
  ChevronDown,
  PanelLeftClose,
  PanelLeftOpen,
  LogOut,
  AppWindow
} from 'lucide-vue-next'

const LayersIcon = Layers
const ChevronDownIcon = ChevronDown
const PanelLeftCloseIcon = PanelLeftClose
const PanelLeftOpenIcon = PanelLeftOpen
const LogOutIcon = LogOut

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const userStore = useUserStore()
const tabsStore = useTabsStore()

const collapsed = computed(() => appStore.sidebarCollapsed)
const currentPath = computed(() => route.path)

const openedGroups = ref<string[]>([])

const toggleGroup = (path: string) => {
  const idx = openedGroups.value.indexOf(path)
  if (idx > -1) {
    openedGroups.value.splice(idx, 1)
  } else {
    openedGroups.value.push(path)
  }
}

const menuList = ref<MenuItemConfig[]>([
  {
    title: '工作台概览',
    path: '/dashboard',
    icon: 'LayoutDashboard',
    isIframe: false
  },
  {
    title: '配置编辑',
    path: '/config',
    icon: 'Settings2',
    isIframe: false
  },
  {
    title: '文件管理',
    path: '/files',
    icon: 'Folder',
    isIframe: false
  },
  {
    title: '黑名单管理',
    path: '/blacklist',
    icon: 'ShieldAlert',
    isIframe: false
  }
])

const iconMap: Record<string, any> = {
  LayoutDashboard,
  Settings2,
  Folder,
  ShieldAlert,
  AppWindow
}

const getIconComponent = (name?: string) => {
  if (!name || !iconMap[name]) return AppWindow
  return iconMap[name]
}

const handleLogout = () => {
  userStore.logout()
  tabsStore.closeAllTabs()
  ElMessage.success('已安全退出登录')
  router.push('/login')
}
</script>
