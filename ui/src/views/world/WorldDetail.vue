<script setup lang="ts">
import WorldInviteList from "./WorldInviteList.vue"
import WorldManager from "./WorldManager.vue"
import WorldMemberManager from "./WorldMemberManager.vue"

import { onMounted, ref, computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useChatStore } from '@/stores/chat';
import { useDialog, useMessage } from 'naive-ui';

const chat = useChatStore();
const route = useRoute();
const router = useRouter();
const message = useMessage();
const dialog = useDialog();

const worldId = computed(() => route.params.worldId as string);
const detail = ref<any>(null);
const sections = ref<any>({});
const loading = ref(false);

const load = async () => {
  if (!worldId.value) return;
  loading.value = true;
  try {
    detail.value = await chat.worldDetail(worldId.value);
    sections.value = await chat.loadWorldSections(worldId.value, ['invites']);
  } catch (e) {
    message.error('加载世界失败');
  } finally {
    loading.value = false;
  }
};

onMounted(load);

const enterWorld = async () => {
  await chat.switchWorld(worldId.value, { force: true });
  router.push({ name: 'home' });
};

const joinWorld = async () => {
  await chat.joinWorld(worldId.value);
  message.success('加入成功');
  await load();
};

const goWorldLobby = () => {
  router.push({ name: 'world-lobby' });
};

const worldManagerVisible = ref(false);
const memberManagerVisible = ref(false);

const isMember = computed(() => !!detail.value?.isMember);
const memberRole = computed(() => detail.value?.memberRole || '');
const roleLabel = computed(() => {
  switch (memberRole.value) {
    case 'owner':
      return '拥有者';
    case 'admin':
      return '管理员';
    case 'spectator':
      return '旁观者';
    case 'member':
      return '成员';
    default:
      return '';
  }
});
const canManageWorld = computed(() => memberRole.value === 'owner' || memberRole.value === 'admin');
const canLeaveWorld = computed(() => isMember.value && memberRole.value !== 'owner');
const isSpectator = computed(() => memberRole.value === 'spectator');

const handleLeaveWorld = () => {
  if (!canLeaveWorld.value) {
    if (memberRole.value === 'owner') {
      message.warning('世界拥有者无法退出，请先转移所有权');
    }
    return;
  }
  dialog.warning({
    title: '退出世界',
    content: '确定要退出该世界吗？退出后需要重新邀请才能加入。',
    positiveText: '确认退出',
    negativeText: '取消',
    maskClosable: false,
    onPositiveClick: async () => {
      try {
        await chat.leaveWorld(worldId.value);
        message.success('已退出世界');
        await load();
        router.push({ name: 'world-lobby' });
      } catch (error: any) {
        message.error(error?.response?.data?.message || '退出失败');
      }
    },
  });
};
</script>

<template>
  <div class="p-4 space-y-4" v-if="detail?.world">
    <n-card :title="detail.world.name">
      <div class="flex items-center gap-2">
        <p class="text-gray-600 flex-1">{{ detail.world.description }}</p>
        <n-tag v-if="roleLabel" size="small" type="info">当前身份：{{ roleLabel }}</n-tag>
      </div>
      <div class="mt-3 world-action-grid">
        <div class="world-action-item">
          <n-button block type="primary" @click="enterWorld">进入</n-button>
        </div>
        <div class="world-action-item">
          <n-button block :disabled="!canManageWorld" @click="worldManagerVisible = true">
            世界管理
          </n-button>
        </div>
        <div class="world-action-item">
          <n-button block @click="goWorldLobby">大厅</n-button>
        </div>
        <div class="world-action-item">
          <n-button block :disabled="!canManageWorld" @click="memberManagerVisible = true">
            成员管理
          </n-button>
        </div>
      </div>
      <div class="mt-3 flex flex-wrap gap-2">
        <n-button v-if="!detail.isMember" @click="joinWorld">加入世界</n-button>
        <n-button v-if="canLeaveWorld" type="error" @click="handleLeaveWorld">
          退出世界
        </n-button>
      </div>
      <n-alert v-if="isSpectator" class="mt-3" type="info" show-icon>
        旁观者默认可以查看全部频道，但需要被频道管理员加入成员角色后才能发言。
      </n-alert>
    </n-card>

    <n-card title="邀请链接" class="world-invite-card">
      <WorldInviteList :world-id="worldId" />
    </n-card>
  </div>
  <n-empty v-else description="世界不存在" />
  <WorldManager :world-id="worldId" v-model:visible="worldManagerVisible" />
  <WorldMemberManager :world-id="worldId" v-model:visible="memberManagerVisible" />
</template>

<style scoped>
.world-action-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.world-action-item :deep(.n-button) {
  height: 44px;
}

.world-action-item {
  display: flex;
  align-items: stretch;
}

.world-invite-card {
  --sc-invite-surface: var(--n-card-color, var(--n-color, #f8fafc));
  --sc-invite-border: var(--n-border-color, rgba(148, 163, 184, 0.4));
  --sc-invite-muted: var(--n-text-color-3, #94a3b8);
  --sc-invite-text: var(--n-text-color-1, #0f172a);
}

.world-invite-card :deep(.n-card__content) {
  max-height: min(60vh, 520px);
  overflow-y: auto;
  padding-right: 4px;
  scrollbar-width: thin;
  scrollbar-color: var(--n-border-color) transparent;
}

.world-invite-card :deep(.n-card) {
  background-color: var(--sc-invite-surface);
  border-color: var(--sc-invite-border);
  transition: background-color 0.2s ease, border-color 0.2s ease;
}

.world-invite-card :deep(.n-card__content),
.world-invite-card :deep(.n-card__footer) {
  background-color: var(--sc-invite-surface);
}

.world-invite-card :deep(.n-card__content::-webkit-scrollbar) {
  width: 6px;
}

.world-invite-card :deep(.n-card__content::-webkit-scrollbar-track) {
  background: transparent;
}

.world-invite-card :deep(.n-card__content::-webkit-scrollbar-thumb) {
  background-color: var(--n-border-color);
  border-radius: 3px;
}
</style>
