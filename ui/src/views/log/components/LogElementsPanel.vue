<script setup lang="ts">
// 特殊元素：元素库、本章元素列表、锚点状态与重新定位、内容/模板/局部样式编辑。
// 元素只存在于 LOG 草稿，不写入聊天。
import { computed, nextTick, ref, watch } from 'vue';
import type { AnchorState } from '@/services/log/logRenderer';
import { renderMessageBody } from '@/services/log/logContent';
import { LOG_ELEMENT_LABELS, resolveDesignSpeakerName, schemaFieldsFor, templatesForElementType, type SchemaField } from '@/services/log/logRenderer';
import type { LogAnchor, LogDesignMessage, LogDraft, LogElement, LogStylePackage } from '@/types/log';
import { nextOrderAt, sameAnchor } from '../logDraftOps';
import { elementImageRef, setElementImageRef, type LogImageRef } from '../logAssetRefs';
import type { LogAssetLibrary } from '../composables/useLogAssetLibrary';
import LogImagePicker from './LogImagePicker.vue';

const props = defineProps<{
  draft: LogDraft;
  update: (mutate: (d: LogDraft) => void) => boolean;
  stylePackage: LogStylePackage | null;
  anchorStates: Map<string, AnchorState> | null;
  sectionId: string;
  messages: LogDesignMessage[];
  complete: boolean;
  canEdit: boolean;
  focusElementId: string | null;
  /** 多来源章节中锚点范围是当前来源片段，文案用“片段”；默认“章节”。 */
  scopeLabel?: string;
  library: LogAssetLibrary;
}>();
const scope = computed(() => props.scopeLabel || '章节');
const emit = defineEmits<{
  (e: 'locate', nodeId: string): void;
  (e: 'request-insert', anchor: LogAnchor): void;
}>();

const expanded = ref<string | null>(null);
const listRef = ref<HTMLElement | null>(null);

const messageIndex = computed(() => new Map(props.messages.map((m, i) => [m.message.id, i])));
const sectionElements = computed(() => {
  const list = props.draft.elements.filter((e) => e.anchor.sectionId === props.sectionId);
  const pos = (e: LogElement) => {
    const a = e.anchor;
    if (a.kind !== 'message') return a.kind === 'sectionStart' ? -1 : Number.MAX_SAFE_INTEGER;
    const i = messageIndex.value.get(a.messageId);
    return i === undefined ? Number.MAX_SAFE_INTEGER - 1 : i * 2 + (a.side === 'after' ? 1 : 0);
  };
  return list.slice().sort((a, b) => pos(a) - pos(b) || a.order - b.order);
});
const otherSectionCount = computed(() => props.draft.elements.length - sectionElements.value.length);

const stateOf = (e: LogElement): AnchorState => props.anchorStates?.get(e.id) || 'unloaded';
const stateLabel = (s: AnchorState) => ({ attached: '', orphaned: '锚点不存在/当前范围不可见', unloaded: '锚点消息尚未加载' }[s]);

const excerpt = (messageId: string) => {
  const m = props.messages.find((x) => x.message.id === messageId);
  if (!m) return '（未加载的消息）';
  return `${resolveDesignSpeakerName(m.message)}：${renderMessageBody(m.message.content).plainText.slice(0, 24)}`;
};
const anchorText = (a: LogAnchor) => {
  if (a.kind !== 'message') return a.kind === 'sectionStart' ? `${scope.value}开头` : `${scope.value}末尾`;
  return `${a.side === 'before' ? '前' : '后'}·${excerpt(a.messageId)}`;
};

watch(() => props.focusElementId, async (id) => {
  if (!id) return;
  expanded.value = id;
  await nextTick();
  listRef.value?.querySelector(`[data-element-id="${id}"]`)?.scrollIntoView({ block: 'nearest' });
}, { immediate: true });

const mutate = (id: string, fn: (e: LogElement, d: LogDraft) => void) => props.update((d) => {
  const e = d.elements.find((x) => x.id === id);
  if (e) fn(e, d);
});

const remove = (id: string) => props.update((d) => {
  d.elements = d.elements.filter((x) => x.id !== id);
});

/** 同一锚点内调整先后（order 交换）。 */
const shift = (e: LogElement, dir: -1 | 1) => {
  const peers = props.draft.elements.filter((x) => sameAnchor(x.anchor, e.anchor)).sort((a, b) => a.order - b.order);
  const i = peers.findIndex((x) => x.id === e.id);
  const j = i + dir;
  if (i < 0 || j < 0 || j >= peers.length) return;
  const a = peers[i].id;
  const b = peers[j].id;
  props.update((d) => {
    const ea = d.elements.find((x) => x.id === a);
    const eb = d.elements.find((x) => x.id === b);
    if (!ea || !eb) return;
    const t = ea.order;
    ea.order = eb.order;
    eb.order = t === eb.order ? t + dir : t;
  });
};
const peerCount = (e: LogElement) => props.draft.elements.filter((x) => sameAnchor(x.anchor, e.anchor)).length;

// ---- 重新定位 ----
const relocating = ref<string | null>(null);
const relocateTarget = ref<string>('sectionStart');
const relocateSide = ref<'before' | 'after'>('after');
const relocateOptions = computed(() => [
  { label: `${scope.value}开头`, value: 'sectionStart' },
  { label: `${scope.value}末尾`, value: 'sectionEnd' },
  ...props.messages.map((m, i) => ({ label: `#${i + 1} ${excerpt(m.message.id)}`, value: `m:${m.message.id}` })),
]);
const applyRelocate = (e: LogElement) => {
  const t = relocateTarget.value;
  const anchor: LogAnchor = t === 'sectionStart' || t === 'sectionEnd'
    ? { kind: t, sectionId: props.sectionId }
    : { kind: 'message', sectionId: props.sectionId, messageId: t.slice(2), side: relocateSide.value };
  props.update((d) => {
    const el = d.elements.find((x) => x.id === e.id);
    if (!el) return;
    el.anchor = anchor;
    el.order = nextOrderAt({ ...d, elements: d.elements.filter((x) => x.id !== e.id) }, anchor);
  });
  relocating.value = null;
};

// ---- 内容编辑 ----
const setImage = (id: string, image: LogImageRef | null) => mutate(id, (e) => setElementImageRef(e.content, image));
const templateOptions = (e: LogElement) => {
  const pkg = props.stylePackage;
  if (!pkg) return [{ label: e.templateId, value: e.templateId }];
  const ids = templatesForElementType(e.type, pkg.templates, pkg.settingsSchema);
  if (!ids.includes(e.templateId)) ids.unshift(e.templateId);
  return ids.map((id) => ({ label: id in pkg.templates ? id : `${id}（不存在）`, value: id }));
};
const styleFields = (e: LogElement): SchemaField[] => (props.stylePackage ? schemaFieldsFor(props.stylePackage.settingsSchema, e.templateId, 'style') : []);
const propFields = (e: LogElement): SchemaField[] => (props.stylePackage ? schemaFieldsFor(props.stylePackage.settingsSchema, e.templateId, 'props') : []);

const setContent = (id: string, key: string, value: unknown) => mutate(id, (e) => {
  if (value === undefined) delete e.content[key];
  else e.content[key] = value;
});
const setTemplate = (id: string, templateId: string) => mutate(id, (e) => {
  e.templateId = templateId;
  if (!props.stylePackage?.settingsSchema?.[templateId]) e.style = {};
  else {
    const allowed = new Set(schemaFieldsFor(props.stylePackage.settingsSchema, templateId, 'style').map((f) => f.key));
    for (const k of Object.keys(e.style)) if (!allowed.has(k)) delete e.style[k];
  }
  if (e.type === 'custom') {
    const allowed = new Set(propFields({ ...e, templateId }).map((f) => f.key));
    const p = { ...(e.content.props as Record<string, unknown> || {}) };
    for (const k of Object.keys(p)) if (!allowed.has(k)) delete p[k];
    e.content.props = p;
  }
});
const setStyle = (id: string, key: string, value: unknown) => mutate(id, (e) => {
  if (value === null || value === undefined || value === '') delete e.style[key];
  else e.style[key] = value;
});
const setProp = (id: string, key: string, value: unknown) => mutate(id, (e) => {
  const p = { ...((e.content.props as Record<string, unknown>) || {}) };
  if (value === null || value === undefined || value === '') delete p[key];
  else p[key] = value;
  e.content.props = p;
});
const propValue = (e: LogElement, key: string) => (e.content.props as Record<string, unknown> | undefined)?.[key];

const levelOptions = [1, 2, 3].map((v) => ({ label: `${v}`, value: v }));
const str = (v: unknown) => (typeof v === 'string' ? v : '');
const num = (v: unknown) => (typeof v === 'number' ? v : null);
const anchorAt = (kind: 'sectionStart' | 'sectionEnd'): LogAnchor => ({ kind, sectionId: props.sectionId });
</script>

<template>
  <div ref="listRef" class="log-el">
    <section class="log-el__lib">
      <div class="log-el__title">插入元素</div>
      <n-space size="small">
        <n-button size="small" :disabled="!canEdit || !sectionId" @click="emit('request-insert', anchorAt('sectionStart'))">{{ scope }}开头…</n-button>
        <n-button size="small" :disabled="!canEdit || !sectionId" @click="emit('request-insert', anchorAt('sectionEnd'))">{{ scope }}末尾…</n-button>
      </n-space>
      <p class="log-el__hint">也可以在预览中选中消息后使用“前插元素/后插元素”。元素跟随锚点消息移动。</p>
    </section>

    <section>
      <div class="log-el__title">本章元素（{{ sectionElements.length }}）</div>
      <p v-if="otherSectionCount" class="log-el__hint">其他章节还有 {{ otherSectionCount }} 个元素。</p>
      <n-empty v-if="!sectionElements.length" size="small" description="本章还没有元素" />
      <div
        v-for="e in sectionElements"
        :key="e.id"
        class="log-el__item"
        :class="{ 'is-open': expanded === e.id, 'is-orphan': stateOf(e) === 'orphaned' }"
        :data-element-id="e.id"
      >
        <div class="log-el__row">
          <button type="button" class="log-el__name" :aria-expanded="expanded === e.id" @click="expanded = expanded === e.id ? null : e.id">
            {{ LOG_ELEMENT_LABELS[e.type] || e.type }}
            <span class="log-el__anchor">{{ anchorText(e.anchor) }}</span>
          </button>
          <n-tag v-if="stateOf(e) !== 'attached'" size="tiny" :type="stateOf(e) === 'orphaned' ? 'error' : 'default'">{{ stateLabel(stateOf(e)) }}</n-tag>
        </div>
        <div class="log-el__actions">
          <n-button size="tiny" :disabled="stateOf(e) !== 'attached'" @click="emit('locate', `el-${e.id}`)">定位</n-button>
          <n-button size="tiny" :disabled="!canEdit || peerCount(e) < 2" @click="shift(e, -1)">前移</n-button>
          <n-button size="tiny" :disabled="!canEdit || peerCount(e) < 2" @click="shift(e, 1)">后移</n-button>
          <n-button size="tiny" :disabled="!canEdit" @click="relocating = relocating === e.id ? null : e.id">重新定位</n-button>
          <n-popconfirm @positive-click="remove(e.id)">
            <template #trigger><n-button size="tiny" type="error" quaternary :disabled="!canEdit">删除</n-button></template>
            删除此 LOG 元素？
          </n-popconfirm>
        </div>
        <div v-if="relocating === e.id" class="log-el__relocate">
          <n-select v-model:value="relocateTarget" size="small" :options="relocateOptions" filterable />
          <n-radio-group v-if="relocateTarget.startsWith('m:')" v-model:value="relocateSide" size="small">
            <n-radio value="before">消息之前</n-radio>
            <n-radio value="after">消息之后</n-radio>
          </n-radio-group>
          <n-button size="small" type="primary" @click="applyRelocate(e)">确定</n-button>
        </div>

        <n-form v-if="expanded === e.id" class="log-el__form" size="small" label-placement="left" label-width="64" :disabled="!canEdit">
          <n-form-item label="模板">
            <n-select :value="e.templateId" :options="templateOptions(e)" @update:value="(v: string) => setTemplate(e.id, v)" />
          </n-form-item>

          <template v-if="e.type === 'cover'">
            <n-form-item label="标题"><n-input :value="str(e.content.title)" maxlength="200" @update:value="(v: string) => setContent(e.id, 'title', v)" /></n-form-item>
            <n-form-item label="副标题"><n-input :value="str(e.content.subtitle)" maxlength="400" @update:value="(v: string) => setContent(e.id, 'subtitle', v)" /></n-form-item>
            <n-form-item label="图片">
              <LogImagePicker :value="elementImageRef(e.content)" :library="library" :disabled="!canEdit" empty-text="无图片" @update:value="(v: LogImageRef | null) => setImage(e.id, v)" />
            </n-form-item>
          </template>
          <template v-else-if="e.type === 'imagePlate'">
            <n-form-item label="图片">
              <LogImagePicker :value="elementImageRef(e.content)" :library="library" :disabled="!canEdit" required empty-text="请选择图片" @update:value="(v: LogImageRef | null) => v && setImage(e.id, v)" />
            </n-form-item>
            <n-form-item label="说明"><n-input :value="str(e.content.caption)" maxlength="400" @update:value="(v: string) => setContent(e.id, 'caption', v)" /></n-form-item>
            <n-form-item label="适配">
              <n-radio-group :value="str(e.content.fit) || 'contain'" @update:value="(v: string) => setContent(e.id, 'fit', v)">
                <n-radio value="contain">完整显示</n-radio>
                <n-radio value="cover">铺满裁切</n-radio>
              </n-radio-group>
            </n-form-item>
          </template>
          <template v-else-if="e.type === 'transition'">
            <n-form-item label="文字"><n-input :value="str(e.content.label)" maxlength="200" @update:value="(v: string) => setContent(e.id, 'label', v)" /></n-form-item>
          </template>
          <template v-else-if="e.type === 'quote' || e.type === 'prologue' || e.type === 'epilogue'">
            <n-form-item label="正文">
              <n-input type="textarea" :value="str(e.content.body)" :autosize="{ minRows: 3, maxRows: 12 }" maxlength="20000" @update:value="(v: string) => setContent(e.id, 'body', v)" />
            </n-form-item>
          </template>
          <template v-else-if="e.type === 'heading'">
            <n-form-item label="文字"><n-input :value="str(e.content.text)" maxlength="200" @update:value="(v: string) => setContent(e.id, 'text', v)" /></n-form-item>
            <n-form-item label="级别"><n-select :value="num(e.content.level) || 2" :options="levelOptions" @update:value="(v: number) => setContent(e.id, 'level', v)" /></n-form-item>
          </template>
          <template v-else-if="e.type === 'toc'">
            <n-form-item label="深度"><n-select :value="num(e.content.depth) || 2" :options="levelOptions" @update:value="(v: number) => setContent(e.id, 'depth', v)" /></n-form-item>
            <p class="log-el__hint">目录由章节与标题元素生成，页码在排版稳定后填充。</p>
          </template>
          <template v-else-if="e.type === 'custom'">
            <p class="log-el__hint">代码组件：参数由模板的 settingsSchema 定义。</p>
            <n-form-item v-for="f in propFields(e)" :key="`p-${f.key}`" :label="f.title">
              <n-switch v-if="f.type === 'boolean'" :value="propValue(e, f.key) === true" @update:value="(v: boolean) => setProp(e.id, f.key, v)" />
              <n-select v-else-if="f.enum" :value="(propValue(e, f.key) as any) ?? null" clearable :options="f.enum.map((x) => ({ label: String(x), value: x }))" @update:value="(v: unknown) => setProp(e.id, f.key, v)" />
              <n-input-number v-else-if="f.type !== 'string'" :value="(propValue(e, f.key) as number) ?? null" :min="f.minimum ?? undefined" :max="f.maximum ?? undefined" :precision="f.type === 'integer' ? 0 : undefined" clearable @update:value="(v: number | null) => setProp(e.id, f.key, v)" />
              <n-input v-else :value="String(propValue(e, f.key) ?? '')" :type="f.multiline ? 'textarea' : 'text'" :autosize="f.multiline ? { minRows: 3, maxRows: 12 } : undefined" :maxlength="f.maxLength ?? 2000" @update:value="(v: string) => setProp(e.id, f.key, v)" />
            </n-form-item>
          </template>

          <template v-if="styleFields(e).length">
            <div class="log-el__subtitle">局部样式</div>
            <n-form-item v-for="f in styleFields(e)" :key="`s-${f.key}`" :label="f.title">
              <n-switch v-if="f.type === 'boolean'" :value="e.style[f.key] === true" @update:value="(v: boolean) => setStyle(e.id, f.key, v)" />
              <n-select v-else-if="f.enum" :value="(e.style[f.key] as any) ?? null" clearable :options="f.enum.map((x) => ({ label: String(x), value: x }))" @update:value="(v: unknown) => setStyle(e.id, f.key, v)" />
              <n-input-number v-else-if="f.type !== 'string'" :value="(e.style[f.key] as number) ?? null" :min="f.minimum ?? undefined" :max="f.maximum ?? undefined" :precision="f.type === 'integer' ? 0 : undefined" clearable @update:value="(v: number | null) => setStyle(e.id, f.key, v)" />
              <n-input v-else :value="String(e.style[f.key] ?? '')" :maxlength="f.maxLength ?? 200" @update:value="(v: string) => setStyle(e.id, f.key, v)" />
            </n-form-item>
          </template>
          <p v-else class="log-el__hint">此模板没有可调的局部样式。</p>
        </n-form>
      </div>
    </section>
  </div>
</template>

<style scoped>
.log-el {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 8px 0 16px;
}

.log-el > section + section {
  padding-top: 14px;
  border-top: 1px solid var(--sc-border-mute, #e5e5e5);
}

.log-el__title {
  font-weight: 600;
  font-size: 12px;
  letter-spacing: 0.02em;
  color: var(--sc-text-secondary, #666);
  margin-bottom: 8px;
}

.log-el__subtitle {
  font-weight: 600;
  font-size: 12px;
  margin: 6px 0;
}

.log-el__hint {
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
  margin: 6px 0;
}

.log-el__item {
  border: 1px solid var(--sc-border-strong, #e5e5e5);
  border-radius: 8px;
  padding: 8px 10px;
  margin-bottom: 8px;
  transition: border-color 0.12s ease, box-shadow 0.12s ease;
}

.log-el__item:hover {
  border-color: color-mix(in srgb, var(--primary-color, #2f6fde) 45%, var(--sc-border-strong, #e5e5e5));
}

.log-el__item.is-open {
  border-color: var(--primary-color, #2f6fde);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--primary-color, #2f6fde) 12%, transparent);
}

.log-el__item.is-orphan {
  border-color: #d03050;
}

.log-el__row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.log-el__name {
  flex: 1;
  min-width: 0;
  text-align: left;
  background: none;
  border: 0;
  padding: 2px 0;
  font: inherit;
  font-weight: 600;
  color: inherit;
  cursor: pointer;
}

.log-el__name:focus-visible {
  outline: 2px solid #2080f0;
}

.log-el__anchor {
  display: block;
  font-weight: 400;
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.log-el__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}

.log-el__relocate {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
}

.log-el__form {
  margin-top: 8px;
}
</style>
