<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useDialog, useMessage } from 'naive-ui';
import { DEFAULT_CARD_TEMPLATE } from '@/utils/characterCardTemplate';

const props = defineProps<{ worldId: string, visible: boolean }>();
const emit = defineEmits(['update:visible']);
const chat = useChatStore();
const message = useMessage();
const dialog = useDialog();
const form = ref<any>({});
const loading = ref(false);
const botOptionsLoading = ref(false);
const botList = ref<any[]>([]);

const diceModeOptions = [
  { label: '内置掷骰', value: 'builtin' },
  { label: 'BOT掷骰', value: 'bot' },
];

const botSelectOptions = computed(() => botList.value.map((item) => ({
  label: item.nick || item.username || item.name || 'Bot',
  value: item.id,
})));

const close = () => emit('update:visible', false);

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
      allowAdminEditMessages: detail.world?.allowAdminEditMessages ?? false,
      allowManageOtherUserChannelIdentities: detail.world?.allowManageOtherUserChannelIdentities ?? false,
      allowMemberEditKeywords: detail.world?.allowMemberEditKeywords ?? false,
      strictWhisperPrivacy: detail.world?.strictWhisperPrivacy ?? true,
      channelDefaultDiceMode: detail.world?.channelDefaultDiceMode || 'builtin',
      channelDefaultBotId: detail.world?.channelDefaultBotId || '',
      characterCardBadgeTemplate: detail.world?.characterCardBadgeTemplate ?? '',
    };
  } catch (e: any) {
    message.error(e?.response?.data?.message || '加载世界信息失败');
  }
}, { immediate: true });

const save = async () => {
  if (form.value.channelDefaultDiceMode === 'bot' && !form.value.channelDefaultBotId) {
    message.error('选择 BOT 掷骰时必须指定默认 BOT');
    return;
  }
  loading.value = true;
  try {
    await chat.worldUpdate(props.worldId, form.value);
    message.success('已保存');
    close();
  } catch (e: any) {
    message.error(e?.response?.data?.message || '保存失败');
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
</script>

<template>
  <n-modal :show="props.visible" preset="dialog" title="世界管理" @update:show="close">
    <div class="manager-body-scroll">
      <n-form label-width="72">
        <n-form-item label="名称">
          <n-input v-model:value="form.name" />
        </n-form-item>
        <n-form-item label="简介">
          <n-input
            type="textarea"
            v-model:value="form.description"
            maxlength="30"
            show-count
          />
        </n-form-item>
        <n-form-item label="可见性">
          <n-select v-model:value="form.visibility" :options="[
            { label: '公开', value: 'public' },
            { label: '私有', value: 'private' },
            { label: '隐藏链接', value: 'unlisted' },
          ]" />
        </n-form-item>
        <n-form-item label="管理权限">
          <div class="manager-permission-group">
            <div class="manager-permission-row">
              <n-switch v-model:value="form.allowAdminEditMessages" />
              <span class="manager-permission-text">允许管理员编辑其他成员发言</span>
            </div>
            <div class="manager-permission-row">
              <n-switch v-model:value="form.allowMemberEditKeywords" />
              <span class="manager-permission-text">允许成员编辑世界术语</span>
            </div>
            <div class="manager-permission-row">
              <n-switch v-model:value="form.allowManageOtherUserChannelIdentities" />
              <span class="manager-permission-text">允许管理其他用户频道角色</span>
            </div>
            <div class="manager-permission-row">
              <n-switch v-model:value="form.strictWhisperPrivacy" />
              <span class="manager-permission-text">不允许管理员查看所有的悄悄话</span>
            </div>
            <div class="manager-permission-block">
              <div class="manager-permission-title">新频道默认掷骰方式</div>
              <n-radio-group v-model:value="form.channelDefaultDiceMode">
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
            </div>
            <div
              v-if="form.channelDefaultDiceMode === 'bot'"
              class="manager-permission-block"
            >
              <div class="manager-permission-title">默认 BOT</div>
              <n-select
                v-model:value="form.channelDefaultBotId"
                :options="botSelectOptions"
                :loading="botOptionsLoading"
                placeholder="选择当前已添加的 BOT"
                clearable
              />
            </div>
            <div class="manager-permission-hint">
              仅影响后续新建频道，不修改现有频道。
            </div>
          </div>
        </n-form-item>
        <n-form-item label="徽章模板">
          <n-input
            v-model:value="form.characterCardBadgeTemplate"
            placeholder="留空则使用个人模板"
          />
          <span style="margin-left: 8px; color: var(--sc-text-secondary); font-size: 13px;">
            示例：{{ DEFAULT_CARD_TEMPLATE }}
          </span>
        </n-form-item>
      </n-form>
    </div>
    <template #action>
      <n-space>
        <n-button quaternary @click="close">取消</n-button>
        <n-button type="error" @click="confirmRemove" :loading="loading">删除世界</n-button>
        <n-button type="primary" @click="save" :loading="loading">保存</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped>
.manager-body-scroll {
  max-height: 70vh;
  overflow: auto;
  padding-right: 4px;
}

.manager-permission-group {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.manager-permission-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.manager-permission-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.manager-permission-title {
  color: var(--sc-text-primary);
  font-size: 13px;
}

.manager-permission-text {
  color: var(--sc-text-secondary);
  font-size: 13px;
}

.manager-permission-hint {
  color: var(--sc-text-secondary);
  font-size: 12px;
}
</style>
