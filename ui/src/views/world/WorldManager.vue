<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useUtilsStore } from '@/stores/utils';
import { useDialog, useMessage } from 'naive-ui';
import { DEFAULT_CARD_TEMPLATE } from '@/utils/characterCardTemplate';
import WorldDiceDefaultsFields from '@/views/world/WorldDiceDefaultsFields.vue';
import WorldCreateCoverField from '@/views/world/WorldCreateCoverField.vue';
import {
  WORLD_DESCRIPTION_MAX_DISPLAY_CHARS,
  WORLD_DESCRIPTION_MAX_WIDTH_UNITS,
  formatDisplayWidthAsCharCount,
  getTextDisplayWidthUnits,
  truncateTextByDisplayWidth,
} from '@/utils/displayWidth';
import { resolveActionErrorMessage } from '@/utils/errorMessage';
import CursorThemeEditorModal from '@/components/cursor/CursorThemeEditorModal.vue';
import { normalizeCursorTheme } from '@/services/cursor/cursorRuntime';
import { resolveMessageSortBasis } from '@/utils/messageSortBasis';

const props = defineProps<{ worldId: string, visible: boolean }>();
const emit = defineEmits(['update:visible', 'deleted']);
const chat = useChatStore();
const utils = useUtilsStore();
const message = useMessage();
const dialog = useDialog();
const form = ref<any>({});
const loading = ref(false);
const botOptionsLoading = ref(false);
const botList = ref<any[]>([]);
const cursorThemeVisible = ref(false);
const coverUploading = ref(false);
const coverEditing = ref(false);

const botSelectOptions = computed(() => botList.value.map((item) => ({
  label: item.nick || item.username || item.name || 'Bot',
  value: item.id,
})));

const close = () => emit('update:visible', false);

const visibilityOptions = [
  { label: '公开', value: 'public' },
  { label: '私有', value: 'private' },
  { label: '不公开访问链接', value: 'unlisted' },
];

const loadBotOptions = async () => {
  botOptionsLoading.value = true;
  try {
    const resp = await chat.botList();
    botList.value = resp?.items || [];
  } catch (e: any) {
    message.error(e?.response?.data?.message || '加载 BOT 列表失败');
  } finally {
    botOptionsLoading.value = false;
  }
};

watch(() => [props.worldId, props.visible] as const, async ([id, visible]) => {
  if (!id || !visible) return;
  try {
    const [detail] = await Promise.all([
      chat.worldDetail(id),
      loadBotOptions(),
    ]);
    form.value = {
      name: detail.world?.name,
      description: detail.world?.description,
      visibility: detail.world?.visibility,
      messageSortBasis: resolveMessageSortBasis([detail.world?.messageSortBasis], utils.config?.messageSortBasis),
      allowAdminEditMessages: detail.world?.allowAdminEditMessages ?? false,
      allowMemberAccessLogLayout: (detail.world as any)?.allowMemberAccessLogLayout ?? false,
      allowManageOtherUserChannelIdentities: detail.world?.allowManageOtherUserChannelIdentities ?? false,
      allowMemberEditKeywords: detail.world?.allowMemberEditKeywords ?? false,
      strictWhisperPrivacy: detail.world?.strictWhisperPrivacy ?? true,
      exportRespectMessageVisibility: detail.world?.exportRespectMessageVisibility ?? true,
      allowMemberExportMessages: detail.world?.allowMemberExportMessages ?? true,
      channelDefaultDiceMode: detail.world?.channelDefaultDiceMode || 'builtin',
      channelDefaultBotIds: Array.isArray((detail.world as any)?.channelDefaultBotIds) ? (detail.world as any).channelDefaultBotIds : [],
      channelDefaultBotId: detail.world?.channelDefaultBotId || '',
      channelDefaultEventBotIds: Array.isArray((detail.world as any)?.channelDefaultEventBotIds) ? (detail.world as any).channelDefaultEventBotIds : [],
      characterCardBadgeTemplate: detail.world?.characterCardBadgeTemplate ?? '',
      cursorTheme: normalizeCursorTheme(detail.world?.cursorTheme, 'world'),
      coverAttachmentId: detail.world?.coverAttachmentId || '',
    };
  } catch (e: any) {
    message.error(e?.response?.data?.message || '加载世界信息失败');
  }
}, { immediate: true });

const save = async () => {
  if (loading.value || coverUploading.value || coverEditing.value) return;
  if (form.value.channelDefaultDiceMode === 'bot' && !form.value.channelDefaultBotId) {
    message.error('选择 BOT 掷骰时必须指定默认 BOT');
    return;
  }
  if (form.value.channelDefaultDiceMode === 'bot' && (!Array.isArray(form.value.channelDefaultBotIds) || form.value.channelDefaultBotIds.length === 0)) {
    message.error('请至少绑定一个 BOT');
    return;
  }
  loading.value = true;
  try {
    await chat.worldUpdate(props.worldId, form.value);
    message.success('已保存');
    close();
  } catch (e: any) {
    message.error(resolveActionErrorMessage(
      e,
      '保存失败',
      '保存失败：权限不足，只有世界管理员可修改非成员编辑相关设置。',
    ));
  } finally {
    loading.value = false;
  }
};

const remove = async () => {
  loading.value = true;
  try {
    await chat.worldDelete(props.worldId);
    message.success('世界已删除');
    close();
    emit('deleted');
  } catch (e: any) {
    message.error(e?.response?.data?.message || '删除失败');
  } finally {
    loading.value = false;
  }
};

const confirmRemove = () => {
  dialog.warning({
    title: '删除世界',
    content: `确定要删除「${form.value.name || '该世界'}」吗？此操作不可恢复，世界内的所有频道和消息将被永久删除。`,
    positiveText: '确认删除',
    negativeText: '取消',
    maskClosable: false,
    onPositiveClick: remove,
  });
};

const updateDescription = (value: string) => {
  form.value.description = truncateTextByDisplayWidth(value, WORLD_DESCRIPTION_MAX_WIDTH_UNITS);
};

const getDescriptionCountLabel = (value?: string) => {
  const usedUnits = getTextDisplayWidthUnits(value || '');
  return `${formatDisplayWidthAsCharCount(usedUnits)}/${WORLD_DESCRIPTION_MAX_DISPLAY_CHARS}`;
};
</script>

<template>
  <n-modal
    :show="props.visible"
    preset="card"
    title="世界设置"
    class="sc-fluid-modal sc-manage-modal"
    :bordered="false"
    :trap-focus="false"
    :close-on-esc="!coverEditing"
    @update:show="close"
  >
    <div class="sc-manage-modal__body">
      <section class="sc-manage-section">
        <header class="sc-manage-section__head">
          <h3 class="sc-manage-section__title">基本信息</h3>
          <p class="sc-manage-section__desc">展示在世界大厅与世界详情页中的信息。</p>
        </header>
        <div class="sc-manage-grid">
          <label class="sc-manage-field">
            <span class="sc-manage-field__label">名称</span>
            <n-input v-model:value="form.name" placeholder="世界名称" />
          </label>
          <div class="sc-manage-field">
            <span class="sc-manage-field__label">可见性</span>
            <n-select v-model:value="form.visibility" :options="visibilityOptions" />
          </div>
        </div>
        <div class="sc-manage-field">
          <span class="sc-manage-field__label">简介</span>
          <div class="world-manager__description-field">
            <n-input
              type="textarea"
              :value="form.description"
              :autosize="{ minRows: 3, maxRows: 8 }"
              placeholder="一句话介绍这个世界"
              @update:value="updateDescription"
            />
            <div class="world-manager__counter">{{ getDescriptionCountLabel(form.description) }}</div>
          </div>
        </div>
        <div class="sc-manage-field">
          <span class="sc-manage-field__label">封面</span>
          <WorldCreateCoverField
            v-if="props.visible && props.worldId"
            :key="props.worldId"
            v-model="form.coverAttachmentId"
            v-model:uploading="coverUploading"
            v-model:editing="coverEditing"
            :world-id="props.worldId"
            :disabled="loading"
          >
            <template #hint>建议使用横向图片（约 4:1），保存后生效。</template>
          </WorldCreateCoverField>
        </div>
      </section>

      <section class="sc-manage-section">
        <header class="sc-manage-section__head">
          <h3 class="sc-manage-section__title">消息与权限</h3>
        </header>
        <div class="sc-manage-row">
          <div class="sc-manage-row__text">
            <div class="sc-manage-row__label">按开始输入时间排序新消息</div>
            <div class="sc-manage-row__hint">关闭后按发送时间排序，只影响之后的新消息。</div>
          </div>
          <n-switch
            v-model:value="form.messageSortBasis"
            class="sc-manage-row__control"
            checked-value="typing_start"
            unchecked-value="send_time"
          />
        </div>
        <div class="sc-manage-row">
          <div class="sc-manage-row__text">
            <div class="sc-manage-row__label">允许管理员编辑其他成员发言</div>
          </div>
          <n-switch v-model:value="form.allowAdminEditMessages" class="sc-manage-row__control" />
        </div>
        <div class="sc-manage-row">
          <div class="sc-manage-row__text">
            <div class="sc-manage-row__label">允许世界成员进入 LOG 排版器</div>
            <div class="sc-manage-row__hint">成员可通过设计链接参与排版并编辑自己的消息；不会授予编辑他人消息、调整来源或发布的权限。</div>
          </div>
          <n-switch v-model:value="form.allowMemberAccessLogLayout" class="sc-manage-row__control" />
        </div>
        <div class="sc-manage-row">
          <div class="sc-manage-row__text">
            <div class="sc-manage-row__label">导出记录遵循消息可见权限</div>
            <div class="sc-manage-row__hint">开启后，普通成员只能导出自己有权查看的聊天记录。世界管理员可执行管理级导出，但严格悄悄话隐私仍然生效。</div>
          </div>
          <n-switch v-model:value="form.exportRespectMessageVisibility" class="sc-manage-row__control" />
        </div>
        <div class="sc-manage-row">
          <div class="sc-manage-row__text">
            <div class="sc-manage-row__label">允许世界成员导出记录</div>
            <div class="sc-manage-row__hint">关闭后，普通世界成员无法导出聊天记录；世界所有者和管理员不受影响。</div>
          </div>
          <n-switch v-model:value="form.allowMemberExportMessages" class="sc-manage-row__control" />
        </div>
        <div class="sc-manage-row">
          <div class="sc-manage-row__text">
            <div class="sc-manage-row__label">允许成员编辑世界术语</div>
          </div>
          <n-switch v-model:value="form.allowMemberEditKeywords" class="sc-manage-row__control" />
        </div>
        <div class="sc-manage-row">
          <div class="sc-manage-row__text">
            <div class="sc-manage-row__label">允许管理其他用户频道角色</div>
          </div>
          <n-switch v-model:value="form.allowManageOtherUserChannelIdentities" class="sc-manage-row__control" />
        </div>
        <div class="sc-manage-row">
          <div class="sc-manage-row__text">
            <div class="sc-manage-row__label">不允许管理员查看所有的悄悄话</div>
          </div>
          <n-switch v-model:value="form.strictWhisperPrivacy" class="sc-manage-row__control" />
        </div>
      </section>

      <section class="sc-manage-section">
        <header class="sc-manage-section__head">
          <h3 class="sc-manage-section__title">新频道默认掷骰方式</h3>
          <p class="sc-manage-section__desc">仅影响后续新建频道，不修改现有频道。</p>
        </header>
        <WorldDiceDefaultsFields
          v-model:mode="form.channelDefaultDiceMode"
          v-model:bot-ids="form.channelDefaultBotIds"
          v-model:bot-id="form.channelDefaultBotId"
          v-model:event-bot-ids="form.channelDefaultEventBotIds"
          :bot-select-options="botSelectOptions"
          :bot-options-loading="botOptionsLoading"
        />
      </section>

      <section class="sc-manage-section">
        <header class="sc-manage-section__head">
          <h3 class="sc-manage-section__title">外观</h3>
        </header>
        <div class="sc-manage-field">
          <span class="sc-manage-field__label">徽章模板</span>
          <n-input v-model:value="form.characterCardBadgeTemplate" placeholder="留空则使用个人模板" />
          <p class="sc-manage-hint">示例：{{ DEFAULT_CARD_TEMPLATE }}；支持 {对象.子项.属性}</p>
        </div>
        <div class="sc-manage-row">
          <div class="sc-manage-row__text">
            <div class="sc-manage-row__label">鼠标样式</div>
            <div class="sc-manage-row__hint">世界配置按类型覆盖平台；可选择继承或浏览器默认。</div>
          </div>
          <n-button size="small" secondary class="sc-manage-row__control" @click="cursorThemeVisible = true">
            管理鼠标样式
          </n-button>
        </div>
      </section>
    </div>
    <template #footer>
      <div class="sc-manage-actions">
        <n-button
          class="sc-manage-actions__start"
          type="error"
          quaternary
          :loading="loading"
          @click="confirmRemove"
        >
          删除世界
        </n-button>
        <n-button @click="close">取消</n-button>
        <n-button type="primary" :loading="loading" :disabled="coverUploading || coverEditing" @click="save">保存</n-button>
      </div>
    </template>
  </n-modal>
  <CursorThemeEditorModal
    v-model:show="cursorThemeVisible"
    v-model="form.cursorTheme"
    scope="world"
    :world-id="props.worldId"
  />
</template>

<style scoped>
.world-manager__description-field {
  position: relative;
  width: 100%;
}

.world-manager__description-field :deep(textarea) {
  padding-bottom: 1.75rem;
}

.world-manager__counter {
  position: absolute;
  right: 12px;
  bottom: 8px;
  font-size: 12px;
  line-height: 1.4;
  color: var(--sc-text-secondary);
  pointer-events: none;
}

</style>
