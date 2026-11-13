<script setup lang="ts">
// 画布底部的审阅工具条：连续导航、审阅进度与批量应用。纯展示组件，状态由审阅中心持有。
import { computed } from 'vue';
import { ArrowsMaximize, ArrowsMinimize, Checks, ChevronLeft, ChevronRight, EyeOff, Keyboard } from '@vicons/tabler';

const props = defineProps<{
  /** 当前批注在导航序列中的位置（1 起，0 表示未选中）与序列长度。 */
  index: number;
  total: number;
  decided: number;
  all: number;
  accepted: number;
  safePending: number;
  canApply: boolean;
  applying: boolean;
  focused: boolean;
  narrow: boolean;
}>();
const emit = defineEmits<{
  (e: 'nav', step: 1 | -1): void;
  (e: 'apply'): void;
  (e: 'accept-safe'): void;
  (e: 'focus'): void;
  (e: 'hide'): void;
}>();

const percent = computed(() => (props.all ? Math.round((props.decided / props.all) * 100) : 0));
const SHORTCUTS: Array<[string, string]> = [
  ['J / K', '下一处 / 上一处'],
  ['A', '采纳并继续'],
  ['R', '放弃并继续'],
  ['U', '撤回决定'],
  ['Esc', '收起批注'],
];
</script>

<template>
  <div class="log-proof-bar" :class="{ 'is-narrow': narrow }" role="toolbar" aria-label="校对审阅">
    <div class="log-proof-bar__nav">
      <button type="button" class="log-proof-bar__icon" :disabled="!total" title="上一处 (K)" aria-label="上一处" @click="emit('nav', -1)">
        <n-icon :component="ChevronLeft" size="17" />
      </button>
      <span class="log-proof-bar__count" aria-live="polite">
        <strong>{{ index || '–' }}</strong><span>/ {{ total }}</span>
      </span>
      <button type="button" class="log-proof-bar__icon" :disabled="!total" title="下一处 (J)" aria-label="下一处" @click="emit('nav', 1)">
        <n-icon :component="ChevronRight" size="17" />
      </button>
    </div>

    <div v-if="!narrow" class="log-proof-bar__progress" :title="`已审 ${decided} / ${all}`">
      <span class="log-proof-bar__track"><i :style="{ width: `${percent}%` }" /></span>
      <span class="log-proof-bar__pct">{{ percent }}%</span>
    </div>

    <i class="log-proof-bar__sep" aria-hidden="true" />

    <button v-if="safePending && !narrow" type="button" class="log-proof-bar__btn" title="采纳所有可自动修正的待审建议" @click="emit('accept-safe')">
      <n-icon :component="Checks" size="15" />采纳可自动 <b>{{ safePending }}</b>
    </button>
    <button
      type="button"
      class="log-proof-bar__btn is-primary"
      :disabled="!canApply || !accepted || applying"
      :title="canApply ? '把已采纳的修改写入当前 LOG 工程' : '没有编辑草稿权限或服务器不支持保存校对正文'"
      @click="emit('apply')"
    >
      {{ applying ? '应用中…' : `应用 ${accepted} 处` }}
    </button>

    <i class="log-proof-bar__sep" aria-hidden="true" />

    <n-popover v-if="!narrow" trigger="hover" placement="top" :show-arrow="false">
      <template #trigger>
        <span class="log-proof-bar__icon is-muted" tabindex="0" aria-label="键盘快捷键"><n-icon :component="Keyboard" size="16" /></span>
      </template>
      <dl class="log-proof-keys">
        <template v-for="[key, text] in SHORTCUTS" :key="key">
          <dt><kbd>{{ key }}</kbd></dt>
          <dd>{{ text }}</dd>
        </template>
      </dl>
    </n-popover>
    <button
      type="button"
      class="log-proof-bar__icon"
      :class="{ 'is-on': focused }"
      :title="focused ? '退出专注审阅' : '专注审阅：收起设计面板，为页边批注留出空间'"
      :aria-pressed="focused"
      aria-label="专注审阅"
      @click="emit('focus')"
    >
      <n-icon :component="focused ? ArrowsMinimize : ArrowsMaximize" size="16" />
    </button>
    <button type="button" class="log-proof-bar__icon" title="隐藏原文中的校对标记" aria-label="隐藏校对标记" @click="emit('hide')">
      <n-icon :component="EyeOff" size="16" />
    </button>
  </div>
</template>

<style scoped>
.log-proof-bar {
  --log-proof-bar-accent: var(--log-wb-accent, var(--primary-color, #2f6fde));
  display: flex;
  align-items: center;
  gap: 4px;
  max-width: 100%;
  min-width: 0;
  padding: 4px;
  border-radius: 12px;
  border: 1px solid var(--log-wb-line, rgba(15, 23, 42, 0.12));
  background: var(--sc-bg-elevated, #fff);
  color: var(--sc-text-primary, #0f172a);
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.16), 0 1px 3px rgba(15, 23, 42, 0.08);
  font-size: 12px;
  pointer-events: auto;
}

.log-proof-bar__nav {
  display: flex;
  align-items: center;
  gap: 1px;
}

.log-proof-bar__count {
  min-width: 64px;
  display: inline-flex;
  align-items: baseline;
  justify-content: center;
  gap: 3px;
  font-variant-numeric: tabular-nums;
  color: var(--log-wb-muted, #64748b);
}

.log-proof-bar__count strong {
  font-size: 13px;
  color: var(--sc-text-primary, #0f172a);
}

.log-proof-bar__progress {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 0 6px;
}

.log-proof-bar__track {
  position: relative;
  width: 84px;
  height: 5px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--log-wb-hover, rgba(15, 23, 42, 0.07));
}

.log-proof-bar__track i {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: inherit;
  background: #16a34a;
  transition: width 0.25s ease;
}

.log-proof-bar__pct {
  min-width: 30px;
  font-variant-numeric: tabular-nums;
  color: var(--log-wb-muted, #64748b);
}

.log-proof-bar__sep {
  flex: none;
  width: 1px;
  height: 18px;
  margin: 0 3px;
  background: var(--log-wb-line, rgba(15, 23, 42, 0.12));
}

.log-proof-bar__icon,
.log-proof-bar__btn {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  height: 30px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
  transition: background-color 0.12s ease, color 0.12s ease, opacity 0.12s ease;
}

.log-proof-bar__icon {
  width: 30px;
}

.log-proof-bar__icon.is-muted {
  color: var(--log-wb-muted, #64748b);
  cursor: help;
}

.log-proof-bar__btn {
  padding: 0 10px;
  white-space: nowrap;
}

.log-proof-bar__btn b {
  font-variant-numeric: tabular-nums;
}

.log-proof-bar__icon:hover:not(:disabled),
.log-proof-bar__btn:hover:not(:disabled) {
  background: var(--log-wb-hover, rgba(15, 23, 42, 0.06));
}

.log-proof-bar__icon.is-on {
  color: var(--log-proof-bar-accent);
  background: color-mix(in srgb, var(--log-proof-bar-accent) 13%, transparent);
}

.log-proof-bar__btn.is-primary {
  padding: 0 14px;
  color: #fff;
  font-weight: 600;
  background: var(--log-proof-bar-accent);
}

.log-proof-bar__btn.is-primary:hover:not(:disabled) {
  background: color-mix(in srgb, var(--log-proof-bar-accent) 86%, #000);
}

.log-proof-bar__icon:disabled,
.log-proof-bar__btn:disabled {
  opacity: 0.42;
  cursor: not-allowed;
}

.log-proof-bar__icon:focus-visible,
.log-proof-bar__btn:focus-visible {
  outline: 2px solid var(--log-proof-bar-accent);
  outline-offset: 1px;
}

.log-proof-keys {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 5px 12px;
  margin: 2px 0;
  font-size: 12px;
}

.log-proof-keys dt,
.log-proof-keys dd {
  margin: 0;
}

.log-proof-keys kbd {
  display: inline-block;
  min-width: 18px;
  padding: 0 5px;
  border: 1px solid var(--sc-border-strong, rgba(15, 23, 42, 0.18));
  border-bottom-width: 2px;
  border-radius: 4px;
  font: 600 11px/17px ui-monospace, SFMono-Regular, Menlo, monospace;
  text-align: center;
}
</style>
