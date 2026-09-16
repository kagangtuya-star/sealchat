<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NButton, NIcon, NInputNumber, NPopover, NSelect, NSwitch, NTooltip, useMessage } from 'naive-ui'
import { Users } from '@vicons/tabler'
import { readDialogueCharacters, type DialogueController, type DialogueControllerPatch, type DialogueCharacterOption } from './theater-dialogue-controller'

const props = defineProps<{ worldId: string; controller: DialogueController; canManage: boolean; save: (patch: DialogueControllerPatch) => Promise<void> }>()
const message = useMessage()
const show = ref(false)
const saving = ref(false)
const loading = ref(false)
const error = ref('')
const keyword = ref('')
const page = ref(0)
const total = ref(0)
const candidates = ref<DialogueCharacterOption[]>([])
const labels = new Map<string, string>()
let generation = 0
let timer: ReturnType<typeof setTimeout> | null = null
const options = computed(() => {
  const items = new Map(candidates.value.map(item => [item.actorKey, { value: item.actorKey, label: `${item.displayName} · ${item.channelName}` }]))
  for (const key of [...props.controller.allowList, ...props.controller.denyList]) {
    if (!items.has(key)) items.set(key, { value: key, label: labels.get(key) || `已保存角色 · ${key}` })
  }
  return [...items.values()]
})
const load = async (next = false) => {
  const epoch = ++generation
  const world = props.worldId
  const nextPage = next ? page.value + 1 : 1
  loading.value = true
  error.value = ''
  try {
    const data = await readDialogueCharacters(world, keyword.value, nextPage)
    if (generation !== epoch || world !== props.worldId) return
    candidates.value = next ? [...candidates.value, ...data.items] : data.items
    for (const item of data.items) labels.set(item.actorKey, `${item.displayName} · ${item.channelName}`)
    page.value = data.page
    total.value = data.total
  } catch { if (generation === epoch) error.value = '角色候选加载失败，请重试；已保存名单保持不变。' }
  finally { if (generation === epoch) loading.value = false }
}
const search = (value: string) => {
  keyword.value = value
  page.value = 0
  total.value = 0
  candidates.value = []
  loading.value = true
  generation++
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => { void load() }, 250)
}
const save = async (patch: DialogueControllerPatch) => {
  if (!props.canManage || saving.value) return
  if ((patch.allowList || patch.denyList) && (loading.value || error.value)) return
  const world = props.worldId
  saving.value = true
  try { await props.save(patch) }
  catch (err) { if (world === props.worldId) message.error((err as { response?: { data?: { error?: { message?: string } } } }).response?.data?.error?.message || '保存失败，已保留服务端确认的设置') }
  finally { if (world === props.worldId) saving.value = false }
}
watch(show, value => { if (value) void load() })
watch(() => props.worldId, () => {
  generation++; labels.clear(); candidates.value = []; keyword.value = ''; page.value = 0; total.value = 0; saving.value = false
  if (timer) clearTimeout(timer)
  if (show.value) void load()
})
onBeforeUnmount(() => { generation++; if (timer) clearTimeout(timer) })
const menuProps = { class: 'theater-dialogue-controller-menu' }
</script>

<template>
  <n-popover v-model:show="show" trigger="click" placement="bottom-end" :show-arrow="false" class="theater-dialogue-controller-panel" :style="{ width: 'min(360px, calc(100vw - 32px))' }">
    <template #trigger>
      <n-tooltip><template #trigger><n-button :class="{ 'is-active': controller.enabled }" aria-label="对话框控制器"><template #icon><n-icon><Users /></n-icon></template></n-button></template>对话框控制器</n-tooltip>
    </template>
    <div class="controller-content" @pointerdown.stop @click.stop>
      <strong>对话框控制器</strong>
      <div v-if="!canManage">当前设置只读，由世界管理员修改。</div>
      <label>多人演出开关<n-switch :value="controller.enabled" :disabled="!canManage || saving" @update:value="save({ enabled: $event })" /></label>
      <label>最大立绘显示量<n-input-number :value="controller.maxPortraits" :min="1" :max="12" :precision="0" :disabled="!canManage || saving" @update:value="value => value !== null && save({ maxPortraits: value })" /></label>
      <label>未发言立绘样式</label>
      <n-select :value="controller.inactiveStyle" :disabled="!canManage || saving" :options="[{ label: '低光', value: 'dim' }, { label: '低光灰阶', value: 'dim-grayscale' }, { label: '不处理', value: 'none' }]" :menu-props="menuProps" @update:value="save({ inactiveStyle: $event })" />
      <label>显示白名单</label>
      <n-select multiple filterable remote :value="controller.allowList" :options="options" :loading="loading" :disabled="!canManage || saving" :menu-props="menuProps" @search="search" @update:value="save({ allowList: $event })" />
      <label>显示黑名单</label>
      <n-select multiple filterable remote :value="controller.denyList" :options="options" :loading="loading" :disabled="!canManage || saving" :menu-props="menuProps" @search="search" @update:value="save({ denyList: $event })" />
      <small>黑名单优先；空名单不限制。名单只影响立绘。</small>
      <div v-if="error" role="alert">{{ error }} <n-button text @click="load()">重试</n-button></div>
      <n-button v-else-if="page * 30 < total" size="small" :loading="loading" @click="load(true)">加载更多角色（{{ candidates.length }}/{{ total }}）</n-button>
      <label>立绘拖拽位置调整开关<n-switch :value="controller.dragEnabled" :disabled="!canManage || saving" @update:value="save({ dragEnabled: $event })" /></label>
    </div>
  </n-popover>
</template>

<style scoped>
.controller-content { display: flex; flex-direction: column; gap: 10px; max-height: min(680px, 75vh); overflow-y: auto; }
label { display: flex; align-items: center; justify-content: space-between; gap: 14px; }
label :deep(.n-input-number) { width: 110px; }
small { opacity: .72; }
</style>
<style>
.n-popover.theater-dialogue-controller-panel { backdrop-filter: blur(18px); background: color-mix(in srgb, var(--n-color) 88%, transparent); }
.n-base-select-menu.theater-dialogue-controller-menu { backdrop-filter: blur(18px); background: color-mix(in srgb, var(--n-color) 92%, transparent); }
</style>
