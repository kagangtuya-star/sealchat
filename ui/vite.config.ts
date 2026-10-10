import { fileURLToPath, URL } from 'node:url'

import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'

import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const devBackend = env.SEALCHAT_DEV_BACKEND?.trim() || 'http://127.0.0.1:13212'

  return {
    base: './',
    build: {
      assetsInlineLimit: 0,
    },
    worker: {
      format: 'es',
    },
    css: {
      preprocessorOptions: {
        scss: {
          api: 'modern-compiler',
        },
      },
    },
    plugins: [
      vue(),
      vueJsx(),
      AutoImport({
        imports: [
          'vue',
          {
            'naive-ui': [
              'useDialog',
              'useMessage',
              'useNotification',
              'useLoadingBar'
            ]
          }
        ]
      }),
      Components({
        resolvers: [NaiveUiResolver()]
      })
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url))
      }
    },
    server: {
      port: 13211,
      strictPort: true,
      proxy: {
        '/api': {
          target: devBackend,
          changeOrigin: true,
        },
        '/ws': {
          target: devBackend,
          changeOrigin: true,
          ws: true,
        },
        '/onebot': {
          target: devBackend,
          changeOrigin: true,
          ws: true,
        },
      },
    },
  }
})
