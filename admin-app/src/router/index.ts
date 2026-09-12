import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import NProgress from 'nprogress'
import 'nprogress/nprogress.css'
import Layout from '@/layout/index.vue'
import { getStoredItem } from '@/lib/storage'

NProgress.configure({ showSpinner: false })

// 路由元信息类型扩展
declare module 'vue-router' {
  interface RouteMeta {
    title: string
    icon?: string
    isIframe?: boolean
    iframeUrl?: string
    keepAlive?: boolean
    hidden?: boolean
  }
}

export const routes: RouteRecordRaw[] = [
  {
    path: '/',
    component: Layout,
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('@/views/dashboard/index.vue'),
        meta: {
          title: '工作台',
          icon: 'LayoutDashboard',
          isIframe: false,
          keepAlive: true
        }
      },
      {
        path: 'config',
        name: 'Config',
        component: () => import('@/views/config/index.vue'),
        meta: {
          title: '配置编辑',
          icon: 'Settings2',
          isIframe: false,
          keepAlive: true
        }
      },
      {
        path: 'files',
        name: 'Files',
        component: () => import('@/views/files/index.vue'),
        meta: {
          title: '文件管理',
          icon: 'Folder',
          isIframe: false,
          keepAlive: true
        }
      },
      {
        path: 'blacklist',
        name: 'Blacklist',
        component: () => import('@/views/blacklist/index.vue'),
        meta: {
          title: '黑名单管理',
          icon: 'ShieldAlert',
          isIframe: false,
          keepAlive: true
        }
      }
    ]
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/login/index.vue'),
    meta: {
      title: '系统登录',
      hidden: true
    }
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/dashboard'
  }
]

const router = createRouter({
  history: createWebHistory('/admin/'),
  routes
})

router.beforeEach((to, _from, next) => {
  NProgress.start()

  const token = getStoredItem('admin_token')

  // 若无 token 且不是去登录页
  if (!token && to.path !== '/login') {
    next('/login')
    return
  }

  // 已登录去登录页，重定向至工作台
  if (token && to.path === '/login') {
    next('/dashboard')
    return
  }

  next()
})

router.afterEach(() => {
  NProgress.done()
})

export default router
