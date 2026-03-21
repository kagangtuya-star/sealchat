<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import type { AnnouncementItem, AnnouncementReminderScope } from '@/models/announcement'
import AnnouncementPopupModal from '@/components/announcement/AnnouncementPopupModal.vue'
import { useAnnouncementStore } from '@/stores/announcement'
import { chatEvent } from '@/stores/chat'
import { useUserStore } from '@/stores/user'

const PUBLIC_ROUTE_NAMES = new Set([
  'user-signin',
  'user-signup',
  'password-recovery',
  'world-private-hint',
  'observer-entry',
])

const route = useRoute()
const announcementStore = useAnnouncementStore()
const user = useUserStore()

const popupVisible = ref(false)
const popupItem = ref<AnnouncementItem | null>(null)
const checking = ref(false)
const queuedCheck = ref(false)

const canCheck = computed(() => {
  const routeName = String(route.name || '').trim()
  return !!user.info.id && !PUBLIC_ROUTE_NAMES.has(routeName)
})

const reminderScopeFilter = computed<AnnouncementReminderScope | undefined>(() => {
  return route.name === 'world-lobby' ? undefined : 'site_wide'
})

const checkPending = async () => {
  if (!canCheck.value || popupVisible.value) {
    return
  }
  if (checking.value) {
    queuedCheck.value = true
    return
  }
  checking.value = true
  try {
    const params = reminderScopeFilter.value ? { reminderScope: reminderScopeFilter.value } : undefined
    const item = await announcementStore.fetchLobbyPending(params)
    if (!item) {
      return
    }
    popupItem.value = item
    popupVisible.value = true
    await announcementStore.markLobbyPopup(item.id)
  } catch (error) {
    console.warn('check global lobby announcement popup failed', error)
  } finally {
    checking.value = false
    if (queuedCheck.value) {
      queuedCheck.value = false
      void checkPending()
    }
  }
}

const handleConnected = () => {
  void checkPending()
}

const handleLobbyAnnouncementUpdated = () => {
  void checkPending()
}

const handleVisibilityChange = () => {
  if (document.visibilityState === 'visible') {
    void checkPending()
  }
}

watch(canCheck, (value) => {
  if (value) {
    void checkPending()
  } else {
    popupVisible.value = false
    popupItem.value = null
  }
}, { immediate: true })

watch(() => route.fullPath, () => {
  void checkPending()
})

onMounted(() => {
  chatEvent.on('connected', handleConnected)
  chatEvent.on('lobby-announcement-updated', handleLobbyAnnouncementUpdated as any)
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onBeforeUnmount(() => {
  chatEvent.off('connected', handleConnected)
  chatEvent.off('lobby-announcement-updated', handleLobbyAnnouncementUpdated as any)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<template>
  <AnnouncementPopupModal
    v-model:visible="popupVisible"
    :item="popupItem"
  />
</template>
