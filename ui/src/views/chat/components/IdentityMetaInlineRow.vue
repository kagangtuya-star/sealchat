<script setup lang="ts">
import CharacterCardBadge from './CharacterCardBadge.vue'
import CharacterRemark from './CharacterRemark.vue'

defineProps<{
  identityId?: string
  identityColor?: string
  channelId?: string
}>()
</script>

<template>
  <span v-if="identityId" class="identity-meta-inline-row">
    <CharacterCardBadge :identity-id="identityId" :identity-color="identityColor" />
    <CharacterRemark :identity-id="identityId" :identity-color="identityColor" :channel-id="channelId" />
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
