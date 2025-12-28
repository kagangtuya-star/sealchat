<script lang="tsx" setup>
import { useChatStore } from '@/stores/chat';
import { useUserStore } from '@/stores/user';
import { ChannelType, type SChannel } from '@/types';
import { clone } from 'lodash-es';
import { useDialog, useMessage } from 'naive-ui';
import { onMounted, ref, watch, type PropType } from 'vue';
import { useI18n } from 'vue-i18n';
import { chatEvent } from '@/stores/chat';

const message = useMessage()
const dialog = useDialog()
const chat = useChatStore();
const user = useUserStore();
const { t } = useI18n()

const show = defineModel<boolean>('show');

const props = defineProps({
  parentId: {
    type: String as PropType<string>,
    default: ''
  }
});


const load = async () => {
  model.value = {
    name: '',
    parent_id: props.parentId,
    permType: user.checkPerm('func_channel_create_public') ? ChannelType.Public : ChannelType.NonPublic
  };
}

onMounted(() => {
  load();
})
watch(() => show.value, (newValue) => {
  if (newValue) {
    load();
  }
});

const model = ref({
  name: '',
  permType: 'public',
  parent_id: '',
})

const isWorldManager = async () => {
  const worldId = chat.currentWorldId;
  if (!worldId) return false;
  try {
    const detail = chat.worldDetailMap?.[worldId] || await chat.worldDetail(worldId);
    const role = detail?.memberRole;
    return role === 'owner' || role === 'admin';
  } catch (error) {
    return false;
  }
};

const openChannelSettings = async (channelId?: string) => {
  if (!channelId) return;
  await chat.channelSwitchTo(channelId);
  chatEvent.emit('channel-member-settings-open');
};

const newChannel = async () => {
  if (!model.value.name.trim()) {
    message.error(t('dialoChannelgNew.channelNameHint'));
    return false;
  }
  if (model.value.permType === ChannelType.Public && !user.checkPerm('func_channel_create_public')) {
    message.error('你没有权限这样做');
    return false;
  }
  if (model.value.permType === ChannelType.NonPublic && !user.checkPerm('func_channel_create_non_public')) {
    message.error('你没有权限这样做');
    return false;
  }
  try {
    const resp = await chat.channelCreate(model.value);
    const createdChannelId =
      (resp as any)?.data?.channel?.id ||
      (resp as any)?.channel?.id ||
      (resp as any)?.id;
    await chat.channelList(chat.currentWorldId, true);
    show.value = false;
    message.success('频道创建成功');
    if (await isWorldManager()) {
      dialog.info({
        title: '频道已创建',
        content: '建议为新频道添加成员并设置权限，是否现在打开频道管理？',
        positiveText: '打开管理',
        negativeText: '稍后',
        onPositiveClick: async () => {
          await openChannelSettings(createdChannelId);
        },
      });
    }
  } catch (err: any) {
    message.error(err?.message || err?.response?.err || '创建频道失败');
    return false;
  }
}

</script>

<template>
  <n-modal v-model:show="show" preset="dialog" :title="$t('dialoChannelgNew.title')"
    :positive-text="$t('dialoChannelgNew.positiveText')" :negative-text="$t('dialoChannelgNew.negativeText')"
    @positive-click="newChannel">

    <n-form ref="formCreate" class="mt-6">
      <n-form-item :label="$t('dialoChannelgNew.channelParent')" v-if="Boolean(props.parentId)">
        <n-select v-model:value="model.parent_id" :options="chat.channelTree.map(channel => ({
          label: channel.name,
          value: channel.id
        }))" :placeholder="$t('dialoChannelgNew.channelParentSelect')" clearable>
        </n-select>
      </n-form-item>

      <n-form-item :label="$t('dialoChannelgNew.channelName')">
        <n-input v-model:value="model.name" :placeholder="$t('dialoChannelgNew.channelNamePlaceholder')"></n-input>
      </n-form-item>

      <n-form-item :label="$t('dialoChannelgNew.channelType')">
        <n-radio-group v-model:value="model.permType" class=" space-x-2">
          <n-radio :value="ChannelType.Public" :disabled="!user.checkPerm('func_channel_create_public')">
            <span>{{ $t('channelType.public') }}</span>
          </n-radio>
          <n-radio :value="ChannelType.NonPublic" :disabled="!user.checkPerm('func_channel_create_non_public')">
            <span>{{ $t('channelType.nonPublic') }}</span>
          </n-radio>
        </n-radio-group>
      </n-form-item>
      <n-text depth="3" class=" -mt-4 block">
        {{ model.permType === ChannelType.Public ? '公开频道可以被所有人看到和进入' : '非公开频道需要邀请才能加入' }}
      </n-text>
    </n-form>

  </n-modal>
</template>

<style lang="scss"></style>
