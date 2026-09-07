<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NCheckbox, NEmpty, NIcon, NInput, NPopover, NSelect, NSpin, useMessage } from 'naive-ui'
import { DeviceFloppy, Search, X } from '@vicons/tabler'
import Avatar from '@/components/avatar.vue'
import RichTextEditor from '@/components/rich-text/RichTextEditor.vue'
import { api } from '@/stores/_config'
import { useWorldClueStore, type WorldClueDetail, type WorldClueRosterMember } from '@/stores/worldClue'

type Override = 'inherit' | 'none' | 'view' | 'edit'
type Effective = 'none' | 'view' | 'edit'
interface AccessItem {
  userId: string
  role: string
  accessOverride: Override
  effectiveAccess: Effective
  hasPrivateContent: boolean
  privateRevision: number
  privateExcerpt?: string
}

const props = defineProps<{
  worldId: string
  clue?: WorldClueDetail | null
  defaultAccess: 'none' | 'view'
  publishing: boolean
  lockSessionId: string
}>()
const emit = defineEmits<{
  (event: 'update:defaultAccess', value: 'none' | 'view'): void
  (event: 'publish'): void
  (event: 'unpublish'): void
  (event: 'private-preview', payload: {
    userId: string
    name: string
    format: 'plain' | 'tiptap'
    content: string
  } | null): void
}>()
const store = useWorldClueStore()
const message = useMessage()
const loading = ref(false)
const access = ref<Record<string, AccessItem>>({})
const search = ref('')
const filter = ref<'all' | 'visible' | 'hidden' | 'private'>('all')
const selected = ref(new Set<string>())
const lastSelectedIndex = ref(-1)
const submitting = ref(new Set<string>())
const batchSubmitting = ref(false)
const previewOverride = ref<Record<string, Override>>({})
const sheetMember = ref<WorldClueRosterMember | null>(null)
const privateLoading = ref(false)
const privateSaving = ref(false)
const privateFormat = ref<'plain' | 'tiptap'>('plain')
const privateContent = ref('')
const privateRevision = ref(0)
const privateInitial = ref('')
const privateSnapshot = computed(() => JSON.stringify([privateFormat.value, privateContent.value]))
const privateDirty = computed(() => privateSnapshot.value !== privateInitial.value)
let privateTimer: ReturnType<typeof setTimeout> | null = null
let privateFlight: Promise<boolean> | null = null
let privateFailed = false
let disposed = false
const switchingPrivate = ref(false)
const privateOwnsLock = ref(false)
const privateAcquiringLock = ref(false)
const privateReleasePending = ref(false)
const privateLockClock = ref(Date.now())
let privateLockRenewTimer: ReturnType<typeof setInterval> | null = null
const privateEditorRef = ref<InstanceType<typeof RichTextEditor> | null>(null)
const pointerCleanups = new Set<() => void>()
onMounted(() => document.addEventListener('visibilitychange', handleVisibilityChange))
function clearPrivateTimer() {
  if (privateTimer) clearTimeout(privateTimer)
  privateTimer = null
}
function privateFieldKey(userId = sheetMember.value?.userId) {
  return userId ? `private:${userId}` : ''
}
function getPrivateLock() {
  privateLockClock.value
  const clueId = props.clue?.id
  const field = privateFieldKey()
  if (!clueId || !field) return undefined
  return (store.editLocksByClue[`${props.worldId}:${clueId}`] || []).find(lock => lock.field === field && lock.expireAt > Date.now())
}
function privateLockOwnerName() {
  const lock = getPrivateLock()
  return lock?.user?.nick || lock?.user?.name || lock?.userId || '其他用户'
}
function ownsPrivateLock() {
  return privateOwnsLock.value && getPrivateLock()?.sessionId === props.lockSessionId
}
function clearPrivateLockRenewTimer() {
  if (privateLockRenewTimer) clearInterval(privateLockRenewTimer)
  privateLockRenewTimer = null
}
function ensurePrivateLockRenewTimer() {
  if (privateLockRenewTimer) return
  privateLockRenewTimer = setInterval(async () => {
    privateLockClock.value = Date.now()
    const worldId = props.worldId
    const clueId = props.clue?.id
    const memberId = sheetMember.value?.userId
    const sessionId = props.lockSessionId
    if (!clueId || !memberId) {
      clearPrivateLockRenewTimer()
      return
    }
    if (!privateOwnsLock.value) return
    const result = await store.acquireEditLock(worldId, clueId, `private:${memberId}`, sessionId).catch(() => null)
    if (disposed || props.worldId !== worldId || props.clue?.id !== clueId || sheetMember.value?.userId !== memberId || props.lockSessionId !== sessionId) return
    if (result && !result.ok && result.conflict) {
      privateOwnsLock.value = false
      message.warning(`${privateLockOwnerName()} 正在编辑此成员的专属信息`)
    }
  }, 4000)
}
async function beginPrivateEdit() {
  if (!props.clue?.id || !sheetMember.value) return true
  if (privateAcquiringLock.value) return false
  privateReleasePending.value = false
  if (ownsPrivateLock()) return true
  const worldId = props.worldId
  const clueId = props.clue.id
  const memberId = sheetMember.value.userId
  const field = `private:${memberId}`
  const sessionId = props.lockSessionId
  privateAcquiringLock.value = true
  let acquired = false
  try {
    const result = await store.acquireEditLock(worldId, clueId, field, sessionId)
    if (!result.ok) return false
    if (disposed || props.worldId !== worldId || props.clue?.id !== clueId || sheetMember.value?.userId !== memberId || props.lockSessionId !== sessionId) {
      await store.releaseEditLock(worldId, clueId, field, sessionId).catch(() => false)
      return false
    }
    acquired = true
    privateOwnsLock.value = true
    ensurePrivateLockRenewTimer()
    return true
  } catch (error: any) {
    message.error(error?.response?.data?.message || '获取编辑权失败')
    return false
  } finally {
    privateAcquiringLock.value = false
    if (acquired && privateReleasePending.value) {
      privateReleasePending.value = false
      void flushPrivate(false).finally(() => { void releasePrivateLock() })
    } else if (!acquired) {
      privateReleasePending.value = false
    }
  }
}
async function releasePrivateLock() {
  const worldId = props.worldId
  const clueId = props.clue?.id
  const field = privateFieldKey()
  const sessionId = props.lockSessionId
  const owned = privateOwnsLock.value
  privateOwnsLock.value = false
  if (!clueId || !field || !owned) {
    if (!sheetMember.value || disposed) clearPrivateLockRenewTimer()
    return
  }
  await store.releaseEditLock(worldId, clueId, field, sessionId).catch(() => false)
  if (!sheetMember.value || disposed) clearPrivateLockRenewTimer()
}
watch(privateSnapshot, () => {
  clearPrivateTimer()
  if (!sheetMember.value || privateLoading.value || disposed) return
  emitPrivatePreview(sheetMember.value)
  if (privateFailed) return
  if (privateDirty.value) privateTimer = setTimeout(() => { void flushPrivate(false) }, 700)
}, { flush: 'sync' })
const stops: { value: Override; label: string }[] = [
  { value: 'inherit', label: '继承' }, { value: 'none', label: '不可见' },
  { value: 'view', label: '查看' }, { value: 'edit', label: '编辑' },
]

const defaultDirty = computed(() => !!props.clue && props.defaultAccess !== (props.clue.defaultAccess || 'none'))
function effective(item: AccessItem): Effective {
  if (item.role === 'owner' || item.role === 'admin') return 'edit'
  let value: Effective = item.accessOverride === 'inherit' ? props.defaultAccess : item.accessOverride
  if (item.role === 'spectator' && value === 'edit') value = 'view'
  return value
}
const allAccess = computed(() => Object.values(access.value))
const stats = computed(() => ({
  visible: allAccess.value.filter(item => effective(item) !== 'none').length,
  editable: allAccess.value.filter(item => effective(item) === 'edit').length,
  hidden: allAccess.value.filter(item => effective(item) === 'none').length,
  private: allAccess.value.filter(item => item.hasPrivateContent).length,
}))
const rosterRows = computed(() => store.roster.map(member => ({ member, item: access.value[member.userId] || {
  userId: member.userId, role: member.role, accessOverride: 'inherit' as Override,
  effectiveAccess: member.role === 'owner' || member.role === 'admin' ? 'edit' as Effective : props.defaultAccess,
  hasPrivateContent: false, privateRevision: 0,
} })))
const rosterCounts = computed(() => ({
  all: rosterRows.value.length,
  visible: rosterRows.value.filter(row => effective(row.item) !== 'none').length,
  hidden: rosterRows.value.filter(row => effective(row.item) === 'none').length,
  private: rosterRows.value.filter(row => row.item.hasPrivateContent).length,
}))
const filteredRows = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  return rosterRows.value.filter(({ member, item }) => {
    const matches = !keyword || [member.nickname, member.username, member.userId].some(value => value?.toLowerCase().includes(keyword))
    if (!matches) return false
    if (filter.value === 'visible') return effective(item) !== 'none'
    if (filter.value === 'hidden') return effective(item) === 'none'
    if (filter.value === 'private') return item.hasPrivateContent
    return true
  })
})

async function load() {
  if (!props.clue?.id) return
  const worldId = props.worldId, clueId = props.clue.id
  loading.value = true
  try {
    const [items, response] = await Promise.all([store.loadRoster(worldId), api.get(`api/v1/worlds/${worldId}/clues/${clueId}/access`)])
    if (props.worldId !== worldId || props.clue?.id !== clueId) return
    void items
    access.value = Object.fromEntries(((response.data?.items || []) as AccessItem[]).map(item => [item.userId, item]))
  } catch (error: any) { message.error(error?.response?.data?.message || '分发信息加载失败') }
  finally { if (props.worldId === worldId && props.clue?.id === clueId) loading.value = false }
}
watch(() => [props.worldId, props.clue?.id] as const, () => { selected.value = new Set(); void load() }, { immediate: true })

async function setAccess(userId: string, value: Override) {
  const item = access.value[userId]
  if (!props.clue || !item || submitting.value.has(userId) || item.role === 'owner' || item.role === 'admin' || (item.role === 'spectator' && value === 'edit')) return
  const previous = item
  submitting.value = new Set([...submitting.value, userId])
  access.value[userId] = { ...item, accessOverride: value }
  try {
    const response = await api.patch(`api/v1/worlds/${props.worldId}/clues/${props.clue.id}/access`, { userId, accessOverride: value })
    access.value[userId] = response.data.item as AccessItem
  } catch (error: any) {
    access.value[userId] = previous
    message.error(error?.response?.data?.message || '权限更新失败')
  } finally {
    const next = new Set(submitting.value); next.delete(userId); submitting.value = next
  }
}

function pointerDown(event: PointerEvent, item: AccessItem) {
  if (item.role === 'owner' || item.role === 'admin') return
  const rail = event.currentTarget as HTMLElement
  rail.setPointerCapture(event.pointerId)
  const original = item.accessOverride
  const choose = (clientX: number) => {
    const rect = rail.getBoundingClientRect()
    const max = item.role === 'spectator' ? 2 : 3
    const index = Math.max(0, Math.min(max, Math.round(((clientX - rect.left) / rect.width) * 3)))
    previewOverride.value = { ...previewOverride.value, [item.userId]: stops[index].value }
  }
  choose(event.clientX)
  const move = (next: PointerEvent) => choose(next.clientX)
  const cleanup = () => {
    rail.removeEventListener('pointermove', move); rail.removeEventListener('pointerup', up); rail.removeEventListener('pointercancel', cancel)
    if (rail.hasPointerCapture(event.pointerId)) rail.releasePointerCapture(event.pointerId)
    pointerCleanups.delete(cleanup)
  }
  const finish = (next: PointerEvent, cancelled: boolean) => {
    cleanup()
    const value = previewOverride.value[item.userId] || original
    const previews = { ...previewOverride.value }; delete previews[item.userId]; previewOverride.value = previews
    if (!cancelled && value !== original) void setAccess(item.userId, value)
    if (rail.hasPointerCapture(next.pointerId)) rail.releasePointerCapture(next.pointerId)
  }
  const up = (next: PointerEvent) => finish(next, false)
  const cancel = (next: PointerEvent) => finish(next, true)
  pointerCleanups.add(cleanup)
  rail.addEventListener('pointermove', move); rail.addEventListener('pointerup', up); rail.addEventListener('pointercancel', cancel)
}

function toggleSelection(userId: string, checked: boolean, event?: MouseEvent) {
  const index = filteredRows.value.findIndex(row => row.member.userId === userId)
  const next = new Set(selected.value)
  if (event?.shiftKey && lastSelectedIndex.value >= 0) {
    const [start, end] = [lastSelectedIndex.value, index].sort((a, b) => a - b)
    filteredRows.value.slice(start, end + 1).forEach(row => {
      if (!['owner', 'admin'].includes(row.item.role)) checked ? next.add(row.member.userId) : next.delete(row.member.userId)
    })
  } else checked ? next.add(userId) : next.delete(userId)
  selected.value = next
  lastSelectedIndex.value = index
}
const selectedHasSpectator = computed(() => [...selected.value].some(id => access.value[id]?.role === 'spectator'))
async function batchSet(value: Override) {
  if (batchSubmitting.value || (value === 'edit' && selectedHasSpectator.value)) return
  batchSubmitting.value = true
  const ids = [...selected.value]
  const results = await Promise.allSettled(ids.map(id => setAccess(id, value)))
  if (results.some(result => result.status === 'rejected')) message.error('部分成员权限更新失败')
  batchSubmitting.value = false
}

const nameOf = (member: WorldClueRosterMember) => member.nickname || member.username || member.userId
function emitPrivatePreview(member: WorldClueRosterMember) {
  emit('private-preview', {
    userId: member.userId,
    name: nameOf(member),
    format: privateFormat.value,
    content: privateContent.value,
  })
}
const actualLabel = (value: Effective) => value === 'none' ? '不可见' : value === 'view' ? '查看' : '编辑'
function setFilter(value: string) {
  if (value === 'all' || value === 'visible' || value === 'hidden' || value === 'private') filter.value = value
}
const filterCount = (value: string) => (rosterCounts.value as Record<string, number>)[value] || 0
async function openPrivate(member: WorldClueRosterMember) {
  if (!props.clue || switchingPrivate.value || privateLoading.value || sheetMember.value?.userId === member.userId) return
  switchingPrivate.value = true
  if (!await flushPrivate()) { switchingPrivate.value = false; return }
  await releasePrivateLock()
  if (disposed) return
  sheetMember.value = member; privateLoading.value = true
  ensurePrivateLockRenewTimer()
  privateFailed = false
  const worldId = props.worldId, clueId = props.clue.id, userId = member.userId
  try {
    const response = await api.get(`api/v1/worlds/${worldId}/clues/${clueId}/private/${userId}`)
    if (disposed || sheetMember.value?.userId !== userId || props.worldId !== worldId || props.clue?.id !== clueId) return
    privateFormat.value = response.data.item.privateContentFormat || 'plain'
    privateContent.value = response.data.item.privateContent || ''
    privateInitial.value = privateSnapshot.value
    privateRevision.value = response.data.item.privateRevision || 0
    emitPrivatePreview(member)
  } catch { message.error('成员专属信息加载失败'); sheetMember.value = null }
  finally { privateLoading.value = false; switchingPrivate.value = false }
}
async function flushPrivate(manual = true): Promise<boolean> {
  clearPrivateTimer()
  if (privateFlight) return privateFlight
  if (privateLoading.value) return false
  if (!sheetMember.value) return true
  if (!props.clue || disposed) return false
  if (!privateDirty.value) return true
  if (!await beginPrivateEdit()) return false
  if (!manual && privateFailed) return false
  const userId = sheetMember.value.userId
  const worldId = props.worldId, clueId = props.clue.id
  privateSaving.value = true
  privateFailed = false
  privateFlight = (async () => {
    try {
      while (privateDirty.value && !disposed) {
        const snapshot = privateSnapshot.value
        const response = await api.put(`api/v1/worlds/${worldId}/clues/${clueId}/private/${userId}`, {
          privateContentFormat: privateFormat.value, privateContent: privateContent.value, expectedRevision: privateRevision.value,
        })
        if (disposed || props.worldId !== worldId || props.clue?.id !== clueId || sheetMember.value?.userId !== userId) return false
        privateRevision.value = response.data.item.privateRevision
        privateInitial.value = snapshot
        // A failed metadata refresh must not turn a committed PUT into a failed save.
        const accessResponse = await api.get(`api/v1/worlds/${worldId}/clues/${clueId}/access`).catch(() => null)
        if (disposed || props.worldId !== worldId || props.clue?.id !== clueId) return false
        const refreshed = ((accessResponse?.data?.items || []) as AccessItem[]).find(item => item.userId === userId)
        if (refreshed) access.value[userId] = refreshed
      }
      return !disposed && !privateDirty.value
    } catch (error: any) {
      privateFailed = true
      clearPrivateTimer()
      message.error(error?.response?.data?.message || '保存失败')
      return false
    } finally { privateSaving.value = false; privateFlight = null }
  })()
  return privateFlight
}
async function closePrivate() {
  if (switchingPrivate.value) return
  switchingPrivate.value = true
  try {
    if (await flushPrivate()) {
      await releasePrivateLock()
      sheetMember.value = null
      clearPrivateLockRenewTimer()
    }
  }
  finally { switchingPrivate.value = false }
}
function finishPrivateEdit(editor?: InstanceType<typeof RichTextEditor> | null) {
  const finish = () => {
    if (!privateOwnsLock.value) {
      if (privateAcquiringLock.value) privateReleasePending.value = true
      return
    }
    void flushPrivate(false).finally(() => { void releasePrivateLock() })
  }
  nextTick(() => {
    if (editor?.hasOpenOverlay?.() || editor?.hasRecentOverlayInteraction?.(500)) {
      window.setTimeout(() => {
        if (!editor?.hasOpenOverlay?.() && !editor?.hasRecentOverlayInteraction?.(500)) finish()
      }, 600)
      return
    }
    finish()
  })
}
function gatePrivatePointer(event: PointerEvent) {
  if (ownsPrivateLock()) return
  event.preventDefault(); event.stopPropagation()
  void beginPrivateEdit().then(acquired => { if (acquired) nextTick(() => privateEditorRef.value?.focus()) })
}
function gatePrivateKeydown(event: KeyboardEvent) {
  if (ownsPrivateLock()) return
  event.preventDefault(); event.stopPropagation()
  void beginPrivateEdit().then(acquired => { if (acquired) nextTick(() => privateEditorRef.value?.focus()) })
}
function handleVisibilityChange() {
  if (document.visibilityState === 'hidden' && privateOwnsLock.value) void releasePrivateLock()
}
defineExpose({ flushPrivate, closePrivate })
onBeforeUnmount(() => {
  disposed = true
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  void releasePrivateLock()
  clearPrivateLockRenewTimer()
  clearPrivateTimer()
  pointerCleanups.forEach(cleanup => cleanup())
  previewOverride.value = {}
  sheetMember.value = null
  selected.value = new Set()
})
</script>

<template>
  <section class="distribution">
    <div v-if="!clue" class="distribution__unsaved">
      <div class="distribution__unsaved-card">
        <label><span>默认权限</span><NSelect :value="defaultAccess" :options="[{label:'默认可查看',value:'view'},{label:'默认不可见',value:'none'}]" @update:value="emit('update:defaultAccess', $event)" /></label>
        <NEmpty description="保存线索后，可继续配置成员分发和专属信息。" />
      </div>
    </div>
    <NSpin v-else :show="loading">
      <div class="distribution__top">
        <label><span>默认权限</span><NSelect :value="defaultAccess" :options="[{label:'默认可查看',value:'view'},{label:'默认不可见',value:'none'}]" @update:value="emit('update:defaultAccess', $event)" /></label>
        <div class="distribution__publish">
          <div><strong>{{ clue.status === 'published' ? '已揭示' : '未揭示' }}</strong><small>{{ stats.visible }} 人可见 · {{ stats.editable }} 人可编辑 · {{ stats.hidden }} 人不可见 · {{ stats.private }} 人有专属信息</small></div>
          <div><NButton type="primary" :loading="publishing" :disabled="defaultDirty" @click="emit('publish')">{{ clue.status === 'published' ? '再次揭示' : '揭示' }}</NButton><NButton v-if="clue.status === 'published'" :disabled="publishing" @click="emit('unpublish')">收回</NButton></div>
          <p v-if="defaultDirty">默认权限有未保存更改，请先保存</p>
        </div>
      </div>
      <p class="distribution__hint">默认情况下，揭示会向世界成员公开；如需秘密分发，可将默认权限改为不可见，再为指定成员设置查看或编辑。成员池仅控制此处管理列表；实际可见范围仍由默认权限与成员覆盖权限决定。</p>
      <div class="distribution__tools">
        <NInput v-model:value="search" clearable placeholder="搜索成员"><template #prefix><NIcon><Search /></NIcon></template></NInput>
        <div class="distribution__filters">
          <button v-for="entry in [{key:'all',label:'全部'},{key:'visible',label:'可见'},{key:'hidden',label:'不可见'},{key:'private',label:'专属信息'}]" :key="entry.key" type="button" :class="{active:filter===entry.key}" @click="setFilter(entry.key)">{{ entry.label }} {{ filterCount(entry.key) }}</button>
        </div>
      </div>
      <NEmpty v-if="!store.roster.length" class="distribution__empty" description="还没有分发成员"><template #extra>从线索箱顶部的成员按钮添加成员后，即可在这里配置权限与专属信息。</template></NEmpty>
      <div v-else class="distribution__track">
        <div class="distribution__source" aria-hidden="true"><i />线索</div>
        <div class="distribution__rows">
          <article v-for="({member,item}) in filteredRows" :key="member.userId" class="member-row" :class="{selected:selected.has(member.userId)}">
            <div class="member-row__identity">
              <NCheckbox v-if="!['owner','admin'].includes(item.role)" :checked="selected.has(member.userId)" @click="toggleSelection(member.userId, !selected.has(member.userId), $event)" />
              <span v-else class="member-row__check-placeholder" />
              <Avatar :src="member.avatar" :fallback-text="nameOf(member)" :size="34" :border="false" use-text-fallback />
              <span><b>{{ nameOf(member) }}</b><small>{{ member.role }}</small></span>
            </div>
            <div v-if="['owner','admin'].includes(item.role)" class="member-row__locked">管理员固有权限 · 编辑</div>
            <div v-else class="permission-wrap">
              <div class="permission-rail" role="radiogroup" :aria-label="`${nameOf(member)} 权限`" @pointerdown="pointerDown($event,item)">
                <button v-for="stop in stops" :key="stop.value" type="button" role="radio" :aria-checked="(previewOverride[item.userId] || item.accessOverride) === stop.value" :disabled="item.role === 'spectator' && stop.value === 'edit'" :class="{active:(previewOverride[item.userId] || item.accessOverride) === stop.value}" @click="setAccess(item.userId,stop.value)"><span>{{ stop.label }}</span><i /></button>
              </div>
              <small>实际：{{ actualLabel(effective(item)) }}</small>
            </div>
            <NPopover trigger="hover" placement="top"><template #trigger><NButton quaternary size="small" class="private-button" @click="openPrivate(member)"><i :class="{active:item.hasPrivateContent}" />专属信息</NButton></template>{{ item.hasPrivateContent ? (item.privateExcerpt || '已有专属信息') : '暂无专属信息' }}</NPopover>
          </article>
        </div>
      </div>
      <div v-if="selected.size" class="batch-bar"><strong>已选择 {{ selected.size }} 人</strong><div><NButton v-for="stop in stops" :key="stop.value" size="small" :disabled="batchSubmitting || (stop.value==='edit' && selectedHasSpectator)" @click="batchSet(stop.value)">{{ stop.label }}</NButton></div><NButton text @click="selected=new Set()">清除选择</NButton></div>
    </NSpin>
    <aside v-if="sheetMember" class="private-sheet">
      <header><div><strong>{{ nameOf(sheetMember) }}</strong><small>成员专属信息 · 仅该成员可见 <em v-if="privateDirty && !privateLoading">· 未保存</em></small><small v-if="getPrivateLock() && !ownsPrivateLock()" class="private-lock-hint">🔒 {{ privateLockOwnerName() }} 正在编辑此成员的专属信息</small><small v-else-if="privateAcquiringLock" class="private-lock-hint">正在获取编辑权…</small></div><div class="private-sheet__header-actions"><NButton circle quaternary size="small" title="保存" aria-label="保存专属信息" :type="privateDirty ? 'primary' : 'default'" :loading="privateSaving" :disabled="privateLoading" @click="flushPrivate()"><template #icon><NIcon><DeviceFloppy /></NIcon></template></NButton><NButton circle quaternary size="small" title="关闭" aria-label="关闭专属信息" :disabled="privateLoading || switchingPrivate" @click="closePrivate"><template #icon><NIcon><X /></NIcon></template></NButton></div></header>
      <NSpin :show="privateLoading" class="private-sheet__body" :inert="privateLoading || switchingPrivate">
        <label><span>内容格式</span><NSelect :value="privateFormat" :options="[{label:'纯文本',value:'plain'},{label:'富文本',value:'tiptap'}]" @update:value="value => { privateFormat=value; privateContent='' }" /></label>
        <div v-if="privateFormat==='tiptap'" class="private-rich-lock-gate" @pointerdown.capture="gatePrivatePointer" @keydown.capture="gatePrivateKeydown">
          <RichTextEditor ref="privateEditorRef" v-model="privateContent" :maxlength="50000" min-height="360px" @focus="beginPrivateEdit" @blur="finishPrivateEdit(privateEditorRef)" />
        </div>
        <NInput v-else v-model:value="privateContent" type="textarea" :autosize="{minRows:12}" :readonly="!ownsPrivateLock()" @focus="beginPrivateEdit" @blur="finishPrivateEdit()" />
        <!-- <small v-if="getPrivateLock() && !ownsPrivateLock()" class="private-lock-hint">🔒 {{ privateLockOwnerName() }} 正在编辑此成员的专属信息</small> -->
      </NSpin>
    </aside>
  </section>
</template>

<style scoped>
.private-sheet__header-actions { display: flex; flex: none; align-items: center; gap: 4px; }
.private-sheet header { flex: none; gap: 10px; }
.private-sheet header > div:first-child { min-width: 0; }
.private-sheet header strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.private-sheet header small { margin-top: 4px; font-size: 11px; }
.private-lock-hint { color: var(--n-warning-color) !important; }
.private-sheet__body label { padding-left: 10px; border-left: 2px solid color-mix(in srgb, var(--primary-color, #3388de) 65%, transparent); }
.distribution { position: relative; box-sizing: border-box; width: min(1050px, 100%); min-height: 430px; margin: 0 auto; }
.distribution__unsaved { display: grid; min-height: 390px; place-items: center; border: 1px solid var(--sc-border-mute); border-radius: 6px; background: color-mix(in srgb, var(--sc-bg-elevated) 92%, transparent); }
.distribution__unsaved-card { display: grid; width: min(440px, 100%); gap: 18px; padding: 18px; border: 1px solid var(--sc-border-mute); border-radius: 6px; background: color-mix(in srgb, var(--sc-bg-surface) 72%, transparent); }
.distribution__unsaved-card label > span { display: block; margin-bottom: 7px; color: var(--sc-text-secondary); font-size: 12px; }
.distribution__top { display: grid; grid-template-columns: minmax(220px, .7fr) minmax(420px, 1.3fr); gap: 14px; }
.distribution__top > label, .distribution__publish { padding: 13px; border: 1px solid var(--sc-border-mute); border-radius: 6px; background: color-mix(in srgb, var(--sc-bg-elevated) 92%, transparent); }
.distribution__top label > span, .private-sheet label > span { display: block; margin-bottom: 7px; color: var(--sc-text-secondary); font-size: 12px; }
.distribution__publish { display: flex; position: relative; align-items: center; justify-content: space-between; gap: 12px; }
.distribution__publish strong, .distribution__publish small { display: block; }.distribution__publish small { margin-top: 5px; color: var(--sc-text-secondary); }.distribution__publish > div:last-of-type { display: flex; gap: 7px; }.distribution__publish p { position: absolute; right: 13px; bottom: -20px; margin: 0; color: var(--n-warning-color); font-size: 11px; }
.distribution__hint { margin: 15px 0 10px; color: var(--sc-text-secondary); font-size: 12px; }
.distribution__tools { display: grid; grid-template-columns: minmax(220px, 1fr) auto; align-items: center; gap: 12px; margin-bottom: 12px; }.distribution__filters { display: flex; gap: 4px; }.distribution__filters button { padding: 6px 9px; color: var(--sc-text-secondary); border: 1px solid var(--sc-border-mute); border-radius: 5px; background: transparent; cursor: pointer; }.distribution__filters button.active { color: var(--primary-color); border-color: color-mix(in srgb,var(--primary-color) 50%,transparent); background: color-mix(in srgb,var(--primary-color) 10%,transparent); }
.distribution__empty { padding: 70px 20px; border: 1px solid var(--sc-border-mute); border-radius: 6px; }.distribution__track { display: grid; grid-template-columns: 76px minmax(0,1fr); }.distribution__source { position: relative; padding-top: 30px; color: var(--sc-text-secondary); text-align: center; }.distribution__source::after { position: absolute; top: 50px; right: 0; bottom: 36px; border-right: 1px solid var(--sc-border-strong); content:''; }.distribution__source i { display: block; width: 9px; height: 9px; margin: 0 auto 5px; border: 2px solid var(--primary-color); border-radius: 50%; }
.distribution__rows { display: grid; gap: 7px; }.member-row { position: relative; display: grid; grid-template-columns: minmax(180px,.8fr) minmax(330px,1.3fr) auto; align-items: center; gap: 14px; min-height: 74px; padding: 9px 10px 9px 18px; border: 1px solid var(--sc-border-mute); border-radius: 6px; background: color-mix(in srgb,var(--sc-bg-elevated) 91%,transparent); }.member-row::before { position:absolute; top:50%; left:-77px; width:76px; border-top:1px solid var(--sc-border-strong); content:''; }.member-row__identity { display:grid; grid-template-columns:22px 34px minmax(0,1fr); align-items:center; gap:8px; }.member-row__identity :deep(.n-checkbox) { opacity:.3; transition:opacity .12s; }.member-row:hover .member-row__identity :deep(.n-checkbox),.member-row.selected .member-row__identity :deep(.n-checkbox) { opacity:1; }.member-row__identity b,.member-row__identity small { display:block; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }.member-row__identity small { width:max-content; margin-top:2px; padding:1px 5px; border:1px solid var(--sc-border-mute); border-radius:4px; }.member-row__identity small,.member-row__locked,.permission-wrap > small { color:var(--sc-text-secondary); font-size:11px; }.member-row__check-placeholder { width:22px; }
.permission-wrap { min-width:0; }.permission-rail { position:relative; display:grid; grid-template-columns:repeat(4,1fr); touch-action:none; user-select:none; }.permission-rail::before { position:absolute; top:24px; right:12.5%; left:12.5%; border-top:1px solid var(--sc-border-strong); content:''; }.permission-rail button { position:relative; z-index:1; height:43px; padding:0; color:var(--sc-text-secondary); font:inherit; font-size:11px; border:0; background:transparent; cursor:pointer; }.permission-rail button span { position:absolute; top:0; right:0; left:0; line-height:1; text-align:center; }.permission-rail button i { position:absolute; top:20px; left:50%; width:9px; height:9px; border:2px solid var(--sc-border-strong); border-radius:50%; background:var(--sc-bg-elevated); transform:translateX(-50%); transition:.12s; }.permission-rail button:hover i,.permission-rail button:focus-visible i { transform:translateX(-50%) scale(1.35); border-color:var(--primary-color); }.permission-rail button.active { color:var(--primary-color); }.permission-rail button.active i { border-color:var(--primary-color); background:var(--primary-color); box-shadow:0 0 0 3px color-mix(in srgb,var(--primary-color) 18%,transparent); }.permission-rail button:disabled { opacity:.3; cursor:not-allowed; }.private-button i { width:7px;height:7px;margin-right:3px;border-radius:50%;background:var(--sc-text-secondary);}.private-button i.active { background:var(--primary-color);box-shadow:0 0 7px color-mix(in srgb,var(--primary-color) 70%,transparent); }
.batch-bar { position:sticky; z-index:4; bottom:8px; display:flex; align-items:center; justify-content:center; gap:12px; width:max-content; max-width:calc(100% - 20px); margin:14px auto 0; padding:9px 12px; border:1px solid color-mix(in srgb,var(--primary-color) 40%,var(--sc-border-mute)); border-radius:7px; background:color-mix(in srgb,var(--sc-bg-surface) 97%,transparent); box-shadow:0 8px 25px #0005; }.batch-bar > div { display:flex; gap:5px; }
.private-sheet { position:absolute; z-index:8; top:0; right:0; bottom:0; display:flex; width:min(460px,100%); flex-direction:column; border-left:1px solid var(--sc-border-strong); background:color-mix(in srgb,var(--sc-bg-surface) 98%,transparent); box-shadow:-16px 0 38px #0005; backdrop-filter:blur(16px); }.private-sheet header { display:flex; align-items:center; justify-content:space-between; padding:14px 16px; border-bottom:1px solid var(--sc-border-mute); }.private-sheet header strong,.private-sheet header small { display:block; }.private-sheet header small { color:var(--sc-text-secondary); }.private-sheet header em { color:var(--n-warning-color); font-style:normal; }.private-sheet__body { min-height:0; flex:1; padding:16px; overflow:auto; }.private-sheet__body label { display:block; margin-bottom:14px; }
@media (max-width:760px) { .distribution__top,.distribution__tools { grid-template-columns:1fr; }.distribution__publish { align-items:flex-start; flex-direction:column; }.distribution__publish p { position:static; }.distribution__filters { overflow-x:auto; }.distribution__track { grid-template-columns:1fr; }.distribution__source { display:none; }.member-row { grid-template-columns:1fr auto; padding:12px; }.member-row::before { display:none; }.member-row__identity { grid-column:1; }.permission-wrap,.member-row__locked { grid-column:1/-1; grid-row:2; }.private-button { grid-column:2; grid-row:1; }.batch-bar { align-items:flex-start; flex-wrap:wrap; width:auto; overflow-x:auto; }.batch-bar > div { overflow-x:auto; }.private-sheet { position:fixed; inset:0 0 0 auto; width:min(96vw,460px); } }
</style>
