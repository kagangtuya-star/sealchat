<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { NDrawer, NDrawerContent, NButton, NIcon, NEmpty, NCard, NInput, NInputNumber, NForm, NFormItem, NModal, NPopconfirm, NTag, NSwitch, useMessage } from 'naive-ui';
import { Plus, Trash, Edit, Link, Unlink, Eye } from '@vicons/tabler';
import { useCharacterCardStore } from '@/stores/characterCard';
import { useCharacterSheetStore } from '@/stores/characterSheet';
import { useChatStore } from '@/stores/chat';
import { useDisplayStore } from '@/stores/display';
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { DEFAULT_CARD_TEMPLATE, getWorldCardTemplate, setWorldCardTemplate } from '@/utils/characterCardTemplate';
import type { CharacterCard, ChannelIdentity } from '@/types';

const props = defineProps<{
  visible: boolean;
  channelId?: string;
}>();

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void;
}>();

const message = useMessage();
const cardStore = useCharacterCardStore();
const sheetStore = useCharacterSheetStore();
const chatStore = useChatStore();
const displayStore = useDisplayStore();

const resolvedChannelId = computed(() => props.channelId || chatStore.curChannel?.id || '');

const channelCards = computed(() => cardStore.getCardsByChannel(resolvedChannelId.value));

const identities = computed<ChannelIdentity[]>(() => {
  const id = resolvedChannelId.value;
  if (!id) return [];
  return chatStore.channelIdentities[id] || [];
});

const badgeEnabled = computed({
  get: () => displayStore.settings.characterCardBadgeEnabled,
  set: (value: boolean) => {
    displayStore.updateSettings({ characterCardBadgeEnabled: value });
  },
});

const badgeTemplate = ref('');
const currentWorldId = computed(() => chatStore.currentWorldId || '');
const canSyncBadgeTemplate = computed(() => {
  const worldId = currentWorldId.value;
  if (!worldId) return false;
  const detail = chatStore.worldDetailMap[worldId];
  const role = detail?.memberRole;
  return role === 'owner' || role === 'admin';
});

const syncBadgeTemplate = () => {
  const worldId = currentWorldId.value;
  if (!worldId) {
    badgeTemplate.value = DEFAULT_CARD_TEMPLATE;
    return;
  }
  const stored = displayStore.settings.characterCardBadgeTemplateByWorld?.[worldId];
  badgeTemplate.value = stored ?? getWorldCardTemplate(worldId);
};

const persistBadgeTemplate = () => {
  const worldId = currentWorldId.value;
  if (!worldId) return;
  const normalized = badgeTemplate.value.trim() || DEFAULT_CARD_TEMPLATE;
  badgeTemplate.value = normalized;
  setWorldCardTemplate(worldId, normalized);
  displayStore.updateSettings({
    characterCardBadgeTemplateByWorld: {
      ...displayStore.settings.characterCardBadgeTemplateByWorld,
      [worldId]: normalized,
    },
  });
};

const resetBadgeTemplate = () => {
  badgeTemplate.value = DEFAULT_CARD_TEMPLATE;
  persistBadgeTemplate();
};

const syncBadgeTemplateToWorld = async () => {
  const worldId = currentWorldId.value;
  if (!worldId) return;
  const normalized = badgeTemplate.value.trim() || DEFAULT_CARD_TEMPLATE;
  badgeTemplate.value = normalized;
  persistBadgeTemplate();
  try {
    await chatStore.worldUpdate(worldId, { characterCardBadgeTemplate: normalized });
    message.success('模板已同步');
  } catch (e: any) {
    message.error(e?.response?.data?.message || '模板同步失败');
  }
};

watch(() => props.visible, async (val) => {
  if (val && resolvedChannelId.value) {
    await cardStore.loadCards(resolvedChannelId.value);
  }
}, { immediate: true });

watch(resolvedChannelId, async (newId) => {
  if (props.visible && newId) {
    await cardStore.loadCards(newId);
  }
});

watch(
  [() => props.visible, currentWorldId],
  ([visible]) => {
    if (visible) {
      syncBadgeTemplate();
    }
  },
  { immediate: true },
);

watch(badgeEnabled, (enabled) => {
  const channelId = resolvedChannelId.value;
  if (!channelId) return;
  if (enabled) {
    void cardStore.requestBadgeSnapshot(channelId);
    void cardStore.getActiveCard(channelId);
    return;
  }
  void cardStore.broadcastActiveBadge(channelId, undefined, 'clear');
});

const handleClose = () => {
  emit('update:visible', false);
};

// Create card modal
const createModalVisible = ref(false);
const newCardName = ref('');
const newCardSheetTypePreset = ref('coc7');
const newCardSheetTypeCustom = ref('');
const newCardAttrs = ref<Record<string, any>>({});
const creating = ref(false);

const sheetTypeOptions = [
  { label: 'COC7', value: 'coc7' },
  { label: 'DND5', value: 'dnd5e' },
  { label: '自定义', value: 'custom' },
];

const resolveSheetType = (preset: string, custom: string) => {
  if (preset === 'custom') {
    return custom.trim();
  }
  return preset;
};

const openCreateModal = () => {
  newCardName.value = '';
  newCardSheetTypePreset.value = 'coc7';
  newCardSheetTypeCustom.value = '';
  newCardAttrs.value = {};
  createModalVisible.value = true;
};

const handleCreateCard = async () => {
  if (!newCardName.value.trim()) {
    message.warning('请输入角色名称');
    return;
  }
  const sheetType = resolveSheetType(newCardSheetTypePreset.value, newCardSheetTypeCustom.value);
  if (!sheetType) {
    message.warning('请输入自定义规制类型');
    return;
  }
  creating.value = true;
  try {
    await cardStore.createCard(resolvedChannelId.value, newCardName.value.trim(), sheetType, newCardAttrs.value);
    message.success('创建成功');
    createModalVisible.value = false;
  } catch (e: any) {
    message.error(e?.response?.data?.error || '创建失败');
  } finally {
    creating.value = false;
  }
};

// Edit card modal
const editModalVisible = ref(false);
const editingCard = ref<CharacterCard | null>(null);
const editCardName = ref('');
const editCardSheetTypePreset = ref('coc7');
const editCardSheetTypeCustom = ref('');
const editCardOriginalName = ref('');
const editCardOriginalSheetType = ref('');
const editCardAttrsJson = ref('');
const saving = ref(false);
const pendingRestore = ref<{
  channelId: string;
  cardId?: string;
  cardName?: string;
  cardType?: string;
  attrs?: Record<string, any>;
} | null>(null);

const setEditSheetType = (value: string) => {
  const normalized = (value || '').trim();
  let lower = normalized.toLowerCase();
  if (lower === 'coc') {
    lower = 'coc7';
  } else if (lower === 'dnd' || lower === 'dnd5') {
    lower = 'dnd5e';
  }
  if (lower === 'coc7' || lower === 'dnd5e') {
    editCardSheetTypePreset.value = lower;
    editCardSheetTypeCustom.value = '';
  } else if (normalized) {
    editCardSheetTypePreset.value = 'custom';
    editCardSheetTypeCustom.value = normalized;
  } else {
    editCardSheetTypePreset.value = 'coc7';
    editCardSheetTypeCustom.value = '';
  }
};

const syncEditOriginals = () => {
  editCardOriginalName.value = editCardName.value;
  editCardOriginalSheetType.value = resolveSheetType(editCardSheetTypePreset.value, editCardSheetTypeCustom.value);
};

const rememberActiveCard = async (channelId: string) => {
  await cardStore.getActiveCard(channelId);
  const active = cardStore.activeCards[channelId];
  const activeId = cardStore.getActiveCardId(channelId);
  if (activeId || active?.name) {
    pendingRestore.value = {
      channelId,
      cardId: activeId || undefined,
      cardName: active?.name || '',
      cardType: active?.type || '',
      attrs: active?.attrs || {},
    };
  } else {
    pendingRestore.value = null;
  }
};

const restoreActiveCard = async () => {
  const pending = pendingRestore.value;
  if (!pending) return;
  pendingRestore.value = null;
  try {
    if (pending.cardId) {
      await cardStore.tagCard(pending.channelId, undefined, pending.cardId);
      return;
    }
    await cardStore.tagCard(pending.channelId);
    if (pending.cardName || pending.attrs) {
      await cardStore.updateCard(pending.channelId, pending.cardName || '', pending.attrs || {});
    }
  } catch (e) {
    console.warn('Failed to restore active character card', e);
  }
};

const openEditModal = async (card: CharacterCard) => {
  editingCard.value = card;
  editCardName.value = card.name;
  setEditSheetType(card.sheetType || 'coc7');
  editCardAttrsJson.value = JSON.stringify(card.attrs || {}, null, 2);
  editModalVisible.value = true;
  syncEditOriginals();
  if (!resolvedChannelId.value) {
    pendingRestore.value = null;
    return;
  }
  try {
    await rememberActiveCard(resolvedChannelId.value);
    if (pendingRestore.value?.cardId === card.id) {
      pendingRestore.value = null;
    } else {
      await cardStore.tagCard(resolvedChannelId.value, card.name, card.id);
    }
    await cardStore.getActiveCard(resolvedChannelId.value);
    const active = cardStore.activeCards[resolvedChannelId.value];
    if (active) {
      editCardName.value = active.name || editCardName.value;
      setEditSheetType(active.type || resolveSheetType(editCardSheetTypePreset.value, editCardSheetTypeCustom.value));
      editCardAttrsJson.value = JSON.stringify(active.attrs || {}, null, 2);
      syncEditOriginals();
    }
  } catch (e) {
    console.warn('Failed to load character card attrs', e);
  }
};

watch(editModalVisible, async (val, oldVal) => {
  if (!val && oldVal) {
    await restoreActiveCard();
  }
});

const handleSaveCard = async () => {
  if (!editingCard.value) return;
  if (!resolvedChannelId.value) {
    message.warning('请先选择频道');
    return;
  }
  if (!editCardName.value.trim()) {
    message.warning('请输入角色名称');
    return;
  }
  let attrs: Record<string, any> = {};
  try {
    attrs = JSON.parse(editCardAttrsJson.value || '{}');
  } catch {
    message.error('属性 JSON 格式错误');
    return;
  }
  saving.value = true;
  try {
    const nextName = editCardOriginalName.value || editCardName.value.trim();
    await cardStore.updateCard(resolvedChannelId.value, nextName, attrs);
    await cardStore.loadCards(resolvedChannelId.value);
    message.success('保存成功');
    editModalVisible.value = false;
  } catch (e: any) {
    message.error(e?.response?.data?.error || '保存失败');
  } finally {
    saving.value = false;
  }
};

const handleDeleteCard = async (card: CharacterCard) => {
  try {
    await cardStore.deleteCard(card.id);
    message.success('已删除');
  } catch (e: any) {
    message.error(e?.response?.data?.error || e?.message || '删除失败');
  }
};

// Bind modal
const bindModalVisible = ref(false);
const bindingCard = ref<CharacterCard | null>(null);
const selectedIdentityId = ref<string | null>(null);

const identityOptions = computed(() => {
  return identities.value.map(i => ({
    label: i.displayName || '未命名身份',
    value: i.id,
  }));
});

const openBindModal = (card: CharacterCard) => {
  bindingCard.value = card;
  selectedIdentityId.value = null;
  bindModalVisible.value = true;
};

const handleBind = async () => {
  if (!bindingCard.value || !selectedIdentityId.value || !resolvedChannelId.value) return;
  try {
    await cardStore.bindIdentity(resolvedChannelId.value, selectedIdentityId.value, bindingCard.value.id);
    message.success('绑定成功');
    bindModalVisible.value = false;
  } catch (e: any) {
    message.error(e?.response?.data?.error || '绑定失败');
  }
};

const getBoundIdentities = (cardId: string) => {
  const result: ChannelIdentity[] = [];
  for (const [identityId, boundCardId] of Object.entries(cardStore.identityBindings)) {
    if (boundCardId === cardId) {
      const identity = identities.value.find(i => i.id === identityId);
      if (identity) result.push(identity);
    }
  }
  return result;
};

const resolveCardAvatarUrl = (cardId: string) => {
  const bound = getBoundIdentities(cardId);
  const identity = bound.find(item => item.avatarAttachmentId) || bound[0];
  if (!identity?.avatarAttachmentId) return '';
  return resolveAttachmentUrl(identity.avatarAttachmentId) || identity.avatarAttachmentId;
};

const handleUnbind = async (identityId: string) => {
  if (!resolvedChannelId.value) return;
  try {
    await cardStore.unbindIdentity(resolvedChannelId.value, identityId);
    message.success('已解绑');
  } catch (e: any) {
    message.error(e?.response?.data?.error || '解绑失败');
  }
};

const formatAttrs = (attrs: Record<string, any> | undefined) => {
  if (!attrs || Object.keys(attrs).length === 0) return '暂无属性';
  return Object.entries(attrs).map(([k, v]) => `${k}: ${v}`).join(', ');
};

const openPreview = async (card: CharacterCard) => {
  const channelId = resolvedChannelId.value;
  if (!channelId) {
    message.warning('请先选择频道');
    return;
  }
  try {
    let cardData = cardStore.activeCards[channelId];
    if (!cardData || cardData.name !== card.name) {
      await cardStore.getActiveCard(channelId);
      cardData = cardStore.activeCards[channelId];
    }
    const avatarUrl = resolveCardAvatarUrl(card.id);
    sheetStore.openSheet(card, channelId, {
      name: cardData?.name || card.name,
      type: cardData?.type || card.sheetType,
      attrs: cardData?.attrs || card.attrs || {},
      avatarUrl: avatarUrl || undefined,
    });
  } catch (e: any) {
    console.warn('Failed to open character preview', e);
    const avatarUrl = resolveCardAvatarUrl(card.id);
    sheetStore.openSheet(card, channelId, {
      name: card.name,
      type: card.sheetType,
      attrs: card.attrs || {},
      avatarUrl: avatarUrl || undefined,
    });
  }
};
</script>

<template>
  <n-drawer
    :show="visible"
    placement="right"
    :width="420"
    @update:show="handleClose"
  >
    <n-drawer-content closable>
      <template #header>
        <div class="character-card-header">
          <span>人物卡管理</span>
          <n-button size="small" type="primary" @click="openCreateModal">
            <template #icon><n-icon :component="Plus" /></template>
            新建
          </n-button>
        </div>
      </template>

      <div class="character-card-settings">
        <div class="settings-row">
          <div>
            <p class="settings-title">聊天角色徽章</p>
            <p class="settings-desc">开启后且可读到人物卡数据时，在昵称后显示简洁属性</p>
          </div>
          <n-switch v-model:value="badgeEnabled">
            <template #checked>已启用</template>
            <template #unchecked>已关闭</template>
          </n-switch>
        </div>
        <div class="settings-row settings-row--template">
          <div>
            <p class="settings-title">徽章模板</p>
            <p class="settings-desc">使用 {属性名} 占位，例如：HP{生命值} SAN{理智} 闪避{闪避}</p>
          </div>
          <div class="settings-template-input">
            <n-input
              v-model:value="badgeTemplate"
              size="small"
              placeholder="HP{生命值} SAN{理智} 闪避{闪避}"
              @blur="persistBadgeTemplate"
            />
            <n-button size="small" quaternary @click="resetBadgeTemplate">恢复默认</n-button>
            <n-button
              v-if="canSyncBadgeTemplate"
              size="small"
              tertiary
              @click="syncBadgeTemplateToWorld"
            >模板同步</n-button>
          </div>
        </div>
      </div>

      <div class="character-card-list">
        <n-empty v-if="channelCards.length === 0" description="暂无人物卡" />
        <n-card
          v-for="card in channelCards"
          :key="card.id"
          size="small"
          class="character-card-item"
        >
          <template #header>
            <span class="card-name">{{ card.name }}</span>
            <n-tag size="small" :bordered="false">{{ card.sheetType || 'custom' }}</n-tag>
          </template>
          <template #header-extra>
            <n-button text size="small" title="预览" @click="openPreview(card)">
              <template #icon><n-icon :component="Eye" /></template>
            </n-button>
            <n-button text size="small" @click="openEditModal(card)">
              <template #icon><n-icon :component="Edit" /></template>
            </n-button>
            <n-button text size="small" @click="openBindModal(card)">
              <template #icon><n-icon :component="Link" /></template>
            </n-button>
            <n-popconfirm @positive-click="handleDeleteCard(card)">
              <template #trigger>
                <n-button text size="small" type="error">
                  <template #icon><n-icon :component="Trash" /></template>
                </n-button>
              </template>
              删除前将从所有群解绑此人物卡，确定删除？
            </n-popconfirm>
          </template>
          <div class="card-attrs">{{ formatAttrs(card.attrs) }}</div>
          <div v-if="getBoundIdentities(card.id).length > 0" class="card-bindings">
            <span class="bindings-label">已绑定：</span>
            <n-tag
              v-for="identity in getBoundIdentities(card.id)"
              :key="identity.id"
              size="small"
              closable
              @close="handleUnbind(identity.id)"
            >
              {{ identity.displayName }}
            </n-tag>
          </div>
        </n-card>
      </div>
    </n-drawer-content>
  </n-drawer>

  <!-- Create Modal -->
  <n-modal
    v-model:show="createModalVisible"
    preset="dialog"
    :show-icon="false"
    title="新建人物卡"
    :positive-text="creating ? '创建中…' : '创建'"
    :positive-button-props="{ loading: creating }"
    negative-text="取消"
    @positive-click="handleCreateCard"
  >
    <n-form label-width="80">
      <n-form-item label="角色名称">
        <n-input v-model:value="newCardName" maxlength="32" placeholder="请输入角色名称" />
      </n-form-item>
      <n-form-item label="卡片类型">
        <n-select v-model:value="newCardSheetTypePreset" :options="sheetTypeOptions" />
        <n-input
          v-if="newCardSheetTypePreset === 'custom'"
          v-model:value="newCardSheetTypeCustom"
          placeholder="输入自定义规制类型"
          class="sheet-type-custom-input"
        />
      </n-form-item>
    </n-form>
  </n-modal>

  <!-- Edit Modal -->
  <n-modal
    v-model:show="editModalVisible"
    preset="dialog"
    :show-icon="false"
    title="编辑人物卡"
    :positive-text="saving ? '保存中…' : '保存'"
    :positive-button-props="{ loading: saving }"
    negative-text="取消"
    @positive-click="handleSaveCard"
  >
    <n-form label-width="80">
      <n-form-item label="角色名称">
        <n-input v-model:value="editCardName" maxlength="32" disabled />
      </n-form-item>
      <n-form-item label="卡片类型">
        <n-select v-model:value="editCardSheetTypePreset" :options="sheetTypeOptions" disabled />
        <n-input
          v-if="editCardSheetTypePreset === 'custom'"
          v-model:value="editCardSheetTypeCustom"
          placeholder="输入自定义规制类型"
          class="sheet-type-custom-input"
          disabled
        />
      </n-form-item>
      <n-form-item label="属性(JSON)">
        <n-input
          v-model:value="editCardAttrsJson"
          type="textarea"
          :autosize="{ minRows: 4, maxRows: 10 }"
          placeholder='例如: {"hp": 10, "hpmax": 10, "san": 50}'
        />
      </n-form-item>
    </n-form>
  </n-modal>

  <!-- Bind Modal -->
  <n-modal
    v-model:show="bindModalVisible"
    preset="dialog"
    :show-icon="false"
    title="绑定身份"
    positive-text="绑定"
    negative-text="取消"
    @positive-click="handleBind"
  >
    <n-form label-width="80">
      <n-form-item label="选择身份">
        <n-select
          v-model:value="selectedIdentityId"
          :options="identityOptions"
          placeholder="选择要绑定的频道身份"
        />
      </n-form-item>
    </n-form>
  </n-modal>
</template>

<style lang="scss" scoped>
.character-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding-right: 1rem;
}

.character-card-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.character-card-settings {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.25rem 0 1rem;
  border-bottom: 1px solid var(--sc-border-color);
  margin-bottom: 1rem;
}

.settings-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.settings-row--template {
  align-items: flex-start;
}

.settings-title {
  font-weight: 500;
  margin-bottom: 0.1rem;
}

.settings-desc {
  color: var(--sc-text-secondary);
  font-size: 0.8rem;
}

.settings-template-input {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  min-width: 210px;
}

.character-card-item {
  .card-name {
    font-weight: 500;
    margin-right: 0.5rem;
  }
  .card-attrs {
    color: var(--sc-text-secondary);
    font-size: 0.85rem;
    margin-bottom: 0.5rem;
  }
  .card-bindings {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.25rem;
    .bindings-label {
      font-size: 0.8rem;
      color: var(--sc-text-tertiary);
    }
  }
}

.sheet-type-custom-input {
  margin-top: 8px;
}
</style>
