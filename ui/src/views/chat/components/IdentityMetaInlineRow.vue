<script setup lang="ts">
import { onMounted, onUpdated, ref } from 'vue'
import CharacterCardBadge from './CharacterCardBadge.vue'
import CharacterRemark from './CharacterRemark.vue'
import { resolveIdentityMetaHostBackground } from '@/utils/identityMetaContrast'

const props = defineProps<{
  identityId?: string
  identityColor?: string
  channelId?: string
}>()

const rowRef = ref<HTMLElement | null>(null)
const hostBackgroundColor = ref('')

const syncHostBackgroundColor = () => {
  hostBackgroundColor.value = resolveIdentityMetaHostBackground(rowRef.value)
}

onMounted(syncHostBackgroundColor)
onUpdated(syncHostBackgroundColor)
</script>

<template>
  <span v-if="props.identityId" ref="rowRef" class="identity-meta-inline-row">
    <CharacterCardBadge
      :identity-id="props.identityId"
      :identity-color="props.identityColor"
      :host-background-color="hostBackgroundColor"
    />
    <CharacterRemark
      :identity-id="props.identityId"
      :identity-color="props.identityColor"
      :channel-id="props.channelId"
      :host-background-color="hostBackgroundColor"
    />
  </span>
</template>

<style scoped>
.identity-meta-inline-row {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  vertical-align: middle;
}
</style>
