<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { Message2 as Message2Icon } from '@vicons/tabler'
import { useChatStore } from '@/stores/chat'
import { useCharacterRemarkStore } from '@/stores/characterRemark'
import { useDisplayStore } from '@/stores/display'

interface Props {
  show: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
}>()

const chat = useChatStore()
const display = useDisplayStore()
const remarkStore = useCharacterRemarkStore()
const message = useMessage()

const draft = ref('')

const currentChannelId = computed(() => chat.curChannel?.id || '')
const activeIdentityId = computed(() => {
  const channelId = currentChannelId.value
  if (!channelId) return ''
  return chat.getActiveIdentityId(channelId) || ''
})
const activeIdentity = computed(() => {
  const channelId = currentChannelId.value
  const identityId = activeIdentityId.value
  if (!channelId || !identityId) return null
  return (chat.channelIdentities[channelId] || []).find((identity) => identity.id === identityId) || null
})
const currentRemark = computed(() => (
  activeIdentityId.value
    ? (remarkStore.getRemarkByIdentity(activeIdentityId.value, currentChannelId.value)?.content || '')
    : ''
))
const canEdit = computed(() => (
  !!currentChannelId.value
  && !!activeIdentityId.value
  && remarkStore.isOwnedByCurrentUser(currentChannelId.value, activeIdentityId.value)
))

const saveRemark = async () => {
  if (!currentChannelId.value || !activeIdentityId.value) {
    message.warning('当前频道没有可编辑的角色身份')
    return
  }
  const result = await remarkStore.saveRemark(currentChannelId.value, activeIdentityId.value, draft.value)
  if (!result.ok) {
    message.error(result.error)
    return
  }
  message.success(draft.value.trim() ? '角色备注已保存' : '角色备注已清空')
}

watch(
  () => [props.show, currentChannelId.value, activeIdentityId.value, currentRemark.value] as const,
  ([visible]) => {
    if (!visible) return
    draft.value = currentRemark.value
  },
  { immediate: true },
)
</script>

<template>
  <n-modal
    preset="card"
    :show="props.show"
    title="角色备注"
    class="remark-manager"
    :style="{ width: '520px' }"
    @update:show="emit('update:show', $event)"
  >
    <section class="remark-manager__section">
      <header>
        <div class="section-title">
          <n-icon :component="Message2Icon" size="16" />
          <span>显示开关</span>
        </div>
        <p class="section-desc">默认会同时显示自己的备注与他人备注</p>
      </header>
      <div class="remark-manager__switches">
        <div class="remark-manager__switch-row">
          <span>显示自己的备注</span>
          <n-switch
            :value="display.settings.showOwnIdentityRemark"
            @update:value="display.updateSettings({ showOwnIdentityRemark: $event })"
          />
        </div>
        <div class="remark-manager__switch-row">
          <span>显示他人的备注</span>
          <n-switch
            :value="display.settings.showOthersIdentityRemark"
            @update:value="display.updateSettings({ showOthersIdentityRemark: $event })"
          />
        </div>
      </div>
    </section>

    <section class="remark-manager__section">
      <header>
        <div class="section-title">
          <span>当前角色备注</span>
          <n-tag v-if="activeIdentity" size="small" type="info">{{ activeIdentity.displayName }}</n-tag>
        </div>
        <p class="section-desc">默认值为空；为空时聊天消息头不显示备注。已有备注也可在消息头双击直接编辑。</p>
      </header>
      <n-input
        v-model:value="draft"
        type="text"
        clearable
        :maxlength="remarkStore.maxLength"
        :disabled="!canEdit"
        placeholder="留空表示不显示角色备注"
        @keyup.enter="saveRemark"
      />
      <div class="remark-manager__actions">
        <n-button secondary :disabled="!canEdit || draft === currentRemark" @click="saveRemark">
          保存备注
        </n-button>
        <n-button tertiary :disabled="!canEdit || !draft" @click="draft = ''">
          清空输入
        </n-button>
      </div>
      <p v-if="!canEdit" class="remark-manager__hint">
        当前没有可编辑的激活身份。请先切换到自己的频道角色。
      </p>
    </section>
  </n-modal>
</template>

<style scoped>
.remark-manager__section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.remark-manager__section + .remark-manager__section {
  margin-top: 20px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.section-desc {
  margin-top: 4px;
  font-size: 13px;
  color: rgba(100, 116, 139, 0.92);
}

.remark-manager__switches {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.remark-manager__switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.remark-manager__actions {
  display: flex;
  gap: 8px;
}

.remark-manager__hint {
  font-size: 12px;
  color: rgba(148, 163, 184, 0.96);
}
</style>
