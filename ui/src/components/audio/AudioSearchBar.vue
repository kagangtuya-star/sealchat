<template>
  <div class="audio-search-bar">
    <n-input
      class="audio-search-bar__input"
      :value="modelValue"
      :placeholder="placeholder"
      clearable
      @update:value="handleUpdate"
      @keyup.enter="emitSearch"
    >
      <template #prefix>
        <n-icon size="16">
          <SearchOutline />
        </n-icon>
      </template>
    </n-input>
    <slot />
  </div>
</template>

<script setup lang="ts">
import { SearchOutline } from '@vicons/ionicons5';

const props = defineProps({
  modelValue: {
    type: String,
    default: '',
  },
  placeholder: {
    type: String,
    default: '搜索音频或标签',
  },
});

const emit = defineEmits(['update:modelValue', 'search']);

function handleUpdate(value: string) {
  emit('update:modelValue', value);
  emit('search', value);
}

function emitSearch() {
  emit('search', props.modelValue);
}
</script>

<style scoped lang="scss">
.audio-search-bar {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.audio-search-bar__input {
  flex: 1;
}
</style>
