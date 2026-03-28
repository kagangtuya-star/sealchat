<script setup lang="tsx">
import Avatar from './avatar.vue'
import type { AvatarDecoration } from '@/types'

const props = defineProps<{
  src?: string,
  userId?: string,
  name?: string,
  decoration?: AvatarDecoration | null,
}>()
</script>

<template>
  <span class="flex items-center space-x-2">
    <Avatar :size="24" :src="src"></Avatar>
    <span class=" text-more">{{ name || '未知' }}</span>
    <slot name="suffix"></slot>
  </span>
</template>
