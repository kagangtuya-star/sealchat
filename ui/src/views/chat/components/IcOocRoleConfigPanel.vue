<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'

interface Props {
  show: boolean
  channelId?: string
}

const props = withDefaults(defineProps<Props>(), {
  show: false,
  channelId: undefined,
})

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
}>()

const chat = useChatStore()
const user = useUserStore()

const localChannelId = ref('')
const icRoleId = ref<string | null>(null)
const oocRoleId = ref<string | null>(null)

const identities = computed(() => {
  const id = localChannelId.value ||  props.channelId || chat.curChannel?.id || ''
  if (!id) return []
  return chat.channelIdentities[id] || []
})

const identityOptions = computed(() => {
  return [
    { label: '(不设置)', value: null },
    ...identities.value.map(identity => ({
      label: identity.displayName,
      value: identity.id,
    })),
  ]
})

watch(
  () => props.show,
  (visible) => {
    if (visible) {
      const channelId = props.channelId || chat.curChannel?.id || ''
      localChannelId.value = channelId
      if (channelId) {
        const config = chat.getChannelIcOocRoleConfig(channelId)
        icRoleId.value = config.icRoleId
        oocRoleId.value = config.oocRoleId
      }
    }
  },
)

const handleSave = () => {
  if (!localChannelId.value) {
    emit('update:show', false)
    return
  }
  chat.setChannelIcOocRoleConfig(localChannelId.value, {
    icRoleId: icRoleId.value,
    oocRoleId: oocRoleId.value,
  })
  emit('update:show', false)
}

const handleClose = () => {
  emit('update:show', false)
}
</script>

<template>
  <n-modal
    :show="props.show"
    preset="card"
    title="配置场内场外默认角色"
    :style="{ width: 'min(480px, 90vw)' }"
    @update:show="emit('update:show', $event)"
  >
    <div class="role-config-panel">
      <div class="config-section">
        <div class="config-label">
          <span class="label-title">场内（IC）默认角色</span>
          <span class="label-desc">切换到场内模式时使用</span>
        </div>
        <n-select
          v-model:value="icRoleId"
          :options="identityOptions"
          placeholder="选择默认场内角色"
          :consistent-menu-width="false"
        />
      </div>

      <div class="config-section">
        <div class="config-label">
          <span class="label-title">场外（OOC）默认角色</span>
          <span class="label-desc">切换到场外模式时使用</span>
        </div>
        <n-select
          v-model:value="oocRoleId"
          :options="identityOptions"
          placeholder="选择默认场外角色"
          :consistent-menu-width="false"
        />
      </div>

      <n-space justify="end" style="margin-top: 1.5rem;">
        <n-button @click="handleClose">取消</n-button>
        <n-button type="primary" @click="handleSave">保存配置</n-button>
      </n-space>
    </div>
  </n-modal>
</template>

<style scoped lang="scss">
.role-config-panel {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.config-section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.config-label {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.label-title {
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--sc-text-primary);
}

.label-desc {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}
</style>
