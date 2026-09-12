<script setup>
import { useRoute } from 'vue-router'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import CookiesConsent from '@/components/CookiesConsent.vue'
import AnnouncementDialog from '@/components/AnnouncementDialog.vue'
import ToastHost from '@/components/ui/ToastHost.vue'

const route = useRoute()
</script>

<template>
  <DefaultLayout>
    <RouterView v-slot="{ Component }">
      <Transition name="page" mode="out-in">
        <!-- 优先采用 route.meta.key（如 files 系列路由保持一致），避免内部钻取时页面被整页销毁重建 -->
        <component :is="Component" :key="route.meta.key || route.path" />
      </Transition>
    </RouterView>
  </DefaultLayout>
  <CookiesConsent />
  <AnnouncementDialog />
  <ToastHost />
</template>

<style>
/* 页面间切换：淡入 + 轻微上滑（对齐 LogShare.CN） */
.page-enter-active,
.page-leave-active {
  transition:
    opacity 0.18s ease,
    transform 0.18s ease;
}

.page-enter-from {
  opacity: 0;
  transform: translateY(8px);
}

.page-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
