<script setup lang="ts">
import { computed, ref, watch, type PropType } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import type { SChannel } from '@/types';
import { useMessage } from 'naive-ui';

const show = defineModel<boolean>('show');

const props = defineProps({
  channel: {
    type: Object as PropType<SChannel | undefined>,
    default: undefined,
  },
});

const chat = useChatStore();
const user = useUserStore();
const message = useMessage();

const submitting = ref(false);
const model = ref({
  name: '',
  worldId: '',
  parentId: '',
  copyRoles: true,
  copyMembers: true,
  copyIdentities: true,
  copyStickyNotes: true,
  copyGallery: true,
  copyIForms: true,
  copyDiceMacros: true,
  copyAudioScenes: true,
  copyAudioState: false,
  copyWebhooks: false,
  copyLocalConfig: true,
});

const sourceChannelName = computed(() => props.channel?.name || '未命名频道');

const loadDefaults = () => {
  const channel = props.channel;
  model.value.name = `${channel?.name || '未命名频道'}-副本`;
  model.value.worldId = channel?.worldId || chat.currentWorldId || '';
  model.value.parentId = channel?.parentId || '';
  model.value.copyRoles = true;
  model.value.copyMembers = true;
  model.value.copyIdentities = true;
  model.value.copyStickyNotes = true;
  model.value.copyGallery = true;
  model.value.copyIForms = true;
  model.value.copyDiceMacros = true;
  model.value.copyAudioScenes = true;
  model.value.copyAudioState = false;
  model.value.copyWebhooks = false;
  model.value.copyLocalConfig = true;
};

watch(
  () => show.value,
  (visible) => {
    if (visible) {
      loadDefaults();
    }
  },
);

watch(
  () => model.value.worldId,
  async (worldId, prevWorldId) => {
    if (worldId && prevWorldId && worldId !== prevWorldId) {
      model.value.parentId = '';
    }
    if (!worldId || worldId === chat.currentWorldId) {
      return;
    }
    try {
      await chat.channelList(worldId, true);
    } catch (error) {
      console.warn('加载目标世界频道失败', error);
    }
  },
);

const worldOptions = computed(() => chat.joinedWorldOptions);

const parentOptions = computed(() => {
  const worldId = model.value.worldId || chat.currentWorldId;
  const tree = chat.channelTreeByWorld[worldId] || [];
  const sourceId = props.channel?.id;
  return tree
    .filter((item) => item.id !== sourceId)
    .map((item) => ({ label: item.name || '未命名频道', value: item.id }));
});

const handleCopy = async () => {
  if (!props.channel?.id) {
    message.error('缺少源频道信息');
    return false;
  }
  const name = model.value.name.trim();
  if (!name) {
    message.error('请输入新频道名称');
    return false;
  }
  const worldId = model.value.worldId || chat.currentWorldId;
  if (!worldId) {
    message.error('请选择目标世界');
    return false;
  }

  submitting.value = true;
  try {
    const resp = await chat.channelCopy(props.channel.id, {
      name,
      worldId,
      parentId: model.value.parentId || '',
      options: {
        copyRoles: model.value.copyRoles,
        copyMembers: model.value.copyMembers,
        copyIdentities: model.value.copyIdentities,
        copyStickyNotes: model.value.copyStickyNotes,
        copyGallery: model.value.copyGallery,
        copyIForms: model.value.copyIForms,
        copyDiceMacros: model.value.copyDiceMacros,
        copyAudioScenes: model.value.copyAudioScenes,
        copyAudioState: model.value.copyAudioState,
        copyWebhooks: model.value.copyWebhooks,
      },
    });

    const newChannelId = resp?.channelId;
    const identityMap = resp?.identityMap;
    if (model.value.copyLocalConfig && newChannelId) {
      chat.copyLocalChannelSettings(props.channel.id, newChannelId, user.info?.id, identityMap);
    }
    await chat.channelList(worldId, true);
    show.value = false;
    message.success('频道复制成功');
    if (newChannelId) {
      if (worldId !== chat.currentWorldId) {
        await chat.switchWorld(worldId, { force: true });
      }
      await chat.channelSwitchTo(newChannelId);
    }
    return true;
  } catch (err: any) {
    message.error(err?.response?.data?.error || err?.message || '频道复制失败');
    return false;
  } finally {
    submitting.value = false;
  }
};
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="dialog"
    title="复制频道"
    :positive-text="submitting ? '复制中…' : '复制'"
    :positive-button-props="{ loading: submitting }"
    negative-text="取消"
    @positive-click="handleCopy"
  >
    <n-form label-width="90" class="mt-4">
      <n-form-item label="源频道">
        <n-input :value="sourceChannelName" disabled />
      </n-form-item>
      <n-form-item label="新频道名称">
        <n-input v-model:value="model.name" placeholder="输入新频道名称" maxlength="64" show-count />
      </n-form-item>
      <n-form-item label="目标世界">
        <n-select v-model:value="model.worldId" :options="worldOptions" placeholder="选择目标世界" />
      </n-form-item>
      <n-form-item label="父频道">
        <n-select
          v-model:value="model.parentId"
          :options="parentOptions"
          placeholder="选择父频道（可选）"
          clearable
        />
      </n-form-item>
      <n-form-item label="复制范围">
        <n-space vertical>
          <n-space>
            <n-checkbox v-model:checked="model.copyRoles">角色与权限</n-checkbox>
            <n-checkbox v-model:checked="model.copyMembers">成员与角色绑定</n-checkbox>
            <n-checkbox v-model:checked="model.copyIdentities">频道身份</n-checkbox>
          </n-space>
          <n-space>
            <n-checkbox v-model:checked="model.copyStickyNotes">便签</n-checkbox>
            <n-checkbox v-model:checked="model.copyGallery">图库</n-checkbox>
            <n-checkbox v-model:checked="model.copyIForms">iForm</n-checkbox>
          </n-space>
          <n-space>
            <n-checkbox v-model:checked="model.copyDiceMacros">骰子宏</n-checkbox>
            <n-checkbox v-model:checked="model.copyAudioScenes">音频场景</n-checkbox>
            <n-checkbox v-model:checked="model.copyAudioState">音频播放状态</n-checkbox>
            <n-checkbox v-model:checked="model.copyWebhooks">Webhook</n-checkbox>
          </n-space>
        </n-space>
      </n-form-item>
      <n-form-item label="本地配置">
        <n-checkbox v-model:checked="model.copyLocalConfig">复制本地配置</n-checkbox>
      </n-form-item>
      <div class="text-xs text-muted">不复制聊天记录</div>
    </n-form>
  </n-modal>
</template>
