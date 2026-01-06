<template>
  <div class="dice-tray">
    <div class="dice-tray__header">
      <div class="dice-tray__header-main">
        <span>默认骰：<strong>{{ currentDefaultDice }}</strong></span>
        <n-button v-if="canEditDefault" size="tiny" text type="primary" @click="modalVisible = true">
          修改
        </n-button>
      </div>
      <div class="dice-tray__header-actions">
        <slot name="header-actions"></slot>
        <n-button quaternary size="tiny" circle class="dice-tray__close" @click="handleTrayClose">
          <n-icon :component="CloseIcon" size="12" />
        </n-button>
      </div>
    </div>
    <div class="dice-tray__body">
      <div class="dice-tray__column dice-tray__column--quick">
        <div class="dice-tray__section-title">快捷骰</div>
        <div class="dice-tray__quick-grid">
          <button
            v-for="faces in quickFaces"
            :key="faces"
            type="button"
            class="dice-tray__quick-btn"
            @click="handleQuickSelect(faces)"
          >
            <span>d{{ faces }}</span>
            <span v-if="quickSelections[faces]" class="dice-tray__quick-count">×{{ quickSelections[faces] }}</span>
          </button>
        </div>
        <div v-if="hasQuickSelection" class="dice-tray__quick-summary">
          <div class="dice-tray__quick-expression">{{ quickExpression }}</div>
          <div class="dice-tray__quick-tools">
            <span class="dice-tray__quick-total">共 {{ quickTotal }} 次</span>
            <n-button text size="tiny" @click="clearQuickSelection">清空</n-button>
          </div>
        </div>
        <div class="dice-tray__macro-panel">
          <div class="dice-tray__macro-header">
            <div>
              <span class="dice-tray__macro-title">数字指令</span>
              <span class="dice-tray__macro-sequence" :class="{ 'is-active': digitSequence }">
                {{ digitSequence || '待输入' }}
              </span>
            </div>
            <div class="dice-tray__macro-actions">
              <n-button text size="tiny" :disabled="!digitSequence" @click="resetDigitSequence">清空</n-button>
              <n-button text size="tiny" :disabled="!currentChannelId" @click="openMacroManager()">管理</n-button>
            </div>
          </div>
          <div class="dice-tray__macro-keypad">
            <button
              v-for="digit in keypadDigits"
              :key="digit"
              type="button"
              class="dice-tray__macro-key"
              @click="handleDigitInput(digit)"
            >
              {{ digit }}
            </button>
          </div>
        </div>
      </div>
      <div v-if="!digitSequence" class="dice-tray__column dice-tray__column--form">
        <div class="dice-tray__section-title dice-tray__section-title--compact">自定义</div>
        <div class="dice-tray__form dice-tray__form--grid">
          <div class="dice-tray__form-row">
            <label>数量</label>
            <n-input-number v-model:value="count" :min="1" size="small" />
          </div>
          <div class="dice-tray__form-row">
            <label>面数</label>
            <n-input-number v-model:value="sides" :min="1" size="small" />
          </div>
          <div class="dice-tray__form-row">
            <label>修正</label>
            <n-input-number v-model:value="modifier" size="small" />
          </div>
          <div class="dice-tray__form-row">
            <label>理由</label>
            <n-input v-model:value="reason" size="small" placeholder="可选，例如攻击" />
          </div>
          <div class="dice-tray__actions">
            <n-button size="small" :disabled="!canSubmit" @click="handleInsert">
              插入到输入框
            </n-button>
            <n-button type="primary" size="small" :disabled="!canSubmit" @click="handleRoll">
              立即掷骰
            </n-button>
          </div>
        </div>
        <div class="dice-tray__history dice-tray__history--compact">
          <div class="dice-tray__section-title dice-tray__section-title--compact">最近检定</div>
          <div v-if="hasHistory" class="dice-tray__history-grid">
            <div v-for="item in displayedHistory" :key="item.id" class="dice-tray__history-card">
              <button type="button" class="dice-tray__history-roll" @click="handleHistoryRoll(item)">
                <span class="dice-tray__history-label">{{ formatHistoryLabel(item.expr) }}</span>
              </button>
              <div class="dice-tray__history-tools">
                <button type="button" class="dice-tray__history-tune" @click="openAdjustModalFromHistory(item)">微调</button>
                <button
                  type="button"
                  class="dice-tray__history-fav"
                  :class="{ 'is-active': item.favorite }"
                  :aria-pressed="item.favorite"
                  @click.stop="toggleFavorite(item.id)"
                >
                  <span v-if="item.favorite">★</span>
                  <span v-else>☆</span>
                </button>
              </div>
            </div>
          </div>
          <div v-else class="dice-tray__macro-empty">
            <p>暂无检定历史</p>
          </div>
        </div>
      </div>
      <div v-else class="dice-tray__column dice-tray__column--history">
        <div class="dice-tray__history dice-tray__history--compact">
          <div class="dice-tray__section-title dice-tray__section-title--compact">指令匹配</div>
          <div v-if="macroResults.length" class="dice-tray__history-list">
            <div
              v-for="entry in macroResults"
              :key="entry.id"
              class="dice-tray__macro-result"
              :class="{ 'dice-tray__macro-result--message': entry.kind === 'message' }"
            >
              <template v-if="entry.kind === 'macro'">
                <button type="button" class="dice-tray__macro-result-btn" @click="handleMacroExecute(entry.macro)">
                  <span class="dice-tray__macro-result-label">{{ entry.macro.label }}</span>
                  <span class="dice-tray__macro-result-expr">{{ formatHistoryLabel(entry.macro.expr) }}</span>
                </button>
                <n-button size="tiny" quaternary @click="openAdjustModal(entry.macro)">微调</n-button>
              </template>
              <template v-else>
                <div class="dice-tray__macro-result-message">{{ entry.message }}</div>
              </template>
            </div>
          </div>
          <div v-else class="dice-tray__macro-empty">
            <p>暂无匹配指令</p>
            <n-button text size="tiny" @click="openMacroManager()">新建指令</n-button>
          </div>
        </div>
      </div>
    </div>
  </div>
  <n-modal
    v-model:show="modalVisible"
    preset="card"
    class="dice-settings-modal"
    :bordered="false"
    title="修改默认骰"
  >
    <n-form size="small" label-placement="left" :show-feedback="false">
      <n-form-item label="面数">
        <n-input v-model:value="defaultDiceInput" placeholder="例如 d20" />
      </n-form-item>
      <n-alert v-if="defaultDiceError" type="warning" :show-icon="false">
        {{ defaultDiceError }}
      </n-alert>
      <div class="dice-tray__settings-actions">
        <n-button @click="modalVisible = false">取消</n-button>
        <n-button type="primary" :disabled="!!defaultDiceError" @click="handleSaveDefault">
          保存
        </n-button>
      </div>
    </n-form>
  </n-modal>

  <n-modal
    v-model:show="macroManagerVisible"
    preset="card"
    class="dice-macro-modal"
    :bordered="false"
    title="管理数字指令"
    @after-leave="resetMacroForm(null)"
  >
    <div class="dice-macro-modal__body">
      <div class="dice-macro-modal__toolbar">
        <span class="dice-macro-modal__channel">当前频道：{{ currentChannelId || '未选择' }}</span>
        <div class="dice-macro-modal__toolbar-actions">
          <n-button text size="tiny" :disabled="!macroList.length" @click="handleMacroExport">导出</n-button>
          <n-button text size="tiny" :disabled="!currentChannelId" @click="triggerMacroImport">导入</n-button>
          <input ref="importInputRef" class="dice-macro-import-input" type="file" accept="application/json" @change="handleMacroImportFile" />
        </div>
      </div>
      <div v-if="macroList.length" class="dice-macro-list">
        <div v-for="item in macroList" :key="item.id" class="dice-macro-item">
          <div class="dice-macro-item__head">
            <span class="dice-macro-item__digits">{{ item.digits }}</span>
            <span class="dice-macro-item__label">{{ item.label }}</span>
            <button type="button" class="dice-macro-item__fav" @click="toggleMacroFavorite(item.id)">
              <span v-if="item.favorite">★</span>
              <span v-else>☆</span>
            </button>
          </div>
          <div class="dice-macro-item__expr">{{ formatHistoryLabel(item.expr) }}</div>
          <div v-if="item.note" class="dice-macro-item__note">{{ item.note }}</div>
          <div class="dice-macro-item__actions">
            <n-button text size="tiny" @click="editMacro(item)">编辑</n-button>
            <n-button text size="tiny" type="error" @click="deleteMacro(item.id)">删除</n-button>
          </div>
        </div>
      </div>
      <div v-else class="dice-macro-empty">暂无指令，立即创建</div>
      <n-form size="small" label-placement="left" :show-feedback="false">
        <n-form-item label="数字">
          <n-input v-model:value="macroForm.digits" placeholder="例如 12" maxlength="8" />
        </n-form-item>
        <n-form-item label="名称">
          <n-input v-model:value="macroForm.label" placeholder="例如 攻击" />
        </n-form-item>
        <n-form-item label="表达式">
          <n-input v-model:value="macroForm.expr" placeholder="例如 .r2d6+3" />
        </n-form-item>
        <n-form-item label="备注">
          <n-input v-model:value="macroForm.note" placeholder="可选" />
        </n-form-item>
        <n-alert v-if="macroFormError" type="warning" :show-icon="false">{{ macroFormError }}</n-alert>
        <div class="dice-macro-modal__actions">
          <n-button @click="resetMacroForm(null)">重置</n-button>
          <n-button type="primary" @click="saveMacro">{{ editingMacroId ? '保存修改' : '添加指令' }}</n-button>
        </div>
      </n-form>
    </div>
  </n-modal>

  <n-modal
    v-model:show="adjustModalVisible"
    preset="card"
    class="dice-adjust-modal"
    :bordered="false"
    :title="`调整：${adjustMacroLabel || '指令'}`"
  >
    <n-form size="small" label-placement="left" :show-feedback="false">
      <n-form-item label="掷骰表达式">
        <n-input v-model:value="adjustExpression" type="textarea" :rows="3" placeholder="直接修改表达式，如 .r2d6+1" />
      </n-form-item>
      <div class="dice-adjust-modal__actions">
        <n-button @click="cancelAdjustRoll">取消</n-button>
        <n-button type="primary" @click="confirmAdjustRoll">掷骰</n-button>
      </div>
    </n-form>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch, reactive } from 'vue';
import { ensureDefaultDiceExpr, isValidDefaultDiceExpr } from '@/utils/dice';
import { api } from '@/stores/_config';
import { useChatStore } from '@/stores/chat';
import { useMessage } from 'naive-ui';
import type { DiceMacro } from '@/types';
import { Close as CloseIcon } from '@vicons/ionicons5';
import { useDiceHistory, type DiceHistoryItem } from '@/views/chat/composables/useDiceHistory';

const props = withDefaults(defineProps<{
  defaultDice?: string
  canEditDefault?: boolean
  builtInDiceEnabled?: boolean
  botFeatureEnabled?: boolean
}>(), {
  defaultDice: 'd20',
  canEditDefault: false,
  builtInDiceEnabled: true,
  botFeatureEnabled: false,
});

const emit = defineEmits<{
  (event: 'insert', expr: string): void
  (event: 'roll', expr: string): void
  (event: 'update-default', expr: string): void
  (event: 'close'): void
}>();

const chat = useChatStore();
const message = useMessage();

const handleTrayClose = () => {
  emit('close');
};

const quickFaces = [2, 4, 6, 8, 10, 12, 20, 100];
const quickSelections = ref<Record<number, number>>({});
const count = ref(1);
const sides = ref<number | null>(null);
const modifier = ref(0);
const reason = ref('');
const modalVisible = ref(false);
const defaultDiceInput = ref(ensureDefaultDiceExpr(props.defaultDice));

const {
  displayedHistory,
  hasHistory,
  recordHistory,
  toggleFavorite,
} = useDiceHistory();

type MacroResultEntry =
  | { id: string; kind: 'macro'; macro: DiceMacro }
  | { id: string; kind: 'message'; message: string };

const MACRO_RESULTS_LIMIT = 5;
const keypadDigits = ['1', '2', '3', '4', '5', '6', '7', '8', '9'];

const macrosByChannel = ref<Record<string, DiceMacro[]>>({});
const macrosLoading = ref(false);
const digitSequence = ref('');
const macroManagerVisible = ref(false);
const macroForm = reactive({ digits: '', label: '', expr: '', note: '' });
const macroFormError = ref('');
const editingMacroId = ref<string | null>(null);
const adjustModalVisible = ref(false);
const adjustExpression = ref('');
const adjustMacroLabel = ref('');
const importInputRef = ref<HTMLInputElement | null>(null);

const currentChannelId = computed(() => chat.curChannel?.id || '');

const resetMacroForm = (macro?: DiceMacro | null) => {
  if (macro) {
    macroForm.digits = macro.digits;
    macroForm.label = macro.label;
    macroForm.expr = macro.expr;
    macroForm.note = macro.note || '';
    editingMacroId.value = macro.id;
  } else {
    macroForm.digits = '';
    macroForm.label = '';
    macroForm.expr = '';
    macroForm.note = '';
    editingMacroId.value = null;
  }
  macroFormError.value = '';
};

const openMacroManager = (macro?: DiceMacro | null) => {
  resetMacroForm(macro ?? null);
  macroManagerVisible.value = true;
};

const validateMacroForm = () => {
  if (!macroForm.digits) {
    macroFormError.value = '请输入数字序列';
    return false;
  }
  if (!/^[1-9]+$/.test(macroForm.digits)) {
    macroFormError.value = '只支持数字 1-9';
    return false;
  }
  if (!macroForm.label.trim()) {
    macroFormError.value = '请输入名称';
    return false;
  }
  if (!macroForm.expr.trim()) {
    macroFormError.value = '请输入掷骰表达式';
    return false;
  }
  macroFormError.value = '';
  return true;
};

const setMacrosForChannel = (channelId: string, items: DiceMacro[]) => {
  macrosByChannel.value = {
    ...macrosByChannel.value,
    [channelId]: items,
  };
};

const upsertMacroForChannel = (channelId: string, macro: DiceMacro) => {
  const list = (macrosByChannel.value[channelId] || []).slice();
  const idx = list.findIndex((item) => item.id === macro.id);
  if (idx !== -1) {
    list[idx] = macro;
  } else {
    list.unshift(macro);
  }
  setMacrosForChannel(channelId, list);
};

const removeMacroForChannel = (channelId: string, macroId: string) => {
  const list = (macrosByChannel.value[channelId] || []).filter((item) => item.id !== macroId);
  setMacrosForChannel(channelId, list);
};

const loadChannelMacros = async (channelId: string, force = false) => {
  if (!channelId) return;
  if (!force && macrosByChannel.value[channelId]) {
    return;
  }
  macrosLoading.value = true;
  try {
    const resp = await api.get<{ items: DiceMacro[] }>(`api/v1/channels/${channelId}/dice-macros`);
    setMacrosForChannel(channelId, resp.data.items || []);
  } catch (error) {
    console.error(error);
    message.error('加载数字指令失败');
  } finally {
    macrosLoading.value = false;
  }
};

watch(currentChannelId, (channelId) => {
  digitSequence.value = '';
  if (channelId) {
    loadChannelMacros(channelId);
  }
}, { immediate: true });

const macroList = computed(() => {
  const items = macrosByChannel.value[currentChannelId.value] || [];
  return [...items].sort((a, b) => {
    if (!!b.favorite !== !!a.favorite) {
      return (b.favorite ? 1 : 0) - (a.favorite ? 1 : 0);
    }
    const bTs = b.updatedAt ? Date.parse(b.updatedAt) : 0;
    const aTs = a.updatedAt ? Date.parse(a.updatedAt) : 0;
    return (bTs || 0) - (aTs || 0);
  });
});

const saveMacro = async () => {
  if (!validateMacroForm()) {
    return;
  }
  const channelId = currentChannelId.value;
  if (!channelId) {
    message.warning('请先选择频道');
    return;
  }
  const existing = editingMacroId.value ? macroList.value.find(item => item.id === editingMacroId.value) : null;
  const payload = {
    digits: macroForm.digits,
    label: macroForm.label.trim(),
    expr: macroForm.expr.trim(),
    note: macroForm.note.trim(),
    favorite: existing?.favorite ?? false,
  };
  try {
    const baseUrl = `api/v1/channels/${channelId}/dice-macros`;
    let resp;
    if (editingMacroId.value) {
      resp = await api.put<{ item: DiceMacro }>(`${baseUrl}/${editingMacroId.value}`, payload);
    } else {
      resp = await api.post<{ item: DiceMacro }>(baseUrl, payload);
    }
    upsertMacroForChannel(channelId, resp.data.item);
    message.success(editingMacroId.value ? '已更新指令' : '已添加指令');
    resetMacroForm(null);
  } catch (error) {
    console.error(error);
    message.error('保存指令失败');
  }
};

const editMacro = (macro: DiceMacro) => {
  openMacroManager(macro);
};

const deleteMacro = async (macroId: string) => {
  const channelId = currentChannelId.value;
  if (!channelId) return;
  try {
    await api.delete(`api/v1/channels/${channelId}/dice-macros/${macroId}`);
    removeMacroForChannel(channelId, macroId);
    if (editingMacroId.value === macroId) {
      resetMacroForm(null);
    }
    message.success('已删除指令');
  } catch (error) {
    console.error(error);
    message.error('删除指令失败');
  }
};

const toggleMacroFavorite = async (macroId: string) => {
  const channelId = currentChannelId.value;
  if (!channelId) return;
  const target = macroList.value.find(item => item.id === macroId);
  if (!target) return;
  const payload = {
    digits: target.digits,
    label: target.label,
    expr: target.expr,
    note: target.note,
    favorite: !target.favorite,
  };
  try {
    const resp = await api.put<{ item: DiceMacro }>(`api/v1/channels/${channelId}/dice-macros/${macroId}`, payload);
    upsertMacroForChannel(channelId, resp.data.item);
  } catch (error) {
    console.error(error);
    message.error('更新收藏状态失败');
  }
};

const baseMacroResults = computed(() => {
  const seq = digitSequence.value.trim();
  if (!seq) return [] as DiceMacro[];
  const starts = macroList.value.filter((item) => item.digits.startsWith(seq));
  const contains = macroList.value.filter(
    (item) => !item.digits.startsWith(seq) && item.digits.includes(seq),
  );
  return [...starts, ...contains];
});

const macroResults = computed<MacroResultEntry[]>(() => {
  const seq = digitSequence.value.trim();
  if (!seq) return [];
  const matched = baseMacroResults.value.slice(0, MACRO_RESULTS_LIMIT).map((macro) => ({
    id: macro.id,
    kind: 'macro' as const,
    macro,
  }));
  if (seq === '555') {
    matched.push({
      id: 'macro-easter-555',
      kind: 'message',
      message: 'Standing by! Complete!',
    });
  }
  return matched;
});

const handleDigitInput = (digit: string) => {
  digitSequence.value += digit;
};

const resetDigitSequence = () => {
  digitSequence.value = '';
};

const handleMacroExecute = (macro: DiceMacro) => {
  emit('roll', macro.expr);
  recordHistory(macro.expr);
  resetDigitSequence();
};

const openAdjustModal = (macro: DiceMacro) => {
  adjustExpression.value = macro.expr;
  adjustMacroLabel.value = macro.label;
  adjustModalVisible.value = true;
};

const confirmAdjustRoll = () => {
  const expr = adjustExpression.value.trim();
  if (!expr) {
    return;
  }
  emit('roll', expr);
  recordHistory(expr);
  adjustModalVisible.value = false;
  resetDigitSequence();
};

const cancelAdjustRoll = () => {
  adjustModalVisible.value = false;
};

const openAdjustModalFromHistory = (item: DiceHistoryItem) => {
  adjustExpression.value = item.expr;
  adjustMacroLabel.value = formatHistoryLabel(item.expr);
  adjustModalVisible.value = true;
};

const triggerMacroImport = () => {
  if (!currentChannelId.value) {
    message.warning('请先选择频道');
    return;
  }
  importInputRef.value?.click();
};

const handleMacroImportFile = async (event: Event) => {
  const input = event.target as HTMLInputElement;
  const file = input?.files?.[0];
  if (!file) {
    return;
  }
  const channelId = currentChannelId.value;
  if (!channelId) {
    input.value = '';
    return;
  }
  try {
    const text = await file.text();
    const parsed = JSON.parse(text);
    const rawList = Array.isArray(parsed?.macros) ? parsed.macros : (Array.isArray(parsed) ? parsed : []);
    if (!Array.isArray(rawList)) {
      throw new Error('invalid format');
    }
    const sanitized = rawList
      .map((item: any) => ({
        digits: String(item?.digits || '').replace(/[^1-9]/g, ''),
        label: String(item?.label || '').trim(),
        expr: String(item?.expr || '').trim(),
        note: String(item?.note || '').trim(),
        favorite: !!item?.favorite,
      }))
      .filter(entry => entry.digits && entry.label && entry.expr);
    if (!sanitized.length) {
      throw new Error('empty list');
    }
    const resp = await api.post<{ items: DiceMacro[] }>(`api/v1/channels/${channelId}/dice-macros/import`, { macros: sanitized });
    setMacrosForChannel(channelId, resp.data.items || []);
    message.success('导入指令成功');
  } catch (error) {
    console.error(error);
    message.error('导入失败，请检查文件格式');
  } finally {
    if (input) {
      input.value = '';
    }
  }
};

const handleMacroExport = async () => {
  if (!macroList.value.length) {
    message.info('暂无指令可导出');
    return;
  }
  const data = {
    channelId: currentChannelId.value,
    macros: macroList.value.map(item => ({
      digits: item.digits,
      label: item.label,
      expr: item.expr,
      note: item.note,
      favorite: item.favorite,
    })),
  };
  const text = JSON.stringify(data, null, 2);
  try {
    if (navigator?.clipboard) {
      await navigator.clipboard.writeText(text);
      message.success('已复制指令到剪贴板');
      return;
    }
    throw new Error('clipboard unavailable');
  } catch (error) {
    console.warn('clipboard copy failed', error);
    const blob = new Blob([text], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `dice-macros-${currentChannelId.value || 'channel'}.json`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    message.success('已下载指令文件');
  }
};

const formatHistoryLabel = (expr: string) => expr.replace(/^\.r/, 'r').replace(/\s+/g, ' ');

const currentDefaultDice = computed(() => ensureDefaultDiceExpr(props.defaultDice));

watch(() => props.defaultDice, (value) => {
  defaultDiceInput.value = ensureDefaultDiceExpr(value);
  if (!sides.value) {
    sides.value = parseInt(defaultDiceInput.value.slice(1), 10) || 20;
  }
}, { immediate: true });

const sanitizedReason = computed(() => reason.value.trim());

const quickExpression = computed(() => {
  const entries = Object.entries(quickSelections.value).filter(([, count]) => count > 0);
  if (!entries.length) {
    return '';
  }
  // 只在内置骰关闭且 bot 开启时，后续骰子不带 .r 前缀
  const useBotMode = !props.builtInDiceEnabled && props.botFeatureEnabled;
  return entries
    .map(([faces, count], index) => {
      if (index === 0) {
        return `.r${count}d${faces}`;
      }
      return useBotMode ? `${count}d${faces}` : `.r${count}d${faces}`;
    })
    .join(' + ');
});

const quickTotal = computed(() =>
  Object.values(quickSelections.value).reduce((sum, count) => sum + count, 0)
);

const hasQuickSelection = computed(() => quickTotal.value > 0);

const expression = computed(() => {
  if (!count.value || !sides.value) {
    return '';
  }
  const amount = Math.max(1, Math.floor(count.value));
  const face = Math.max(1, Math.floor(sides.value));
  const parts = [`.r${amount}d${face}`];
  if (modifier.value) {
    const delta = Math.trunc(modifier.value);
    if (delta > 0) {
      parts.push(`+${delta}`);
    } else {
      parts.push(`${delta}`);
    }
  }
  if (sanitizedReason.value) {
    parts.push(`#${sanitizedReason.value}`);
  }
  return parts.join(' ');
});

const combinedExpression = computed(() => quickExpression.value || expression.value);

const canSubmit = computed(() => !!combinedExpression.value);

const handleQuickSelect = (faces: number) => {
  quickSelections.value = {
    ...quickSelections.value,
    [faces]: (quickSelections.value[faces] || 0) + 1,
  };
};

const clearQuickSelection = () => {
  quickSelections.value = {};
};

const handleInsert = () => {
  if (canSubmit.value && combinedExpression.value) {
    emit('insert', combinedExpression.value);
    recordHistory(combinedExpression.value);
    if (hasQuickSelection.value) {
      clearQuickSelection();
    }
  }
};

const handleRoll = () => {
  if (canSubmit.value && combinedExpression.value) {
    emit('roll', combinedExpression.value);
    recordHistory(combinedExpression.value);
    if (hasQuickSelection.value) {
      clearQuickSelection();
    }
  }
};

const handleHistoryRoll = (item: DiceHistoryItem) => {
  emit('roll', item.expr);
  recordHistory(item.expr);
};

const defaultDiceError = computed(() => {
  if (!defaultDiceInput.value) {
    return '请输入默认骰，例如 d20';
  }
  if (!isValidDefaultDiceExpr(defaultDiceInput.value)) {
    return '格式不正确，示例：d20';
  }
  return '';
});

const handleSaveDefault = () => {
  if (defaultDiceError.value) {
    return;
  }
  emit('update-default', ensureDefaultDiceExpr(defaultDiceInput.value));
  modalVisible.value = false;
};
</script>

<style scoped>
.dice-tray {
  min-width: 280px;
  max-width: 420px;
  padding: 10px;
  background: var(--sc-bg-elevated, #fff);
  border: 1px solid var(--sc-border-strong, #e5e7eb);
  border-radius: 10px;
  color: var(--sc-fg-primary, #111);
}

.dice-tray__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: 13px;
}

.dice-tray__header-main {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.dice-tray__header-actions {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.dice-tray__close {
  color: var(--sc-fg-muted, #6b7280);
}

.dice-tray__close:hover {
  color: var(--sc-fg-primary, #111);
  background: rgba(15, 23, 42, 0.08);
}

.dice-tray__body {
  display: flex;
  gap: 4px;
}

.dice-tray__column {
  flex: 1;
  padding: 6px;
  border-radius: 8px;
  background: var(--sc-bg-layer, #fafafa);
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.dice-tray__column--quick {
  flex: 0 0 110px;
}

.dice-tray__column--form,
.dice-tray__column--history {
  flex: 1;
}

.dice-tray__section-title {
  font-size: 12px;
  color: var(--sc-fg-muted, #666);
  margin-bottom: 6px;
}

.dice-tray__section-title--compact {
  margin-bottom: 0.3rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #6b7280);
}

.dice-tray__quick-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px;
}

.dice-tray__quick-btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--sc-border-mute, #d1d5db);
  border-radius: 8px;
  padding: 0.28rem 0;
  font-size: 0.84rem;
  background: var(--sc-bg-layer, #fff);
  color: var(--sc-fg-primary, #111);
  transition: background 0.2s ease, color 0.2s ease;
}

.dice-tray__quick-btn:hover {
  background: rgba(15, 23, 42, 0.04);
}

.dice-tray__quick-count {
  position: absolute;
  top: -0.35rem;
  right: -0.35rem;
  font-size: 0.65rem;
  background: var(--sc-accent, #2563eb);
  color: #fff;
  border-radius: 999px;
  padding: 0.05rem 0.35rem;
  box-shadow: 0 2px 6px rgba(15, 23, 42, 0.2);
}

.dice-tray__quick-summary {
  margin-top: 0.45rem;
  padding: 0.35rem 0.45rem;
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.04);
  font-size: 0.8rem;
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}

.dice-tray__quick-expression {
  word-break: break-all;
  font-family: var(--sc-code-font, 'SFMono-Regular', Menlo, Consolas, monospace);
}

.dice-tray__quick-tools {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.8rem;
}

.dice-tray__macro-panel {
  margin-top: 0.75rem;
  padding-top: 0.75rem;
  border-top: 1px dashed var(--sc-border-mute, #d1d5db);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dice-tray__macro-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.82rem;
}

.dice-tray__macro-title {
  font-weight: 600;
  margin-right: 0.4rem;
}

.dice-tray__macro-sequence {
  font-family: var(--sc-code-font, 'SFMono-Regular', Menlo, Consolas, monospace);
  padding: 0.1rem 0.35rem;
  border-radius: 4px;
  background: rgba(15, 23, 42, 0.05);
  color: var(--sc-fg-primary, #111);
}

.dice-tray__macro-sequence.is-active {
  background: rgba(37, 99, 235, 0.1);
  color: var(--sc-accent, #2563eb);
}

.dice-tray__macro-actions {
  display: flex;
  gap: 0.25rem;
}

.dice-tray__macro-keypad {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.28rem;
}

.dice-tray__macro-key {
  border: 1px solid var(--sc-border-mute, #d1d5db);
  border-radius: 8px;
  padding: 0.3rem 0;
  font-size: 0.95rem;
  font-weight: 600;
  background: var(--sc-bg-layer, #fff);
  color: var(--sc-fg-primary, #111);
  transition: background 0.2s ease, border-color 0.2s ease;
}

.dice-tray__macro-key:hover {
  background: rgba(15, 23, 42, 0.08);
}

.dice-tray__macro-results,
.dice-tray__macro-empty {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.dice-tray__macro-result {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.35rem;
}

.dice-tray__macro-result-btn {
  flex: 1;
  border: 1px solid var(--sc-border-mute, #d1d5db);
  border-radius: 6px;
  padding: 0.35rem 0.5rem;
  text-align: left;
  background: var(--sc-bg-layer, #f8fafc);
  color: var(--sc-fg-primary, #111);
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.dice-tray__macro-result-btn:hover {
  background: rgba(15, 23, 42, 0.08);
}

.dice-tray__macro-result-label {
  font-weight: 600;
}

.dice-tray__macro-result-expr {
  font-size: 0.75rem;
  color: var(--sc-fg-muted, #6b7280);
}

.dice-tray__macro-result-message {
  width: 100%;
  text-align: center;
  padding: 0.4rem 0.5rem;
  border-radius: 6px;
  background: rgba(37, 99, 235, 0.1);
  color: var(--sc-accent, #2563eb);
  font-weight: 600;
}

.dice-tray__macro-result--message {
  justify-content: center;
}

.dice-tray__macro-empty {
  font-size: 0.85rem;
  color: var(--sc-fg-muted, #6b7280);
}

.dice-tray__form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dice-tray__form--grid {
  gap: 0.3rem;
}

.dice-tray__form-row {
  display: grid;
  grid-template-columns: 48px 1fr;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.78rem;
  color: var(--sc-text-secondary, #6b7280);
}

.dice-tray__form-row label {
  font-weight: 500;
}

.dice-tray__form :deep(.n-form-item) {
  margin: 0;
}

.dice-tray__actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  margin-top: 8px;
}

.dice-tray__settings-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
}

.dice-tray__history {
  margin-top: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid var(--sc-border-mute, #e2e8f0);
  color: var(--sc-fg-primary, #111);
}

.dice-tray__history--compact {
  border-top: none;
  padding-top: 0;
  margin-top: 0;
}


.dice-tray__history-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.35rem;
}

.dice-tray__history-card {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.35));
  border-radius: 0.45rem;
  padding: 0.35rem;
  background: var(--sc-bg-layer-strong, rgba(248, 250, 252, 0.85));
}

.dice-tray__history-roll {
  flex: 1;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--sc-fg-primary, #111);
  padding: 0;
  font-size: 0.75rem;
  text-align: left;
  transition: color 0.2s ease;
}

.dice-tray__history-roll:hover {
  color: var(--sc-accent, #2563eb);
}

.dice-tray__history-label {
  display: inline-block;
  width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dice-tray__history-tools {
  display: inline-flex;
  gap: 0.25rem;
  align-items: center;
}

.dice-tray__history-fav {
  width: 1.2rem;
  height: 1.2rem;
  border-radius: 999px;
  border: 1px solid var(--sc-border-mute, #d1d5db);
  background: transparent;
  color: var(--sc-fg-muted, #6b7280);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 0.75rem;
  transition: background 0.2s ease, color 0.2s ease, border-color 0.2s ease;
}

.dice-tray__history-fav.is-active {
  color: var(--sc-accent, #2563eb);
  border-color: currentColor;
  background: rgba(37, 99, 235, 0.08);
}

.dice-macro-modal :global(.n-card__content),
.dice-adjust-modal :global(.n-card__content) {
  padding-top: 0;
}

.dice-macro-modal :global(.n-card) {
  width: min(520px, 90vw);
}

.dice-adjust-modal :global(.n-card) {
  width: min(380px, 90vw);
}

.dice-macro-modal__body {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.dice-macro-modal__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.85rem;
  color: var(--sc-fg-muted, #6b7280);
}

.dice-macro-modal__toolbar-actions {
  display: flex;
  gap: 0.35rem;
  align-items: center;
}

.dice-macro-import-input {
  display: none;
}

.dice-macro-list {
  max-height: 220px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.dice-macro-item {
  border: 1px solid var(--sc-border-mute, #d1d5db);
  border-radius: 8px;
  padding: 0.4rem 0.5rem;
  background: var(--sc-bg-layer, #fff);
}

.dice-macro-item__head {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.dice-macro-item__digits {
  font-family: var(--sc-code-font, 'SFMono-Regular', Menlo, Consolas, monospace);
  font-weight: 600;
}

.dice-macro-item__label {
  font-weight: 600;
  flex: 1;
}

.dice-macro-item__fav {
  border: none;
  background: transparent;
  color: var(--sc-accent, #2563eb);
  font-size: 1rem;
}

.dice-macro-item__expr {
  font-size: 0.8rem;
  margin-top: 0.2rem;
}

.dice-macro-item__note {
  font-size: 0.75rem;
  color: var(--sc-fg-muted, #6b7280);
}

.dice-macro-item__actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.35rem;
  margin-top: 0.2rem;
}

.dice-macro-empty {
  font-size: 0.85rem;
  color: var(--sc-fg-muted, #6b7280);
}

.dice-macro-modal__actions,
.dice-adjust-modal__actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 0.5rem;
}

:global([data-display-palette='night']) .dice-tray {
  background: var(--sc-bg-elevated, #2a282a);
  border-color: var(--sc-border-strong, rgba(255, 255, 255, 0.12));
  color: var(--sc-fg-primary, #eee);
}

:global([data-display-palette='night']) .dice-tray__column {
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: var(--sc-fg-primary, #eee);
}

:global([data-display-palette='night']) .dice-tray__column--quick {
  background: rgba(255, 255, 255, 0.03);
}

:global([data-display-palette='night']) .dice-tray__column--form {
  background: rgba(255, 255, 255, 0.06);
}

:global([data-display-palette='night']) .dice-tray__close {
  color: rgba(226, 232, 240, 0.8);
}

:global([data-display-palette='night']) .dice-tray__close:hover {
  color: var(--sc-fg-primary, #f8fafc);
  background: rgba(255, 255, 255, 0.12);
}

:global([data-display-palette='night']) .dice-tray__quick-btn {
  border-color: rgba(255, 255, 255, 0.2);
  background: rgba(15, 23, 42, 0.35);
  color: var(--sc-fg-primary, #eee);
}

:global([data-display-palette='night']) .dice-tray__quick-btn:hover {
  background: rgba(255, 255, 255, 0.12);
}

:global([data-display-palette='night']) .dice-tray__quick-count {
  background: var(--sc-accent-night, #60a5fa);
  color: #0f172a;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.35);
}

:global([data-display-palette='night']) .dice-tray__quick-summary {
  background: rgba(255, 255, 255, 0.08);
}

:global([data-display-palette='night']) .dice-tray__history {
  border-top-color: rgba(255, 255, 255, 0.12);
  color: var(--sc-fg-primary, #f8fafc);
}

:global([data-display-palette='night']) .dice-tray__history-card {
  background: rgba(18, 24, 39, 0.65);
  border-color: rgba(255, 255, 255, 0.16);
  color: var(--sc-fg-primary, #f8fafc);
}

:global([data-display-palette='night']) .dice-tray__history-roll {
  border-color: rgba(255, 255, 255, 0.2);
  background: rgba(15, 23, 42, 0.35);
  color: var(--sc-fg-primary, #f8fafc);
}

:global([data-display-palette='night']) .dice-tray__history-roll:hover {
  background: rgba(255, 255, 255, 0.12);
}

:global([data-display-palette='night']) .dice-tray__history-fav {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(226, 232, 240, 0.8);
}

:global([data-display-palette='night']) .dice-tray__history-fav.is-active {
  color: var(--sc-accent-night, #60a5fa);
  border-color: var(--sc-accent-night, #60a5fa);
  background: rgba(96, 165, 250, 0.15);
}

:global([data-display-palette='night']) .dice-tray__history-tune {
  border-color: rgba(255, 255, 255, 0.2);
  color: var(--sc-fg-primary, #f8fafc);
  background: rgba(15, 23, 42, 0.35);
}

:global([data-display-palette='night']) .dice-tray__history-tune:hover {
  background: rgba(255, 255, 255, 0.12);
}

:global([data-display-palette='night']) .dice-tray__macro-panel {
  border-top-color: rgba(255, 255, 255, 0.12);
}

:global([data-display-palette='night']) .dice-tray__macro-sequence {
  background: rgba(255, 255, 255, 0.08);
  color: var(--sc-fg-primary, #f8fafc);
}

:global([data-display-palette='night']) .dice-tray__macro-sequence.is-active {
  background: rgba(96, 165, 250, 0.2);
  color: var(--sc-accent-night, #60a5fa);
}

:global([data-display-palette='night']) .dice-tray__macro-key {
  border-color: rgba(255, 255, 255, 0.2);
  background: rgba(15, 23, 42, 0.35);
  color: var(--sc-fg-primary, #f8fafc);
}

:global([data-display-palette='night']) .dice-tray__macro-key:hover {
  background: rgba(96, 165, 250, 0.18);
}

:global([data-display-palette='night']) .dice-tray__macro-result-btn {
  border-color: rgba(255, 255, 255, 0.2);
  background: rgba(15, 23, 42, 0.35);
  color: var(--sc-fg-primary, #f8fafc);
}

:global([data-display-palette='night']) .dice-tray__macro-result-expr {
  color: rgba(226, 232, 240, 0.8);
}

:global([data-display-palette='night']) .dice-tray__macro-result-message {
  background: rgba(96, 165, 250, 0.2);
  color: var(--sc-accent-night, #60a5fa);
}

:global([data-display-palette='night']) .dice-macro-item {
  border-color: rgba(255, 255, 255, 0.15);
  background: rgba(15, 23, 42, 0.4);
}

:global([data-display-palette='night']) .dice-macro-item__note {
  color: rgba(226, 232, 240, 0.7);
}

:global([data-display-palette='night']) .dice-macro-empty {
  color: rgba(226, 232, 240, 0.7);
}

:global([data-display-palette='night']) .dice-macro-modal :global(.n-card),
:global([data-display-palette='night']) .dice-adjust-modal :global(.n-card) {
  background: var(--sc-bg-elevated, #2a282a);
  color: var(--sc-fg-primary, #f8fafc);
  border: 1px solid rgba(255, 255, 255, 0.12);
}

.dice-settings-modal :global(.n-card__content) {
  padding-top: 0;
}

.dice-settings-modal :global(.n-card) {
  background: var(--sc-bg-elevated, #fff);
  color: var(--sc-fg-primary, #111);
  max-width: 360px;
  width: min(360px, 90vw);
  margin: 0 auto;
}

::global([data-display-palette='night']) .dice-settings-modal :global(.n-card) {
  background: var(--sc-bg-elevated, #2a282a);
  color: #fff;
  border: 1px solid rgba(255, 255, 255, 0.12);
}
</style>
.dice-tray__history-tools {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.dice-tray__history-tune {
  border: 1px solid var(--sc-border-mute, #d1d5db);
  border-radius: 999px;
  padding: 0.15rem 0.6rem;
  font-size: 0.75rem;
  color: var(--sc-fg-primary, #111);
  background: var(--sc-bg-layer, #fff);
  transition: background 0.2s ease, color 0.2s ease;
}

.dice-tray__history-tune:hover {
  background: rgba(15, 23, 42, 0.08);
}
