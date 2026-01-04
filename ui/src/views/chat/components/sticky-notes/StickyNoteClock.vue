<script setup lang="ts">
import { computed } from 'vue'
import type { StickyNote, ClockTypeData } from '@/stores/stickyNote'
import { useStickyNoteStore } from '@/stores/stickyNote'

const props = defineProps<{
  note: StickyNote
  isEditing: boolean
}>()

const stickyNoteStore = useStickyNoteStore()

const typeData = computed<ClockTypeData>(() => {
  const parsed = stickyNoteStore.parseTypeData<ClockTypeData>(props.note)
  return parsed || { segments: 4, filled: 0 }
})

const segments = computed(() => {
  const { segments: total, filled } = typeData.value
  return Array.from({ length: total }, (_, i) => i < filled)
})

function toggleSegment(index: number) {
  const { segments: total, filled } = typeData.value
  const newFilled = index < filled ? index : index + 1
  stickyNoteStore.updateTypeData(props.note.id, { segments: total, filled: newFilled })
}

function setSegments(count: number) {
  const newCount = Math.max(2, Math.min(12, count))
  stickyNoteStore.updateTypeData(props.note.id, {
    segments: newCount,
    filled: Math.min(typeData.value.filled, newCount)
  })
}

function fillAll() {
  stickyNoteStore.updateTypeData(props.note.id, {
    ...typeData.value,
    filled: typeData.value.segments
  })
}

function clearAll() {
  stickyNoteStore.updateTypeData(props.note.id, {
    ...typeData.value,
    filled: 0
  })
}

function getSegmentStyle(index: number, total: number) {
  const angle = 360 / total
  const startAngle = -90 + index * angle
  const endAngle = startAngle + angle
  return {
    '--start-angle': `${startAngle}deg`,
    '--end-angle': `${endAngle}deg`,
    '--segment-angle': `${angle}deg`
  }
}

function getSegmentPath(index: number, total: number): string {
  const angle = 360 / total
  const startAngle = -90 + index * angle
  const endAngle = startAngle + angle
  const startRad = (startAngle * Math.PI) / 180
  const endRad = (endAngle * Math.PI) / 180
  const outerR = 45
  const innerR = 18
  const cx = 50
  const cy = 50

  const x1 = cx + outerR * Math.cos(startRad)
  const y1 = cy + outerR * Math.sin(startRad)
  const x2 = cx + outerR * Math.cos(endRad)
  const y2 = cy + outerR * Math.sin(endRad)
  const x3 = cx + innerR * Math.cos(endRad)
  const y3 = cy + innerR * Math.sin(endRad)
  const x4 = cx + innerR * Math.cos(startRad)
  const y4 = cy + innerR * Math.sin(startRad)

  const largeArc = angle > 180 ? 1 : 0

  return `M ${x1} ${y1} A ${outerR} ${outerR} 0 ${largeArc} 1 ${x2} ${y2} L ${x3} ${y3} A ${innerR} ${innerR} 0 ${largeArc} 0 ${x4} ${y4} Z`
}
</script>

<template>
  <div class="sticky-note-clock">
    <div class="sticky-note-clock__circle">
      <svg viewBox="0 0 100 100" class="sticky-note-clock__svg">
        <circle
          cx="50"
          cy="50"
          r="45"
          fill="none"
          stroke="rgba(0, 0, 0, 0.1)"
          stroke-width="2"
        />
        <g v-for="(filled, index) in segments" :key="index">
          <path
            :d="getSegmentPath(index, typeData.segments)"
            :class="[
              'sticky-note-clock__segment',
              { 'sticky-note-clock__segment--filled': filled }
            ]"
            @click="toggleSegment(index)"
          />
        </g>
        <circle
          cx="50"
          cy="50"
          r="15"
          fill="rgba(255, 255, 255, 0.8)"
          stroke="rgba(0, 0, 0, 0.2)"
          stroke-width="1"
        />
        <text
          x="50"
          y="50"
          text-anchor="middle"
          dominant-baseline="central"
          class="sticky-note-clock__count"
        >
          {{ typeData.filled }}/{{ typeData.segments }}
        </text>
      </svg>
    </div>

    <div class="sticky-note-clock__controls">
      <button class="sticky-note-clock__btn" @click="clearAll">清空</button>
      <div class="sticky-note-clock__segment-control">
        <button class="sticky-note-clock__adj" @click="setSegments(typeData.segments - 1)">-</button>
        <span class="sticky-note-clock__segment-label">{{ typeData.segments }}格</span>
        <button class="sticky-note-clock__adj" @click="setSegments(typeData.segments + 1)">+</button>
      </div>
      <button class="sticky-note-clock__btn" @click="fillAll">填满</button>
    </div>
  </div>
</template>

<style scoped>
.sticky-note-clock {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: 16px;
  gap: 12px;
}

.sticky-note-clock__circle {
  width: 120px;
  height: 120px;
}

.sticky-note-clock__svg {
  width: 100%;
  height: 100%;
}

.sticky-note-clock__segment {
  fill: rgba(0, 0, 0, 0.08);
  stroke: rgba(0, 0, 0, 0.2);
  stroke-width: 1;
  cursor: pointer;
  transition: fill 0.15s;
}

.sticky-note-clock__segment:hover {
  fill: rgba(0, 0, 0, 0.15);
}

.sticky-note-clock__segment--filled {
  fill: rgba(0, 0, 0, 0.5);
}

.sticky-note-clock__segment--filled:hover {
  fill: rgba(0, 0, 0, 0.4);
}

.sticky-note-clock__count {
  font-size: 10px;
  fill: rgba(0, 0, 0, 0.7);
  font-weight: bold;
}

.sticky-note-clock__controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sticky-note-clock__btn {
  padding: 4px 10px;
  border: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
  color: rgba(0, 0, 0, 0.7);
}

.sticky-note-clock__btn:hover {
  background: rgba(0, 0, 0, 0.15);
}

.sticky-note-clock__segment-control {
  display: flex;
  align-items: center;
  gap: 4px;
}

.sticky-note-clock__adj {
  width: 24px;
  height: 24px;
  border: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
  color: rgba(0, 0, 0, 0.7);
}

.sticky-note-clock__adj:hover {
  background: rgba(0, 0, 0, 0.15);
}

.sticky-note-clock__segment-label {
  font-size: 12px;
  color: rgba(0, 0, 0, 0.6);
  min-width: 30px;
  text-align: center;
}
</style>
