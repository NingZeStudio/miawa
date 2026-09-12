import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import pinia from './store'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import '@/assets/styles/index.css'

import { useTabsStore } from '@/store/modules/tabs'
import { bridgeHost } from '@/core/bridge-host'

const app = createApp(App)

app.use(pinia)
app.use(router)
app.use(ElementPlus)

// 注入宿主 bridge
const tabsStore = useTabsStore()
tabsStore.setRouter(router)
bridgeHost.setRouter(router)
bridgeHost.setTabsStore(tabsStore)

app.mount('#app')
