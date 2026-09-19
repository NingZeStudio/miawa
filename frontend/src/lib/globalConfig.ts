// __APP_VERSION__ 由 vite.config.js 构建期从 package.json 注入，单一版本源
declare const __APP_VERSION__: string

export const globalConfig = {
  site: {
    name: '柠泽资源站',
    nameFull: '柠泽资源站状态',
    nameEn: 'Lemwood Mirror',
    version: __APP_VERSION__,
    description: 'Minecraft 启动器与工具链的高速镜像下载服务',
    url: import.meta.env.VITE_SITE_URL || 'https://miawa.cn/',
    language: 'zh-CN',
    author: 'Lemwood & YanSui'
  },

  contact: {
    email: '3436464181@qq.com',
    qq: '3436464181',
    qqGroup: '1077373741'
  },

  links: {
    qqGroup: 'https://qun.qq.com/universal-share/share?ac=1&authKey=4tb0yflRdC0FjWZGhKHxfTxijnNc0crs399pxm782Lipx%2BoV6xmV%2BoA8%2BcQBUn7m&busi_data=eyJncm91cENvZGUiOiIxMTA0NjkwODM3IiwidG9rZW4iOiIrU2owaDFCMDRmVjJKaUdmdXA5M1RZNHlpVXZaYkZRTUh1bVA4V0ZVNlpGcUpuRjBrbHFRei9CeHI3NXZFb2xBIiwidWluIjoiMzQzNjQ2NDE4MSJ9&data=wr1XZ8qxXdkFazXoz2cuv1qBridVSJ8kVaDYI3vpZxTBbZNezhedFRTuCSeM_-GlzZRIeUHhE36zD5VXan3JqA&svctype=4&tempid=h5_group_info',
    githubRepo: 'https://github.com/leemwood/lemwood_mirror/',
    githubRepoNewWeb: 'https://github.com/JanePHPDev/lemwood_mirror_NewWeb',
    githubOrg: 'https://github.com/NingZeStudio/lemwood-mirror',
    logshare: 'https://logshare.cn/',
    beian: 'https://beian.miit.gov.cn/'
  },

  legal: {
    icp: '新ICP备2024015133号-6',
    cookieConsent: '本网站使用 Cookies 以优化您的体验。继续使用即表示您同意我们的 Cookie 政策。'
  },

  api: {
    baseUrl: import.meta.env.VITE_API_BASE_URL || '/api/v2',
    endpoints: {
      status: '/launchers',
      latest: '/latest',
      stats: '/stats',
      bandwidth: '/bandwidth',
      files: '/files',
      scan: '/admin/scans',
      powConfig: '/pow/config',
      downloadChallenge: '/downloads/challenge',
      downloadAuthorize: '/downloads/authorize',
      downloadPrepare: '/downloads/prepare',
      downloadLanding: '/downloads/landing',
      fileIntegrity: '/files/integrity',
      nodeInfo: '/node/info'
    }
  },

  launchers: {
    zl: {
      displayName: 'ZalithLauncher',
      logoUrl: new URL('../assets/images/34c1ec9e07f826df.webp', import.meta.url).href
    },
    zl2: {
      displayName: 'ZalithLauncher2',
      logoUrl: new URL('../assets/images/ee0028bd82493eb3.webp', import.meta.url).href
    },
    hmcl: {
      displayName: 'Hello Minecraft! Launcher',
      logoUrl: new URL('../assets/images/3835841e4b9b7abf.jpeg', import.meta.url).href
    },
    MG: {
      displayName: 'MobileGlues',
      logoUrl: new URL('../assets/images/3625548d2639a024.png', import.meta.url).href
    },
    fcl: {
      displayName: 'FoldCraftLauncher',
      logoUrl: new URL('../assets/images/dc5e0ee14d8f54f0.png', import.meta.url).href
    },
    FCL_Turnip: {
      displayName: 'Turnip 驱动插件列表',
      logoUrl: new URL('../assets/images/Image_1770256620866_693.webp', import.meta.url).href,
      isPluginList: true
    },
    NativeLibPlugin: {
      displayName: '原生插件列表',
      isPluginList: true
    },
    amcl: {
      displayName: 'Axe Minecraft Launcher',
      logoUrl: new URL('../assets/images/amcl_axe.png', import.meta.url).href
    },
    shizuku: {
      displayName: 'Shizuku',
      logoUrl: new URL('../assets/images/f7067665f073b4cc.png', import.meta.url).href
    },
    leaves: {
      displayName: 'Leaves 服务端',
      logoUrl: new URL('../assets/images/Leaves.png', import.meta.url).href
    },
    leaf: {
      displayName: 'Leaf 服务端',
      logoUrl: new URL('../assets/images/leaf.png', import.meta.url).href
    },
    luminol: {
      displayName: 'Luminol 服务端',
      logoUrl: new URL('../assets/images/c25a955166388e1257c23d01c78a62e6.webp', import.meta.url).href
    },
    fcl_dl: {
      displayName: 'FoldCraftLauncher (DL)',
      logoUrl: new URL('../assets/images/dc5e0ee14d8f54f0.png', import.meta.url).href
    },
    fcl_di: {
      displayName: '【直装版】FoldCraftLauncher',
      logoUrl: new URL('../assets/images/dc5e0ee14d8f54f0.png', import.meta.url).href
    },
    aamc: {
      displayName: 'Angel Aura Amethyst',
      logoUrl: new URL('../assets/images/amethyst.png', import.meta.url).href
    },
    axolotl: {
      displayName: 'Axolotl Launcher',
      logoUrl: new URL('../assets/images/axolotl.png', import.meta.url).href
    }
  },

  download: {
    baseUrl: '',
    sourceLabels: {
      home: 'home-latest-download',
      files: 'files-download',
      verify: 'verify-download'
    }
  },

  storage: {
    keys: {
      cookiesConsented: 'cookies-consented',
      // displayMode 存三态（light/dark/system），darkMode 存实际生效值（供 StatsView 的 useDark 读取）
      displayMode: 'display_mode',
      darkMode: 'vueuse-color-scheme',
      announcementShown: 'lemwood_announcement_shown',
      lastAnnouncementId: 'lemwood_last_announcement_id'
    }
  }
}
