<script setup lang="ts">
// Style Studio · 消息规则：自上而下首个匹配；编辑结果重新序列化写回工作副本的 rules JSON 文本。
import { computed } from 'vue';
import { ArrowDown, ArrowUp, Plus, Trash } from '@vicons/tabler';
import type { LogStyleRule } from '@/types/log';
import { LOG_MESSAGE_TYPES } from '@/services/log/logTemplateCompiler';
import { formatJson, type StyleWorkingCopy, type TemplateUsage } from '../logStyleWorkingCopy';

const props = defineProps<{
  edit: (mutate: (c: StyleWorkingCopy) => void) => void;
  readonly: boolean;
  rules: LogStyleRule[] | null;
  error: string;
  templateIds: string[];
  usage: Record<string, TemplateUsage>;
}>();
const emit = defineEmits<{ (e: 'goto', section: 'source'): void; (e: 'template', id: string): void }>();

const typeLabel: Record<string, string> = { ic: '场内', ooc: '场外', narration: '旁白', dice: '骰子', bot: '机器人', image: '图片', unknown: '未知' };
const typeOptions = [{ label: '任意类型', value: '' }, ...LOG_MESSAGE_TYPES.map((t) => ({ label: `${typeLabel[t] || t}（${t}）`, value: t }))];
/** 只有展开后恰好一个正文插槽的模板能用于消息；其余仍列出但禁用，便于看清原因。 */
const templateOptions = computed(() => props.templateIds.map((id) => {
  const slots = props.usage[id]?.bodySlots;
  return { label: slots === 1 ? id : `${id}（${slots === null || slots === undefined ? '无效' : `${slots} 个正文插槽`}）`, value: id, disabled: slots !== 1 };
}));

const write = (next: LogStyleRule[]) => {
  if (!props.readonly) props.edit((c) => { c.rulesText = formatJson(next); });
};
/** 空字符串表示不限制，写回时省略该字段，保持与既有规则 JSON 一致。 */
const patch = (index: number, key: keyof LogStyleRule, value: string) => {
  if (!props.rules) return;
  const next = props.rules.map((r) => ({ ...r }));
  const v = value.trim();
  if (v) (next[index] as Record<string, string>)[key] = v;
  else if (key !== 'templateId') delete next[index][key];
  write(next);
};
const move = (index: number, dir: -1 | 1) => {
  if (!props.rules) return;
  const next = props.rules.slice();
  const [r] = next.splice(index, 1);
  next.splice(index + dir, 0, r);
  write(next);
};
const remove = (index: number) => {
  if (props.rules) write(props.rules.filter((_, i) => i !== index));
};
const add = () => {
  if (!props.rules) return;
  const firstMessage = props.templateIds.find((id) => props.usage[id]?.bodySlots === 1) || props.templateIds[0] || '';
  // 新规则插在兜底规则之前，避免被它挡住。
  const fallback = props.rules.findIndex((r) => !r.messageType && !r.identityKey && !r.variantKey);
  const next = props.rules.slice();
  next.splice(fallback < 0 ? next.length : fallback, 0, { messageType: 'ooc', templateId: firstMessage });
  write(next);
};

const isFallback = (r: LogStyleRule) => !r.messageType && !r.identityKey && !r.variantKey;
const shadowedFrom = computed(() => (props.rules || []).findIndex(isFallback));
const missingFallback = computed(() => !!props.rules && !props.rules.some(isFallback));
</script>

<template>
  <div class="lsr">
    <n-alert v-if="!rules" type="warning" :show-icon="false">
      规则源码无法解析：{{ error }}。
      <button type="button" class="lsr__link" @click="emit('goto', 'source')">在高级源码中修正</button>
    </n-alert>
    <template v-else>
      <p class="lsr__hint">每条消息按顺序取第一条满足全部已填条件的规则。角色 / 差分键匹配消息的角色或差分标识，留空表示不限。LOG 工程里的“显示规则”与单条消息选择优先于这里。</p>
      <ol class="lsr__list">
        <li v-for="(r, i) in rules" :key="i" class="lsr__row" :class="{ 'is-shadowed': shadowedFrom >= 0 && i > shadowedFrom }">
          <span class="lsr__index">{{ i + 1 }}</span>
          <n-select size="small" class="lsr__type" :value="r.messageType || ''" :options="typeOptions" :disabled="readonly" aria-label="消息类型" @update:value="(v: string) => patch(i, 'messageType', v)" />
          <n-input size="small" class="lsr__key" :value="r.identityKey || ''" placeholder="角色键" maxlength="100" :disabled="readonly" @update:value="(v: string) => patch(i, 'identityKey', v)" />
          <n-input size="small" class="lsr__key" :value="r.variantKey || ''" placeholder="差分键" maxlength="100" :disabled="readonly" @update:value="(v: string) => patch(i, 'variantKey', v)" />
          <span class="lsr__arrow" aria-hidden="true">→</span>
          <n-select size="small" class="lsr__tpl" :value="r.templateId" :options="templateOptions" :status="usage[r.templateId]?.bodySlots === 1 ? undefined : 'error'" :disabled="readonly" aria-label="模板" @update:value="(v: string) => patch(i, 'templateId', v)" />
          <span class="lsr__ops">
            <n-button size="tiny" quaternary :disabled="readonly || i === 0" aria-label="上移" @click="move(i, -1)"><template #icon><n-icon :component="ArrowUp" /></template></n-button>
            <n-button size="tiny" quaternary :disabled="readonly || i === rules.length - 1" aria-label="下移" @click="move(i, 1)"><template #icon><n-icon :component="ArrowDown" /></template></n-button>
            <n-button size="tiny" quaternary aria-label="打开模板" @click="emit('template', r.templateId)">模板</n-button>
            <n-button size="tiny" quaternary type="error" :disabled="readonly" aria-label="删除规则" @click="remove(i)"><template #icon><n-icon :component="Trash" /></template></n-button>
          </span>
        </li>
      </ol>
      <p v-if="!rules.length" class="lsr__hint">没有规则时所有消息使用渲染器的通用结构。</p>
      <p v-if="missingFallback && rules.length" class="lsr__hint is-warn">建议在最后放一条“任意类型”的兜底规则，否则未匹配的消息使用通用结构。</p>
      <p v-if="shadowedFrom >= 0 && shadowedFrom < rules.length - 1" class="lsr__hint is-warn">第 {{ shadowedFrom + 1 }} 条是不限条件的兜底规则，排在它后面的规则不会生效。</p>
      <n-button v-if="!readonly" size="small" @click="add">
        <template #icon><n-icon :component="Plus" /></template>添加规则
      </n-button>
    </template>
  </div>
</template>

<style scoped>
.lsr {
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: flex-start;
}

.lsr > * {
  align-self: stretch;
}

.lsr > .n-button {
  align-self: flex-start;
}

.lsr__list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.lsr__row {
  display: grid;
  grid-template-columns: 22px 130px minmax(70px, 1fr) minmax(70px, 1fr) 14px minmax(120px, 1.2fr) auto;
  align-items: center;
  gap: 6px;
}

.lsr__row.is-shadowed {
  opacity: 0.6;
}

.lsr__index {
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
  text-align: right;
}

.lsr__arrow {
  color: var(--sc-text-secondary, #666);
}

.lsr__ops {
  display: flex;
  gap: 2px;
}

.lsr__hint {
  margin: 0;
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
}

.lsr__hint.is-warn {
  color: #9a5b00;
}

.lsr__link {
  padding: 0;
  border: 0;
  background: none;
  color: var(--primary-color, #2f6fde);
  font: inherit;
  cursor: pointer;
}

@media (max-width: 720px) {
  .lsr__row {
    grid-template-columns: 22px minmax(0, 1fr) minmax(0, 1fr);
    padding-bottom: 6px;
    border-bottom: 1px solid var(--sc-border-mute, #eee);
  }

  .lsr__type,
  .lsr__tpl {
    grid-column: 2 / -1;
  }

  .lsr__arrow {
    display: none;
  }

  .lsr__ops {
    grid-column: 1 / -1;
    justify-content: flex-end;
  }
}
</style>
