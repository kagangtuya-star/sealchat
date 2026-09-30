<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NInput, NPagination, useThemeVars } from 'naive-ui'
import { speechAPI, speechError } from './api'
import { useUserStore } from '@/stores/user'
import type { SpeechProviderMeta, SpeechVoice, SystemVoice, VoiceContext } from './types'
import VoiceCard from './VoiceCard.vue'
import {
  collectVoiceFacets, emptyVoiceFilters, itemSelection, matchesSource, matchesSearch, matchesVoiceFilters,
  personalVoiceItem, resolveLegacySystemSelection, resolveSystemVoiceContext, selectsItem, systemVoiceItem, voiceSourceOptions, voiceSourceLabel,
  type VoiceCatalogItem, type VoiceSourceKey, type VoiceFacetKey, type VoiceSelection,
} from './voice-catalog'

// `browse` picks a voice for auditions; `select` binds one and must surface a
// stale binding instead of replacing it. Neither mode ever changes the
// selection on its own.
const props = defineProps<{
  mode: 'browse' | 'select'
  voiceContext: VoiceContext | null
  voiceContexts: VoiceContext[]
}>()
const selection = defineModel<VoiceSelection>({ required: true })
const user = useUserStore()
const theme = useThemeVars()
const PAGE_SIZE = 20 // Server default page size of GET /tts/voices.
const TAG_PREVIEW = 12

const source = ref<VoiceSourceKey>('all')
const searchInput = ref('')
const search = ref('')
const page = ref(1)
const filters = ref(emptyVoiceFilters())
// Facets stay folded away so the grid keeps most of the height.
const filtersOpen = ref(false)
const tagsExpanded = ref(false)
const system = ref<SystemVoice[] | null>(null)
const providers = ref<SpeechProviderMeta[]>([])
const sourceOptions = computed(() => voiceSourceOptions(providers.value))
const systemSource = computed(() => source.value.startsWith('system:'))
const personal = ref<SpeechVoice[]>([])
const personalTotal = ref(0)
// Personal voices looked up by id when bound but not on the current page; null = not accessible.
const resolved = ref<Record<string, VoiceCatalogItem | null>>({})
const resolveEpoch = ref(0)
const loading = ref(false)
const error = ref('')
const searchRef = ref<InstanceType<typeof NInput> | null>(null)
const gridRef = ref<HTMLElement | null>(null)
let serial = 0
let searchTimer: ReturnType<typeof setTimeout> | undefined

// System voices arrive as a full list on every response; personal voices are
// scoped, searched, filtered (kind/tag) and paged by the server, so
// `personalTotal` is the real filtered count. Stale responses are dropped by serial.
async function load() {
  const current = ++serial
  const userId = user.info.id
  const scope = systemSource.value ? 'all' : source.value as 'all' | 'mine' | 'public'
  error.value = ''
  if (systemSource.value && system.value) { loading.value = false; return }
  loading.value = true
  try {
    const { kind, tag } = filters.value
    const value = await speechAPI.voices(systemSource.value
      ? { scope: 'all', page: 1 }
      : { scope, search: search.value, page: page.value, kind: kind || undefined, tag: tag || undefined })
    if (current !== serial || userId !== user.info.id) return
    system.value = value.system
    providers.value = value.providers ?? []
    // Platform only needs the preset list; its personal page is not displayed.
    if (systemSource.value) return
    personal.value = value.items
    personalTotal.value = value.total
  } catch (e) { if (current === serial) error.value = speechError(e) }
  finally { if (current === serial) loading.value = false }
}
watch(() => user.info.id, () => { system.value = null; providers.value = []; personal.value = []; personalTotal.value = 0; resolved.value = {}; resolveSerial++; resolveEpoch.value++ }, { flush: 'sync' })
watch([() => props.voiceContext, () => props.voiceContexts], () => {
  page.value = 1
  filters.value = emptyVoiceFilters()
  tagsExpanded.value = false
  personal.value = []
  personalTotal.value = 0
  resolved.value = {}
  resolveSerial++
  resolveEpoch.value++
}, { flush: 'sync', deep: true })
watch([
  source, search, page, () => user.info.id,
  () => props.voiceContext, () => props.voiceContexts,
  () => filters.value.kind, () => filters.value.tag,
], () => void load(), { immediate: true, deep: true })

function setSource(value: VoiceSourceKey) {
  if (source.value === value) return
  source.value = value
  page.value = 1
  filters.value = emptyVoiceFilters()
  tagsExpanded.value = false
}
function applySearch() {
  clearTimeout(searchTimer)
  const next = searchInput.value.trim()
  if (next === search.value) return
  search.value = next
  page.value = 1
}
watch(searchInput, () => { clearTimeout(searchTimer); searchTimer = setTimeout(applySearch, 350) })
function toggleFilter(key: VoiceFacetKey, value: string) {
  filters.value = { ...filters.value, [key]: filters.value[key] === value ? '' : value }
  page.value = 1
}
// Facets come from the current response, so an active option may vanish when
// nothing matches; this stays reachable as the way out. Search is kept.
function clearFilters() {
  filters.value = emptyVoiceFilters()
  tagsExpanded.value = false
  page.value = 1
}

const presetItems = computed(() => (system.value ?? []).flatMap(voice => {
  const context = resolveSystemVoiceContext(voice, props.voiceContexts, props.voiceContext)
  return context ? [systemVoiceItem(voice, context)] : []
}))
// Previews and creating voices are managed in the workbench, not picked here.
const personalItems = computed(() => personal.value.filter(voice => voice.lifecycle === 'saved').map(voice => personalVoiceItem(voice, user.info.id)))
const scopedPresets = computed(() => presetItems.value.filter(item => matchesSource(item, source.value) && matchesSearch(item, search.value)))
const scopedPersonal = computed(() => personalItems.value.filter(item => matchesSource(item, source.value)))
// Facets only describe what the current response actually contains.
const facets = computed(() => collectVoiceFacets([...scopedPersonal.value, ...scopedPresets.value]))
const filteredPresets = computed(() => scopedPresets.value.filter(item => matchesVoiceFilters(item, filters.value)))
// Defensive only: the server already applied scope/search/kind/tag.
const filteredPersonal = computed(() => scopedPersonal.value.filter(item => matchesVoiceFilters(item, filters.value)))
// Personal voices carry no language metadata, so a language filter matches none
// of them and they must not occupy pagination slots.
const effectivePersonalTotal = computed(() => filters.value.language ? 0 : personalTotal.value)
const pagePersonalCount = computed(() => filters.value.language ? 0 : personal.value.length)
// "All" continues the server's personal pages with local preset pages, so each
// page holds at most PAGE_SIZE entries and no preset is skipped.
const visible = computed<VoiceCatalogItem[]>(() => {
  const start = (page.value - 1) * PAGE_SIZE
  if (systemSource.value) return filteredPresets.value.slice(start, start + PAGE_SIZE)
  if (source.value !== 'all') return filteredPersonal.value
  const offset = Math.max(0, start - effectivePersonalTotal.value)
  const room = Math.max(0, PAGE_SIZE - pagePersonalCount.value)
  return [...filteredPersonal.value, ...filteredPresets.value.slice(offset, offset + room)]
})
const total = computed(() => {
  if (systemSource.value) return filteredPresets.value.length
  if (source.value === 'all') return effectivePersonalTotal.value + filteredPresets.value.length
  return effectivePersonalTotal.value
})
const hasFilters = computed(() => Object.values(filters.value).some(Boolean))
const activeFilterCount = computed(() => Object.values(filters.value).filter(Boolean).length)
const emptyMessage = computed(() => {
  const provider = providers.value.find(entry => source.value === `system:${entry.kind}`)
  if (provider && !presetItems.value.some(item => item.providerKind === provider.kind)) return `当前没有可用的${provider.name}预设音色。`
  return hasFilters.value || search.value ? '没有符合条件的音色。' : '当前来源暂无音色。'
})

// A bound personal voice may be off-page, private, deleted or unhealthy. It is
// looked up once per reload; failure keeps an explicit unavailable state.
const missingPersonalId = computed(() => {
  const value = selection.value
  return value.type === 'personal' && !personalItems.value.some(item => item.id === value.id) ? value.id : ''
})
let resolveSerial = 0
watch([missingPersonalId, () => user.info.id, resolveEpoch], async ([id, userId]) => {
  if (!id || id in resolved.value) return
  const current = ++resolveSerial
  let item: VoiceCatalogItem | null = null
  try { item = personalVoiceItem(await speechAPI.voice(id), userId) } catch { /* Unavailable binding; never choose a fallback. */ }
  if (current === resolveSerial && userId === user.info.id) resolved.value = { ...resolved.value, [id]: item }
}, { immediate: true })

type CurrentState = { status: 'inherit' } | { status: 'pending' } | { status: 'ready'; item: VoiceCatalogItem } | { status: 'unavailable' }
const current = computed<CurrentState>(() => {
  const value = selection.value
  if (value.type === 'inherit') return { status: 'inherit' }
  if (value.type === 'system') {
    if (!system.value) return { status: 'pending' }
    const item = resolveLegacySystemSelection(value, system.value, props.voiceContexts, props.voiceContext)
    return item ? { status: 'ready', item } : { status: 'unavailable' }
  }
  const item = personalItems.value.find(entry => entry.id === value.id) ?? resolved.value[value.id]
  if (item === undefined) return { status: 'pending' }
  return item?.available ? { status: 'ready', item } : { status: 'unavailable' }
})

function isItemSelected(item: VoiceCatalogItem): boolean {
  if (selection.value.type === 'system') {
    return current.value.status === 'ready' && current.value.item.key === item.key
  }
  return selectsItem(selection.value, item)
}

function choose(item: VoiceCatalogItem) { if (item.available) selection.value = itemSelection(item) }
function followDefault() { selection.value = { type: 'inherit' } }
function reselect() {
  gridRef.value?.scrollTo({ top: 0 })
  searchRef.value?.focus()
}
onBeforeUnmount(() => { serial++; resolveSerial++; clearTimeout(searchTimer) })
// Owners call this after creating, saving, editing or deleting voices.
function reload() {
  resolved.value = {}
  resolveEpoch.value++
  return load()
}
defineExpose({ reload })
</script>

<template>
  <div class="vp" :style="{ '--vp-accent': `var(--primary-color, ${theme.primaryColor})`, '--vp-warning': theme.warningColor, '--vp-error': theme.errorColor }">
    <div class="vp-toolbar">
      <div class="vp-search">
        <NInput
          ref="searchRef"
          v-model:value="searchInput"
          clearable
          placeholder="搜索音色名称或标签"
          @keyup.enter="applySearch"
          @clear="() => { searchInput = ''; applySearch() }"
        />
        <button
          type="button"
          class="vp-chip vp-filter-toggle"
          :class="{ 'is-active': hasFilters, 'is-open': filtersOpen }"
          :aria-expanded="filtersOpen"
          @click="filtersOpen = !filtersOpen"
        >筛选<template v-if="activeFilterCount"> · {{ activeFilterCount }}</template></button>
      </div>
      <div class="vp-chips vp-categories" role="group" aria-label="音色来源">
        <button
          v-for="option in sourceOptions"
          :key="option.value"
          type="button"
          class="vp-chip is-category"
          :class="{ 'is-active': source === option.value }"
          :aria-pressed="source === option.value"
          @click="setSource(option.value)"
        >{{ option.label }}</button>
      </div>
      <div v-if="filtersOpen" class="vp-filters">
        <p v-if="!facets.length" class="vp-filters__empty">当前结果没有可用的筛选项。</p>
        <div v-for="facet in facets" :key="facet.key" class="vp-facet">
          <span class="vp-facet__label">{{ facet.label }}</span>
          <div class="vp-chips" :class="{ 'is-tags': facet.key === 'tag', 'is-scroll': facet.key === 'tag' && tagsExpanded }" role="group" :aria-label="facet.label">
            <button
              v-for="option in facet.key === 'tag' && !tagsExpanded ? facet.options.slice(0, TAG_PREVIEW) : facet.options"
              :key="option.value"
              type="button"
              class="vp-chip"
              :class="{ 'is-active': filters[facet.key] === option.value }"
              :aria-pressed="filters[facet.key] === option.value"
              @click="toggleFilter(facet.key, option.value)"
            >{{ option.label }}</button>
            <button
              v-if="facet.key === 'tag' && facet.options.length > TAG_PREVIEW"
              type="button"
              class="vp-chip is-more"
              @click="tagsExpanded = !tagsExpanded"
            >{{ tagsExpanded ? '收起' : `更多 ${facet.options.length - TAG_PREVIEW}` }}</button>
          </div>
        </div>
        <button v-if="activeFilterCount > 0" type="button" class="vp-chip is-more vp-filters__clear" @click="clearFilters">清除筛选</button>
      </div>
    </div>

    <div class="vp-current" :class="{ 'is-warning': current.status === 'unavailable' }" aria-live="polite">
      <div class="vp-current__row">
        <button
          type="button"
          class="vp-default"
          :class="{ 'is-active': selection.type === 'inherit' }"
          :aria-pressed="selection.type === 'inherit'"
          :title="mode === 'select' ? '不绑定具体音色，随平台默认设置变化' : '使用平台默认音色试听'"
          @click="followDefault"
        ><span class="vp-default__dot" aria-hidden="true" />跟随平台默认音色</button>
        <span class="vp-current__state">
          <span class="vp-current__label">当前：</span>
          <template v-if="current.status === 'inherit'">跟随平台默认音色</template>
          <template v-else-if="current.status === 'pending'">正在确认音色…</template>
          <template v-else-if="current.status === 'ready'">
            <strong class="vp-current__name">{{ current.item.name }}</strong>
            <span class="vp-current__source"> · {{ voiceSourceLabel(current.item, providers) }}</span>
          </template>
          <strong v-else class="vp-current__name">{{ mode === 'select' ? '当前绑定音色已不可用' : '所选音色已不可用' }}</strong>
        </span>
      </div>
      <div v-if="current.status === 'unavailable'" class="vp-current__warning">
        <small>当前模型不可用，需要重新选择；也可能已删除、改为私有或服务不可用。</small>
        <div class="vp-current__actions">
          <button type="button" class="vp-chip" @click="reselect">重新选择</button>
          <button type="button" class="vp-chip" @click="followDefault">{{ mode === 'select' ? '清除绑定，跟随默认' : '改用平台默认' }}</button>
        </div>
      </div>
    </div>

    <div ref="gridRef" class="vp-body" :class="{ 'is-loading': loading }">
      <p v-if="error" class="vp-note is-error" role="alert">{{ error }}</p>
      <div v-if="visible.length" class="vp-grid">
        <VoiceCard
          v-for="item in visible"
          :key="item.key"
          :item="item"
          :providers="providers"
          :selected="isItemSelected(item)"
          @select="choose"
        />
      </div>
      <p v-else-if="!loading" class="vp-note">{{ emptyMessage }}</p>
      <p v-if="loading" class="vp-note">加载中…</p>
    </div>

    <div class="vp-footer">
      <small class="vp-footer__hint">已有试听会直接重放；其他音色会生成试听音频。</small>
      <NPagination v-if="total > PAGE_SIZE" v-model:page="page" :item-count="total" :page-size="PAGE_SIZE" :page-slot="5" size="small" :disabled="loading" />
    </div>
  </div>
</template>

<style scoped>
/* Colors come only from theme variables: --sc-* (day/night palettes, overridden
   by custom themes) and --primary-color (custom accent, else Naive primary).
   Soft fills are mixed from the text color so they stay visible on any surface. */
.vp {
  --vp-soft: color-mix(in srgb, var(--sc-text-primary) 6%, transparent);
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: 100%;
  min-height: 0;
  min-width: 0;
  color: var(--sc-text-primary);
}
.vp-toolbar { display: flex; flex-direction: column; gap: 8px; flex: none; min-width: 0; }
.vp-search { display: flex; align-items: center; gap: 8px; min-width: 0; }
.vp-search > :first-child { flex: 1 1 auto; min-width: 0; }
.vp-chips { display: flex; flex-wrap: wrap; gap: 6px; min-width: 0; }
.vp-toolbar > .vp-chips { column-gap: 18px; row-gap: 6px; }
.vp-categories { align-items: center; column-gap: 22px !important; }
.vp-chips.is-scroll { max-height: 120px; overflow-y: auto; padding-right: 4px; }
.vp-chip {
  max-width: 100%;
  padding: 3px 12px;
  overflow: hidden;
  border: 1px solid transparent;
  border-radius: 999px;
  background: var(--vp-soft);
  color: var(--sc-text-secondary);
  font: inherit;
  font-size: 13px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
  transition: color .15s ease, border-color .15s ease, background-color .15s ease;
}
.vp-chip:hover { color: var(--sc-text-primary); }
.vp-chip:focus-visible { outline: 2px solid var(--vp-accent); outline-offset: 1px; }
.vp-chip.is-active {
  border-color: color-mix(in srgb, var(--vp-accent) 55%, transparent);
  background: color-mix(in srgb, var(--vp-accent) 14%, transparent);
  color: var(--sc-text-primary);
}
.vp-chip.is-category {
  flex: none;
  max-width: none;
  padding: 2px 4px;
  overflow: visible;
  border: 0;
  border-radius: 2px;
  background: transparent;
  font-size: 14px;
  text-overflow: clip;
  white-space: nowrap;
}
.vp-chip.is-category.is-active {
  background: transparent;
  color: var(--vp-accent);
  box-shadow: inset 0 -1px 0 var(--vp-accent);
}
.vp-chip.is-more { background: transparent; border-color: var(--sc-border-mute); }
.vp-chip.vp-filter-toggle { flex: none; padding: 4px 14px; border-color: var(--sc-border-mute); border-radius: 5px; }
.vp-chip.vp-filter-toggle.is-open:not(.is-active) { color: var(--sc-text-primary); border-color: var(--sc-border-strong); }
/* Opened on demand; capped so even many tags never push the grid away. */
.vp-filters {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 168px;
  padding: 8px 10px;
  overflow-y: auto;
  border: 1px solid var(--sc-border-mute);
  border-radius: 6px;
}
.vp-filters__empty { margin: 0; font-size: 12px; color: var(--sc-text-secondary); }
.vp-filters__clear { align-self: flex-end; flex: none; }
.vp-facet { display: flex; align-items: flex-start; gap: 10px; min-width: 0; }
.vp-facet__label { flex: none; width: 3em; padding-top: 3px; font-size: 12px; color: var(--sc-text-secondary); }
.vp-facet .vp-chips { flex: 1; column-gap: 16px; row-gap: 6px; }
.vp-facet .vp-chips.is-tags {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(96px, 1fr));
  gap: 6px 14px;
  align-items: center;
}
.vp-facet .vp-chips.is-tags .vp-chip { justify-self: start; text-align: left; }
.vp-facet .vp-chip {
  flex: none;
  max-width: none;
  padding: 1px 2px;
  overflow: visible;
  border: 0;
  border-radius: 2px;
  background: transparent;
  font-size: 12px;
  line-height: 22px;
  text-overflow: clip;
  white-space: nowrap;
  word-break: keep-all;
}
.vp-facet .vp-chip.is-active {
  background: transparent;
  color: var(--vp-accent);
  box-shadow: inset 0 -1px 0 var(--vp-accent);
}
.vp-facet .vp-chip.is-more { color: var(--sc-text-secondary); box-shadow: none; }

.vp-current { display: flex; flex-direction: column; gap: 6px; flex: none; min-width: 0; }
.vp-current.is-warning { padding: 6px 10px; border-radius: 6px; background: color-mix(in srgb, var(--vp-warning) 14%, transparent); }
.vp-current__row { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 14px; min-width: 0; font-size: 13px; }
.vp-default {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
  border: 0;
  background: transparent;
  color: var(--sc-text-secondary);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.vp-default:hover, .vp-default.is-active { color: var(--sc-text-primary); }
.vp-default__dot { flex: none; width: 12px; height: 12px; border: 1px solid var(--sc-border-strong); border-radius: 50%; }
.vp-default.is-active .vp-default__dot { border: 4px solid var(--vp-accent); }
.vp-default:focus-visible { outline: 2px solid var(--vp-accent); outline-offset: 2px; }
.vp-current__state {
  display: flex;
  flex: 1 1 200px;
  align-items: baseline;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
}
.vp-current__label { flex: none; color: var(--sc-text-secondary); }
.vp-current__name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.vp-current__source { flex: none; font-size: 12px; color: var(--vp-accent); }
.vp-current__warning { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 6px 10px; min-width: 0; }
.vp-current__warning small { flex: 1 1 200px; min-width: 0; font-size: 12px; color: var(--sc-text-secondary); }
.vp-current__actions { display: flex; flex-wrap: wrap; gap: 6px; }

.vp-body { flex: 1 1 auto; min-height: 0; overflow-y: auto; overscroll-behavior: contain; }
.vp-body.is-loading .vp-grid { opacity: .6; }
.vp-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 8px; }
.vp-note { margin: 16px 0; font-size: 13px; text-align: center; color: var(--sc-text-secondary); }
.vp-note.is-error { color: var(--vp-error); }
.vp-footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; flex: none; }
.vp-footer__hint { font-size: 12px; color: var(--sc-text-secondary); }

/* Narrow screens: the surrounding dialog body scrolls as a whole instead of a
   nested grid scroller; the search/category bar stays reachable. */
@media (max-width: 720px) {
  .vp { height: auto; }
  .vp-toolbar {
    position: sticky;
    top: 0;
    z-index: 1;
    padding: 4px 0 8px;
    background: var(--sc-bg-elevated);
  }
  .vp-body { overflow: visible; }
  .vp-grid { grid-template-columns: minmax(0, 1fr); }
  .vp-facet { flex-direction: column; gap: 4px; }
  .vp-facet__label { width: auto; padding-top: 0; }
}
</style>
