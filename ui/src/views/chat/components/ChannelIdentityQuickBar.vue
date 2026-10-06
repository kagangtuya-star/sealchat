<script setup lang="ts">
import { computed } from 'vue'
import AvatarVue from '@/components/avatar.vue'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import type { ChannelIdentity } from '@/types'

const props = withDefaults(defineProps<{
  channelId: string
  identities: ChannelIdentity[]
  activeIdentityId?: string | null
  limit?: number
}>(), {
  activeIdentityId: null,
  limit: 5,
})

const emit = defineEmits<{
  (event: 'select', identityId: string): void
}>()

const chat = useChatStore()
const user = useUserStore()

// 当前角色固定在最左侧，其余按最近发言时间降序；无发言记录的角色保持原有稳定顺序
const visibleIdentities = computed(() => {
  const channelId = props.channelId
  const activeId = String(props.activeIdentityId || '').trim()
  const limit = Math.max(0, Math.floor(props.limit || 0))
  if (!channelId || !limit) {
    return [] as ChannelIdentity[]
  }
  return props.identities
    .filter((identity) => identity.id)
    .map((identity, index) => ({
      identity,
      index,
      active: identity.id === activeId,
      lastSpokenAt: chat.getIdentityLastSpokenAt(channelId, identity.id),
    }))
    .sort((a, b) => Number(b.active) - Number(a.active) || (b.lastSpokenAt - a.lastSpokenAt) || (a.index - b.index))
    .slice(0, limit)
    .map((item) => item.identity)
})

const resolveAvatarSrc = (identity: ChannelIdentity) => {
  const variant = chat.getActiveIdentityVariant(props.channelId, identity.id)
  const resolved = resolveAttachmentUrl(variant?.avatarAttachmentId || identity.avatarAttachmentId)
  if (resolved) {
    return resolved
  }
  return identity.isTemporary ? '' : (user.info.avatar || '')
}

const resolveDisplayName = (identity: ChannelIdentity) => (
  chat.getActiveIdentityVariant(props.channelId, identity.id)?.displayName || identity.displayName
)
</script>

<template>
  <div
    v-if="visibleIdentities.length"
    class="identity-quick-bar"
    role="toolbar"
    aria-label="快速角色栏"
  >
    <button
      v-for="identity in visibleIdentities"
      :key="identity.id"
      type="button"
      class="identity-quick-bar__item"
      :class="{ 'identity-quick-bar__item--active': identity.id === props.activeIdentityId }"
      :style="identity.color ? { '--identity-quick-bar-color': identity.color } : undefined"
      :title="resolveDisplayName(identity)"
      :aria-label="`切换到 ${resolveDisplayName(identity)}`"
      @click="emit('select', identity.id)"
    >
      <AvatarVue
        :size="28"
        :border="false"
        :src="resolveAvatarSrc(identity)"
        :use-text-fallback="Boolean(identity.isTemporary)"
        :fallback-text="resolveDisplayName(identity)"
        class="identity-quick-bar__avatar"
      />
    </button>
  </div>
</template>

<style scoped>
.identity-quick-bar {
  display: inline-flex;
  align-items: center;
  gap: 0;
  min-width: 0;
  max-width: min(72vw, 240px);
  flex: 0 0 auto;
  flex-wrap: nowrap;
  overflow: visible;
  padding: 0 2px;
}

.identity-quick-bar__item {
  appearance: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  padding: 1px;
  border: 1px solid transparent;
  border-radius: 9999px;
  background: transparent;
  cursor: pointer;
  transition: transform 0.15s ease, border-color 0.15s ease;
  position: relative;
}

.identity-quick-bar__item + .identity-quick-bar__item {
  margin-left: -5px;
}

.identity-quick-bar__item--active {
  transform: translateY(-3px);
  border-color: var(--identity-quick-bar-color, var(--sc-border-strong, currentColor));
  z-index: 2;
}

.identity-quick-bar__item--active::after {
  content: '';
  position: absolute;
  left: 5px;
  right: 5px;
  bottom: -4px;
  height: 2px;
  border-radius: 999px;
  background: var(--identity-quick-bar-color, var(--sc-border-strong, currentColor));
}

.identity-quick-bar__item:hover,
.identity-quick-bar__item:focus-visible {
  transform: translateY(-1px);
  border-color: var(--identity-quick-bar-color, var(--sc-border-strong, currentColor));
}

.identity-quick-bar__item--active:hover,
.identity-quick-bar__item--active:focus-visible {
  transform: translateY(-3px);
}

.identity-quick-bar__item:focus-visible {
  outline: none;
}

.identity-quick-bar__avatar {
  border-radius: 9999px;
  overflow: hidden;
}
</style>
