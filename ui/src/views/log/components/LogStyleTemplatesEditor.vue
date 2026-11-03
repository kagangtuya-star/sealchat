<script setup lang="ts">
// Style Studio · 模板：浏览、新建、编辑与删除模板。用途只从规则、片段引用、元素类型匹配、
// 参数 Schema 与当前 LOG 的引用推导，不另立模板类型协议；编辑的是工作副本中的同一份 HTML。
import { computed, nextTick, ref, watch } from 'vue';
import { Plus, Search, Trash } from '@vicons/tabler';
import { LOG_MESSAGE_TYPES, LOG_TEMPLATE_FIELDS } from '@/services/log/logTemplateCompiler';
import {
  ELEMENT_STARTER_TYPES,
  TEMPLATE_STARTERS,
  addTemplate,
  draftRefCount,
  formatJson,
  parseStyleJson,
  removeTemplate,
  suggestTemplateId,
  templateIdProblem,
  templateRemovalBlockers,
  templateRoles,
  templateStarter,
  type StyleWorkingCopy,
  type TemplateStarterKind,
  type TemplateUsage,
} from '../logStyleWorkingCopy';

const props = defineProps<{
  copy: StyleWorkingCopy;
  edit: (mutate: (c: StyleWorkingCopy) => void) => void;
  readonly: boolean;
  usage: Record<string, TemplateUsage>;
  /** 按模板 ID 归集的前端检查问题。 */
  issues: Record<string, string[]>;
}>();
const active = defineModel<string>('active', { required: true });
const emit = defineEmits<{ (e: 'goto', section: 'rules' | 'source'): void }>();

const query = ref('');
const ids = computed(() => Object.keys(props.copy.templates).sort((a, b) => a.localeCompare(b)));
const visible = computed(() => {
  const q = query.value.trim().toLowerCase();
  return q ? ids.value.filter((id) => id.toLowerCase().includes(q)) : ids.value;
});
watch(ids, (list) => {
  if (!list.includes(active.value)) active.value = list[0] || '';
}, { immediate: true });

const rolesOf = (id: string) => (props.usage[id] ? templateRoles(props.usage[id]) : []);
const current = computed(() => (active.value && active.value in props.copy.templates ? active.value : ''));
const currentUsage = computed(() => (current.value ? props.usage[current.value] : undefined));
const blockers = computed(() => (current.value ? templateRemovalBlockers(props.copy, current.value, currentUsage.value) : []));
const draftRefs = computed(() => draftRefCount(currentUsage.value?.draftRefs || null));

const html = computed({
  get: () => (current.value ? props.copy.templates[current.value] : ''),
  set: (v: string) => {
    const id = current.value;
    if (id && !props.readonly) props.edit((c) => { c.templates = { ...c.templates, [id]: v }; });
  },
});

// ---- 插入字段 ----
const editorRef = ref<{ textareaElRef?: HTMLTextAreaElement | null } | null>(null);
const fieldOptions = computed(() => [
  ...LOG_TEMPLATE_FIELDS.map((f) => ({ label: f, value: f })),
  ...(currentUsage.value?.propNames || []).map((p) => ({ label: `props.${p}`, value: `props.${p}` })),
]);
const pickedField = ref<string | null>(null);
const insert = async (snippet: string) => {
  if (props.readonly || !current.value) return;
  const el = editorRef.value?.textareaElRef || null;
  const src = html.value;
  const start = el ? el.selectionStart : src.length;
  const end = el ? el.selectionEnd : src.length;
  html.value = src.slice(0, start) + snippet + src.slice(end);
  await nextTick();
  if (el) {
    el.focus();
    el.setSelectionRange(start + snippet.length, start + snippet.length);
  }
};
const insertField = (mode: 'text' | 'if') => {
  const f = pickedField.value;
  if (!f) return;
  void insert(mode === 'text' ? `<span data-log-text="${f}"></span>` : `<div data-log-if="${f}"></div>`);
};
const partOptions = computed(() => ids.value.filter((id) => id !== current.value).map((id) => ({ label: id, value: id })));
const pickedPart = ref<string | null>(null);
const insertSlot = () => void insert('<div data-log-slot="body"></div>');
const insertPart = () => {
  if (pickedPart.value) void insert(`<div data-log-part="${pickedPart.value}"></div>`);
};

// ---- 新建 ----
const creating = ref(false);
const newKind = ref<TemplateStarterKind>('message');
const newElementType = ref('heading');
const newId = ref('');
const newRuleType = ref<string | null>(null);
const newProblem = computed(() => (newId.value ? templateIdProblem(props.copy, newId.value.trim()) : ''));
const startCreate = () => {
  creating.value = true;
  newKind.value = 'message';
  newRuleType.value = null;
  newId.value = suggestTemplateId(props.copy, newKind.value, newElementType.value);
};
watch([newKind, newElementType], () => {
  if (creating.value) newId.value = suggestTemplateId(props.copy, newKind.value, newElementType.value);
});
const createError = ref('');
const confirmCreate = () => {
  const id = newId.value.trim();
  const starter = templateStarter(newKind.value, id, newElementType.value);
  let error = '';
  props.edit((c) => {
    error = addTemplate(c, id, starter.html, starter.schema);
    if (error || newKind.value !== 'message' || !newRuleType.value) return;
    // 带消息类型的规则插在最前面，不会挡住其他类型；规则源码无法解析时不改动。
    const rules = parseStyleJson(c).rules;
    if (rules) c.rulesText = formatJson([{ messageType: newRuleType.value, templateId: id }, ...rules]);
  });
  createError.value = error;
  if (error) return;
  creating.value = false;
  active.value = id;
};

const remove = () => {
  const id = current.value;
  if (!id || props.readonly || blockers.value.length) return;
  props.edit((c) => removeTemplate(c, id));
};

const elementLabel: Record<string, string> = {
  cover: '封面', imagePlate: '插图页', pageBreak: '分页', transition: '转场', quote: '引文',
  prologue: '序言', epilogue: '尾声', heading: '标题', toc: '目录', custom: '自定义',
};
const kindHint = computed(() => TEMPLATE_STARTERS.find((k) => k.kind === newKind.value)?.hint || '');
</script>

<template>
  <div class="lst">
    <aside class="lst__list" aria-label="模板列表">
      <div class="lst__list-head">
        <n-input v-model:value="query" size="small" clearable placeholder="筛选模板" aria-label="筛选模板">
          <template #prefix><n-icon :component="Search" /></template>
        </n-input>
        <n-button size="small" type="primary" secondary :disabled="readonly" title="新建模板" @click="startCreate">
          <template #icon><n-icon :component="Plus" /></template>新建
        </n-button>
      </div>
      <div class="lst__items" role="listbox" aria-label="模板">
        <button
          v-for="id in visible"
          :key="id"
          type="button"
          role="option"
          class="lst__item"
          :class="{ 'is-on': id === current }"
          :aria-selected="id === current"
          @click="active = id; creating = false"
        >
          <span class="lst__item-id">{{ id }}</span>
          <span class="lst__roles">
            <span v-for="r in rolesOf(id)" :key="r" class="lst__role">{{ r }}</span>
            <span v-if="issues[id]?.length" class="lst__role is-warn">{{ issues[id].length }} 个问题</span>
          </span>
        </button>
      </div>
    </aside>

    <section v-if="creating" class="lst__detail" aria-label="新建模板">
      <h4 class="lst__title">新建模板</h4>
      <n-radio-group v-model:value="newKind" class="lst__kinds" name="template-kind">
        <n-radio v-for="k in TEMPLATE_STARTERS" :key="k.kind" :value="k.kind">{{ k.label }}</n-radio>
      </n-radio-group>
      <p class="lst__hint">{{ kindHint }}</p>
      <n-form size="small" label-placement="left" label-width="84" @submit.prevent>
        <n-form-item v-if="newKind === 'element'" label="元素类型">
          <n-select v-model:value="newElementType" :options="ELEMENT_STARTER_TYPES.map((t) => ({ label: `${elementLabel[t] || t}（${t}）`, value: t }))" />
        </n-form-item>
        <n-form-item label="模板 ID" :validation-status="newProblem ? 'error' : undefined" :feedback="newProblem || createError">
          <n-input v-model:value="newId" maxlength="64" placeholder="字母开头" />
        </n-form-item>
        <n-form-item v-if="newKind === 'message'" label="消息规则">
          <n-select v-model:value="newRuleType" clearable placeholder="不添加规则（可在消息上单独选用）" :options="LOG_MESSAGE_TYPES.map((t) => ({ label: `类型为 ${t} 的消息使用此模板`, value: t }))" />
        </n-form-item>
      </n-form>
      <p v-if="newKind === 'element'" class="lst__hint">元素按模板 ID 匹配类型：ID 等于类型名或以类型名开头时，插入该类元素可以选用它。</p>
      <n-space size="small">
        <n-button size="small" type="primary" :disabled="!newId.trim() || !!newProblem" @click="confirmCreate">创建</n-button>
        <n-button size="small" @click="creating = false">取消</n-button>
      </n-space>
    </section>

    <section v-else-if="current" class="lst__detail" :aria-label="`模板 ${current}`">
      <div class="lst__detail-head">
        <h4 class="lst__title"><code>{{ current }}</code></h4>
        <n-popconfirm v-if="!readonly" :disabled="!!blockers.length" @positive-click="remove">
          <template #trigger>
            <n-button size="tiny" quaternary type="error" :disabled="!!blockers.length" :title="blockers.join('；') || '删除模板'">
              <template #icon><n-icon :component="Trash" /></template>删除
            </n-button>
          </template>
          删除模板 {{ current }}？<template v-if="draftRefs">当前 LOG 有 {{ draftRefs }} 处引用它，之后把新版本应用到当前 LOG 时需要调整。</template>
        </n-popconfirm>
      </div>
      <p v-if="blockers.length && !readonly" class="lst__hint is-warn">暂不能删除：{{ blockers.join('；') }}。</p>

      <dl class="lst__usage">
        <div><dt>消息规则</dt><dd>
          <template v-if="currentUsage?.ruleIndexes.length">第 {{ currentUsage.ruleIndexes.map((i) => i + 1).join('、') }} 条 <button type="button" class="lst__link" @click="emit('goto', 'rules')">查看规则</button></template>
          <template v-else>未被规则引用</template>
        </dd></div>
        <div><dt>正文插槽</dt><dd>{{ currentUsage?.bodySlots === null || currentUsage?.bodySlots === undefined ? '模板无效，无法判断' : currentUsage.bodySlots === 1 ? '1 个（可用于消息）' : `${currentUsage.bodySlots} 个（不能用于消息）` }}</dd></div>
        <div v-if="currentUsage?.elementTypes.length"><dt>元素</dt><dd>{{ currentUsage.elementTypes.map((t) => elementLabel[t] || t).join('、') }}</dd></div>
        <div v-if="currentUsage?.parts.length"><dt>引用片段</dt><dd>{{ currentUsage.parts.join('、') }}</dd></div>
        <div v-if="currentUsage?.usedAsPartBy.length"><dt>被引用于</dt><dd>{{ currentUsage.usedAsPartBy.join('、') }}</dd></div>
        <div v-if="currentUsage?.propNames.length"><dt>参数</dt><dd>{{ currentUsage.propNames.map((p) => `props.${p}`).join('、') }} <button type="button" class="lst__link" @click="emit('goto', 'source')">编辑 Schema</button></dd></div>
        <div v-if="currentUsage?.draftRefs"><dt>当前 LOG</dt><dd>
          {{ draftRefs ? [
            currentUsage.draftRefs.elements && `${currentUsage.draftRefs.elements} 个元素`,
            currentUsage.draftRefs.header && '页眉',
            currentUsage.draftRefs.footer && '页脚',
            currentUsage.draftRefs.displayRules && `${currentUsage.draftRefs.displayRules} 条显示规则`,
            currentUsage.draftRefs.overrides && `${currentUsage.draftRefs.overrides} 条消息单独选用`,
          ].filter(Boolean).join('、') : '未引用' }}
        </dd></div>
      </dl>

      <div v-if="!readonly" class="lst__insert" role="group" aria-label="插入模板语法">
        <n-select v-model:value="pickedField" size="small" filterable :options="fieldOptions" placeholder="字段" class="lst__insert-field" />
        <n-button size="tiny" :disabled="!pickedField" @click="insertField('text')">插入文本</n-button>
        <n-button size="tiny" :disabled="!pickedField" @click="insertField('if')">条件块</n-button>
        <n-button size="tiny" @click="insertSlot">正文插槽</n-button>
        <n-select v-model:value="pickedPart" size="small" :options="partOptions" placeholder="片段" class="lst__insert-part" />
        <n-button size="tiny" :disabled="!pickedPart" @click="insertPart">引用片段</n-button>
      </div>
      <n-input
        ref="editorRef"
        v-model:value="html"
        type="textarea"
        class="lst__code"
        :readonly="readonly"
        :autosize="{ minRows: 10, maxRows: 28 }"
        spellcheck="false"
        :aria-label="`模板 ${current} 的 HTML`"
      />
      <ul v-if="issues[current]?.length" class="lst__issues">
        <li v-for="(m, i) in issues[current]" :key="i">{{ m }}</li>
      </ul>
      <p class="lst__hint">只允许受限标签与 data-log-* 绑定；不允许事件、内联样式、id、src/href。修改后预览会自动刷新（检查通过时）。</p>
    </section>
    <section v-else class="lst__detail lst__hint">还没有模板。</section>
  </div>
</template>

<style scoped>
.lst {
  display: grid;
  grid-template-columns: minmax(180px, 230px) minmax(0, 1fr);
  gap: 14px;
  min-height: 0;
}

.lst__list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.lst__list-head {
  display: flex;
  gap: 6px;
}

.lst__items {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 62vh;
  overflow: auto;
}

.lst__item {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 3px;
  padding: 7px 9px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: none;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.lst__item:hover {
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.05));
}

.lst__item.is-on {
  border-color: color-mix(in srgb, var(--primary-color, #2f6fde) 45%, transparent);
  background: color-mix(in srgb, var(--primary-color, #2f6fde) 10%, transparent);
}

.lst__item:focus-visible {
  outline: 2px solid var(--primary-color, #2f6fde);
  outline-offset: -2px;
}

.lst__item-id {
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 12px;
  overflow-wrap: anywhere;
}

.lst__roles {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
}

.lst__role {
  padding: 0 5px;
  border-radius: 999px;
  font-size: 10px;
  line-height: 16px;
  color: var(--sc-text-secondary, #666);
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.06));
}

.lst__role.is-warn {
  color: #9a5b00;
  background: rgba(217, 154, 30, 0.16);
}

.lst__detail {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.lst__detail-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.lst__title {
  margin: 0;
  font-size: 14px;
}

.lst__kinds {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
}

.lst__usage {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 4px 14px;
  margin: 0;
  padding: 8px 10px;
  border-radius: 8px;
  font-size: 12px;
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.04));
}

.lst__usage div {
  display: flex;
  gap: 6px;
  min-width: 0;
}

.lst__usage dt {
  flex: none;
  color: var(--sc-text-secondary, #666);
}

.lst__usage dd {
  margin: 0;
  overflow-wrap: anywhere;
}

.lst__insert {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.lst__insert-field {
  width: 180px;
}

.lst__insert-part {
  width: 130px;
}

.lst__code :deep(textarea) {
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 12px;
  line-height: 1.55;
}

.lst__issues {
  margin: 0;
  padding-left: 18px;
  font-size: 12px;
  color: #9a5b00;
}

.lst__hint {
  margin: 0;
  font-size: 12px;
  color: var(--sc-text-secondary, #666);
}

.lst__hint.is-warn {
  color: #9a5b00;
}

.lst__link {
  padding: 0;
  border: 0;
  background: none;
  color: var(--primary-color, #2f6fde);
  font: inherit;
  cursor: pointer;
}

@media (max-width: 720px) {
  .lst {
    grid-template-columns: minmax(0, 1fr);
  }

  .lst__items {
    flex-direction: row;
    overflow-x: auto;
    max-height: none;
  }

  .lst__item {
    flex: none;
    max-width: 200px;
  }
}
</style>
