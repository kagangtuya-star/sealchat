<script setup lang="ts">
// Style Studio · 设计变量：以表格编辑 tokens；每次修改重新序列化写回工作副本的 tokens JSON 文本，
// 与高级源码共享同一份数据。
import { computed, ref } from 'vue';
import { Plus, Trash } from '@vicons/tabler';
import type { LogTokenValue } from '@/types/log';
import { isSafeTokenValue } from '@/services/log/logTemplateCompiler';
import { STYLE_KEY, formatJson, tokenCssHint, tokenDefault, tokenKind, type StyleWorkingCopy, type TokenKind } from '../logStyleWorkingCopy';

const props = defineProps<{
  edit: (mutate: (c: StyleWorkingCopy) => void) => void;
  readonly: boolean;
  /** 解析后的 tokens；源码 JSON 无法解析时为 null。 */
  tokens: Record<string, LogTokenValue> | null;
  error: string;
}>();
const emit = defineEmits<{ (e: 'goto', section: 'source'): void }>();

const entries = computed(() => Object.entries(props.tokens || {}));
const kinds: Array<{ label: string; value: TokenKind }> = [
  { label: '颜色', value: 'color' },
  { label: '文本', value: 'text' },
  { label: '数字', value: 'number' },
  { label: '开关', value: 'boolean' },
];

const write = (next: Record<string, LogTokenValue>) => {
  if (!props.readonly) props.edit((c) => { c.tokensText = formatJson(next); });
};
const setValue = (key: string, value: LogTokenValue | null) => {
  if (value === null || !props.tokens) return;
  write({ ...props.tokens, [key]: value });
};
const setKind = (key: string, kind: TokenKind) => {
  if (!props.tokens) return;
  write({ ...props.tokens, [key]: tokenDefault(kind) });
};
const remove = (key: string) => {
  if (!props.tokens) return;
  const next = { ...props.tokens };
  delete next[key];
  write(next);
};
const unsafe = (v: LogTokenValue) => !isSafeTokenValue(v);

const newKey = ref('');
const newKind = ref<TokenKind>('color');
const newProblem = computed(() => {
  const k = newKey.value.trim();
  if (!k) return '';
  if (!STYLE_KEY.test(k)) return '须以字母开头，只含字母、数字、下划线或连字符';
  if (props.tokens && k in props.tokens) return '已存在';
  return '';
});
const add = () => {
  const k = newKey.value.trim();
  if (!k || newProblem.value || !props.tokens) return;
  write({ ...props.tokens, [k]: tokenDefault(newKind.value) });
  newKey.value = '';
};

const copyHint = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // 剪贴板不可用时用户仍可手动选中提示文本
  }
};
</script>

<template>
  <div class="lsk">
    <n-alert v-if="!tokens" type="warning" :show-icon="false">
      设计变量源码无法解析：{{ error }}。
      <button type="button" class="lsk__link" @click="emit('goto', 'source')">在高级源码中修正</button>
    </n-alert>
    <template v-else>
      <p class="lsk__hint">设计变量是样式包的默认值，每个 LOG 还可以在“样式”面板中单独覆盖。数字与文本变量在 CSS 中写作 <code>var(--log-名称)</code>，开关变量是根节点上的 <code>data-log-名称</code> 属性。</p>
      <div class="lsk__table" role="table" aria-label="设计变量">
        <div v-for="[key, value] in entries" :key="key" class="lsk__row" role="row">
          <code class="lsk__key" role="cell">{{ key }}</code>
          <n-select class="lsk__kind" size="small" :value="tokenKind(value)" :options="kinds" :disabled="readonly" :aria-label="`${key} 的类型`" @update:value="(k: TokenKind) => setKind(key, k)" />
          <div class="lsk__value" role="cell">
            <n-switch v-if="typeof value === 'boolean'" :value="value" :disabled="readonly" @update:value="(v: boolean) => setValue(key, v)" />
            <n-input-number v-else-if="typeof value === 'number'" size="small" :value="value" :disabled="readonly" @update:value="(v: number | null) => setValue(key, v)" />
            <n-color-picker v-else-if="tokenKind(value) === 'color'" size="small" :modes="['hex']" :value="value" :disabled="readonly" @update:value="(v: string) => setValue(key, v)" />
            <n-input v-else size="small" :value="value" maxlength="200" :status="unsafe(value) ? 'error' : undefined" :disabled="readonly" @update:value="(v: string) => setValue(key, v)" />
          </div>
          <button type="button" class="lsk__css" :title="`复制 ${tokenCssHint(key, value)}`" @click="copyHint(tokenCssHint(key, value))">{{ tokenCssHint(key, value) }}</button>
          <n-button size="tiny" quaternary type="error" :disabled="readonly" :aria-label="`删除变量 ${key}`" @click="remove(key)">
            <template #icon><n-icon :component="Trash" /></template>
          </n-button>
        </div>
        <p v-if="!entries.length" class="lsk__hint">还没有设计变量。</p>
      </div>
      <p v-if="entries.some(([, v]) => unsafe(v))" class="lsk__hint is-warn">文本变量不能包含 ; { } \ &lt; &gt; @ ( )，最长 200 字。</p>
      <div v-if="!readonly" class="lsk__add">
        <n-input v-model:value="newKey" size="small" maxlength="64" placeholder="新变量名，如 accent" :status="newProblem ? 'error' : undefined" @keydown.enter.prevent="add" />
        <n-select v-model:value="newKind" class="lsk__kind" size="small" :options="kinds" />
        <n-button size="small" :disabled="!newKey.trim() || !!newProblem" @click="add">
          <template #icon><n-icon :component="Plus" /></template>添加
        </n-button>
        <span v-if="newProblem" class="lsk__hint is-warn">{{ newProblem }}</span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.lsk {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.lsk__table {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.lsk__row {
  display: grid;
  grid-template-columns: minmax(90px, 160px) 90px minmax(140px, 1fr) minmax(0, 190px) auto;
  align-items: center;
  gap: 8px;
}

.lsk__key {
  font-size: 12px;
  overflow-wrap: anywhere;
}

.lsk__kind {
  width: 90px;
}

.lsk__value {
  min-width: 0;
}

.lsk__css {
  overflow: hidden;
  padding: 2px 6px;
  border: 0;
  border-radius: 6px;
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.05));
  color: var(--sc-text-secondary, #666);
  font: 11px/1.5 "SFMono-Regular", Consolas, monospace;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: copy;
}

.lsk__add {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.lsk__add > :first-child {
  flex: 1 1 160px;
  max-width: 240px;
}

.lsk__hint {
  margin: 0;
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
}

.lsk__hint.is-warn {
  color: #9a5b00;
}

.lsk__link {
  padding: 0;
  border: 0;
  background: none;
  color: var(--primary-color, #2f6fde);
  font: inherit;
  cursor: pointer;
}

@media (max-width: 720px) {
  .lsk__row {
    grid-template-columns: minmax(0, 1fr) 90px auto;
  }

  .lsk__value {
    grid-column: 1 / -1;
  }

  .lsk__css {
    grid-column: 1 / 3;
  }
}
</style>
