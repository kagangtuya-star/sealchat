<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import type { ExportPayload, ViewerManifest, ExportMessage, DisplayOptions } from './types'

const payload = window.__EXPORT_DATA__ as ExportPayload | undefined
const manifest = window.__EXPORT_INDEX__ as ViewerManifest | undefined

const mode = computed<'part' | 'index' | 'empty'>(() => {
  if (payload) return 'part'
  if (manifest) return 'index'
  return 'empty'
})

const display = reactive<Required<DisplayOptions>>({
  layout: payload?.display_options?.layout ?? 'bubble',
  palette: payload?.display_options?.palette ?? 'night',
  showAvatar: payload?.display_options?.showAvatar ?? true,
  mergeNeighbors: payload?.display_options?.mergeNeighbors ?? true,
})

watch(
  () => ({ ...display }),
  () => {
    const root = document.documentElement
    root.dataset.viewerPalette = display.palette
    root.dataset.viewerLayout = display.layout
  },
  { immediate: true, deep: true },
)

const keyword = ref('')
const regexMode = ref(false)
const caseSensitive = ref(false)
const searchError = ref('')
const currentHit = ref(0)

const hitEntries = computed(() => {
  if (!payload || !keyword.value.trim()) {
    searchError.value = ''
    return []
  }
  try {
    const matcher = buildMatcher(keyword.value, regexMode.value, caseSensitive.value)
    const results: { id: string; index: number }[] = []
    payload.messages.forEach((msg, index) => {
      if (matcher(resolveMessageHtml(msg)) || matcher(msg.sender_name ?? '')) {
        results.push({ id: msg.id, index })
      }
    })
    searchError.value = ''
    return results
  } catch (error: any) {
    searchError.value = error?.message ?? '搜索表达式无效'
    return []
  }
})

const hitIdSet = computed(() => new Set(hitEntries.value.map((entry) => entry.id)))

watch(hitEntries, (hits) => {
  currentHit.value = hits.length > 0 ? 0 : -1
  queueScrollToCurrent()
})

const totalMessages = computed(() => payload?.messages?.length ?? 0)
const activeHitLabel = computed(() => {
  if (!hitEntries.value.length || currentHit.value < 0) return '0 / 0'
  return `${currentHit.value + 1} / ${hitEntries.value.length}`
})

function queueScrollToCurrent() {
  nextTick(() => {
    const hit = hitEntries.value[currentHit.value]
    if (!hit) return
    const el = document.querySelector<HTMLElement>(`[data-message-id="${hit.id}"]`)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' })
      el.classList.add('search-hit-active')
      setTimeout(() => el.classList.remove('search-hit-active'), 1200)
    }
  })
}

function buildMatcher(term: string, useRegex: boolean, sensitive: boolean) {
  if (!useRegex) {
    const normalized = sensitive ? term : term.toLowerCase()
    return (value: string) => {
      const target = sensitive ? value : value.toLowerCase()
      return target.includes(normalized)
    }
  }
  const flags = sensitive ? 'g' : 'gi'
  const reg = new RegExp(term, flags)
  return (value: string) => reg.test(value)
}

const highlightedMessages = computed(() => {
  if (!payload) return []
  if (!keyword.value.trim()) return payload.messages
  const matcher = buildMatcher(keyword.value, regexMode.value, caseSensitive.value)
  return payload.messages.map((msg) => ({
    ...msg,
    highlighted: matcher(resolveMessageHtml(msg)),
  }))
})

function nextHit(delta: number) {
  if (!hitEntries.value.length) return
  currentHit.value = (currentHit.value + delta + hitEntries.value.length) % hitEntries.value.length
  queueScrollToCurrent()
}

function formatTime(value?: string) {
  if (!value) return '--'
  try {
    return new Date(value).toLocaleString()
  } catch {
    return value
  }
}

function messageClass(msg: ExportMessage) {
  return [
    'viewer-message',
    hitIdSet.value.has(msg.id) ? 'search-hit' : '',
    display.layout === 'compact' ? 'viewer-message--compact' : '',
  ]
}

function resolveAvatarSrc(msg: ExportMessage) {
  const value = String(msg.sender_avatar || '').trim()
  if (!value || value.startsWith('id:')) {
    return ''
  }
  return value
}

function resolveAvatarFallback(name?: string) {
  const source = String(name || '').trim()
  if (!source) return '匿'
  return source.slice(0, 1).toUpperCase()
}

function resolveMessageHtml(msg: ExportMessage) {
  return msg.content_html || msg.content || ''
}

const manifestParts = computed(() => manifest?.parts ?? [])
</script>

<template>
  <div class="viewer-root" :data-palette="display.palette">
    <div class="viewer-shell" v-if="mode === 'part' && payload">
      <header>
        <h1>{{ payload.channel_name }}</h1>
        <div class="viewer-meta">
          <span class="viewer-chip">分片 {{ payload.part_index }} / {{ payload.part_total }}</span>
          <span class="viewer-chip">消息 {{ totalMessages }}</span>
          <span class="viewer-chip">
            时间 {{ formatTime(payload.slice_start || payload.start_time) }} →
            {{ formatTime(payload.slice_end || payload.end_time) }}
          </span>
        </div>
      </header>

      <section class="viewer-controls">
        <input v-model="keyword" placeholder="关键词 / 正则表达式" />
        <label>
          <input type="checkbox" v-model="regexMode" />
          正则
        </label>
        <label>
          <input type="checkbox" v-model="caseSensitive" />
          区分大小写
        </label>
        <button type="button" @click="nextHit(-1)">上一条</button>
        <button type="button" @click="nextHit(1)">下一条</button>
        <span class="viewer-chip">{{ activeHitLabel }}</span>
      </section>

      <p v-if="searchError" class="viewer-chip" style="border-color: rgba(244,63,94,0.5); color: #fecdd3">
        {{ searchError }}
      </p>

      <section class="viewer-controls" style="margin-top: 1rem">
        <div>
          <label>版式：</label>
          <button type="button" @click="display.layout = 'bubble'">气泡</button>
          <button type="button" @click="display.layout = 'compact'">紧凑</button>
        </div>
        <div>
          <label>主题：</label>
          <button type="button" @click="display.palette = 'day'">日间</button>
          <button type="button" @click="display.palette = 'night'">夜间</button>
        </div>
      </section>

      <div class="message-list">
        <article
          v-for="msg in payload.messages"
          :key="msg.id"
          :data-message-id="msg.id"
          :class="messageClass(msg)"
        >
          <div class="viewer-message__avatar">
            <img v-if="resolveAvatarSrc(msg)" :src="resolveAvatarSrc(msg)" :alt="msg.sender_name || '匿名'" />
            <span v-else>{{ resolveAvatarFallback(msg.sender_name) }}</span>
          </div>
          <div class="viewer-message__main">
            <div class="viewer-message__header">
              <strong :style="msg.sender_color ? { color: msg.sender_color } : undefined">{{ msg.sender_name || '匿名' }}</strong>
              <span>{{ formatTime(msg.created_at as string) }}</span>
            </div>
            <div class="viewer-message__body" v-html="resolveMessageHtml(msg)"></div>
          </div>
        </article>
      </div>
    </div>

    <div class="viewer-shell" v-else-if="mode === 'index' && manifest">
      <header>
        <h1>{{ manifest.channel_name }}</h1>
        <div class="viewer-meta">
          <span class="viewer-chip">分片 {{ manifest.part_total }}</span>
          <span class="viewer-chip">消息 {{ manifest.total_messages }}</span>
          <span class="viewer-chip">切片 {{ manifest.slice_limit }}/并发 {{ manifest.max_concurrency }}</span>
        </div>
      </header>

      <div class="parts-grid">
        <div class="parts-card" v-for="part in manifestParts" :key="part.file">
          <h3>Part {{ part.part_index }}</h3>
          <p>消息：{{ part.messages }}</p>
          <p>范围：{{ formatTime(part.slice_start) }} → {{ formatTime(part.slice_end) }}</p>
          <p class="hash" v-if="part.sha256">SHA256: {{ part.sha256.slice(0, 10) }}⋯</p>
          <a :href="part.file">打开分片</a>
        </div>
      </div>
    </div>

    <div v-else class="viewer-shell viewer-empty">
      <p>未找到导出数据。请通过导出功能重新生成文件。</p>
    </div>
  </div>
</template>
