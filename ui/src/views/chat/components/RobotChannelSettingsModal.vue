<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { useUtilsStore } from '@/stores/utils';
import type { BotProfileOption, ChannelBotSettings } from '@/types';

interface Props {
  show: boolean;
  channelId: string;
  channelName?: string;
}

const props = defineProps<Props>();
const emit = defineEmits(['update:show']);
const utils = useUtilsStore();
const message = useMessage();

const loading = ref(false);
const saving = ref(false);
const settings = ref<ChannelBotSettings>({
	channelId: '',
	enabled: false,
});
const botOptions = ref<BotProfileOption[]>([]);

const modalTitle = computed(() => props.channelName ? `机器人设置 · ${props.channelName}` : '机器人设置');
const show = computed({
  get: () => props.show,
  set: (val: boolean) => emit('update:show', val),
});

const fetchData = async () => {
  if (!props.channelId) return;
  loading.value = true;
  try {
    const [settingsResp, optionsResp] = await Promise.all([
      utils.channelBotSettingsGet(props.channelId),
      utils.botProfileOptions(),
    ]);
    settings.value = settingsResp.data || { channelId: props.channelId, enabled: false };
    botOptions.value = optionsResp.data?.items || [];
  } catch (error) {
    message.error('加载机器人配置失败');
  } finally {
    loading.value = false;
  }
};

watch(() => props.show, (val) => {
  if (val) {
    fetchData();
  }
});

watch(() => props.channelId, (val, oldVal) => {
  if (props.show && val && val !== oldVal) {
    fetchData();
  }
});

const showRemoteHint = computed(() => {
	if (!settings.value.enabled) return false;
	const remoteChannel = (settings.value.remoteChannelId || '').trim();
	const remoteGroup = (settings.value.remoteGroupId || '').trim();
	const remoteNumeric = (settings.value.remoteNumericId || '').trim();
	return !remoteChannel && !remoteGroup && !remoteNumeric;
});

const save = async () => {
	if (settings.value.enabled && !settings.value.botId) {
		message.warning('请选择默认机器人');
		return;
	}
	if (showRemoteHint.value) {
		message.warning('未填写远端频道/群组 ID，将默认使用频道 ID 向 OneBot 广播，建议补充配置');
	}
	saving.value = true;
  try {
	const payload = {
		botId: settings.value.botId,
		remoteChannelId: settings.value.remoteChannelId,
		remoteGroupId: settings.value.remoteGroupId,
		remoteNumericId: settings.value.remoteNumericId,
		enabled: settings.value.enabled,
	};
    const resp = await utils.channelBotSettingsSave(props.channelId, payload);
    settings.value = resp.data;
    message.success('机器人设置已更新');
    show.value = false;
  } catch (error) {
    message.error('保存失败，请检查权限或配置');
  } finally {
    saving.value = false;
  }
};
</script>

<template>
  <n-modal v-model:show="show" :title="modalTitle" preset="card" style="width: 520px;">
    <n-spin :show="loading">
      <n-form label-placement="left" label-width="auto">
        <n-form-item label="启用机器人">
          <n-switch v-model:value="settings.enabled" />
        </n-form-item>
        <template v-if="settings.enabled">
          <n-form-item label="默认机器人">
            <n-select
              v-model:value="settings.botId"
              :options="botOptions.map(item => ({ label: item.name, value: item.id }))"
              placeholder="选择一个 OneBot 档案"
            />
          </n-form-item>
          <n-form-item label="远端频道 ID">
            <n-input v-model:value="settings.remoteChannelId" placeholder="可选，映射到 OneBot group_id/channel_id" />
          </n-form-item>
          <n-form-item label="远端群组 ID">
            <n-input v-model:value="settings.remoteGroupId" placeholder="可选，供不同平台区分" />
          </n-form-item>
          <n-form-item label="远端数字 ID">
            <n-input v-model:value="settings.remoteNumericId" placeholder="可选，纯数字群号/频道号" />
          </n-form-item>
          <n-alert
            v-if="showRemoteHint"
            type="warning"
            class="mb-2"
            :bordered="false"
            :show-icon="false"
          >
            未填写远端 ID 时，系统会使用当前频道 ID 向 OneBot 端广播，建议根据实际群号/频道号补全配置（若核心只接受数字，请填写“远端数字 ID”）。
          </n-alert>
        </template>
      </n-form>
    </n-spin>
    <template #footer>
      <div class="flex justify-end space-x-2">
        <n-button @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="save">保存</n-button>
      </div>
    </template>
  </n-modal>
</template>
