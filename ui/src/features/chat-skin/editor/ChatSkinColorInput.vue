<script setup lang="ts">
import { normalizeChatSkinColor } from '../chatSkinSchema'

// 颜色输入：取色器（支持透明度）+ 清除。空值表示「继承主题」。
const props = withDefaults(defineProps<{
  value: string | undefined
  clearable?: boolean
  placeholder?: string
}>(), {
  clearable: true,
  placeholder: '继承',
})

const emit = defineEmits<{ (e: 'update:value', value: string): void }>()

const update = (value: string | null) => {
  emit('update:value', normalizeChatSkinColor(value || ''))
}
</script>

<template>
  <div class="csk-color">
    <n-color-picker
      class="csk-color__picker"
      size="small"
      :value="props.value || null"
      :modes="['hex']"
      :show-alpha="true"
      :swatches="['#ffffff', '#fdf6e6', '#fde2e4', '#e0f2fe', '#dcfce7', '#ede9fe', '#1f2937', '#0f172a', '#e85d75', '#f5c542', '#6366f1', '#14b8a6']"
      @update:value="update"
    >
      <template #label>
        <span class="csk-color__label">{{ props.value || props.placeholder }}</span>
      </template>
    </n-color-picker>
    <n-button v-if="clearable && props.value" size="tiny" quaternary class="csk-color__clear" @click="update(null)">清除</n-button>
  </div>
</template>

<style scoped>
.csk-color { display: flex; align-items: center; gap: 4px; min-width: 0; }
.csk-color__picker { flex: 1 1 auto; min-width: 0; }
.csk-color__label { font-size: 12px; font-family: ui-monospace, monospace; }
.csk-color__clear { flex: none; }
</style>
