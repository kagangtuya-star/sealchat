<script setup lang="ts">
import { defineAsyncComponent } from 'vue'

import type { Dice3DBotRule, Dice3DWorldConfig } from '@/types'
import DiceAttachmentPicker from './DiceAttachmentPicker.vue'
import DiceSurfaceSelector from './DiceSurfaceSelector.vue'
import DiceTextureGrid from './DiceTextureGrid.vue'

const DiceSkinPreview = defineAsyncComponent(() => import('./DiceSkinPreview.vue'))

const props = defineProps<{ config: Dice3DWorldConfig; label: string }>()

const addRule = () => props.config.botRules.push({
  id: `platform-rule-${Date.now()}`, name: '自定义规则', enabled: true,
  pattern: String.raw`(?i)(?P<values>\d+)\[(?P<count>\d*)d(?P<sides>\d+)\]`,
  countGroup: 'count', sidesGroup: 'sides', valuesGroup: 'values',
  valueSeparatorPattern: String.raw`\+`, priority: 0,
})
const updateRuleIDs = (rule: Dice3DBotRule, field: 'channelIds' | 'botUserIds', value: string) => {
  rule[field] = value.split(',').map(item => item.trim()).filter(Boolean)
}
</script>

<template>
  <div class="platform-dice-editor">
    <DiceSkinPreview :skin="config.defaultSkin" :label="label" />
    <n-grid :cols="2" :x-gap="12">
      <n-form-item-gi label="启用 3D 骰子"><n-switch v-model:value="config.enabled" /></n-form-item-gi>
      <n-form-item-gi label="显示区域"><n-select v-model:value="config.surfaceMode" :options="[{ label: '自动', value: 'auto' }, { label: '聊天区域', value: 'chat' }, { label: '小剧场', value: 'theater' }, { label: '全屏', value: 'fullscreen' }, { label: '自定义', value: 'custom' }]" /></n-form-item-gi>
    </n-grid>
    <DiceSurfaceSelector v-if="config.surfaceMode === 'custom'" v-model="config.customSurface" />
    <n-grid :cols="2" :x-gap="12">
      <n-form-item-gi label="骰面底色"><n-color-picker v-model:value="config.defaultSkin.faceBackground" :show-alpha="false" :modes="['hex']" /></n-form-item-gi>
      <n-form-item-gi label="数字颜色"><n-color-picker v-model:value="config.defaultSkin.faceForeground" :show-alpha="false" :modes="['hex']" /></n-form-item-gi>
      <n-form-item-gi label="边缘颜色"><n-color-picker v-model:value="config.defaultSkin.edgeColor" :show-alpha="false" :modes="['hex']" /></n-form-item-gi>
      <n-form-item-gi label="分界线颜色"><n-color-picker v-model:value="config.defaultSkin.outlineColor" :show-alpha="false" :modes="['hex']" /></n-form-item-gi>
      <n-form-item-gi label="骰子大小"><n-slider v-model:value="config.defaultSkin.scale" :min="0.5" :max="2" :step="0.05" /></n-form-item-gi>
    </n-grid>
    <n-form-item label="骰面图集"><DiceTextureGrid v-model="config.defaultSkin" platform /></n-form-item>
    <n-grid :cols="2" :x-gap="12">
      <n-form-item-gi label="运动速度"><n-slider v-model:value="config.motion.speed" :min="0.25" :max="3" :step="0.05" /></n-form-item-gi>
      <n-form-item-gi label="投掷力度"><n-slider v-model:value="config.motion.throwForce" :min="0.25" :max="3" :step="0.05" /></n-form-item-gi>
      <n-form-item-gi label="入场方向"><n-select v-model:value="config.motion.entryEdge" :options="[{ label: '随机', value: 'random' }, { label: '上方', value: 'top' }, { label: '右侧', value: 'right' }, { label: '下方', value: 'bottom' }, { label: '左侧', value: 'left' }]" /></n-form-item-gi>
      <n-form-item-gi label="停留时间"><n-input-number v-model:value="config.motion.lingerMs" :min="500" :max="30000"><template #suffix>ms</template></n-input-number></n-form-item-gi>
      <n-form-item-gi label="最大骰子数"><n-input-number v-model:value="config.motion.maxDice" :min="1" :max="100" /></n-form-item-gi>
      <n-form-item-gi label="结算后可交互"><n-switch v-model:value="config.motion.interactive" /></n-form-item-gi>
    </n-grid>
    <n-grid :cols="2" :x-gap="12">
      <n-form-item-gi label="投掷音效"><n-switch v-model:value="config.audio.enabled" /></n-form-item-gi>
      <n-form-item-gi label="音量"><n-slider v-model:value="config.audio.volume" :min="0" :max="1" :step="0.05" /></n-form-item-gi>
    </n-grid>
    <n-form-item label="自定义音效"><DiceAttachmentPicker v-model="config.audio.soundAssetId" platform accept="audio/*" /></n-form-item>
    <n-collapse>
      <n-collapse-item title="BOT 骰点匹配规则" name="bot-rules">
        <n-card v-for="(rule, index) in config.botRules" :key="rule.id" size="small" :title="rule.name || `规则 ${index + 1}`" class="platform-rule">
          <template #header-extra><n-button text type="error" @click="config.botRules.splice(index, 1)">删除</n-button></template>
          <n-grid :cols="2" :x-gap="10">
            <n-form-item-gi label="名称"><n-input v-model:value="rule.name" /></n-form-item-gi>
            <n-form-item-gi label="优先级"><n-input-number v-model:value="rule.priority" /></n-form-item-gi>
          </n-grid>
          <n-form-item label="启用"><n-switch v-model:value="rule.enabled" /></n-form-item>
          <n-form-item label="频道 ID"><n-input :value="rule.channelIds?.join(', ') || ''" @update:value="updateRuleIDs(rule, 'channelIds', $event)" /></n-form-item>
          <n-form-item label="BOT 用户 ID"><n-input :value="rule.botUserIds?.join(', ') || ''" @update:value="updateRuleIDs(rule, 'botUserIds', $event)" /></n-form-item>
          <n-form-item label="匹配正则"><n-input v-model:value="rule.pattern" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" /></n-form-item>
          <n-grid :cols="2" :x-gap="10">
            <n-form-item-gi label="数量组"><n-input v-model:value="rule.countGroup" /></n-form-item-gi>
            <n-form-item-gi label="骰面组"><n-input v-model:value="rule.sidesGroup" /></n-form-item-gi>
            <n-form-item-gi label="结果组"><n-input v-model:value="rule.valuesGroup" /></n-form-item-gi>
            <n-form-item-gi label="分隔正则"><n-input v-model:value="rule.valueSeparatorPattern" /></n-form-item-gi>
          </n-grid>
        </n-card>
        <n-button dashed block @click="addRule">增加 BOT 匹配规则</n-button>
      </n-collapse-item>
    </n-collapse>
  </div>
</template>

<style scoped>
.platform-dice-editor { display: flex; flex-direction: column; gap: 10px; padding-top: 10px; }.platform-dice-editor :deep(.n-form-item) { margin-bottom: 4px; }.platform-rule { margin-bottom: 10px; }
</style>
