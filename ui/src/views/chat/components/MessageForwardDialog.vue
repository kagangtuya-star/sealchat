<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import type { SChannel, ChannelIdentity, ChannelIdentityVariant } from '@/types';
import { useChatStore } from '@/stores/chat';
import { nanoid } from 'nanoid';

interface Props {
  visible: boolean;
  sourceChannelId: string;
  sourceWorldId?: string;
  messageIds: string[];
  messages?: any[];
}

interface TargetSelection {
  identityId: string;
  identityVariantId: string;
}

const props = defineProps<Props>();
const emit = defineEmits<{
  (event: 'update:visible', visible: boolean): void;
  (event: 'success', data: any): void;
}>();

const chat = useChatStore();
const message = useMessage();
const selectedWorldId = ref('');
const selectedChannelIds = ref<string[]>([]);
const targetSelections = reactive<Record<string, TargetSelection>>({});
const channelLoading = ref(false);
const identityLoading = ref(false);
const submitting = ref(false);
const whisperConfirmed = ref(false);
const operationClientId = ref(nanoid());

const normalizeId = (value: unknown) => String(value || '').trim();

const flattenChannels = (items: SChannel[] = []) => {
  const result: SChannel[] = [];
  const queue = [...items];
  while (queue.length > 0) {
    const item = queue.shift();
    if (!item) continue;
    result.push(item);
    if (item.children?.length) {
      queue.push(...item.children);
    }
  }
  return result;
};

const sourceWorldId = computed(() => normalizeId(props.sourceWorldId) || normalizeId(chat.currentWorldId));
const worldIds = computed(() => {
  const ids = new Set<string>();
  if (sourceWorldId.value) ids.add(sourceWorldId.value);
  if (chat.currentWorldId) ids.add(normalizeId(chat.currentWorldId));
  for (const item of chat.myWorldCache.owned || []) {
    const id = normalizeId(item?.world?.id || item?.id);
    if (id) ids.add(id);
  }
  return Array.from(ids);
});
const worldOptions = computed(() => worldIds.value.map((id) => ({
  value: id,
  label: chat.worldMap[id]?.name || `世界 ${id.slice(0, 8)}`,
})));

const channels = computed(() => {
  const tree = selectedWorldId.value ? chat.channelTreeByWorld[selectedWorldId.value] || [] : [];
  return flattenChannels(tree as SChannel[]).filter((item) => {
    return Boolean(item?.id) && !item.isPrivate && item.permType !== 'private';
  });
});
const channelWorldById = computed(() => {
  const result: Record<string, string> = {};
  Object.entries(chat.channelTreeByWorld).forEach(([worldId, tree]) => {
    flattenChannels((tree || []) as SChannel[]).forEach((item) => {
      if (item.id) result[item.id] = worldId;
    });
  });
  return result;
});
const channelOptions = computed(() => channels.value.map((item) => ({
  value: item.id,
  label: item.name || '未命名频道',
})).filter((item) => item.value !== normalizeId(props.sourceChannelId)));
const channelById = computed(() => {
  const result: Record<string, SChannel> = {};
  Object.values(chat.channelTreeByWorld).forEach((tree) => {
    flattenChannels((tree || []) as SChannel[]).forEach((item) => {
      if (item.id) result[item.id] = item;
    });
  });
  return result;
});
const selectedChannelIdsInWorld = computed({
  get: () => selectedChannelIds.value.filter((channelId) => channelWorldById.value[channelId] === selectedWorldId.value),
  set: (channelIds: string[]) => {
    const retained = selectedChannelIds.value.filter((channelId) => channelWorldById.value[channelId] !== selectedWorldId.value);
    selectedChannelIds.value = Array.from(new Set([...retained, ...channelIds]));
  },
});
const selectedRows = computed(() => selectedChannelIds.value.map((channelId) => {
  const selection = targetSelections[channelId] || { identityId: '', identityVariantId: '' };
  const identities = chat.getScopedChannelIdentities(channelId) as ChannelIdentity[];
  const variants = selection.identityId
    ? chat.getIdentityVariants(channelId, selection.identityId) as ChannelIdentityVariant[]
    : [];
  return {
    channelId,
    channel: channelById.value[channelId],
    selection,
    identities,
    variants,
  };
}));
const hasWhisper = computed(() => (props.messages || []).some((item) => {
  return item?.isWhisper === true || item?.is_whisper === true;
}));
const canSubmit = computed(() => {
  return !submitting.value && !identityLoading.value && selectedChannelIds.value.length > 0 && props.messageIds.length > 0;
});

const ensureWorldChannels = async (worldId: string) => {
  if (!worldId || chat.channelTreeByWorld[worldId]) return;
  channelLoading.value = true;
  try {
    await chat.channelList(worldId, false, { autoSwitch: false, preserveCurrentChannel: true, refreshUnread: false });
  } finally {
    channelLoading.value = false;
  }
};

const syncTargetIdentities = async (channelIds: string[]) => {
  const ids = Array.from(new Set(channelIds.map(normalizeId).filter(Boolean)));
  Object.keys(targetSelections).forEach((id) => {
    if (!ids.includes(id)) delete targetSelections[id];
  });
  if (!ids.length) return;
  identityLoading.value = true;
  try {
    await Promise.all(ids.map(async (channelId) => {
      await Promise.all([
        chat.loadChannelIdentities(channelId),
        chat.loadChannelIdentityVariants(channelId),
      ]);
      const identities = chat.getScopedChannelIdentities(channelId) as ChannelIdentity[];
      const current = targetSelections[channelId];
      const identityId = current?.identityId && identities.some((item) => item.id === current.identityId)
        ? current.identityId
        : (identities.find((item) => item.isDefault)?.id || identities[0]?.id || '');
      targetSelections[channelId] = {
        identityId,
        identityVariantId: current?.identityVariantId || '',
      };
    }));
  } finally {
    identityLoading.value = false;
  }
};

const openDialog = async () => {
  operationClientId.value = nanoid();
  whisperConfirmed.value = false;
  selectedChannelIds.value = [];
  Object.keys(targetSelections).forEach((id) => delete targetSelections[id]);
  selectedWorldId.value = sourceWorldId.value || worldIds.value[0] || '';
  if (!chat.joinedWorldIds.length) {
    try {
      await chat.refreshJoinedWorldState();
    } catch {
      // 频道加载仍可依赖已有缓存。
    }
  }
  if (selectedWorldId.value) {
    await ensureWorldChannels(selectedWorldId.value);
  }
};

const closeDialog = () => {
  if (!submitting.value) emit('update:visible', false);
};

const handleModalShowUpdate = (visible: boolean) => {
  if (!visible) closeDialog();
};

const updateIdentity = (channelId: string, identityId: string | number | null) => {
  const selection = targetSelections[channelId] || { identityId: '', identityVariantId: '' };
  selection.identityId = normalizeId(identityId);
  selection.identityVariantId = '';
  targetSelections[channelId] = selection;
};

const updateVariant = (channelId: string, variantId: string | number | null) => {
  const selection = targetSelections[channelId] || { identityId: '', identityVariantId: '' };
  selection.identityVariantId = normalizeId(variantId);
  targetSelections[channelId] = selection;
};

const submit = async () => {
  if (!canSubmit.value) {
    message.warning('请选择目标频道');
    return;
  }
  if (hasWhisper.value && !whisperConfirmed.value) {
    message.warning('请确认悄悄话会在目标频道公开');
    return;
  }
  submitting.value = true;
  try {
    const data = await chat.messageForwardBatch(props.sourceChannelId, {
      messageIds: props.messageIds,
      targets: selectedRows.value.map((row) => ({
        channelId: row.channelId,
        identityId: row.selection.identityId,
        identityVariantId: row.selection.identityVariantId,
      })),
      clientId: operationClientId.value,
      whisperConfirmed: whisperConfirmed.value,
    });
    message.success(`已转发 ${props.messageIds.length} 条消息到 ${selectedRows.value.length} 个频道`);
    emit('success', data);
    emit('update:visible', false);
  } catch (error) {
    message.error((error as Error)?.message || '转发失败');
  } finally {
    submitting.value = false;
  }
};

watch(selectedWorldId, async (worldId) => {
  await ensureWorldChannels(worldId);
});
watch(selectedChannelIds, (ids) => {
  void syncTargetIdentities(ids);
}, { deep: true });
watch(() => props.visible, (visible) => {
  if (visible) void openDialog();
});
</script>

<template>
  <n-modal
    :show="visible"
    preset="card"
    title="转发消息"
    style="width: 720px; max-width: 95vw;"
    :mask-closable="!submitting"
    :close-on-esc="!submitting"
    :auto-focus="false"
    @update:show="handleModalShowUpdate"
  >
    <n-space vertical size="large">
      <n-alert type="info">
        已选择 {{ messageIds.length }} 条消息。富文本和图片引用将原样复制。
      </n-alert>

      <n-form label-placement="top">
        <n-form-item label="目标世界">
          <n-select v-model:value="selectedWorldId" :options="worldOptions" placeholder="选择世界" />
        </n-form-item>
        <n-form-item label="目标频道">
          <n-select
            v-model:value="selectedChannelIdsInWorld"
            multiple
            filterable
            clearable
            :loading="channelLoading"
            :options="channelOptions"
            placeholder="选择一个或多个频道"
          />
        </n-form-item>
      </n-form>

      <n-alert v-if="hasWhisper" type="warning">
        选中消息包含悄悄话。转发后会以普通消息公开到目标频道。
        <n-checkbox v-model:checked="whisperConfirmed" style="display: block; margin-top: 8px;">
          我确认公开转发
        </n-checkbox>
      </n-alert>

      <n-spin :show="identityLoading">
        <n-space v-if="selectedRows.length" vertical>
          <n-card v-for="row in selectedRows" :key="row.channelId" size="small">
            <n-space vertical>
              <strong>{{ row.channel?.name || row.channelId }}</strong>
              <n-form-item label="频道角色">
                <n-select
                  :value="row.selection.identityId || null"
                  clearable
                  :options="row.identities.map((item) => ({ label: item.displayName || '默认角色', value: item.id }))"
                  placeholder="默认角色"
                  @update:value="updateIdentity(row.channelId, $event)"
                />
              </n-form-item>
              <n-form-item v-if="row.variants.length" label="身份变体">
                <n-select
                  :value="row.selection.identityVariantId || null"
                  clearable
                  :options="row.variants.map((item) => ({ label: item.displayName || item.keyword || item.note || item.id, value: item.id }))"
                  placeholder="不使用变体"
                  @update:value="updateVariant(row.channelId, $event)"
                />
              </n-form-item>
            </n-space>
          </n-card>
        </n-space>
        <n-empty v-else description="尚未选择目标频道" />
      </n-spin>

      <n-space justify="end">
        <n-button :disabled="submitting" @click="closeDialog">取消</n-button>
        <n-button type="primary" :loading="submitting" :disabled="!canSubmit" @click="submit">开始转发</n-button>
      </n-space>
    </n-space>
  </n-modal>
</template>
