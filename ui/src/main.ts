import './assets/main.css'

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { i18n, setLocale, setLocaleByNavigatorWithStorage } from './lang'

import App from './App.vue'
import router from './router'
import { useDisplayStore } from './stores/display'
import { startFontSurfaceAutoMarking } from './services/font/fontSurfaceAdapter'

const app = createApp(App)
const pinia = createPinia()

app.use(i18n)
app.use(pinia)
app.use(router)

import '@imengyu/vue3-context-menu/lib/vue3-context-menu.css'
import ContextMenu from '@imengyu/vue3-context-menu'

app.use(ContextMenu)

setLocaleByNavigatorWithStorage()

import './assets/main.css'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/zh-cn'
import 'dayjs/locale/ja'

dayjs.locale(document.documentElement.lang);
dayjs.extend(relativeTime)

import { api } from './stores/_config'
import { useUserStore } from './stores/user'
import { useChatStore } from './stores/chat'

router.beforeEach(async (to, from, next) => {
  // 允许未登录访问的公开路由
  const publicRoutes = ['user-signin', 'user-signup', 'password-recovery', 'world-private-hint'];
  if (publicRoutes.includes(to.name as string)) {
    return next();
  }

  const worldId = typeof to.params.worldId === 'string' ? to.params.worldId.trim() : '';
  const channelId = typeof to.params.channelId === 'string' ? to.params.channelId.trim() : '';

  const user = useUserStore();
  const chat = useChatStore();
  const result = await user.checkUserSession();
  if (result === 'ok' || result === 'network-error') {
    if (result === 'network-error') {
      console.warn('网络不稳定，跳过登录状态刷新');
    }
    if (to.name === 'world-channel' && worldId) {
      try {
        const resp = await api.get(`/api/v1/worlds/${worldId}`);
        if (resp?.data?.isMember) {
          chat.disableObserverMode();
        } else {
          chat.enableObserverMode(worldId, channelId);
        }
        return next();
      } catch (error: any) {
        const status = error?.response?.status;
        if (status === 403 || status === 404) {
          return next({
            name: 'world-private-hint',
            params: { worldId },
            query: { redirect: to.fullPath },
          });
        }
        try {
          await api.get(`/api/v1/public/worlds/${worldId}`);
          chat.enableObserverMode(worldId, channelId);
          return next();
        } catch {
          return next();
        }
      }
    }
    chat.disableObserverMode();
    return next();
  }

  if (to.name === 'world-channel' && worldId) {
    try {
      await api.get(`/api/v1/public/worlds/${worldId}`);
      chat.enableObserverMode(worldId, channelId);
      return next();
    } catch {
      return next({
        name: 'world-private-hint',
        params: { worldId },
        query: { redirect: to.fullPath },
      });
    }
  }

  next({ name: 'user-signin', query: { redirect: to.fullPath } })
  // window.location.href = '//' + window.location.hostname + ":4455/login";
  return;
})

// import AutoImport from 'unplugin-auto-import/vite'
// import { VueHooksPlusResolver } from '@vue-hooks-plus/resolvers'

// export const AutoImportDeps = () =>
//   AutoImport({
//     imports: ['vue', 'vue-router'],
//     include: [/\.[tj]sx?$/, /\.vue$/, /\.vue\?vue/, /\.md$/],
//     dts: 'src/auto-imports.d.ts',
//     resolvers: [VueHooksPlusResolver()],
//   })

// 这几句详见 https://www.naiveui.com/zh-CN/os-theme/docs/style-conflict
const meta = document.createElement('meta')
meta.name = 'naive-ui-style'
document.head.appendChild(meta)

const displayStore = useDisplayStore(pinia)
displayStore.applyTheme()
displayStore.restoreGlobalFontAsset()
  .then(() => {
    displayStore.applyTheme()
  })
  .catch((error) => {
    console.warn('恢复缓存字体失败，继续使用默认字体链', error)
    displayStore.applyTheme()
  })

app.mount('#app')
startFontSurfaceAutoMarking()
