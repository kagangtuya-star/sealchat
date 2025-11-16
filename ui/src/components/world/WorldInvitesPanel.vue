<script setup lang="ts">
import type { WorldInviteModel } from '@/stores/world';
import { NCard, NButton, NForm, NFormItem, NInputNumber, NSwitch, NSkeleton, useMessage, useDialog } from 'naive-ui';
import { reactive, ref } from 'vue';
import { useWorldStore } from '@/stores/world';

const props = defineProps<{
  invites: WorldInviteModel[];
  loading?: boolean;
}>();

const emit = defineEmits<{
  (e: 'refresh'): void;
}>();

const worldStore = useWorldStore();
const message = useMessage();
const dialog = useDialog();
const initialForm = () => ({
  channelId: '',
  maxUses: 0,
  expireHours: 24,
  isSingleUse: false,
});
const form = reactive(initialForm());

const submitting = ref(false);

const shareLink = (code: string) => {
  if (!code) return '';
  return `${window.location.origin}/#/invite/${code}`;
};

const createInvite = async () => {
  submitting.value = true;
  try {
    const invite = await worldStore.createInvite(form);
    emit('refresh');
    if (invite?.code) {
      const link = shareLink(invite.code);
      await navigator.clipboard?.writeText(link).catch(() => {});
      dialog.success({
        title: '邀请已生成',
        content: () => link,
        positiveText: '好的',
      });
      message.success('邀请链接已复制');
    }
    Object.assign(form, initialForm());
  } finally {
    submitting.value = false;
  }
};

const copyLink = async (code?: string) => {
  if (!code) return;
  const link = shareLink(code);
  await navigator.clipboard?.writeText(link).catch(() => {});
  message.success('已复制邀请链接');
};
</script>

<template>
  <div class="space-y-4">
    <n-card>
      <template v-if="loading">
        <n-skeleton height="120px" />
      </template>
      <template v-else>
        <n-form label-placement="left" label-width="90">
          <n-form-item label="限制次数">
            <n-input-number v-model:value="form.maxUses" :min="0" :max="1000" placeholder="0 表示无限制" />
          </n-form-item>
          <n-form-item label="有效期(小时)">
            <n-input-number v-model:value="form.expireHours" :min="0" :max="720" />
          </n-form-item>
          <n-form-item label="一次性邀请">
            <n-switch v-model:value="form.isSingleUse" />
          </n-form-item>
        </n-form>
        <n-button type="primary" :loading="submitting.value" @click="createInvite">生成邀请</n-button>
      </template>
    </n-card>

    <n-card>
      <template v-if="loading">
        <n-skeleton v-for="i in 4" :key="i" height="48px" class="mb-2" />
      </template>
      <template v-else>
        <div v-if="props.invites.length === 0" class="text-center text-gray-400 py-6">
          暂无邀请记录
        </div>
        <div v-else class="space-y-2">
          <div v-for="invite in props.invites" :key="invite.id" class="border rounded-md p-3 flex flex-col gap-2">
            <div class="flex items-center justify-between">
              <div>
                <div class="font-medium">邀请链接</div>
                <div class="text-xs text-gray-500">
                  次数：{{ invite.usedCount }}/{{ invite.maxUses || '∞' }} ·
                  {{ invite.isRevoked ? '已撤销' : '有效' }}
                </div>
              </div>
              <div class="text-xs text-gray-500">
                创建于 {{ invite.createdAt ? new Date(invite.createdAt as any).toLocaleString() : '--' }}
              </div>
            </div>
            <div class="flex items-center justify-between text-sm">
              <div class="truncate text-blue-600">
                {{ shareLink(invite.code || '') }}
              </div>
              <div class="flex gap-2">
                <n-button size="tiny" text @click="copyLink(invite.code)">复制</n-button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </n-card>
  </div>
</template>
