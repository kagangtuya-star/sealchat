<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch, type PropType } from 'vue';
import { useMessage } from 'naive-ui';
import type { SChannel, UserInfo, UserRoleModel } from '@/types';
import { chatEvent, useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';

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
const message = useMessage();

const diceModeOptions = [
  { label: '内置掷骰', value: 'builtin' },
  { label: 'BOT掷骰', value: 'bot' },
];

const currentMode = ref<'builtin' | 'bot'>('builtin');
const currentBotId = ref('');
const worldMode = ref<'builtin' | 'bot'>('builtin');
const worldBotId = ref('');

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
  currentMode.value = botEnabled ? 'bot' : 'builtin';
};

const resetWorldModeFromDetail = () => {
  const detail = worldDetail.value;
  const mode = detail?.world?.channelDefaultDiceMode === 'bot' ? 'bot' : 'builtin';
  worldMode.value = mode;
  worldBotId.value = String(detail?.world?.channelDefaultBotId || '').trim();
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
    currentBotId.value = '';
    return;
  }
  channelBotsLoading.value = true;
  try {
    const resp = await chat.channelMemberListAll(channelId.value, 200);
    const items = resp?.data?.items || [];
    const current = items.find((item: UserRoleModel) => item.roleId === botRoleId.value && item.user?.id);
    currentBotId.value = current?.user?.id || '';
  } catch (error: any) {
    message.error(error?.response?.data?.error || '加载频道机器人失败');
  } finally {
    channelBotsLoading.value = false;
  }
};

const syncChannelBotSelection = async (nextBotId: string) => {
  if (!channelId.value || !botRoleId.value) {
    return;
  }
  const resp = await chat.channelMemberListAll(channelId.value, 200);
  const items = resp?.data?.items || [];
  const existingIds = items
    .filter((item: UserRoleModel) => item.roleId === botRoleId.value && item.user?.id)
    .map((item: UserRoleModel) => item.user?.id || '')
    .filter(Boolean);
  if (nextBotId && !existingIds.includes(nextBotId)) {
    await chat.userRoleLink(botRoleId.value, [nextBotId]);
  }
  const toRemove = nextBotId ? existingIds.filter(id => id !== nextBotId) : existingIds;
  if (toRemove.length) {
    await chat.userRoleUnlink(botRoleId.value, toRemove);
  }
  currentBotId.value = nextBotId;
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
    if (!currentBotId.value) {
      message.error('请选择当前频道要使用的 BOT');
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
      await syncChannelBotSelection(currentBotId.value);
      await chat.updateChannelFeatures(channelId.value, {
        botFeatureEnabled: true,
        builtInDiceEnabled: false,
      });
    } else {
      await syncChannelBotSelection('');
      await chat.updateChannelFeatures(channelId.value, {
        botFeatureEnabled: false,
        builtInDiceEnabled: true,
      });
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
  worldSaving.value = true;
  try {
    await chat.worldUpdate(worldId.value, {
      channelDefaultDiceMode: worldMode.value,
      channelDefaultBotId: worldBotId.value,
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
  () => [props.channel?.id, props.channel?.builtInDiceEnabled, props.channel?.botFeatureEnabled] as const,
  async ([id]) => {
    if (!id) {
      canManageChannel.value = false;
      currentBotId.value = '';
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
      worldBotId.value = '';
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
          <n-select
            v-if="currentMode === 'bot'"
            v-model:value="currentBotId"
            :options="botSelectOptions"
            :loading="botOptionsLoading || channelBotsLoading"
            :disabled="channelSectionDisabled || channelSaving || !hasBotOptions"
            placeholder="选择当前频道要使用的 BOT"
            clearable
          />
          <div v-if="currentMode === 'bot' && !botOptionsLoading && !hasBotOptions" class="tab-dice-mode__hint">
            暂无可用机器人令牌，请先在后台创建。
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
          <n-radio-group
            v-model:value="worldMode"
            :disabled="worldSaving || worldDetailLoading"
          >
            <n-space>
              <n-radio
                v-for="item in diceModeOptions"
                :key="`world-${item.value}`"
                :value="item.value"
              >
                {{ item.label }}
              </n-radio>
            </n-space>
          </n-radio-group>
          <n-select
            v-if="worldMode === 'bot'"
            v-model:value="worldBotId"
            :options="botSelectOptions"
            :loading="botOptionsLoading"
            :disabled="worldSaving || !hasBotOptions"
            placeholder="选择新频道默认使用的 BOT"
            clearable
          />
          <div class="tab-dice-mode__hint">
            仅影响后续新建频道，不修改现有频道。
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
</style>
