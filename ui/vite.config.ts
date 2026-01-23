import { fileURLToPath, URL } from 'node:url'
import fs from 'node:fs'
import path from 'node:path'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'

import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

const rootDir = fileURLToPath(new URL('.', import.meta.url))

const twemojiSourceDir = path.resolve(rootDir, 'node_modules/@twemoji/api/assets')

const copyTwemojiAssets = (targetDir: string) => {
  const targetSvgDir = path.join(targetDir, 'svg')
  if (!fs.existsSync(twemojiSourceDir)) {
    return
  }
  if (fs.existsSync(targetSvgDir)) {
    return
  }
  fs.mkdirSync(targetDir, { recursive: true })
  fs.cpSync(twemojiSourceDir, targetDir, { recursive: true })
}

const twemojiAssetsPlugin = () => {
  let outDir = 'dist'
  return {
    name: 'sealchat-copy-twemoji-assets',
    configResolved(config: { build?: { outDir?: string } }) {
      outDir = config.build?.outDir || 'dist'
    },
    buildStart() {
      copyTwemojiAssets(path.resolve(rootDir, 'public/twemoji'))
    },
    closeBundle() {
      copyTwemojiAssets(path.resolve(rootDir, outDir, 'twemoji'))
    }
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  base: './',
  build: {
    assetsInlineLimit: 0,
  },
  css: {
    preprocessorOptions: {
      scss: {
        api: 'modern-compiler',
      },
    },
  },
  plugins: [
    twemojiAssetsPlugin(),
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
  }
})
