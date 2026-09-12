<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { getStats, getBandwidth } from '@/services/api'
import { useDark } from '@vueuse/core'
import { globalConfig } from '@/lib/globalConfig'
import {
  PhActivity as Activity,
  PhArrowDown as ArrowDown,
  PhArrowUp as ArrowUp,
  PhChartBar as BarChart3,
  PhGauge as Gauge,
  PhMapPin as MapPin,
  PhChartPie as ChartPie,
  PhTrendUp as TrendingUp
} from '@phosphor-icons/vue'
import Card from '@/components/ui/Card.vue'
import CardContent from '@/components/ui/CardContent.vue'
import CardDescription from '@/components/ui/CardDescription.vue'
import CardHeader from '@/components/ui/CardHeader.vue'
import CardTitle from '@/components/ui/CardTitle.vue'
import Skeleton from '@/components/ui/Skeleton.vue'
import { getLauncherDisplayName } from '@/lib/launcher-info'
import { useSeoMeta } from '@/composables/useSeoMeta'

useSeoMeta(
  {
    title: '统计信息',
    description: `查看${globalConfig.site.name}的访问统计、下载统计和地理分布数据`
  },
  globalConfig.site.nameFull
)()

import { use, registerMap as echartsRegisterMap } from 'echarts/core'
import { BarChart, LineChart, MapChart, PieChart } from 'echarts/charts'
import { CanvasRenderer } from 'echarts/renderers'
import {
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
  VisualMapComponent
} from 'echarts/components'
import VChart from 'vue-echarts'
import { toMapProvince, isProvinceFullName } from '@/lib/chinaRegion'

use([
  CanvasRenderer, BarChart, LineChart, MapChart, PieChart,
  TitleComponent, TooltipComponent, LegendComponent, GridComponent, VisualMapComponent
])

const stats = ref({})
const bandwidth = ref({})
const loading = ref(true)
const isDark = useDark()
let bandwidthTimer = null

// 带宽轮询：5s 一次（站点级限流 300 req/min/IP，秒级轮询多开页面即触发）；
// 页面隐藏时暂停，恢复可见时立即刷新并续跑。
const BANDWIDTH_INTERVAL_MS = 5000

const startBandwidthPolling = () => {
  if (bandwidthTimer) return
  bandwidthTimer = setInterval(refreshBandwidth, BANDWIDTH_INTERVAL_MS)
}

const stopBandwidthPolling = () => {
  if (bandwidthTimer) {
    clearInterval(bandwidthTimer)
    bandwidthTimer = null
  }
}

const onVisibilityChange = () => {
  if (document.hidden) {
    stopBandwidthPolling()
  } else {
    refreshBandwidth()
    startBandwidthPolling()
  }
}

const formatMbps = (v) => {
  if (!Number.isFinite(v)) return '0'
  return v.toFixed(2)
}

const bandwidthUtilization = computed(() => {
  const u = bandwidth.value.utilization_percent
  if (!Number.isFinite(u)) return 0
  return Math.min(100, Math.max(0, Math.round(u)))
})

const isBandwidthIdle = computed(() => {
  const v = bandwidth.value.current_bandwidth_mbps
  return !Number.isFinite(v) || v < 0.01
})

const formatBytes = (bytes) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB']
  const i = Math.max(0, Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const diskPercentage = computed(() => {
  if (!stats.value.disk || !stats.value.disk.total) return 0
  return Math.round((stats.value.disk.used / stats.value.disk.total) * 100)
})

const avgDailyVisits = computed(() => {
  if (!stats.value.total_days || stats.value.total_days === 0) return 0
  return stats.value.total_visits / stats.value.total_days
})

const avgDailyDownloads = computed(() => {
  if (!stats.value.total_days || stats.value.total_days === 0) return 0
  return stats.value.total_downloads / stats.value.total_days
})

const visitGrowth = computed(() => {
  const avg = avgDailyVisits.value
  const recent = (stats.value.last_30_visits || 0) / 30
  if (avg === 0) return 0
  return ((recent - avg) / avg * 100)
})

const downloadGrowth = computed(() => {
  const avg = avgDailyDownloads.value
  const recent = (stats.value.last_30_downloads || 0) / 30
  if (avg === 0) return 0
  return ((recent - avg) / avg * 100)
})

// 覆盖省份数：剔除「海外」「其他」聚合项
const provinceCount = computed(() => {
  return (stats.value.geo_distribution || []).filter(g => g.country !== '海外' && g.country !== '其他').length
})

// 热门资源排行：只取 Top5（与左侧地图卡高度对齐），launcher key → 显示名 + logo，并算出占比条数据
const topDownloads = computed(() => {
  const ranks = (stats.value.top_downloads || []).slice(0, 6)
  if (!ranks.length) return []
  const total = ranks.reduce((sum, r) => sum + (r.count || 0), 0) || 1
  const max = Math.max(...ranks.map(r => r.count || 0), 1)
  return ranks.map(r => {
    const info = globalConfig.launchers?.[r.launcher]
    return {
      key: r.launcher,
      name: getLauncherDisplayName(r.launcher),
      logo: info?.logoUrl || '',
      count: r.count || 0,
      percent: Math.round(((r.count || 0) / total) * 1000) / 10,
      barWidth: Math.round(((r.count || 0) / max) * 1000) / 10
    }
  })
})

// 前三名皇冠（三款造型，currentColor 继承徽标配色）：王者冠 / 贵族冠 / 骑士冠
const crownPaths = [
  // 第一名：三尖王冠，中尖最高，带底座横杠
  'M4 17 L3 7.5 L7.5 11 L12 4.5 L16.5 11 L21 7.5 L20 17 Q12 19 4 17 Z M5 19.2 Q12 20.8 19 19.2 L19 20.4 Q12 22 5 20.4 Z M12 2.6 a1.1 1.1 0 1 1 0 2.2 a1.1 1.1 0 1 1 0 -2.2 Z',
  // 第二名：圆弧贵族冠，冠顶三连弧
  'M5.5 16.5 Q4.2 8.5 9 7.2 L10 9.5 Q12 8.2 14 9.5 L15 7.2 Q19.8 8.5 18.5 16.5 Q12 18.2 5.5 16.5 Z M5 18.8 Q12 20.4 19 18.8 L19 20 Q12 21.6 5 20 Z',
  // 第三名：单尖骑士冠，极简三角
  'M6 16.5 L12 6.5 L18 16.5 Q12 18.4 6 16.5 Z M5.6 18.6 Q12 20.2 18.4 18.6 L18.4 19.8 Q12 21.4 5.6 19.8 Z'
]

// 国内访问分布：中国地图热力图。后端条目含省级简称与地级市，先归一化到省级
// （城市并入所属省），海外与未知来源不上图；仅展示省级 + 直辖市 + 港澳台。
// 地图 GeoJSON 已带南海诸岛扩展小图，主图裁掉南海范围避免挤压大陆畸变。
const chinaMapReady = ref(false)

fetch('/geo/china.json')
  .then((r) => r.json())
  .then((geoJson) => {
    echartsRegisterMap('china', geoJson)
    chinaMapReady.value = true
  })
  .catch((e) => console.error('加载中国地图数据失败', e))

const provinceStats = computed(() => {
  const items = (stats.value.geo_distribution || []).filter(
    g => g.country !== '其他' && g.country !== '海外'
  )
  const byProvince = new Map()
  for (const g of items) {
    const name = isProvinceFullName(g.country) ? g.country : toMapProvince(g.country)
    if (!name) continue
    byProvince.set(name, (byProvince.get(name) || 0) + (g.count || 0))
  }
  return [...byProvince.entries()]
    .map(([name, value]) => ({ name, value }))
    .sort((a, b) => b.value - a.value)
})

const provinceTotal = computed(() => provinceStats.value.reduce((s, p) => s + p.value, 0) || 1)

const chinaMapOption = computed(() => {
  if (!chinaMapReady.value) return {}
  const textColor = isDark.value ? '#a1a1aa' : '#52525b'

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      backgroundColor: isDark.value ? '#18181b' : '#ffffff',
      borderColor: isDark.value ? '#27272a' : '#e4e4e7',
      textStyle: { color: isDark.value ? '#fafafa' : '#09090b' },
      formatter: (params) => {
        const v = Number(params.value)
        if (!Number.isFinite(v) || v <= 0) return `<b>${params.name}</b><br/>暂无访问记录`
        const count = v.toLocaleString()
        const pct = ((v / provinceTotal.value) * 100).toFixed(1)
        return `<b>${params.name}</b><br/>访问 ${count} 次 · 占比 ${pct}%`
      }
    },
    visualMap: {
      min: 0,
      max: Math.max(...provinceStats.value.map(p => p.value), 1),
      left: '8px',
      bottom: '8px',
      calculable: false,
      text: ['高', '低'],
      textStyle: { color: textColor, fontSize: 10 },
      inRange: {
        // teal 系低饱和热力色带
        color: ['#e8fbf7', '#99f6e4', '#2dd4bf', '#14b8a6', '#0d9488', '#0f766e']
      }
    },
    series: [{
      name: '访问分布',
      type: 'map',
      map: 'china',
      roam: false,
      aspectScale: 0.75,
      // 保持完整版图，居中自适应容器，避免南海诸岛、新疆、黑龙江等被裁剪
      layoutCenter: ['50%', '50%'],
      layoutSize: '98%',
      selectedMode: false,
      data: provinceStats.value,
      emphasis: {
        label: { show: false },
        itemStyle: { areaColor: '#f97316' }
      }
    }]
  }
})

// 省份占比：南丁格尔玫瑰图。占比 <1% 的省份并入「其他」，其余样本全部展示
const roseOption = computed(() => {
  const textColor = isDark.value ? '#a1a1aa' : '#52525b'
  const gapColor = isDark.value ? '#18181b' : '#ffffff'
  const palette = ['#0f766e', '#0d9488', '#14b8a6', '#2dd4bf', '#5eead4', '#99f6e4', '#5eb8c9', '#94a3b8', '#a8a29e', '#cbd5e1', '#b9c4d0']

  const total = provinceTotal.value
  const main = []
  let tailSum = 0
  for (const p of provinceStats.value) {
    if (p.value / total < 0.01) { tailSum += p.value; continue }
    main.push(p)
  }
  if (tailSum > 0) main.push({ name: '其他', value: tailSum })

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      backgroundColor: isDark.value ? '#18181b' : '#ffffff',
      borderColor: isDark.value ? '#27272a' : '#e4e4e7',
      textStyle: { color: isDark.value ? '#fafafa' : '#09090b' },
      formatter: (params) => {
        const v = Number(params.value)
        const count = Number.isFinite(v) ? v.toLocaleString() : '0'
        const pct = ((v / provinceTotal.value) * 100).toFixed(1)
        return `<b>${params.name}</b><br/>访问 ${count} 次 · 占比 ${pct}%`
      }
    },
    series: [{
      name: '省份占比',
      type: 'pie',
      roseType: 'area',
      radius: ['15%', '70%'],
      center: ['50%', '50%'],
      data: main.map((p, i) => ({
        name: p.name,
        value: p.value,
        itemStyle: {
          color: p.name === '其他' ? '#94a3b8' : palette[i % palette.length]
        }
      })),
      label: { color: textColor, fontSize: 10, formatter: '{b}' },
      labelLine: { length: 6, length2: 8, lineStyle: { color: textColor } },
      itemStyle: { borderRadius: 3, borderColor: gapColor, borderWidth: 1.5 }
    }]
  }
})

const trendOption = computed(() => {
  const textColor = isDark.value ? '#a1a1aa' : '#52525b'
  const splitLineColor = isDark.value ? '#27272a' : '#e4e4e7'
  // 柱体底色与悬浮层背景（亮暗两套）
  const barBase = isDark.value ? 'rgba(56, 189, 248, 0.35)' : 'rgba(14, 165, 233, 0.25)'
  const tooltipBg = isDark.value ? '#18181b' : '#ffffff'

  if (!stats.value.daily_stats) return {}

  const rawData = [...stats.value.daily_stats].reverse()
  const dates = rawData.map(d => d.date.slice(5))
  const visits = rawData.map(d => d.visit_count || 0)
  const downloads = rawData.map(d => d.download_count || 0)
  const traffic = rawData.map(d => d.traffic_bytes || 0)

  // 7 日移动平均：反映中长期趋势，过滤单日波动（窗口不足时用现有均值）
  const movingAvg = visits.map((_, i) => {
    const win = visits.slice(Math.max(0, i - 6), i + 1)
    return Math.round(win.reduce((s, v) => s + v, 0) / win.length)
  })

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: tooltipBg,
      borderColor: splitLineColor,
      textStyle: { color: isDark.value ? '#fafafa' : '#09090b' },
      formatter: (params) => {
        let html = `<b>${params[0].axisValue}</b>`
        params.forEach(p => {
          const val = p.seriesName === '下载流量' ? formatBytes(p.value) : Number(p.value).toLocaleString()
          html += `<br/>${p.marker} ${p.seriesName}: ${val}`
        })
        return html
      }
    },
    legend: {
      data: ['下载量', '访问量', '访问 7 日均线', '下载流量'],
      textStyle: { color: textColor, fontSize: 11 },
      itemWidth: 14,
      itemHeight: 8,
      bottom: 0
    },
    grid: {
      left: '10px', right: '10px', bottom: '32px', top: '20px', containLabel: true
    },
    xAxis: {
      type: 'category',
      data: dates,
      axisLine: { lineStyle: { color: splitLineColor } },
      axisTick: { show: false },
      axisLabel: { color: textColor, fontSize: 10 }
    },
    yAxis: [
      {
        type: 'value',
        name: '次数',
        nameTextStyle: { color: textColor, fontSize: 10, padding: [0, 0, 0, -12] },
        splitLine: { lineStyle: { type: 'dashed', color: splitLineColor } },
        axisLine: { show: false },
        axisLabel: { color: textColor, fontSize: 10 }
      },
      {
        type: 'value',
        name: '流量',
        nameTextStyle: { color: textColor, fontSize: 10, padding: [0, -20, 0, 0] },
        splitLine: { show: false },
        axisLine: { show: false },
        axisLabel: {
          color: textColor,
          fontSize: 10,
          formatter: (v) => formatBytes(v)
        }
      }
    ],
    series: [
      // 下载量：渐变圆角柱（底层）
      {
        name: '下载量',
        type: 'bar',
        barMaxWidth: 14,
        data: downloads,
        itemStyle: {
          borderRadius: [4, 4, 0, 0],
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: '#0ea5e9' },
              { offset: 1, color: barBase }
            ]
          }
        }
      },
      // 访问量 7 日均线：细虚线，压在柱上表现趋势
      {
        name: '访问 7 日均线',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: movingAvg,
        lineStyle: { color: '#64748b', width: 1.5, type: 'dashed' },
        itemStyle: { color: '#64748b' }
      },
      // 访问量：细面积线（主体）
      {
        name: '访问量',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: visits,
        lineStyle: { color: '#f97316', width: 2 },
        itemStyle: { color: '#f97316' },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(249, 115, 22, 0.25)' },
              { offset: 1, color: 'rgba(249, 115, 22, 0)' }
            ]
          }
        }
      },
      // 下载流量：右轴细线 + 极淡填充
      {
        name: '下载流量',
        type: 'line',
        smooth: true,
        symbol: 'none',
        yAxisIndex: 1,
        data: traffic,
        lineStyle: { color: '#10b981', width: 1.5 },
        itemStyle: { color: '#10b981' },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(16, 185, 129, 0.15)' },
              { offset: 1, color: 'rgba(16, 185, 129, 0)' }
            ]
          }
        }
      }
    ]
  }
})

const refreshBandwidth = async () => {
  try {
    const res = await getBandwidth()
    bandwidth.value = res.data
  } catch (e) {
    console.error('Failed to load bandwidth', e)
  }
}

onMounted(async () => {
  try {
    const statsRes = await getStats()
    stats.value = statsRes.data
  } catch (e) {
    console.error('Failed to load data', e)
  } finally {
    loading.value = false
  }

  await refreshBandwidth()
  startBandwidthPolling()
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onUnmounted(() => {
  stopBandwidthPolling()
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <div class="space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">数据洞察</h1>
      <p class="mt-1 text-sm text-muted-foreground">全站访问与下载动态，实时呈现。</p>
    </div>

    <div v-if="loading" class="space-y-6">
      <Card class="shadow-sm">
        <CardHeader class="pb-2"><Skeleton class="h-5 w-24" /></CardHeader>
        <CardContent>
          <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
            <div v-for="i in 5" :key="i"><Skeleton class="h-12 w-28" /></div>
          </div>
          <div class="mt-4"><Skeleton class="h-2 w-full rounded-full" /></div>
        </CardContent>
      </Card>
      <div class="grid gap-4 lg:grid-cols-7">
        <Card class="lg:col-span-4 h-[420px] shadow-sm">
          <CardHeader><Skeleton class="h-5 w-32" /></CardHeader>
          <CardContent class="h-full"><Skeleton class="h-full w-full rounded" /></CardContent>
        </Card>
        <Card class="lg:col-span-3 h-[420px] shadow-sm">
          <CardHeader><Skeleton class="h-5 w-32" /></CardHeader>
          <CardContent class="h-full"><Skeleton class="h-full w-full rounded" /></CardContent>
        </Card>
      </div>
      <Card class="shadow-sm">
        <CardHeader><Skeleton class="h-5 w-40" /></CardHeader>
        <CardContent class="h-[350px]"><Skeleton class="h-full w-full rounded" /></CardContent>
      </Card>
    </div>

    <template v-else>
      <div class="grid gap-4 lg:grid-cols-2">
        <!-- 站点概览：与带宽状态卡同款布局，指标列 + 磁盘占用进度条 -->
        <Card class="shadow-sm">
          <CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle class="flex items-center gap-2 text-base">
              <Activity weight="duotone" class="h-4 w-4 text-primary" />
              站点概览
            </CardTitle>
            <span class="text-xs text-muted-foreground">运行 {{ stats.total_days ?? '-' }} 天</span>
          </CardHeader>
          <CardContent>
            <div class="grid grid-cols-2 gap-4 2xl:grid-cols-3">
            <div>
              <p class="text-xs text-muted-foreground">总访问量</p>
              <div class="flex items-baseline gap-2">
                <p class="text-2xl font-bold">{{ stats.total_visits?.toLocaleString() || '-' }}</p>
                <span
                  v-if="visitGrowth !== 0"
                  class="inline-flex items-center gap-0.5 text-xs font-medium"
                  :class="visitGrowth >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'"
                >
                  <ArrowUp weight="duotone" v-if="visitGrowth >= 0" class="h-3 w-3" />
                  <ArrowDown weight="duotone" v-else class="h-3 w-3" />
                  {{ Math.abs(visitGrowth).toFixed(1) }}%
                </span>
              </div>
              <p class="mt-1 text-xs text-muted-foreground">近 30 日 {{ stats.last_30_visits?.toLocaleString() || 0 }}</p>
            </div>
            <div>
              <p class="text-xs text-muted-foreground">总下载量</p>
              <div class="flex items-baseline gap-2">
                <p class="text-2xl font-bold">{{ stats.total_downloads?.toLocaleString() || '-' }}</p>
                <span
                  v-if="downloadGrowth !== 0"
                  class="inline-flex items-center gap-0.5 text-xs font-medium"
                  :class="downloadGrowth >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'"
                >
                  <ArrowUp weight="duotone" v-if="downloadGrowth >= 0" class="h-3 w-3" />
                  <ArrowDown weight="duotone" v-else class="h-3 w-3" />
                  {{ Math.abs(downloadGrowth).toFixed(1) }}%
                </span>
              </div>
              <p class="mt-1 text-xs text-muted-foreground">近 30 日 {{ stats.last_30_downloads?.toLocaleString() || 0 }}</p>
            </div>
            <div>
              <p class="text-xs text-muted-foreground">总流量</p>
              <p class="text-2xl font-bold">{{ formatBytes(stats.total_traffic_bytes) }}</p>
              <p class="mt-1 text-xs text-muted-foreground">近 30 日 {{ formatBytes(stats.last_30_traffic_bytes) }}</p>
            </div>
            <div>
              <p class="text-xs text-muted-foreground">覆盖省份</p>
              <p class="text-2xl font-bold">{{ provinceCount || '-' }}</p>
              <p class="mt-1 text-xs text-muted-foreground">省级行政区</p>
            </div>
            <div>
              <p class="text-xs text-muted-foreground">磁盘占用</p>
              <p class="text-2xl font-bold">{{ diskPercentage }}<span class="text-sm font-normal text-muted-foreground">%</span></p>
              <p class="mt-1 text-xs text-muted-foreground">{{ formatBytes(stats.disk?.used) }} / {{ formatBytes(stats.disk?.total) }}</p>
            </div>
          </div>
          <div class="mt-4">
            <div class="mb-1 flex items-center justify-between text-xs">
              <span class="text-muted-foreground">磁盘使用率</span>
              <span class="font-medium" :class="diskPercentage > 90 ? 'text-red-600 dark:text-red-400' : diskPercentage > 75 ? 'text-amber-600 dark:text-amber-400' : 'text-green-600 dark:text-green-400'">{{ diskPercentage }}%</span>
            </div>
            <div class="h-2 w-full overflow-hidden rounded-full bg-secondary">
              <div
                class="h-full rounded-full transition-all duration-500"
                :class="diskPercentage > 90 ? 'bg-red-500' : diskPercentage > 75 ? 'bg-amber-500' : 'bg-green-500'"
                :style="{ width: `${diskPercentage}%` }"
              ></div>
            </div>
          </div>
        </CardContent>
        </Card>

        <Card class="shadow-sm">
          <CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle class="flex items-center gap-2 text-base">
              <Gauge weight="duotone" class="h-4 w-4 text-primary" />
              服务器带宽状态
            </CardTitle>
            <span class="text-xs text-muted-foreground">每 5 秒自动刷新</span>
          </CardHeader>
          <CardContent>
            <div class="grid grid-cols-2 gap-4 2xl:grid-cols-3">
              <div>
                <p class="text-xs text-muted-foreground">当前带宽</p>
                <p class="text-2xl font-bold">
                  <span v-if="isBandwidthIdle" class="text-muted-foreground">空闲中</span>
                  <template v-else>
                    {{ formatMbps(bandwidth.current_bandwidth_mbps) }}
                    <span class="text-sm font-normal text-muted-foreground">Mbps</span>
                  </template>
                </p>
                <p class="mt-1 text-xs text-muted-foreground">{{ isBandwidthIdle ? '当前无传输任务' : '正在向外传输数据' }}</p>
              </div>
              <div>
                <p class="text-xs text-muted-foreground">峰值上限</p>
                <p class="text-2xl font-bold">{{ bandwidth.peak_bandwidth_mbps ?? '-' }} <span class="text-sm font-normal text-muted-foreground">Mbps</span></p>
                <p class="mt-1 text-xs text-muted-foreground">配置的带宽上限</p>
              </div>
              <div>
                <p class="text-xs text-muted-foreground">峰值（已观测）</p>
                <p class="text-2xl font-bold">{{ formatMbps(bandwidth.peak_observed_mbps) }} <span class="text-sm font-normal text-muted-foreground">Mbps</span></p>
                <p class="mt-1 text-xs text-muted-foreground">历史最高记录</p>
              </div>
              <div>
                <p class="text-xs text-muted-foreground">近1分钟下载</p>
                <p class="text-2xl font-bold">{{ bandwidth.recent_downloads ?? bandwidth.active_downloads ?? '-' }}</p>
                <p class="mt-1 text-xs text-muted-foreground">
                  活跃下载 <span :class="bandwidth.active_downloads ? 'text-emerald-600 dark:text-emerald-400' : ''">{{ bandwidth.active_downloads || 0 }}</span> 个
                </p>
              </div>
              <div>
                <p class="text-xs text-muted-foreground">累计传输</p>
                <p class="text-2xl font-bold">{{ formatBytes(bandwidth.total_bytes_served) }}</p>
                <p class="mt-1 text-xs text-muted-foreground">服务启动以来累计</p>
              </div>
            </div>
            <div class="mt-4">
              <div class="mb-1 flex items-center justify-between text-xs">
                <span class="text-muted-foreground">带宽利用率</span>
              <span v-if="isBandwidthIdle" class="font-medium text-green-600 dark:text-green-400">空闲</span>
              <span v-else class="font-medium" :class="bandwidthUtilization > 90 ? 'text-red-600 dark:text-red-400' : bandwidthUtilization > 75 ? 'text-amber-600 dark:text-amber-400' : 'text-green-600 dark:text-green-400'">{{ bandwidthUtilization }}%</span>
            </div>
            <div class="h-2 w-full overflow-hidden rounded-full bg-secondary">
              <div
                class="h-full rounded-full transition-all duration-500"
                :class="isBandwidthIdle ? 'bg-secondary' : bandwidthUtilization > 90 ? 'bg-red-500' : bandwidthUtilization > 75 ? 'bg-amber-500' : 'bg-green-500'"
                :style="{ width: isBandwidthIdle ? '0%' : `${bandwidthUtilization}%` }"
              ></div>
            </div>
          </div>
        </CardContent>
        </Card>
      </div>

      <div class="grid gap-4 lg:grid-cols-7">
        <!-- 最近 30 天趋势：与热门排行并排 -->
        <Card class="lg:col-span-4 shadow-sm">
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <BarChart3 weight="duotone" class="h-4 w-4 text-orange-500" />
              最近 30 天趋势
            </CardTitle>
            <CardDescription>下载柱与访问曲线叠加，7 日均值虚线反映中长期走势。</CardDescription>
          </CardHeader>
          <CardContent class="pl-2">
            <div class="h-[350px] w-full">
              <VChart class="chart" :option="trendOption" autoresize />
            </div>
          </CardContent>
        </Card>

        <Card class="lg:col-span-3 flex flex-col shadow-sm">
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <TrendingUp weight="duotone" class="h-4 w-4 text-green-500" />
              热门资源排行
            </CardTitle>
            <CardDescription>下载量最高的启动器与插件，皇冠属于前三名。</CardDescription>
          </CardHeader>
          <CardContent class="flex-1 overflow-hidden">
            <div class="space-y-1.5">
              <div
                v-for="(item, i) in topDownloads"
                :key="`download-${item.key}`"
                class="flex items-center gap-3 rounded-lg border bg-card px-3 py-2.5 transition-colors hover:bg-accent/50"
              >
                <!-- 启动器 logo（无 logo 回退首字符），前三名叠加专属皇冠角标 -->
                <div class="relative shrink-0">
                  <img
                    v-if="item.logo"
                    :src="item.logo"
                    alt=""
                    class="h-8 w-8 rounded-md border object-cover"
                    loading="lazy"
                  />
                  <div
                    v-else
                    class="flex h-8 w-8 items-center justify-center rounded-md border bg-muted text-xs font-bold text-muted-foreground"
                  >
                    {{ item.name.slice(0, 1) }}
                  </div>
                  <span
                    v-if="i < 3"
                    class="absolute -right-1.5 -top-1.5 flex h-4 w-4 items-center justify-center rounded-full border border-border bg-background shadow-sm"
                    :class="i === 0 ? 'text-amber-500' : i === 1 ? 'text-zinc-400' : 'text-orange-600'"
                  >
                    <svg viewBox="0 0 24 24" fill="currentColor" class="h-3 w-3" aria-hidden="true">
                      <path :d="crownPaths[i]" />
                    </svg>
                  </span>
                </div>

                <div class="min-w-0 flex-1">
                  <div class="flex items-baseline justify-between gap-2">
                    <p class="truncate text-sm font-medium leading-none">{{ item.name }}</p>
                    <div class="flex shrink-0 items-baseline gap-1.5">
                      <span class="text-sm font-bold tabular-nums">{{ item.count.toLocaleString() }}</span>
                      <span class="w-10 text-right text-[10px] tabular-nums text-muted-foreground">{{ item.percent }}%</span>
                    </div>
                  </div>
                  <!-- 占比条：以第一名长度为 100% 基准 -->
                  <div class="mt-1.5 h-1.5 w-full overflow-hidden rounded-full bg-muted">
                    <div
                      class="h-full rounded-full bg-primary/60 transition-all duration-500"
                      :style="{ width: `${item.barWidth}%` }"
                    ></div>
                  </div>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <!-- 访问分布：整行收尾，卡内左「标题+地图」、右「标题+说明+玫瑰图」，两列头部对齐、图表等高 -->
      <Card class="shadow-sm">
        <CardContent class="grid gap-6 p-5 lg:grid-cols-2 lg:gap-8">
          <div class="min-w-0">
            <div class="flex items-center gap-2 text-base font-semibold">
              <MapPin weight="duotone" class="h-4 w-4 text-primary" />
              国内访问分布
            </div>
            <p class="mt-1 text-sm text-muted-foreground">
              按国内访问来源省份统计，海外合并为「海外」，城市级访问并入所属省份展示。
            </p>
            <div class="mt-4">
              <div v-if="chinaMapReady" class="h-[360px] sm:h-[420px] w-full">
                <VChart class="chart" :option="chinaMapOption" autoresize />
              </div>
              <div v-else class="flex h-[360px] sm:h-[420px] items-center justify-center">
                <Skeleton class="h-full w-full rounded" />
              </div>
            </div>
          </div>

          <!-- 右列：标题与左列对齐，说明文字移动端左对齐/桌面端右对齐，玫瑰图移动端定高、桌面端等高 -->
          <div class="flex min-w-0 flex-col">
            <div class="flex items-center gap-2 text-base font-semibold">
              <ChartPie weight="duotone" class="h-4 w-4 text-teal-500" />
              省份访问占比
            </div>
            <div class="mt-1 space-y-0.5 text-left sm:text-right">
              <p class="text-xs leading-relaxed text-muted-foreground">
                扇区面积对应访问量占比，展示全部省级行政区样本。
              </p>
              <p class="text-xs leading-relaxed text-muted-foreground">
                占比不足 1% 的省份并入「其他」；城市级访问已归并至所属省份。
              </p>
              <p class="text-xs leading-relaxed text-muted-foreground">
                地图仅示意访问热度，不代表任何领土立场；海外及未知来源不在图中展示。
              </p>
            </div>
            <div class="mt-4 h-[360px] sm:h-[420px] lg:h-auto lg:min-h-0 lg:flex-1">
              <VChart class="chart" :option="roseOption" autoresize />
            </div>
          </div>
        </CardContent>
      </Card>
    </template>
  </div>
</template>

<style scoped>
.chart {
  height: 100%;
  width: 100%;
}
</style>
