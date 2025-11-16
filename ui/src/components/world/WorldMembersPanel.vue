<script setup lang="ts">
import type { WorldMemberModel } from '@/stores/world';
import { NCard, NTag, NSkeleton, NButton } from 'naive-ui';

const props = defineProps<{
  members: WorldMemberModel[];
  loading?: boolean;
  disableRemove?: boolean;
  ownerId?: string;
}>();

const emit = defineEmits<{
  (e: 'remove', member: WorldMemberModel): void;
}>();

const handleRemove = (member: WorldMemberModel) => {
  if (props.disableRemove) return;
  emit('remove', member);
};
</script>

<template>
  <n-card>
    <template v-if="loading">
      <div class="space-y-2">
        <n-skeleton v-for="i in 5" :key="i" height="48px" />
      </div>
    </template>
    <template v-else>
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-gray-500">
            <th class="py-2">成员</th>
            <th class="py-2">状态</th>
            <th class="py-2">加入时间</th>
            <th class="py-2">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="member in members" :key="member.id" class="border-t">
            <td class="py-2">{{ member.nickname || member.userId }}</td>
            <td class="py-2">
              <n-tag :type="member.state === 'banned' ? 'error' : 'success'" size="small">
                {{ member.state === 'banned' ? '已封禁' : '正常' }}
              </n-tag>
            </td>
            <td class="py-2">
              {{ member.joinedAt ? new Date(member.joinedAt).toLocaleString() : '--' }}
            </td>
            <td class="py-2">
              <n-button
                size="tiny"
                tertiary
                type="error"
                :disabled="disableRemove || member.userId === ownerId"
                @click="handleRemove(member)"
              >
                移除
              </n-button>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-if="members.length === 0" class="text-center text-gray-400 py-6">暂无成员</div>
    </template>
  </n-card>
</template>
