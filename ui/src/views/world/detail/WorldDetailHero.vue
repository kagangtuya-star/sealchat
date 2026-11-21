<script setup lang="ts">
import { computed } from 'vue';
import { ArrowLeft, DoorEnter, History, Settings, UserPlus, Users } from '@vicons/tabler';
import WorldCoverMedia from '../WorldCoverMedia.vue';

const props = defineProps<{
  world: any;
  roleLabel: string;
  memberRole: string;
  memberCount: number;
  ownerName: string;
  isMember: boolean;
  isSpectator: boolean;
  canManage: boolean;
  entering: boolean;
  joining: boolean;
}>();

const emit = defineEmits<{
  (e: 'back'): void;
  (e: 'enter'): void;
  (e: 'join'): void;
  (e: 'open-settings'): void;
  (e: 'open-members'): void;
  (e: 'open-invites'): void;
  (e: 'switch-variant'): void;
}>();

const VISIBILITY_LABELS: Record<string, string> = {
  public: '公开',
  private: '私有',
  unlisted: '不公开（凭链接访问）',
};

const visibilityLabel = computed(() => VISIBILITY_LABELS[props.world?.visibility] || '公开');
const roleTagType = computed(() => {
  if (props.memberRole === 'owner') return 'warning';
  if (props.memberRole === 'admin') return 'info';
  return 'default';
});
</script>

<template>
  <section class="world-console-panel world-hero">
    <WorldCoverMedia class="world-hero__cover" :attachment="world?.coverAttachmentId" />
    <div class="world-hero__body">
      <div class="world-hero__topbar">
        <n-button quaternary size="small" @click="emit('back')">
          <template #icon>
            <n-icon><ArrowLeft /></n-icon>
          </template>
          世界大厅
        </n-button>
        <n-button quaternary size="small" @click="emit('switch-variant')">
          <template #icon>
            <n-icon><History /></n-icon>
          </template>
          回到旧版
        </n-button>
      </div>

      <div class="world-hero__main">
        <div class="world-hero__info">
          <h1 class="world-hero__title">{{ world.name }}</h1>
          <div class="world-hero__meta">
            <n-tag size="small" :bordered="false" :type="roleLabel ? roleTagType : 'default'">
              {{ roleLabel || '未加入' }}
            </n-tag>
            <n-tag size="small" :bordered="false">{{ visibilityLabel }}</n-tag>
            <span class="world-hero__meta-text">{{ memberCount }} 名成员</span>
            <span v-if="ownerName" class="world-hero__meta-text">拥有者：{{ ownerName }}</span>
          </div>
          <p class="world-hero__desc" :class="{ 'world-hero__desc--empty': !world.description }">
            {{ world.description || '暂无简介' }}
          </p>
        </div>

        <div class="world-hero__actions">
          <n-button type="primary" :loading="entering" @click="emit('enter')">
            <template #icon>
              <n-icon><DoorEnter /></n-icon>
            </template>
            进入世界
          </n-button>
          <n-button v-if="!isMember" secondary type="primary" :loading="joining" @click="emit('join')">
            加入世界
          </n-button>
          <template v-if="canManage">
            <n-button secondary @click="emit('open-settings')">
              <template #icon>
                <n-icon><Settings /></n-icon>
              </template>
              基础设置
            </n-button>
            <n-button secondary @click="emit('open-members')">
              <template #icon>
                <n-icon><Users /></n-icon>
              </template>
              成员管理
            </n-button>
            <n-button secondary @click="emit('open-invites')">
              <template #icon>
                <n-icon><UserPlus /></n-icon>
              </template>
              邀请
            </n-button>
          </template>
        </div>
      </div>

      <n-alert v-if="isSpectator" type="info" :bordered="false" class="world-hero__notice">
        旁观者默认可以查看全部频道，但需要被频道管理员加入成员角色后才能发言。
      </n-alert>
    </div>
    <slot name="nav" />
  </section>
</template>

<style scoped>
.world-hero {
  overflow: hidden;
}

.world-hero__cover {
  aspect-ratio: 4 / 1;
  max-height: 240px;
  width: 100%;
  overflow: hidden;
  background: color-mix(in srgb, var(--sc-text-primary) 6%, transparent);
  border-bottom: 1px solid var(--sc-border-mute);
}

.world-hero__body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 20px 18px;
}

.world-hero__topbar {
  display: flex;
  margin: 0 -8px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.world-hero__main {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px 32px;
}

.world-hero__info {
  flex: 1 1 360px;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.world-hero__title {
  margin: 0;
  color: var(--sc-text-primary);
  font-size: 1.6rem;
  font-weight: 700;
  line-height: 1.25;
  word-break: break-word;
}

.world-hero__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 12px;
}

.world-hero__meta-text {
  color: var(--sc-text-secondary);
  font-size: 13px;
}

.world-hero__desc {
  margin: 0;
  max-width: 72ch;
  color: var(--sc-text-secondary);
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}

.world-hero__desc--empty {
  opacity: 0.7;
}

.world-hero__actions {
  flex: 0 1 auto;
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.world-hero__notice {
  margin-top: 2px;
}

@media (max-width: 640px) {
  .world-hero__cover {
    aspect-ratio: 5 / 2;
  }

  .world-hero__body {
    padding: 12px 14px 14px;
  }

  .world-hero__title {
    font-size: 1.35rem;
  }

  .world-hero__actions {
    justify-content: flex-start;
    width: 100%;
  }

  .world-hero__actions > :first-child {
    flex: 1 1 100%;
  }
}
</style>
