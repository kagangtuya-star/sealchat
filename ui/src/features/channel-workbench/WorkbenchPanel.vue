<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NButton, NDropdown, NIcon, NTooltip, type DropdownOption } from 'naive-ui'
import {
  AlertTriangle,
  ArrowBackUp,
  ArrowsMaximize,
  ArrowsMinimize,
  ChevronDown,
  ChevronUp,
  DotsVertical,
  ExternalLink,
  Eye,
  EyeOff,
  Focus2,
  Link,
  Presentation,
  AppWindow,
} from '@vicons/tabler'
import { AppsOutline } from '@vicons/ionicons5'
import { WORKBENCH_WEB_SANDBOX, workbenchSourceTypeLabel, type WorkbenchResolvedSource } from './sources'
import type { WorkbenchPanel } from './types'

// 单个面板：内容 iframe + 个人操作。iframe 在折叠、展开、聚焦时保持同一元素不重载；
// 弹出到浮窗时卸载，避免同一资源同时运行两个 SealChat 运行时。

const props = defineProps<{
  panel: WorkbenchPanel
  minimalActionsPlacement: 'inside' | 'outside'
  title: string
  resolved: WorkbenchResolvedSource
  collapsed: boolean
  focused: boolean
  spotlighted: boolean
  poppedOut: 'transient' | 'pinned' | null
  canManage: boolean
  hiddenForPlayers: boolean
  missingChannel: boolean
  /** 移动端标签模式中未激活的面板保持挂载但不显示；首次激活前不加载 iframe。 */
  mounted: boolean
}>()

const emit = defineEmits<{
  'toggle-collapse': []
  'toggle-focus': []
  popout: [transient: boolean]
  'take-back': []
  'open-window': []
  'toggle-visibility': []
  spotlight: []
}>()

const WEB_LOAD_TIMEOUT = 9000
const frameRef = ref<HTMLIFrameElement | null>(null)
const loaded = ref(false)
const loadTimedOut = ref(false)
const menuVisible = ref(false)
let loadTimer: number | null = null

const frameUrl = computed(() => {
  const resolved = props.resolved
  if (resolved.kind === 'web' && resolved.display === 'link') return ''
  return 'url' in resolved ? resolved.url : ''
})
const showFrame = computed(() => props.mounted && !props.poppedOut && !props.missingChannel && !!frameUrl.value)
const isWeb = computed(() => props.resolved.kind === 'web')
const canPopout = computed(() => props.resolved.kind === 'surface' || props.resolved.kind === 'chat' || (props.resolved.kind === 'web' && props.resolved.display !== 'link'))
const canOpenWindow = computed(() => props.resolved.kind !== 'unbound' && props.resolved.kind !== 'invalid' && !props.missingChannel)
const state = computed(() => {
  if (props.poppedOut) return 'popped'
  if (props.missingChannel || props.resolved.kind === 'invalid') return 'error'
  if (props.resolved.kind === 'unbound') return 'unbound'
  if (props.collapsed) return 'collapsed'
  return props.focused ? 'focused' : 'ready'
})

const clearLoadTimer = () => {
  if (loadTimer !== null) {
    window.clearTimeout(loadTimer)
    loadTimer = null
  }
}

watch([showFrame, frameUrl], ([visible]) => {
  clearLoadTimer()
  loaded.value = false
  loadTimedOut.value = false
  // 外部网站可能以 X-Frame-Options / CSP 拒绝嵌入，浏览器不会告知父页面；超时后给出新窗口打开的提示。
  if (visible && isWeb.value) loadTimer = window.setTimeout(() => { loadTimedOut.value = !loaded.value }, WEB_LOAD_TIMEOUT)
}, { immediate: true })

const handleLoad = () => {
  loaded.value = true
  loadTimedOut.value = false
  clearLoadTimer()
}

onBeforeUnmount(clearLoadTimer)

const toggleChatRibbon = () => {
  frameRef.value?.contentWindow?.postMessage({ type: 'sealchat.embed.toggleActionRibbon' }, window.location.origin)
}

const menuOptions = computed<DropdownOption[]>(() => {
  const options: DropdownOption[] = []
  if (canPopout.value && !props.poppedOut) {
    options.push({ key: 'popout-transient', label: '临时弹出（刷新后关闭）' })
    options.push({ key: 'popout-pinned', label: '固定弹出（刷新后保留）' })
  }
  if (props.poppedOut) options.push({ key: 'take-back', label: '收回到布局' })
  if (canOpenWindow.value) options.push({ key: 'open-window', label: '在新窗口打开' })
  if (props.canManage) {
    options.push({ type: 'divider', key: 'gm-divider' })
    options.push({ key: 'toggle-visibility', label: props.hiddenForPlayers ? '对玩家显示' : '对玩家隐藏' })
    options.push({ key: 'spotlight', label: props.spotlighted ? '已在聚焦中' : '聚焦给所有人', disabled: props.spotlighted })
  }
  return options
})

const handleMenu = (key: string) => {
  switch (key) {
    case 'popout-transient': emit('popout', true); break
    case 'popout-pinned': emit('popout', false); break
    case 'take-back': emit('take-back'); break
    case 'open-window': emit('open-window'); break
    case 'toggle-visibility': emit('toggle-visibility'); break
    case 'spotlight': emit('spotlight'); break
  }
}
</script>

<template>
  <section
    class="wb-panel"
    data-sc-ui="workbench-panel"
    :data-sc-panel-id="panel.id"
    :data-sc-panel-type="panel.source.type === 'surface' ? panel.source.surface : panel.source.type"
    :data-sc-panel-state="state"
    :data-sc-actions-placement="minimalActionsPlacement"
    :class="{
      'is-collapsed': collapsed && !focused,
      'is-focused': focused,
      'is-spotlighted': spotlighted,
      'is-minimal': panel.chrome === 'minimal',
      'is-actions-open': menuVisible,
      'is-hidden-for-players': hiddenForPlayers,
    }"
    :aria-label="`${title}${hiddenForPlayers ? '（对玩家隐藏）' : ''}${spotlighted ? '（GM 聚焦中）' : ''}`"
  >
    <header class="wb-panel__header" @dblclick="emit('toggle-focus')">
      <span class="wb-panel__title" :title="title" role="heading" aria-level="2">
        <NIcon v-if="spotlighted" class="wb-panel__badge" aria-hidden="true"><Presentation /></NIcon>
        <NIcon v-if="hiddenForPlayers" class="wb-panel__badge is-muted" aria-hidden="true"><EyeOff /></NIcon>
        {{ title }}
        <span v-if="hiddenForPlayers" class="sr-only">（对玩家隐藏）</span>
      </span>
      <span class="wb-panel__actions" @dblclick.stop>
        <NTooltip v-if="resolved.kind === 'chat' && showFrame" :delay="400">
          <template #trigger>
            <button type="button" class="wb-panel__action" aria-label="更多跑团功能" @click="toggleChatRibbon">
              <NIcon><AppsOutline /></NIcon>
            </button>
          </template>
          更多跑团功能
        </NTooltip>
        <button
          v-if="canManage"
          type="button"
          class="wb-panel__action"
          :aria-label="hiddenForPlayers ? '对玩家显示' : '对玩家隐藏'"
          :title="hiddenForPlayers ? '当前对玩家隐藏，点击显示' : '当前对玩家可见，点击隐藏'"
          @click="emit('toggle-visibility')"
        >
          <NIcon><EyeOff v-if="hiddenForPlayers" /><Eye v-else /></NIcon>
        </button>
        <button
          v-if="!focused"
          type="button"
          class="wb-panel__action"
          :aria-label="collapsed ? '展开面板内容' : '折叠面板'"
          :title="collapsed ? '展开内容' : '折叠'"
          :aria-expanded="!collapsed"
          @click="emit('toggle-collapse')"
        >
          <NIcon><ChevronDown v-if="collapsed" /><ChevronUp v-else /></NIcon>
        </button>
        <button
          type="button"
          class="wb-panel__action"
          :aria-label="focused ? '退出展开' : '展开到整个工作台'"
          :title="focused ? '退出展开' : '展开（Focus）'"
          :aria-pressed="focused"
          @click="emit('toggle-focus')"
        >
          <NIcon><ArrowsMinimize v-if="focused" /><ArrowsMaximize v-else /></NIcon>
        </button>
        <button
          v-if="canPopout && !poppedOut"
          type="button"
          class="wb-panel__action"
          aria-label="临时弹出为浮窗"
          title="临时弹出为浮窗"
          @click="emit('popout', true)"
        >
          <NIcon><AppWindow /></NIcon>
        </button>
        <NDropdown v-model:show="menuVisible" trigger="click" :options="menuOptions" placement="bottom-end" @select="handleMenu">
          <button type="button" class="wb-panel__action" aria-label="更多面板操作" title="更多">
            <NIcon><DotsVertical /></NIcon>
          </button>
        </NDropdown>
      </span>
    </header>

    <div v-show="!collapsed || focused" class="wb-panel__body">
      <div v-if="poppedOut" class="wb-panel__placeholder">
        <NIcon size="22"><AppWindow /></NIcon>
        <p>已{{ poppedOut === 'pinned' ? '固定' : '临时' }}弹出为浮窗</p>
        <NButton size="small" @click="emit('take-back')">
          <template #icon><NIcon><ArrowBackUp /></NIcon></template>
          收回
        </NButton>
      </div>
      <div v-else-if="missingChannel" class="wb-panel__placeholder is-error" role="alert">
        <NIcon size="22"><AlertTriangle /></NIcon>
        <p>引用的频道不存在或你无权访问</p>
        <p v-if="canManage" class="wb-panel__hint">在设计模式中为该面板重新绑定频道</p>
      </div>
      <div v-else-if="resolved.kind === 'unbound'" class="wb-panel__placeholder">
        <NIcon size="22"><Link /></NIcon>
        <p>{{ workbenchSourceTypeLabel(panel.source) }}：{{ resolved.reason }}</p>
        <p v-if="canManage" class="wb-panel__hint">在设计模式中选择要显示的资源</p>
      </div>
      <div v-else-if="resolved.kind === 'invalid'" class="wb-panel__placeholder is-error" role="alert">
        <NIcon size="22"><AlertTriangle /></NIcon>
        <p>{{ resolved.reason }}</p>
      </div>
      <div v-else-if="resolved.kind === 'web' && resolved.display === 'link'" class="wb-panel__placeholder">
        <NIcon size="22"><ExternalLink /></NIcon>
        <p class="wb-panel__url">{{ resolved.url }}</p>
        <NButton size="small" type="primary" secondary @click="emit('open-window')">在新窗口打开</NButton>
      </div>
      <template v-else-if="showFrame">
        <iframe
          ref="frameRef"
          class="wb-panel__frame"
          :src="frameUrl"
          :title="title"
          :sandbox.attr="isWeb ? WORKBENCH_WEB_SANDBOX : undefined"
          :referrerpolicy="isWeb ? 'no-referrer' : undefined"
          :loading="isWeb ? 'lazy' : undefined"
          frameborder="0"
          allow="autoplay; clipboard-read; clipboard-write; fullscreen"
          @load="handleLoad"
        />
        <div v-if="isWeb && loadTimedOut" class="wb-panel__web-hint" role="status">
          <NIcon><AlertTriangle /></NIcon>
          <span>网页长时间未加载，可能拒绝被嵌入（X-Frame-Options / CSP）。</span>
          <NButton size="tiny" @click="emit('open-window')">新窗口打开</NButton>
        </div>
      </template>
      <div v-else class="wb-panel__placeholder is-idle">
        <NIcon size="22"><Focus2 /></NIcon>
        <p>切换到该面板后加载</p>
      </div>
    </div>
  </section>
</template>

<style scoped>
.wb-panel { position: relative; display: flex; flex-direction: column; min-width: 0; min-height: 0; overflow: hidden; border: 1px solid var(--sc-workbench-panel-border, var(--sc-border-mute, rgba(127, 127, 127, .2))); border-radius: var(--sc-workbench-panel-radius, 10px); color: var(--sc-text-primary, inherit); background: var(--sc-workbench-panel-bg, var(--sc-bg-surface, #fff)); box-shadow: var(--sc-workbench-panel-shadow, 0 1px 2px rgba(0, 0, 0, .06)); }
.wb-panel.is-collapsed { align-self: start; }
.wb-panel.is-spotlighted { border-color: var(--sc-workbench-accent, var(--primary-color, #3b82f6)); box-shadow: 0 0 0 2px color-mix(in srgb, var(--sc-workbench-accent, var(--primary-color, #3b82f6)) 45%, transparent); }
.wb-panel.is-hidden-for-players { border-style: dashed; }
.wb-panel__header { display: flex; flex: 0 0 34px; align-items: center; gap: 6px; min-width: 0; padding: 0 4px 0 10px; border-bottom: 1px solid var(--sc-border-mute, rgba(127, 127, 127, .16)); background: var(--sc-workbench-header-bg, var(--sc-bg-elevated, transparent)); user-select: none; }
.wb-panel.is-collapsed .wb-panel__header { border-bottom-color: transparent; }
.wb-panel.is-minimal { overflow: visible; border: 0; background: transparent; box-shadow: none; }
.wb-panel.is-minimal .wb-panel__body { width: 100%; overflow: hidden; border-radius: inherit; }
.wb-panel.is-minimal.is-collapsed { min-height: 40px; }
.wb-panel.is-minimal .wb-panel__header { position: absolute; z-index: 3; top: 6px; right: 6px; max-width: 100%; padding: 3px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--sc-workbench-header-bg, var(--sc-bg-elevated, #fff)) 88%, transparent); box-shadow: 0 2px 8px rgba(0, 0, 0, .12); backdrop-filter: blur(8px); -webkit-backdrop-filter: blur(8px); opacity: 0; pointer-events: none; transform: translateY(-2px); transition: opacity .15s ease, transform .15s ease; }
.wb-panel.is-minimal .wb-panel__title { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.wb-panel.is-minimal.is-collapsed .wb-panel__header { opacity: 1; pointer-events: auto; transform: none; }
.wb-panel.is-minimal.is-collapsed .wb-panel__title { position: static; width: auto; height: auto; max-width: min(180px, 45vw); clip: auto; }
.wb-panel.is-minimal .wb-panel__actions { flex-wrap: wrap; justify-content: flex-end; }
.wb-panel.is-minimal[data-sc-actions-placement='outside'] .wb-panel__header { top: -6px; right: -6px; }
.wb-panel.is-minimal:hover, .wb-panel.is-minimal:focus-within, .wb-panel.is-minimal.is-actions-open { z-index: 6; }
.wb-panel.is-minimal:hover .wb-panel__header, .wb-panel.is-minimal:focus-within .wb-panel__header, .wb-panel.is-minimal.is-actions-open .wb-panel__header { opacity: 1; pointer-events: auto; transform: translateY(0); }
@media (hover: none), (max-width: 699px) {
  .wb-panel.is-minimal .wb-panel__header, .wb-panel.is-minimal[data-sc-actions-placement='outside'] .wb-panel__header { top: 6px; right: 6px; opacity: 1; pointer-events: auto; transform: none; }
}
@media (prefers-reduced-motion: reduce) {
  .wb-panel.is-minimal .wb-panel__header { transition: none; }
}
.wb-panel__title { display: inline-flex; flex: 1; align-items: center; gap: 4px; min-width: 0; overflow: hidden; font-size: 13px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.wb-panel__badge { flex: none; color: var(--sc-workbench-accent, var(--primary-color, #3b82f6)); }
.wb-panel__badge.is-muted { color: var(--sc-text-secondary, #888); }
.wb-panel__actions { display: inline-flex; flex: none; gap: 1px; }
.wb-panel__action { display: inline-grid; width: 26px; height: 26px; padding: 0; place-items: center; border: 0; border-radius: 6px; color: var(--sc-text-secondary, inherit); background: transparent; cursor: pointer; }
.wb-panel__action:hover, .wb-panel__action:focus-visible { color: var(--sc-text-primary, inherit); background: var(--sc-bg-hover, rgba(127, 127, 127, .12)); }
.wb-panel__body { position: relative; flex: 1; min-height: 0; }
.wb-panel__frame { display: block; width: 100%; height: 100%; border: 0; background: var(--sc-bg-surface, transparent); }
.wb-panel.is-minimal .wb-panel__frame { background: transparent; }
.wb-panel__placeholder { display: flex; height: 100%; min-height: 96px; flex-direction: column; align-items: center; justify-content: center; gap: 6px; padding: 12px; color: var(--sc-text-secondary, #888); font-size: 13px; text-align: center; }
.wb-panel__placeholder p { margin: 0; }
.wb-panel__placeholder.is-error { color: var(--sc-color-error, #d03050); }
.wb-panel__hint { font-size: 12px; opacity: .8; }
.wb-panel__url { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.wb-panel__web-hint { position: absolute; right: 8px; bottom: 8px; left: 8px; display: flex; align-items: center; gap: 6px; padding: 6px 8px; border: 1px solid var(--sc-border-mute, rgba(127, 127, 127, .2)); border-radius: 8px; color: var(--sc-text-primary, inherit); background: var(--sc-bg-elevated, #fff); font-size: 12px; box-shadow: 0 4px 12px rgba(0, 0, 0, .12); }
.wb-panel__web-hint span { flex: 1; min-width: 0; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
</style>
