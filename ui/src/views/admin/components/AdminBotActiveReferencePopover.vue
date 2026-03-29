<script setup lang="ts">
interface ActiveReferenceItem {
  kind?: string
  integrationId?: string
  name?: string
  source?: string
  scopeType?: string
  worldId?: string
  worldName?: string
  channelId?: string
  channelName?: string
}

const props = withDefaults(defineProps<{
  count?: number
  references?: ActiveReferenceItem[]
}>(), {
  count: 0,
  references: () => [],
});

const referenceTypeLabel = (item: ActiveReferenceItem) => {
  if ((item.kind || '').trim() === 'channel_webhook') {
    return '频道 Webhook';
  }
  if ((item.scopeType || '').trim() === 'channel') {
    return '频道未读提醒';
  }
  return '世界未读提醒';
};

const worldLabel = (item: ActiveReferenceItem) => item.worldName || item.worldId || '未知世界';
const channelLabel = (item: ActiveReferenceItem) => item.channelName || item.channelId || '未知频道';
</script>

<template>
  <span v-if="count <= 0" class="admin-bot-ref-tag admin-bot-ref-tag--idle">
    可清理
  </span>

  <n-popover
    v-else
    trigger="click"
    placement="bottom-start"
    :show-arrow="true"
    style="max-width: 320px"
  >
    <template #trigger>
      <button type="button" class="admin-bot-ref-tag admin-bot-ref-tag--active">
        active 引用 {{ count }}
      </button>
    </template>

    <div class="admin-bot-ref-popover">
      <div class="admin-bot-ref-popover__title">激活来源</div>
      <div v-if="references.length" class="admin-bot-ref-popover__list">
        <div v-for="item in references" :key="item.integrationId || `${item.kind}-${item.worldId}-${item.channelId}`" class="admin-bot-ref-popover__item">
          <div class="admin-bot-ref-popover__type">{{ referenceTypeLabel(item) }}</div>
          <div class="admin-bot-ref-popover__meta">世界：{{ worldLabel(item) }}</div>
          <div v-if="item.channelId || item.channelName" class="admin-bot-ref-popover__meta">频道：{{ channelLabel(item) }}</div>
        </div>
      </div>
      <div v-else class="admin-bot-ref-popover__empty">
        当前未加载到引用详情，但该 BOT 仍存在 active 引用。
      </div>
    </div>
  </n-popover>
</template>

<style scoped>
.admin-bot-ref-tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 24px;
  padding: 0 8px;
  border-radius: 6px;
  border: 1px solid transparent;
  font-size: 12px;
  line-height: 1;
  white-space: nowrap;
}

.admin-bot-ref-tag--idle {
  color: var(--n-text-color-3);
  background: rgba(148, 163, 184, 0.12);
  border-color: rgba(148, 163, 184, 0.18);
}

.admin-bot-ref-tag--active {
  color: #fca5a5;
  background: rgba(127, 29, 29, 0.08);
  border-color: rgba(239, 68, 68, 0.28);
  cursor: pointer;
  transition: border-color 0.18s ease, background-color 0.18s ease;
}

.admin-bot-ref-tag--active:hover {
  background: rgba(127, 29, 29, 0.14);
  border-color: rgba(248, 113, 113, 0.38);
}

.admin-bot-ref-popover {
  width: min(280px, 72vw);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.admin-bot-ref-popover__title {
  font-size: 12px;
  font-weight: 600;
  color: var(--n-text-color-2);
}

.admin-bot-ref-popover__list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.admin-bot-ref-popover__item {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 8px 10px;
  border-radius: 8px;
  background: rgba(148, 163, 184, 0.08);
}

.admin-bot-ref-popover__type {
  font-size: 12px;
  font-weight: 600;
  color: var(--n-text-color-1);
}

.admin-bot-ref-popover__meta,
.admin-bot-ref-popover__empty {
  font-size: 12px;
  color: var(--n-text-color-3);
  line-height: 1.5;
}
</style>
