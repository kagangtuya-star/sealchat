<script setup lang="ts">
import { computed } from 'vue'
import type { StickyNote, RoundCounterTypeData } from '@/stores/stickyNote'
import { useStickyNoteStore } from '@/stores/stickyNote'

const props = defineProps<{
  note: StickyNote
  isEditing: boolean
}>()

const stickyNoteStore = useStickyNoteStore()

const typeData = computed<RoundCounterTypeData>(() => {
  const parsed = stickyNoteStore.parseTypeData<RoundCounterTypeData>(props.note)
  return parsed || { round: 1, direction: 'up' }
})

const displayRound = computed(() => {
  const { round, limit } = typeData.value
  if (limit !== undefined && limit > 0) {
    return `${round}/${limit}`
  }
  return String(round)
})

function changeRound(delta: number) {
  const { round, direction, limit } = typeData.value
  let newRound = round + delta
  if (limit !== undefined && limit > 0) {
    newRound = Math.max(0, Math.min(limit, newRound))
  } else {
    newRound = Math.max(0, newRound)
  }
  stickyNoteStore.updateTypeData(props.note.id, { ...typeData.value, round: newRound })
}

function nextRound() {
  const delta = typeData.value.direction === 'up' ? 1 : -1
  changeRound(delta)
}

function prevRound() {
  const delta = typeData.value.direction === 'up' ? -1 : 1
  changeRound(delta)
}

function resetRound() {
  const startValue = typeData.value.direction === 'up' ? 1 : (typeData.value.limit || 1)
  stickyNoteStore.updateTypeData(props.note.id, { ...typeData.value, round: startValue })
}

function toggleDirection() {
  const newDir = typeData.value.direction === 'up' ? 'down' : 'up'
  stickyNoteStore.updateTypeData(props.note.id, { ...typeData.value, direction: newDir })
}

function setLimit(e: Event) {
  const target = e.target as HTMLInputElement
  const val = parseInt(target.value, 10)
  if (!isNaN(val) && val > 0) {
    stickyNoteStore.updateTypeData(props.note.id, { ...typeData.value, limit: val })
  } else if (target.value === '') {
    const { limit, ...rest } = typeData.value
    stickyNoteStore.updateTypeData(props.note.id, rest)
  }
}
</script>

<template>
  <div class="sticky-note-round">
    <div class="sticky-note-round__label">回合</div>

    <div class="sticky-note-round__display">
      <button class="sticky-note-round__nav" @click="prevRound">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
          <path d="M15.41 7.41L14 6l-6 6 6 6 1.41-1.41L10.83 12z"/>
        </svg>
      </button>

      <div class="sticky-note-round__value">{{ displayRound }}</div>

      <button class="sticky-note-round__nav" @click="nextRound">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
          <path d="M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6z"/>
        </svg>
      </button>
    </div>

    <div class="sticky-note-round__controls">
      <button class="sticky-note-round__btn" @click="resetRound">重置</button>
      <button class="sticky-note-round__btn" @click="toggleDirection">
        {{ typeData.direction === 'up' ? '↑ 递增' : '↓ 递减' }}
      </button>
    </div>

    <div class="sticky-note-round__limit">
      <label>上限：</label>
      <input
        type="number"
        :value="typeData.limit || ''"
        placeholder="无"
        min="1"
        class="sticky-note-round__limit-input"
        @change="setLimit"
      />
    </div>
  </div>
</template>

<style scoped>
.sticky-note-round {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 16px;
  gap: 12px;
}

.sticky-note-round__label {
  font-size: 12px;
  color: rgba(0, 0, 0, 0.5);
  text-transform: uppercase;
  letter-spacing: 1px;
}

.sticky-note-round__display {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sticky-note-round__nav {
  width: 40px;
  height: 40px;
  border: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 50%;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(0, 0, 0, 0.6);
  transition: background 0.15s;
}

.sticky-note-round__nav:hover {
  background: rgba(0, 0, 0, 0.15);
}

.sticky-note-round__value {
  font-size: 36px;
  font-weight: bold;
  color: rgba(0, 0, 0, 0.85);
  min-width: 80px;
  text-align: center;
}

.sticky-note-round__controls {
  display: flex;
  gap: 8px;
}

.sticky-note-round__btn {
  padding: 6px 12px;
  border: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
  color: rgba(0, 0, 0, 0.7);
}

.sticky-note-round__btn:hover {
  background: rgba(0, 0, 0, 0.15);
}

.sticky-note-round__limit {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: rgba(0, 0, 0, 0.6);
}

.sticky-note-round__limit-input {
  width: 60px;
  padding: 4px 8px;
  border: 1px solid rgba(0, 0, 0, 0.2);
  border-radius: 4px;
  font-size: 12px;
  text-align: center;
}

.sticky-note-round__limit-input:focus {
  outline: none;
  border-color: rgba(0, 0, 0, 0.4);
}
</style>
