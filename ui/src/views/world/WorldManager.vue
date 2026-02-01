<script setup lang="ts">
import { ref, watch } from 'vue';
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

const close = () => emit('update:visible', false);

watch(() => props.worldId, async (id) => {
  if (!id) return;
  const detail = await chat.worldDetail(id);
  form.value = {
    name: detail.world?.name,
    description: detail.world?.description,
    visibility: detail.world?.visibility,
    allowAdminEditMessages: detail.world?.allowAdminEditMessages ?? false,
    allowMemberEditKeywords: detail.world?.allowMemberEditKeywords ?? false,
    characterCardBadgeTemplate: detail.world?.characterCardBadgeTemplate ?? '',
  };
}, { immediate: true });

const save = async () => {
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
          <n-switch v-model:value="form.allowAdminEditMessages" />
          <span style="margin-left: 8px; color: var(--sc-text-secondary); font-size: 13px;">
            允许管理员编辑其他成员发言
          </span>
        </n-form-item>
        <n-form-item label="管理权限">
          <n-switch v-model:value="form.allowMemberEditKeywords" />
          <span style="margin-left: 8px; color: var(--sc-text-secondary); font-size: 13px;">
            允许成员编辑世界术语
          </span>
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
</style>
