<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch, type PropType } from 'vue';
import { useMessage } from 'naive-ui';
import type { SChannel, UserInfo, UserRoleModel } from '@/types';
import { chatEvent, useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { useCharacterCardStore } from '@/stores/characterCard';
import WorldDiceDefaultsFields from '@/views/world/WorldDiceDefaultsFields.vue';

const props = defineProps({
  channel: {
    type: Object as PropType<SChannel>,
  },
});

const emit = defineEmits<{
  (e: 'update'): void;
}>();

const chat = useChatStore();
const userStore = useUserStore();
const characterCardStore = useCharacterCardStore();
const message = useMessage();

const diceModeOptions = [
  { label: '内置掷骰', value: 'builtin' },
  { label: 'BOT掷骰', value: 'bot' },
  { label: '关闭掷骰', value: 'disabled' },
];

const currentMode = ref<'builtin' | 'bot' | 'disabled'>('builtin');
const currentBotIds = ref<string[]>([]);
const currentPrimaryBotId = ref('');
const currentEventBotIds = ref<string[]>([]);
const worldMode = ref<'builtin' | 'bot' | 'disabled'>('builtin');
const worldBotIds = ref<string[]>([]);
const worldBotId = ref('');
const worldEventBotIds = ref<string[]>([]);

const botList = ref<UserInfo[]>([]);
const botOptionsLoading = ref(false);
const channelBotsLoading = ref(false);
const channelSaving = ref(false);
const worldSaving = ref(false);
const permissionLoading = ref(false);
const worldDetailLoading = ref(false);
const canManageChannel = ref(false);

const channelId = computed(() => String(props.channel?.id || '').trim());
const worldId = computed(() => String(props.channel?.worldId || '').trim());
const botRoleId = computed(() => channelId.value ? `ch-${channelId.value}-bot` : '');
const isSystemAdmin = computed(() => Boolean(userStore.checkPerm?.('mod_admin')));

const isPrivateChannel = computed(() => {
  const channel = props.channel;
  if (!channel) return false;
  if (channel.isPrivate) return true;
  if (channel.friendInfo) return true;
  return String(channel.permType || '').toLowerCase() === 'private';
});

const botSelectOptions = computed(() => botList.value.map((item) => ({
  label: item.nick || item.username || 'Bot',
  value: item.id,
})));
const primaryBotSelectOptions = computed(() => {
  const selected = new Set(currentBotIds.value);
  return botSelectOptions.value.filter((item) => selected.has(String(item.value || '')));
});
const eventBotSelectOptions = computed(() => {
  const selected = new Set(currentBotIds.value);
  return botSelectOptions.value.filter((item) => selected.has(String(item.value || '')));
});

const hasBotOptions = computed(() => botList.value.length > 0);

const worldDetail = computed(() => {
  if (!worldId.value) return null;
  return chat.worldDetailMap?.[worldId.value] || null;
});

const canManageWorldDefaults = computed(() => {
  if (isSystemAdmin.value) return true;
  const detail = worldDetail.value;
  if (!detail) return false;
  const role = String(detail.memberRole || '').trim();
  const ownerId = String(detail.world?.ownerId || '').trim();
  return role === 'owner' || role === 'admin' || ownerId === userStore.info.id;
});

const channelSectionDisabled = computed(() => isPrivateChannel.value || !canManageChannel.value);
const worldSectionVisible = computed(() => !!worldId.value && canManageWorldDefaults.value);

const resetCurrentModeFromChannel = () => {
  const channel = props.channel;
  const botEnabled = channel?.botFeatureEnabled === true;
  const builtInEnabled = channel?.builtInDiceEnabled !== false;
  if (botEnabled) {
    currentMode.value = 'bot';
    return;
  }
  currentMode.value = builtInEnabled ? 'builtin' : 'disabled';
};

const resetWorldModeFromDetail = () => {
  const detail = worldDetail.value;
  const mode = detail?.world?.channelDefaultDiceMode === 'bot'
    ? 'bot'
    : detail?.world?.channelDefaultDiceMode === 'disabled'
      ? 'disabled'
      : 'builtin';
  worldMode.value = mode;
  const configuredWorldBotIds = Array.isArray((detail?.world as any)?.channelDefaultBotIds)
    ? Array.from(new Set<string>(((detail?.world as any)?.channelDefaultBotIds || []).map((id: unknown) => String(id || '').trim()).filter(Boolean)))
    : [];
  worldBotId.value = String(detail?.world?.channelDefaultBotId || '').trim();
  worldBotIds.value = configuredWorldBotIds.length > 0
    ? configuredWorldBotIds
    : (worldBotId.value ? [worldBotId.value] : []);
  const configuredWorldEventBotIds = Array.isArray((detail?.world as any)?.channelDefaultEventBotIds)
    ? Array.from(new Set<string>(((detail?.world as any)?.channelDefaultEventBotIds || []).map((id: unknown) => String(id || '').trim()).filter((id: string) => worldBotIds.value.includes(id))))
    : [];
  worldEventBotIds.value = configuredWorldEventBotIds;
};

const loadBotOptions = async (force = false) => {
  if (botOptionsLoading.value) return;
  botOptionsLoading.value = true;
  try {
    const resp = await chat.botList(force);
    botList.value = resp?.items || [];
  } catch (error: any) {
    message.error(error?.response?.data?.message || '获取机器人列表失败');
  } finally {
    botOptionsLoading.value = false;
  }
};

const refreshChannelBotSelection = async () => {
  if (!channelId.value || !botRoleId.value) {
    currentBotIds.value = [];
    currentPrimaryBotId.value = '';
    currentEventBotIds.value = [];
    return;
  }
  channelBotsLoading.value = true;
  try {
    const resp = await chat.channelMemberListAll(channelId.value, 200);
    const items = resp?.data?.items || [];
    const existingIds = items
      .filter((item: UserRoleModel) => item.roleId === botRoleId.value && item.user?.id)
      .map((item: UserRoleModel) => item.user?.id || '')
      .filter(Boolean);
    currentBotIds.value = Array.from(new Set(existingIds));
    const primary = String(props.channel?.primaryBotId || '').trim();
    currentPrimaryBotId.value = primary && currentBotIds.value.includes(primary)
      ? primary
      : (currentBotIds.value[0] || '');
    const configuredEventBotIds = Array.isArray(props.channel?.eventBotIds)
      ? Array.from(new Set((props.channel?.eventBotIds || []).map((id) => String(id || '').trim()).filter((id) => currentBotIds.value.includes(id))))
      : [];
    const nextEventBotIds = configuredEventBotIds.length > 0
      ? configuredEventBotIds
      : currentBotIds.value.slice();
    if (currentPrimaryBotId.value && !nextEventBotIds.includes(currentPrimaryBotId.value)) {
      nextEventBotIds.push(currentPrimaryBotId.value);
    }
    currentEventBotIds.value = Array.from(new Set(nextEventBotIds.filter((id) => currentBotIds.value.includes(id))));
  } catch (error: any) {
    message.error(error?.response?.data?.error || '加载频道机器人失败');
  } finally {
    channelBotsLoading.value = false;
  }
};

const syncChannelBotBindings = async (nextBotIds: string[]) => {
  if (!channelId.value || !botRoleId.value) {
    return;
  }
  const resp = await chat.channelMemberListAll(channelId.value, 200);
  const items = resp?.data?.items || [];
  const existingIds = items
    .filter((item: UserRoleModel) => item.roleId === botRoleId.value && item.user?.id)
    .map((item: UserRoleModel) => item.user?.id || '')
    .filter(Boolean);
  const normalizedNext = Array.from(new Set(nextBotIds.map((id) => String(id || '').trim()).filter(Boolean)));
  const toAdd = normalizedNext.filter((id) => !existingIds.includes(id));
  if (toAdd.length) {
    await chat.userRoleLink(botRoleId.value, toAdd);
  }
  const toRemove = existingIds.filter((id) => !normalizedNext.includes(id));
  if (toRemove.length) {
    await chat.userRoleUnlink(botRoleId.value, toRemove);
  }
  currentBotIds.value = normalizedNext;
  currentEventBotIds.value = currentEventBotIds.value.filter((id) => normalizedNext.includes(id));
  if (currentPrimaryBotId.value && !normalizedNext.includes(currentPrimaryBotId.value)) {
    currentPrimaryBotId.value = normalizedNext[0] || '';
  }
};

const loadChannelPermission = async () => {
  if (!channelId.value || isPrivateChannel.value) {
    canManageChannel.value = false;
    return;
  }
  permissionLoading.value = true;
  try {
    const [canManageInfo, canRoleLink] = await Promise.all([
      chat.hasChannelPermission(channelId.value, 'func_channel_manage_info', userStore.info.id),
      chat.hasChannelPermission(channelId.value, 'func_channel_role_link', userStore.info.id),
    ]);
    canManageChannel.value = !!(canManageInfo || canRoleLink);
  } catch {
    canManageChannel.value = false;
  } finally {
    permissionLoading.value = false;
  }
};

const loadWorldDetail = async (force = false) => {
  if (!worldId.value) return;
  worldDetailLoading.value = true;
  try {
    await chat.worldDetail(worldId.value, force ? { force: true } : undefined);
    resetWorldModeFromDetail();
  } catch (error: any) {
    message.error(error?.response?.data?.message || '加载世界信息失败');
  } finally {
    worldDetailLoading.value = false;
  }
};

const saveChannelMode = async () => {
  if (!channelId.value || channelSectionDisabled.value) {
    return;
  }
  if (currentMode.value === 'bot') {
    if (currentBotIds.value.length === 0) {
      message.error('请至少绑定一个 BOT');
      return;
    }
    if (!hasBotOptions.value) {
      message.error('暂无可用机器人令牌，请先在后台创建');
      return;
    }
  }
  channelSaving.value = true;
  try {
    if (currentMode.value === 'bot') {
      const primary = currentPrimaryBotId.value && currentBotIds.value.includes(currentPrimaryBotId.value)
        ? currentPrimaryBotId.value
        : currentBotIds.value[0];
      const eventBotIds = Array.from(new Set(
        (currentEventBotIds.value.length > 0 ? currentEventBotIds.value : currentBotIds.value)
          .map((id) => String(id || '').trim())
          .filter((id) => currentBotIds.value.includes(id)),
      ));
      if (primary && !eventBotIds.includes(primary)) {
        eventBotIds.push(primary);
      }
      await syncChannelBotBindings(currentBotIds.value);
      await chat.updateChannelFeatures(channelId.value, {
        botFeatureEnabled: true,
        builtInDiceEnabled: false,
        primaryBotId: primary,
        eventBotIds,
      });
      if (chat.curChannel?.id === channelId.value && primary) {
        void characterCardStore.revalidateCharacterApi(channelId.value);
      }
      currentPrimaryBotId.value = primary;
      currentEventBotIds.value = eventBotIds;
    } else if (currentMode.value === 'builtin') {
      await syncChannelBotBindings([]);
      await chat.updateChannelFeatures(channelId.value, {
        botFeatureEnabled: false,
        builtInDiceEnabled: true,
        primaryBotId: '',
        eventBotIds: [],
      });
      currentPrimaryBotId.value = '';
      currentEventBotIds.value = [];
    } else {
      await syncChannelBotBindings([]);
      await chat.updateChannelFeatures(channelId.value, {
        botFeatureEnabled: false,
        builtInDiceEnabled: false,
        primaryBotId: '',
        eventBotIds: [],
      });
      currentPrimaryBotId.value = '';
      currentEventBotIds.value = [];
    }
    message.success('频道默认掷骰方式已更新');
    emit('update');
  } catch (error: any) {
    message.error(error?.response?.data?.error || error?.response?.data?.message || '保存频道掷骰设置失败');
  } finally {
    channelSaving.value = false;
  }
};

const saveWorldDefaults = async () => {
  if (!worldId.value || !worldSectionVisible.value) {
    return;
  }
  if (worldMode.value === 'bot' && !worldBotId.value) {
    message.error('选择 BOT 掷骰时必须指定默认 BOT');
    return;
  }
  if (worldMode.value === 'bot' && worldBotIds.value.length === 0) {
    message.error('请至少绑定一个 BOT');
    return;
  }
  worldSaving.value = true;
  try {
    await chat.worldUpdate(worldId.value, {
      channelDefaultDiceMode: worldMode.value,
      channelDefaultBotId: worldBotId.value,
      channelDefaultBotIds: worldBotIds.value,
      channelDefaultEventBotIds: worldEventBotIds.value,
    });
    message.success('新频道默认掷骰方式已更新');
    await loadWorldDetail(true);
  } catch (error: any) {
    message.error(error?.response?.data?.message || '保存世界默认掷骰设置失败');
  } finally {
    worldSaving.value = false;
  }
};

const handleBotListUpdated = async () => {
  await loadBotOptions(true);
};

watch(
  () => [props.channel?.id, props.channel?.builtInDiceEnabled, props.channel?.botFeatureEnabled, props.channel?.primaryBotId, props.channel?.eventBotIds] as const,
  async ([id]) => {
    if (!id) {
      canManageChannel.value = false;
      currentBotIds.value = [];
      currentPrimaryBotId.value = '';
      currentEventBotIds.value = [];
      return;
    }
    resetCurrentModeFromChannel();
    await Promise.all([
      loadBotOptions(),
      refreshChannelBotSelection(),
      loadChannelPermission(),
    ]);
  },
  { immediate: true },
);

watch(
  () => worldId.value,
  async (id) => {
    if (!id) {
      worldMode.value = 'builtin';
      worldBotIds.value = [];
      worldBotId.value = '';
      worldEventBotIds.value = [];
      return;
    }
    await loadWorldDetail();
  },
  { immediate: true },
);

watch(worldDetail, () => {
  resetWorldModeFromDetail();
});

onMounted(() => {
  chatEvent.on('bot-list-updated', handleBotListUpdated as any);
});

onUnmounted(() => {
  chatEvent.off('bot-list-updated', handleBotListUpdated as any);
});
</script>

<template>
  <div class="tab-dice-mode">
    <n-space vertical :size="16">
      <n-card size="small" title="当前频道默认掷骰方式">
        <n-space vertical :size="12">
          <n-radio-group
            v-model:value="currentMode"
            :disabled="channelSectionDisabled || permissionLoading || channelSaving"
          >
            <n-space>
              <n-radio
                v-for="item in diceModeOptions"
                :key="item.value"
                :value="item.value"
              >
                {{ item.label }}
              </n-radio>
            </n-space>
          </n-radio-group>
          <div v-if="currentMode === 'bot'" class="tab-dice-mode__bot-config">
            <div class="tab-dice-mode__intro">
              BOT 掷骰分 3 步：先把 BOT 绑定到频道，再指定 1 个主控 BOT，最后决定哪些 BOT 接收频道事件。
            </div>
            <div class="tab-dice-mode__field">
              <div class="tab-dice-mode__field-title">1. 频道已绑定 BOT</div>
              <div class="tab-dice-mode__field-desc">这些 BOT 会加入当前频道。至少选择 1 个，才能启用 BOT 掷骰。</div>
              <n-select
                v-model:value="currentBotIds"
                :options="botSelectOptions"
                :loading="botOptionsLoading || channelBotsLoading"
                :disabled="channelSectionDisabled || channelSaving || !hasBotOptions"
                placeholder="选择要绑定到当前频道的 BOT"
                clearable
                multiple
              />
            </div>
            <template v-if="currentBotIds.length > 0">
              <div class="tab-dice-mode__field">
                <div class="tab-dice-mode__field-title">2. 主控 BOT</div>
                <div class="tab-dice-mode__field-desc">负责处理掷骰命令，以及频道里的角色卡相关能力。</div>
                <n-select
                  v-model:value="currentPrimaryBotId"
                  :options="primaryBotSelectOptions"
                  :loading="botOptionsLoading || channelBotsLoading"
                  :disabled="channelSectionDisabled || channelSaving"
                  placeholder="选择主控 BOT"
                  clearable
                />
              </div>
              <div class="tab-dice-mode__field">
                <div class="tab-dice-mode__field-title">3. 接收频道事件的 BOT</div>
                <div class="tab-dice-mode__field-desc">决定哪些 BOT 会收到本频道事件。留空时，默认全部已绑定 BOT 都会接收。</div>
                <n-select
                  v-model:value="currentEventBotIds"
                  :options="eventBotSelectOptions"
                  :loading="botOptionsLoading || channelBotsLoading"
                  :disabled="channelSectionDisabled || channelSaving"
                  placeholder="选择接收频道事件的 BOT"
                  clearable
                  multiple
                />
              </div>
            </template>
          </div>
          <div v-if="currentMode === 'bot' && !botOptionsLoading && !hasBotOptions" class="tab-dice-mode__hint">
            暂无可用机器人令牌，请先在后台创建。
          </div>
          <div v-if="currentMode === 'bot' && currentBotIds.length > 0" class="tab-dice-mode__hint">
            频道事件指的是频道中的消息收发与BOT指令。
          </div>
          <div v-if="isPrivateChannel" class="tab-dice-mode__hint">
            私聊频道不支持在这里修改默认掷骰处理方式。
          </div>
          <div v-else-if="!permissionLoading && !canManageChannel" class="tab-dice-mode__hint">
            你需要具备频道管理信息或机器人角色关联权限。
          </div>
          <n-button
            type="primary"
            :loading="channelSaving"
            :disabled="channelSectionDisabled || permissionLoading"
            @click="saveChannelMode"
          >
            保存当前频道设置
          </n-button>
        </n-space>
      </n-card>

      <n-card v-if="worldSectionVisible" size="small" title="新频道默认掷骰方式">
        <n-space vertical :size="12">
          <WorldDiceDefaultsFields
            v-model:mode="worldMode"
            v-model:bot-ids="worldBotIds"
            v-model:bot-id="worldBotId"
            v-model:event-bot-ids="worldEventBotIds"
            :bot-select-options="botSelectOptions"
            :bot-options-loading="botOptionsLoading"
            :disabled="worldSaving || worldDetailLoading"
            hint="仅影响后续新建频道，不修改现有频道。"
          />
          <div v-if="worldMode === 'bot' && !botOptionsLoading && !hasBotOptions" class="tab-dice-mode__hint">
            暂无可用机器人令牌，请先在后台创建。
          </div>
          <n-button
            type="primary"
            secondary
            :loading="worldSaving"
            :disabled="worldDetailLoading"
            @click="saveWorldDefaults"
          >
            保存新频道默认值
          </n-button>
        </n-space>
      </n-card>
    </n-space>
  </div>
</template>

<style scoped>
.tab-dice-mode {
  padding-top: 8px;
}

.tab-dice-mode__hint {
  color: var(--sc-text-secondary);
  font-size: 12px;
}

.tab-dice-mode__bot-config {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.tab-dice-mode__intro {
  color: var(--sc-text-secondary);
  font-size: 12px;
  line-height: 1.6;
}

.tab-dice-mode__field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tab-dice-mode__field-title {
  color: var(--sc-text-primary);
  font-size: 13px;
  font-weight: 600;
}

.tab-dice-mode__field-desc {
  color: var(--sc-text-secondary);
  font-size: 12px;
  line-height: 1.6;
}
</style>
