<script setup lang="ts">
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'

import type { Dice3DBotRule, Dice3DMemberProfile, Dice3DWorldConfig, DiceVisualPayload } from '@/types'
import { useDisplayStore } from '@/stores/display'
import { useUtilsStore } from '@/stores/utils'
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader'
import { loadDice3DSettings, saveDice3DProfile, saveDice3DWorldSettings } from '../api'
import { downloadDiceSkinTemplate, importDiceSkinPackage } from '../diceSkinTransfer'
import { dice3dRuntime } from '../runtime'
import DiceSurfaceSelector from './DiceSurfaceSelector.vue'
import DiceTextureGrid from './DiceTextureGrid.vue'
import DiceAttachmentPicker from './DiceAttachmentPicker.vue'

const DiceSkinPreview = defineAsyncComponent(() => import('./DiceSkinPreview.vue'))

const props = defineProps<{ show: boolean, worldId: string, canManageWorld?: boolean }>()
const emit = defineEmits<{ (event: 'update:show', value: boolean): void, (event: 'profile-saved', profile: Dice3DMemberProfile): void }>()
const message = useMessage()
const display = useDisplayStore()
const utils = useUtilsStore()
const dice3dLocalEnabled = computed({
  get: () => display.settings.dice3dEnabled !== false,
  set: (value: boolean) => display.updateSettings({ dice3dEnabled: value }),
})
const loading = ref(false)
const saving = ref(false)
const tab = ref<'world' | 'personal'>('personal')
const config = ref<Dice3DWorldConfig | null>(null)
const profile = ref<Dice3DMemberProfile | null>(null)

const ruleTestText = ref('[2d6=2+1]')
const skinPackageInputRef = ref<HTMLInputElement | null>(null)
const importingSkin = ref(false)
const worldPresetId = ref<string | null>(null)
const personalPresetId = ref<string | null>(null)
let loadedPersonalSkin = ''
const activeSkin = computed(() => {
  if (tab.value === 'world') return config.value?.defaultSkin
  return profile.value?.useOverride ? profile.value.skin : config.value?.defaultSkin
})
const platformDiceStyles = computed(() => utils.config?.themeManagement?.platformDice3DStyles || [])
const platformDiceStyleOptions = computed(() => platformDiceStyles.value.map(item => ({ label: item.name, value: item.id })))
const skinSignature = (skin?: Dice3DMemberProfile['skin']) => JSON.stringify(skin || null)
const cloneSettingsValue = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T

const setProfile = (value: Dice3DMemberProfile) => {
  loadedPersonalSkin = skinSignature(value.skin)
  profile.value = cloneSettingsValue(value)
}

const setLoadedSettings = (result: Awaited<ReturnType<typeof loadDice3DSettings>>) => {
  config.value = cloneSettingsValue(result.config)
  const loadedProfile = cloneSettingsValue(result.profile)
  if (result.revision === 0) loadedProfile.skin = cloneSettingsValue(result.config.defaultSkin)
  setProfile(loadedProfile)
}

const enablePersonalSkinOverride = (clearTextures = false) => {
  if (!profile.value) return
  profile.value.useOverride = true
  if (clearTextures && Object.keys(profile.value.skin.textures || {}).length > 0) {
    profile.value.skin.textures = {}
  }
}

watch(() => profile.value?.skin, (skin) => {
  if (profile.value && skinSignature(skin) !== loadedPersonalSkin) enablePersonalSkinOverride()
}, { deep: true })

const applyPlatformPreset = (presetId: string | null) => {
	if (tab.value === 'world') worldPresetId.value = presetId
	else personalPresetId.value = presetId
	const preset = platformDiceStyles.value.find(item => item.id === presetId)
	if (!preset) {
		if (tab.value === 'world' && config.value) config.value.platformStyleId = ''
		return
	}
	if (tab.value === 'world' && config.value) {
		config.value = { ...cloneSettingsValue(preset.config), version: 1, platformStyleId: preset.id }
	}
	if (tab.value === 'personal' && profile.value) {
		profile.value.skin = cloneSettingsValue(preset.config.defaultSkin)
		profile.value.audio = cloneSettingsValue(preset.config.audio)
		enablePersonalSkinOverride()
	}
}

const handleSkinPackage = async (event: Event) => {
	const input = event.target as HTMLInputElement
	const file = input.files?.[0]
	input.value = ''
	if (!file || !activeSkin.value) return
	importingSkin.value = true
	try {
		const result = await importDiceSkinPackage(file, async assetFile => {
			const uploaded = await uploadImageAttachment(assetFile, {
				channelId: 'dice3d-skin', rootId: props.worldId, rootIdType: 'dice3d_skin', confirm: true, skipCompression: true,
			})
			return uploaded.attachmentId
		})
		if (tab.value === 'world' && config.value) config.value.defaultSkin = result.skin
		if (tab.value === 'personal' && profile.value) {
			profile.value.skin = result.skin
			enablePersonalSkinOverride()
		}
		if (tab.value === 'world') worldPresetId.value = null
		else personalPresetId.value = null
		message.success(`已导入骰面合集：${result.name}；保存后生效`)
	} catch (error: any) {
		message.error(error?.message || '导入骰面合集失败')
	} finally {
		importingSkin.value = false
	}
}

const addBotRule = () => {
	if (!config.value) return
	config.value.botRules.push({
		id: `rule-${Date.now()}`,
		name: '自定义规则',
		enabled: true,
		// 默认模板：海豹注解式 2[1d6]；亦可用 (?i)(?:\[|\b)(?P<count>\d*)d(?P<sides>\d+)=(?P<values>\d+(?:\+\d+)*)(?:\]|\b)
		pattern: String.raw`(?i)(?P<values>\d+)\[(?P<count>\d*)d(?P<sides>\d+)\]`,
		countGroup: 'count',
		sidesGroup: 'sides',
		valuesGroup: 'values',
		valueSeparatorPattern: String.raw`\+`,
		priority: 0,
	})
}

const addDockStack = () => {
	if (!profile.value) return
	profile.value.dockStacks ||= []
	if (profile.value.dockStacks.length >= 8) return
	profile.value.dockStacks.push({
		id: `stack-${Date.now()}`,
		label: 'd20',
		expression: '.r1d20',
		color: profile.value.skin.faceBackground || '#f5f6fa',
	})
}

const setPersonalAudioOverride = (enabled: boolean) => {
	if (!profile.value) return
	profile.value.audio = enabled
		? cloneSettingsValue(config.value?.audio || { enabled: true, volume: 0.65 })
		: undefined
}

const testBotRule = (rule: Dice3DBotRule) => {
	try {
		const browserPattern = rule.pattern.replace(/\(\?P</g, '(?<')
		const regex = new RegExp(browserPattern, 'i')
		const match = regex.exec(ruleTestText.value)
		if (!match) return '未匹配'
		const groups = match.groups || {}
			return `${groups[rule.countGroup] || '1'}d${groups[rule.sidesGroup] || '?'} = ${groups[rule.valuesGroup] || '?'}`
	} catch (error) {
		return error instanceof Error ? error.message : '正则错误'
	}
}

const testFullDiceSet = () => {
	if (!config.value || !profile.value || !activeSkin.value) return
	const now = Date.now()
	const payload: DiceVisualPayload = {
		version: 1,
		rollId: `dice3d-preview-${now}-${Math.random().toString(36).slice(2, 8)}`,
		messageId: '',
		channelId: '',
		actorUserId: '',
		seed: now,
		groups: [
			{ type: 'd2', results: [2] },
			{ type: 'd4', results: [3] },
			{ type: 'd6', results: [4] },
			{ type: 'd8', results: [5] },
			{ type: 'd10', results: [7] },
			{ type: 'd12', results: [9] },
			{ type: 'd20', results: [17] },
			{ type: 'd100', results: [73] },
		],
		appearance: cloneSettingsValue(activeSkin.value),
		motion: { ...cloneSettingsValue(config.value.motion), maxDice: Math.max(9, config.value.motion.maxDice) },
		audio: cloneSettingsValue(tab.value === 'personal' && profile.value.audio ? profile.value.audio : config.value.audio),
		surfaceMode: config.value.surfaceMode,
		customSurface: cloneSettingsValue(config.value.customSurface),
		createdAt: now,
	}
	if (!dice3dRuntime.forwardToTheater(payload)) dice3dRuntime.play(payload)
}

const updateRuleIDs = (rule: Dice3DBotRule, field: 'channelIds' | 'botUserIds', raw: string) => {
	rule[field] = raw.split(',').map(value => value.trim()).filter(Boolean)
}

const load = async () => {
  if (!props.worldId) return
  loading.value = true
  try {
    const result = await loadDice3DSettings(props.worldId)
    setLoadedSettings(result)
    worldPresetId.value = result.config.platformStyleId || null
    personalPresetId.value = null
    if (!utils.config) void utils.configGet()
  } catch (error: any) {
    message.error(error?.response?.data?.message || '加载 3D 骰子配置失败')
  } finally {
    loading.value = false
  }
}

watch(() => [props.show, props.worldId] as const, ([show]) => {
	if (show) void load()
})

watch(() => props.canManageWorld, (allowed) => {
	if (!allowed && tab.value === 'world') tab.value = 'personal'
}, { immediate: true })

const save = async () => {
  if (!config.value || !profile.value) return
  saving.value = true
  try {
    if (tab.value === 'world') {
			config.value.version = 1
      config.value = await saveDice3DWorldSettings(props.worldId, config.value)
		  if (config.value.enabled) dice3dRuntime.requestLoad()
      message.success('世界 3D 骰子配置已保存')
    } else {
			profile.value.version = 1
      const result = await saveDice3DProfile(props.worldId, profile.value)
      profile.value = result.profile
      message.success('个人骰子已保存')
    }
    const persisted = await loadDice3DSettings(props.worldId)
    setLoadedSettings(persisted)
    emit('profile-saved', profile.value)
  } catch (error: any) {
    message.error(error?.response?.data?.message || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-drawer :show="show" width="min(720px, 96vw)" placement="right" @update:show="emit('update:show', $event)">
    <n-drawer-content title="3D 骰子" closable>
      <input ref="skinPackageInputRef" type="file" accept=".zip,application/zip" hidden @change="handleSkinPackage">
      <n-spin :show="loading">
        <template v-if="config && profile">
          <DiceSkinPreview v-if="activeSkin" :skin="activeSkin" :label="tab === 'world' ? '世界默认预览' : profile?.useOverride ? '我的骰子预览' : '世界默认预览'" />
          <div class="dice-preview-actions">
            <n-button secondary block @click="testFullDiceSet">测试投掷全部骰型</n-button>
          </div>
          <n-tabs v-model:value="tab" type="segment" class="dice-settings-tabs">
          <n-tab-pane name="personal" tab="我的骰子">
            <n-form label-placement="top" size="small">
              <n-form-item label="启用 3D 骰子">
                <div class="dice-local-enable">
                  <n-switch v-model:value="dice3dLocalEnabled">
                    <template #checked>已启用</template>
                    <template #unchecked>已关闭</template>
                  </n-switch>
                  <span class="dice-local-enable__hint">仅本机生效；关闭后不播放 3D 动画，不影响掷骰结果</span>
                </div>
              </n-form-item>
              <n-form-item label="使用个人骰子皮肤覆盖世界默认">
                <n-switch v-model:value="profile.useOverride" />
              </n-form-item>
				  <n-form-item label="平台骰子样式">
					<div class="dice-style-toolbar">
					  <n-select :value="personalPresetId" :options="platformDiceStyleOptions" clearable placeholder="套用管理员预设" @update:value="applyPlatformPreset" />
					  <n-button secondary :loading="importingSkin" @click="skinPackageInputRef?.click()">上传骰面 ZIP</n-button>
					  <n-button quaternary @click="downloadDiceSkinTemplate">下载模板</n-button>
					</div>
				  </n-form-item>
			  <n-grid :cols="2" :x-gap="12">
                <n-form-item-gi label="骰面底色"><n-color-picker v-model:value="profile.skin.faceBackground" :show-alpha="false" :modes="['hex']" @update:value="enablePersonalSkinOverride(true)" /></n-form-item-gi>
                <n-form-item-gi label="数字颜色"><n-color-picker v-model:value="profile.skin.faceForeground" :show-alpha="false" :modes="['hex']" @update:value="enablePersonalSkinOverride(true)" /></n-form-item-gi>
                <n-form-item-gi label="边缘颜色"><n-color-picker v-model:value="profile.skin.edgeColor" :show-alpha="false" :modes="['hex']" @update:value="enablePersonalSkinOverride(true)" /></n-form-item-gi>
                <n-form-item-gi label="分界线颜色"><n-color-picker v-model:value="profile.skin.outlineColor" :show-alpha="false" :modes="['hex']" @update:value="enablePersonalSkinOverride(true)" /></n-form-item-gi>
                <n-form-item-gi label="骰子大小"><n-slider v-model:value="profile.skin.scale" :min="0.5" :max="2" :step="0.05" @update:value="enablePersonalSkinOverride()" /></n-form-item-gi>
			  </n-grid>
				  <n-form-item label="单独上传骰面图集"><DiceTextureGrid v-model="profile.skin" :world-id="worldId" /></n-form-item>
			  <n-form-item label="覆盖世界投掷音效">
				<n-switch :value="Boolean(profile.audio)" @update:value="setPersonalAudioOverride" />
			  </n-form-item>
			  <template v-if="profile.audio">
				<n-form-item label="个人投掷音效"><n-switch v-model:value="profile.audio.enabled" /></n-form-item>
				<n-form-item label="个人音量"><n-slider v-model:value="profile.audio.volume" :min="0" :max="1" :step="0.05" /></n-form-item>
					<n-form-item label="个人投掷音效文件"><DiceAttachmentPicker v-model="profile.audio.soundAssetId" :world-id="worldId" accept="audio/*" /></n-form-item>
			  </template>
              <n-form-item label="屏幕角骰子堆"><n-switch v-model:value="profile.dockEnabled" /></n-form-item>
              <n-form-item label="默认位置">
                <n-select v-model:value="profile.dockCorner" :options="[
                  { label: '右下', value: 'bottom-right' }, { label: '左下', value: 'bottom-left' },
                  { label: '右上', value: 'top-right' }, { label: '左上', value: 'top-left' }, { label: '自由位置', value: 'free' },
                ]" />
              </n-form-item>
			  <n-card v-for="(stack, index) in profile.dockStacks" :key="stack.id" size="small" :title="`骰子堆 ${index + 1}`" style="margin-bottom: 10px">
				<template #header-extra><n-button text type="error" @click="profile.dockStacks.splice(index, 1)">删除</n-button></template>
				  <n-grid :cols="2" :x-gap="12">
				  <n-form-item-gi label="标签"><n-input v-model:value="stack.label" /></n-form-item-gi>
				  <n-form-item-gi label="表达式"><n-input v-model:value="stack.expression" /></n-form-item-gi>
				  </n-grid>
				<n-form-item label="颜色"><n-color-picker v-model:value="stack.color" :show-alpha="false" :modes="['hex']" /></n-form-item>
			  </n-card>
			  <n-button dashed block :disabled="profile.dockStacks.length >= 8" @click="addDockStack">增加骰子堆</n-button>
            </n-form>
          </n-tab-pane>

			  <n-tab-pane v-if="canManageWorld" name="world" tab="世界默认">
            <n-form label-placement="top" size="small">
              <n-form-item label="启用 3D 骰子"><n-switch v-model:value="config.enabled" /></n-form-item>
				  <n-form-item label="平台骰子样式">
					<div class="dice-style-toolbar">
					  <n-select :value="worldPresetId" :options="platformDiceStyleOptions" clearable placeholder="套用管理员预设" @update:value="applyPlatformPreset" />
					  <n-button secondary :loading="importingSkin" @click="skinPackageInputRef?.click()">上传骰面 ZIP</n-button>
					  <n-button quaternary @click="downloadDiceSkinTemplate">下载模板</n-button>
					</div>
				  </n-form-item>
				  <n-form-item label="显示区域">
                <n-select v-model:value="config.surfaceMode" :options="[
                  { label: '自动：聊天区 / 小剧场左侧', value: 'auto' }, { label: '聊天区域', value: 'chat' },
					  { label: '小剧场区域', value: 'theater' }, { label: '全屏', value: 'fullscreen' },
					  { label: '自定义区域', value: 'custom' },
					]" />
				  </n-form-item>
					  <section v-if="config.surfaceMode === 'custom'" class="dice-surface-panel">
						<DiceSurfaceSelector v-model="config.customSurface" />
						<n-grid :cols="2" :x-gap="12">
					  <n-form-item-gi label="左侧位置"><n-slider v-model:value="config.customSurface.x" :min="0" :max="0.9" :step="0.01" /></n-form-item-gi>
					  <n-form-item-gi label="顶部位置"><n-slider v-model:value="config.customSurface.y" :min="0" :max="0.9" :step="0.01" /></n-form-item-gi>
					  <n-form-item-gi label="宽度"><n-slider v-model:value="config.customSurface.width" :min="0.1" :max="1" :step="0.01" /></n-form-item-gi>
					  <n-form-item-gi label="高度"><n-slider v-model:value="config.customSurface.height" :min="0.1" :max="1" :step="0.01" /></n-form-item-gi>
						</n-grid>
					  </section>
			  <n-grid :cols="2" :x-gap="12">
					<n-form-item-gi label="骰面底色"><n-color-picker v-model:value="config.defaultSkin.faceBackground" :show-alpha="false" :modes="['hex']" /></n-form-item-gi>
					<n-form-item-gi label="数字颜色"><n-color-picker v-model:value="config.defaultSkin.faceForeground" :show-alpha="false" :modes="['hex']" /></n-form-item-gi>
					<n-form-item-gi label="边缘颜色"><n-color-picker v-model:value="config.defaultSkin.edgeColor" :show-alpha="false" :modes="['hex']" /></n-form-item-gi>
					<n-form-item-gi label="分界线颜色"><n-color-picker v-model:value="config.defaultSkin.outlineColor" :show-alpha="false" :modes="['hex']" /></n-form-item-gi>
					<n-form-item-gi label="骰子大小"><n-slider v-model:value="config.defaultSkin.scale" :min="0.5" :max="2" :step="0.05" /></n-form-item-gi>
			  </n-grid>
			  <n-form-item label="单独上传骰面图集"><DiceTextureGrid v-model="config.defaultSkin" :world-id="worldId" /></n-form-item>
              <n-grid :cols="2" :x-gap="12">
                <n-form-item-gi label="运动速度"><n-slider v-model:value="config.motion.speed" :min="0.25" :max="3" :step="0.05" /></n-form-item-gi>
                <n-form-item-gi label="投掷力度"><n-slider v-model:value="config.motion.throwForce" :min="0.25" :max="3" :step="0.05" /></n-form-item-gi>
					<n-form-item-gi label="骰子入场方向"><n-select v-model:value="config.motion.entryEdge" :options="[{ label: '随机', value: 'random' }, { label: '从上方', value: 'top' }, { label: '从右侧', value: 'right' }, { label: '从下方', value: 'bottom' }, { label: '从左侧', value: 'left' }]" /></n-form-item-gi>
                <n-form-item-gi label="停留时间（毫秒）"><n-input-number v-model:value="config.motion.lingerMs" :min="500" :max="30000" /></n-form-item-gi>
                <n-form-item-gi label="最大同时骰子"><n-input-number v-model:value="config.motion.maxDice" :min="1" :max="100" /></n-form-item-gi>
              </n-grid>
              <n-form-item label="允许结算后物理交互"><n-switch v-model:value="config.motion.interactive" /></n-form-item>
			  <n-form-item label="投掷音效"><n-switch v-model:value="config.audio.enabled" /></n-form-item>
			  <n-form-item label="音量"><n-slider v-model:value="config.audio.volume" :min="0" :max="1" :step="0.05" /></n-form-item>
				  <n-form-item label="自定义投掷音效文件"><DiceAttachmentPicker v-model="config.audio.soundAssetId" :world-id="worldId" accept="audio/*" /></n-form-item>
			  <n-divider>BOT 骰点匹配</n-divider>
			  <n-form-item label="规则测试文本"><n-input v-model:value="ruleTestText" /></n-form-item>
			  <n-card v-for="(rule, index) in config.botRules" :key="rule.id" size="small" :title="rule.name || `规则 ${index + 1}`" style="margin-bottom: 12px">
				<template #header-extra><n-button text type="error" @click="config.botRules.splice(index, 1)">删除</n-button></template>
				<n-grid :cols="2" :x-gap="12">
				  <n-form-item-gi label="名称"><n-input v-model:value="rule.name" /></n-form-item-gi>
				  <n-form-item-gi label="优先级"><n-input-number v-model:value="rule.priority" /></n-form-item-gi>
				</n-grid>
				<n-form-item label="启用"><n-switch v-model:value="rule.enabled" /></n-form-item>
				<n-form-item label="频道 ID（逗号分隔）"><n-input :value="rule.channelIds?.join(', ') || ''" @update:value="updateRuleIDs(rule, 'channelIds', $event)" /></n-form-item>
				<n-form-item label="BOT 用户 ID（逗号分隔）"><n-input :value="rule.botUserIds?.join(', ') || ''" @update:value="updateRuleIDs(rule, 'botUserIds', $event)" /></n-form-item>
				<n-form-item label="正则"><n-input v-model:value="rule.pattern" type="textarea" :autosize="{ minRows: 2, maxRows: 6 }" /></n-form-item>
				<n-grid :cols="2" :x-gap="12">
				  <n-form-item-gi label="数量捕获组"><n-input v-model:value="rule.countGroup" /></n-form-item-gi>
				  <n-form-item-gi label="骰面捕获组"><n-input v-model:value="rule.sidesGroup" /></n-form-item-gi>
				  <n-form-item-gi label="结果捕获组"><n-input v-model:value="rule.valuesGroup" /></n-form-item-gi>
				  <n-form-item-gi label="结果分隔正则"><n-input v-model:value="rule.valueSeparatorPattern" /></n-form-item-gi>
				</n-grid>
				<n-alert type="info" :show-icon="false">测试：{{ testBotRule(rule) }}</n-alert>
			  </n-card>
			  <n-button dashed block @click="addBotRule">增加 BOT 匹配规则</n-button>
            </n-form>
          </n-tab-pane>
          </n-tabs>
        </template>
      </n-spin>
      <template #footer><n-button type="primary" :loading="saving" :disabled="loading || !config || !profile" @click="save">保存</n-button></template>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.dice-settings-tabs { margin-top: 14px; }
.dice-preview-actions { margin-top: 8px; }
.dice-local-enable { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; width: 100%; }
.dice-local-enable__hint { color: var(--sc-text-secondary, #71717a); font-size: 12px; line-height: 1.4; }
.dice-style-toolbar { width: 100%; display: grid; grid-template-columns: minmax(180px, 1fr) auto auto; gap: 8px; }
.dice-surface-panel { margin-bottom: 14px; padding: 12px; border: 1px solid var(--sc-border-muted, rgba(148,163,184,.22)); border-radius: 11px; background: color-mix(in srgb, var(--sc-bg-surface, #18181b) 96%, transparent); }
@media (max-width: 560px) { .dice-style-toolbar { grid-template-columns: 1fr 1fr; }.dice-style-toolbar :deep(.n-select) { grid-column: 1 / -1; } }
</style>
