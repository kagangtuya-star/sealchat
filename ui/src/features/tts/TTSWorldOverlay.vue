<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useThemeVars } from 'naive-ui'
defineProps<{ title: string; wide?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const theme = useThemeVars()
const panel = ref<HTMLElement | null>(null)
let previousFocus: HTMLElement | null = null
onMounted(async () => {
  previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
  await nextTick()
  panel.value?.focus()
})
onBeforeUnmount(() => { if (previousFocus?.isConnected) previousFocus.focus() })
function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.stopPropagation(); emit('close'); return }
  if (event.key !== 'Tab' || !panel.value) return
  const items = Array.from(panel.value.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), a[href], [tabindex="0"]')).filter(item => item.getClientRects().length > 0)
  const first = items[0], last = items.at(-1)
  if (!first || !last) { event.preventDefault(); return }
  if (event.shiftKey && (document.activeElement === first || document.activeElement === panel.value)) { event.preventDefault(); last.focus() }
  else if (!event.shiftKey && (document.activeElement === last || document.activeElement === panel.value)) { event.preventDefault(); first.focus() }
}
</script>

<template>
  <Teleport to="body">
    <div class="tw-overlay" :style="{ '--tw-bg': theme.modalColor, '--tw-text': theme.textColor1, '--tw-border': theme.borderColor, '--tw-accent': theme.primaryColor }" @click.self="emit('close')">
      <section ref="panel" class="tw-panel" :class="{ 'tw-panel--wide': wide }" tabindex="-1" role="dialog" aria-modal="true" :aria-label="title" @keydown="keydown">
        <header class="tw-header"><h2>{{ title }}</h2><button type="button" aria-label="关闭" @click="emit('close')">✕</button></header>
        <div class="tw-body"><slot /></div>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.tw-overlay { position: fixed; inset: 0; z-index: 2600; display: flex; align-items: center; justify-content: center; padding: 24px; background: rgb(0 0 0 / 45%); color: var(--tw-text); }
.tw-panel { width: min(480px, 100%); max-height: calc(100dvh - 48px); display: flex; flex-direction: column; outline: none; border: 1px solid var(--tw-border); border-radius: 12px; background: var(--tw-bg); box-shadow: 0 24px 80px rgb(0 0 0 / 25%); }
.tw-panel--wide { width: min(1440px, 100%); height: min(900px, calc(100dvh - 48px)); }
.tw-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 18px 24px; border-bottom: 1px solid var(--tw-border); }
h2 { margin: 0; font-size: 18px; }
.tw-header button { border: none; background: transparent; color: inherit; cursor: pointer; font-size: 20px; padding: 4px 8px; }
.tw-body { min-height: 0; overflow: auto; padding: 24px; }
.tw-panel--wide .tw-body { flex: 1; display: flex; flex-direction: column; }
@media (max-width: 900px) {
  .tw-overlay { padding: 0; }
  .tw-panel--wide { width: 100%; height: 100dvh; max-height: 100dvh; border-radius: 0; border: none; }
  .tw-panel:not(.tw-panel--wide) { width: calc(100% - 24px); }
  .tw-header { padding: 16px; }
  .tw-body { padding: 16px; }
}
</style>
