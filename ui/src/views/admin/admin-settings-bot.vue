<script setup lang="tsx">
import { useUtilsStore } from '@/stores/utils';
import type { BotProfileView } from '@/types';
import { Robot } from '@vicons/tabler';
import { useDialog, useMessage } from 'naive-ui';
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';

const { t } = useI18n();
const emit = defineEmits(['close']);
const utils = useUtilsStore();
const message = useMessage();
const dialog = useDialog();

const showModal = ref(false);
const newTokenName = ref('bot');
const tokens = ref({
  total: 0,
  items: [] as any[],
});

interface BotProfileForm {
  name: string;
  avatarUrl: string;
  channelRoleName: string;
  connMode: 'forward_ws' | 'reverse_ws';
  remoteSelfId: string;
  forwardHost: string;
  forwardPort: number;
  forwardApiPath: string;
  forwardEventPath: string;
  forwardUniversal: string;
  reverseApiText: string;
  reverseEventText: string;
  reverseUniversalText: string;
  reverseUseUniversal: boolean;
  reverseReconnectSec: number;
  accessToken: string;
  defaultChannelId: string;
  enabled: boolean;
}

const createProfileDefaults = (): BotProfileForm => ({
  name: '',
  avatarUrl: '',
  channelRoleName: '',
  connMode: 'forward_ws',
  remoteSelfId: '',
  forwardHost: '0.0.0.0',
  forwardPort: 33212,
  forwardApiPath: '/onebot/ws/api',
  forwardEventPath: '/onebot/ws/event',
  forwardUniversal: '/onebot/ws/',
  reverseApiText: '',
  reverseEventText: '',
  reverseUniversalText: '',
  reverseUseUniversal: false,
  reverseReconnectSec: 10,
  accessToken: '',
  defaultChannelId: '',
  enabled: true,
});

const botProfiles = ref<BotProfileView[]>([]);
const loadingProfiles = ref(false);
const showProfileModal = ref(false);
const profileForm = ref<BotProfileForm>(createProfileDefaults());
const editingProfileId = ref<string | null>(null);
const savingProfile = ref(false);

const cancel = () => emit('close');

const profileModalTitle = computed(() => editingProfileId.value ? '编辑机器人' : '新增机器人');

const parseMultiline = (text: string) => text.split(/\r?\n/).map((item) => item.trim()).filter(Boolean);
const toMultiline = (values?: string[]) => (values && values.length ? values.join('\n') : '');

const addToken = async () => {
  try {
    await utils.botTokenAdd(newTokenName.value);
    message.success('添加成功');
    await loadTokens();
  } catch (error) {
    message.error('添加失败: ' + (error as any).response?.data?.message || '未知错误');
  }
  newTokenName.value = 'bot';
};

const loadTokens = async () => {
  const resp = await utils.botTokenList();
  tokens.value = resp.data;
};

const loadBotProfiles = async () => {
  loadingProfiles.value = true;
  try {
    const resp = await utils.adminBotList();
    botProfiles.value = resp.data?.items || [];
  } catch (error) {
    message.error('加载机器人失败: ' + ((error as any)?.response?.data?.message || '未知错误'));
  } finally {
    loadingProfiles.value = false;
  }
};

const openProfileModal = (profile?: BotProfileView) => {
  if (profile) {
    profileForm.value = {
      name: profile.name,
      avatarUrl: profile.avatarUrl || '',
      channelRoleName: profile.channelRoleName || '',
      connMode: profile.connMode,
      remoteSelfId: profile.remoteSelfId || '',
      forwardHost: profile.forwardHost || '0.0.0.0',
      forwardPort: profile.forwardPort || 33212,
      forwardApiPath: profile.forwardApiPath || '/onebot/ws/api',
      forwardEventPath: profile.forwardEventPath || '/onebot/ws/event',
      forwardUniversal: profile.forwardUniversal || '/onebot/ws/',
      reverseApiText: toMultiline(profile.reverseApiEndpoints),
      reverseEventText: toMultiline(profile.reverseEventUrls),
      reverseUniversalText: toMultiline(profile.reverseUniversalUrls),
      reverseUseUniversal: !!profile.reverseUseUniversal,
      reverseReconnectSec: profile.reverseReconnectSec || 10,
      accessToken: profile.accessToken || '',
      defaultChannelId: profile.defaultChannelId || '',
      enabled: profile.enabled,
    };
    editingProfileId.value = profile.id;
  } else {
    profileForm.value = createProfileDefaults();
    editingProfileId.value = null;
  }
  showProfileModal.value = true;
};

const buildProfilePayload = () => {
  return {
    name: profileForm.value.name.trim(),
    avatarUrl: profileForm.value.avatarUrl.trim(),
    channelRoleName: profileForm.value.channelRoleName.trim(),
    connMode: profileForm.value.connMode,
    remoteSelfId: profileForm.value.remoteSelfId.trim(),
    forward: {
      host: profileForm.value.forwardHost.trim(),
      port: profileForm.value.forwardPort,
      apiPath: profileForm.value.forwardApiPath.trim(),
      eventPath: profileForm.value.forwardEventPath.trim(),
      universalPath: profileForm.value.forwardUniversal.trim(),
    },
    reverse: {
      apiEndpoints: parseMultiline(profileForm.value.reverseApiText),
      eventEndpoints: parseMultiline(profileForm.value.reverseEventText),
      universalEndpoints: parseMultiline(profileForm.value.reverseUniversalText),
      useUniversal: profileForm.value.reverseUseUniversal,
      reconnectInterval: profileForm.value.reverseReconnectSec,
    },
    accessToken: profileForm.value.accessToken.trim(),
    defaultChannelId: profileForm.value.defaultChannelId.trim(),
    enabled: profileForm.value.enabled,
  };
};

const saveProfile = async () => {
  if (!profileForm.value.name.trim()) {
    message.warning('请填写机器人名称');
    return;
  }
  savingProfile.value = true;
  try {
    const payload = buildProfilePayload();
    if (editingProfileId.value) {
      await utils.adminBotUpdate(editingProfileId.value, payload);
    } else {
      await utils.adminBotCreate(payload);
    }
    message.success('保存成功');
    showProfileModal.value = false;
    await loadBotProfiles();
  } catch (error) {
    message.error('保存失败: ' + ((error as any)?.response?.data?.message || '未知原因'));
  } finally {
    savingProfile.value = false;
  }
};

const deleteProfile = (profile: BotProfileView) => {
  dialog.warning({
    title: '删除机器人',
    content: `确定要删除 ${profile.name} 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await utils.adminBotDelete(profile.id);
        message.success('已删除机器人');
        await loadBotProfiles();
      } catch (error) {
        message.error('删除失败: ' + ((error as any)?.response?.data?.message || '未知错误'));
      }
    },
  });
};

const testProfile = async (profile: BotProfileView) => {
  try {
    const resp = await utils.adminBotTest(profile.id);
    message.success(resp.data?.message || '已提交测试请求');
  } catch (error) {
    message.error('测试失败: ' + ((error as any)?.response?.data?.message || '未知错误'));
  }
};

const deleteItem = (item: any) => {
  dialog.warning({
    title: t('dialogLogOut.title'),
    content: '确定要删除吗？',
    positiveText: t('dialogLogOut.positiveText'),
    negativeText: t('dialogLogOut.negativeText'),
    onPositiveClick: async () => {
      try {
        await utils.botTokenDelete(item.id);
        message.success('删除成功');
        await loadTokens();
      } catch (error) {
        message.error('删除失败: ' + ((error as any).response?.data?.message || '未知错误'));
      }
    },
  });
};

const botStatusType = (status?: string) => {
  switch (status) {
    case 'connected':
      return 'success';
    case 'connecting':
      return 'warning';
    case 'disabled':
      return 'default';
    default:
      return 'error';
  }
};

onMounted(async () => {
  await Promise.all([loadTokens(), loadBotProfiles()]);
});
</script>

<template>
  <div class="overflow-y-auto pr-2 space-y-4" style="max-height: 61vh; margin-top: 0;">
    <n-card size="small" title="机器人档案">
      <template #header-extra>
        <n-button type="primary" size="small" @click="openProfileModal()">
          <template #icon>
            <n-icon :component="Robot" />
          </template>
          新增机器人
        </n-button>
      </template>
      <n-spin :show="loadingProfiles">
        <n-empty v-if="!botProfiles.length" description="暂未创建机器人">
          <n-button size="small" type="primary" @click="openProfileModal()">立即创建</n-button>
        </n-empty>
        <div v-else class="space-y-3">
          <div
            v-for="item in botProfiles"
            :key="item.id"
            class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-gray-200/50 dark:border-gray-700/60 px-4 py-3 bg-white/60 dark:bg-gray-800/30"
          >
            <div class="flex items-center space-x-3">
              <n-avatar :round="true" :size="38" :src="item.avatarUrl">
                {{ item.name?.slice(0, 1) || 'Bot' }}
              </n-avatar>
              <div>
                <div class="font-medium text-base">{{ item.name }}</div>
                <div class="text-xs text-gray-500 dark:text-gray-400">
                  账号：{{ item.userId || '—' }}
                </div>
                <div class="text-xs text-gray-500 dark:text-gray-400">
                  模式：{{ item.connMode === 'forward_ws' ? '正向 WebSocket' : '反向 WebSocket' }}
                </div>
                <div class="text-xs text-gray-400">
                  默认频道：{{ item.defaultChannelId || '未绑定' }}
                </div>
              </div>
            </div>
            <div class="flex items-center space-x-2">
              <n-tag size="small" :type="botStatusType(item.runtime?.status)">
                {{ item.runtime?.status || (item.enabled ? 'disconnected' : 'disabled') }}
              </n-tag>
              <n-button size="small" @click="testProfile(item)">连接测试</n-button>
              <n-button size="small" @click="openProfileModal(item)">编辑</n-button>
              <n-button size="small" type="error" @click="deleteProfile(item)">删除</n-button>
            </div>
          </div>
        </div>
      </n-spin>
    </n-card>

    <n-card size="small" title="Token 管理">
      <n-list>
        <template #header>
          当前 token 列表
        </template>

        <n-list-item v-for="i in tokens.items" :key="i.id">
          <template #suffix>
            <div class="flex items-center space-x-2">
              <div style="width: 9rem;">
                <span>到期时间</span>
                <n-date-picker v-model:value="i.expiresAt" type="date" />
              </div>
              <div>
                <span>操作</span>
                <n-button size="small" @click="deleteItem(i)">删除</n-button>
              </div>
            </div>
          </template>
          <n-thing :title="i.name" :description="i.token" />
        </n-list-item>

        <template #footer>
          <n-button @click="showModal = true">添加</n-button>
        </template>
      </n-list>
    </n-card>
  </div>

  <div class="space-x-2 float-right">
    <n-button @click="cancel">关闭</n-button>
  </div>

  <n-modal
    v-model:show="showModal"
    preset="dialog"
    :title="'创建 Token'"
    :positive-text="$t('dialoChannelgNew.positiveText')"
    :negative-text="$t('dialoChannelgNew.negativeText')"
    @positive-click="addToken"
  >
    <n-input v-model:value="newTokenName" placeholder="输入 token 名称" />
  </n-modal>

  <n-drawer
    v-model:show="showProfileModal"
    :width="520"
    placement="right"
    :closable="false"
  >
    <n-drawer-content :title="profileModalTitle">
      <n-form label-placement="left" label-width="auto">
        <n-form-item label="机器人名称">
          <n-input v-model:value="profileForm.name" placeholder="用于展示的名称" />
        </n-form-item>
        <n-form-item label="头像地址">
          <n-input v-model:value="profileForm.avatarUrl" placeholder="可选，填写图片 URL" />
        </n-form-item>
        <n-form-item label="角色名">
          <n-input v-model:value="profileForm.channelRoleName" placeholder="可选，用于频道内展示" />
        </n-form-item>
        <n-form-item label="连接模式">
          <n-select
            v-model:value="profileForm.connMode"
            :options="[
              { label: '正向 WebSocket（OneBot 作为客户端接入平台）', value: 'forward_ws' },
              { label: '反向 WebSocket（平台主动连接 OneBot）', value: 'reverse_ws' },
            ]"
          />
        </n-form-item>
        <n-form-item label="启用状态">
          <n-switch v-model:value="profileForm.enabled" />
        </n-form-item>

        <n-divider>正向 WebSocket</n-divider>
        <n-form-item label="监听地址">
          <div class="flex items-center space-x-2 w-full">
            <n-input v-model:value="profileForm.forwardHost" placeholder="0.0.0.0" />
            <n-input-number v-model:value="profileForm.forwardPort" style="width: 120px" />
          </div>
        </n-form-item>
        <n-form-item label="API Path">
          <n-input v-model:value="profileForm.forwardApiPath" />
        </n-form-item>
        <n-form-item label="Event Path">
          <n-input v-model:value="profileForm.forwardEventPath" />
        </n-form-item>
        <n-form-item label="Universal Path">
          <n-input v-model:value="profileForm.forwardUniversal" />
        </n-form-item>

        <n-divider>反向 WebSocket</n-divider>
        <n-form-item label="API URLs">
          <n-input
            type="textarea"
            v-model:value="profileForm.reverseApiText"
            placeholder="每行一个反向连接地址"
          />
        </n-form-item>
        <n-form-item label="Event URLs">
          <n-input
            type="textarea"
            v-model:value="profileForm.reverseEventText"
            placeholder="每行一个 event 地址"
          />
        </n-form-item>
        <n-form-item label="Universal URLs">
          <n-input
            type="textarea"
            v-model:value="profileForm.reverseUniversalText"
            placeholder="每行一个 universal 地址"
          />
        </n-form-item>
        <n-form-item label="优先使用 Universal">
          <n-switch v-model:value="profileForm.reverseUseUniversal" />
        </n-form-item>
        <n-form-item label="重连间隔(秒)">
          <n-input-number v-model:value="profileForm.reverseReconnectSec" :min="3" />
        </n-form-item>

        <n-divider>鉴权与默认频道</n-divider>
        <n-form-item label="远端机器人 ID" feedback="用于填充 self_id，必须为纯数字">
          <n-input v-model:value="profileForm.remoteSelfId" placeholder="必填，OneBot 自身 QQ/频道数字 ID" />
        </n-form-item>
        <n-form-item label="Access Token">
          <n-input v-model:value="profileForm.accessToken" placeholder="空值表示无需校验" />
        </n-form-item>
        <n-form-item label="默认频道 ID">
          <n-input v-model:value="profileForm.defaultChannelId" placeholder="可选，配置后用于默认绑定" />
        </n-form-item>
      </n-form>

      <template #footer>
        <div class="flex justify-end space-x-2">
          <n-button @click="showProfileModal = false">取消</n-button>
          <n-button type="primary" :loading="savingProfile" @click="saveProfile">保存</n-button>
        </div>
      </template>
    </n-drawer-content>
  </n-drawer>
</template>
