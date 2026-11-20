<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage, useThemeVars } from 'naive-ui'
import { ChevronBack, ChevronForward, InformationCircleOutline } from '@vicons/ionicons5'
import {
  TYPING_HEROES_METRICS,
  TYPING_HEROES_PREFS_KEY,
  buildTypingHeroesIdentityOptions,
  fetchTypingHeroesBoard,
  fetchTypingHeroesIdentities,
  formatTypingHeroesPeriod,
  formatTypingHeroesValue,
  isTypingHeroesIdentityUnavailable,
  normalizeTypingHeroesPrefs,
  pickTypingHeroesIdentity,
  saveTypingHeroesPublic,
  typingHeroesBarPercent,
  typingHeroesStatValue,
  typingHeroesTier,
  type TypingHeroesBoard,
  type TypingHeroesEntry,
  type TypingHeroesIdentityInfo,
  type TypingHeroesMetric,
  type TypingHeroesPeriodKind,
  type TypingHeroesSubject,
} from './typingHeroes'

const MAX_OFFSET = 24

const props = defineProps<{ currentWorldId?: string }>()

const { t, locale } = useI18n()
const message = useMessage()
const themeVars = useThemeVars()

function loadPrefs() {
  try {
    const raw = localStorage.getItem(TYPING_HEROES_PREFS_KEY)
    return normalizeTypingHeroesPrefs(raw ? JSON.parse(raw) : null)
  } catch {
    return normalizeTypingHeroesPrefs(null)
  }
}

const initialPrefs = loadPrefs()
const period = ref<TypingHeroesPeriodKind>(initialPrefs.period)
const metric = ref<TypingHeroesMetric>(initialPrefs.metric)
const subject = ref<TypingHeroesSubject>(initialPrefs.subject)
const identityId = ref(initialPrefs.identityId)
const offset = ref(0)

const board = ref<TypingHeroesBoard | null>(null)
const loading = ref(false)
const loadError = ref('')
const identityNotice = ref('')
const identities = ref<TypingHeroesIdentityInfo[]>([])
const identitiesLoaded = ref(false)
const identitiesLoading = ref(false)
const publicSaving = ref(false)

let boardSeq = 0
let identitySeq = 0

function savePrefs() {
  try {
    localStorage.setItem(TYPING_HEROES_PREFS_KEY, JSON.stringify({
      period: period.value,
      metric: metric.value,
      subject: subject.value,
      identityId: identityId.value,
    }))
  } catch { /* ignore */ }
}

const canQuery = computed(() => subject.value === 'user' || !!identityId.value)

async function loadBoard() {
  const seq = ++boardSeq
  if (!canQuery.value) {
    board.value = null
    loading.value = false
    return
  }
  const requestedIdentity = identityId.value
  loading.value = true
  loadError.value = ''
  try {
    const data = await fetchTypingHeroesBoard({
      period: period.value,
      offset: offset.value,
      metric: metric.value,
      subject: subject.value,
      identityId: requestedIdentity,
    })
    if (seq !== boardSeq) return
    board.value = data
  } catch (err) {
    if (seq !== boardSeq) return
    board.value = null
    if (subject.value === 'identity' && isTypingHeroesIdentityUnavailable(err)) {
      identityNotice.value = t('inputStats.heroes.identityUnavailable')
      if (identityId.value === requestedIdentity) identityId.value = ''
      void loadIdentities(false)
      return
    }
    loadError.value = t('inputStats.heroes.loadFailed')
  } finally {
    if (seq === boardSeq) loading.value = false
  }
}

async function loadIdentities(autoPick: boolean) {
  const seq = ++identitySeq
  identitiesLoading.value = true
  try {
    const items = await fetchTypingHeroesIdentities()
    if (seq !== identitySeq) return
    identities.value = items
    identitiesLoaded.value = true
    if (identityId.value && !items.some(item => item.id === identityId.value)) {
      identityId.value = ''
    }
    if (autoPick && !identityId.value) {
      identityId.value = pickTypingHeroesIdentity(items, initialPrefs.identityId, props.currentWorldId)
    }
  } catch {
    if (seq === identitySeq) loadError.value = t('inputStats.heroes.loadFailed')
  } finally {
    if (seq === identitySeq) identitiesLoading.value = false
  }
}

function setPeriod(value: string | number | boolean) {
  if (value !== 'week' && value !== 'month') return
  period.value = value
  offset.value = 0
}

function shiftPeriod(step: number) {
  offset.value = Math.min(MAX_OFFSET, Math.max(0, offset.value + step))
}

function setSubject(value: string | number | boolean) {
  if (value !== 'user' && value !== 'identity') return
  subject.value = value
  identityNotice.value = ''
  if (value === 'identity' && !identitiesLoaded.value) {
    void loadIdentities(true)
  }
}

function selectIdentity(value: string | null) {
  identityNotice.value = ''
  identityId.value = value || ''
}

async function togglePublic(value: boolean) {
  const current = board.value
  if (!current || publicSaving.value) return
  publicSaving.value = true
  current.isPublic = value
  try {
    await saveTypingHeroesPublic(value)
  } catch {
    current.isPublic = !value
    message.error(t('inputStats.heroes.publicSaveFailed'))
  } finally {
    publicSaving.value = false
  }
}

watch([period, offset, metric, subject, identityId], () => {
  savePrefs()
  void loadBoard()
})

onMounted(() => {
  if (subject.value === 'identity') {
    void loadIdentities(true)
  }
  void loadBoard()
})

// === 展示 ===
const metricLabel = (m: TypingHeroesMetric) => t(`inputStats.heroes.metrics.${m}`)
const metricUnit = (m: TypingHeroesMetric) => t(`inputStats.heroes.units.${m}`)
const metricHint = (m: TypingHeroesMetric) => t(`inputStats.heroes.metricHints.${m}`, { min: board.value?.speedMinActiveMinutes ?? 10 })

const periodLabel = computed(() => {
  const p = board.value?.period
  if (!p || p.kind !== period.value || p.offset !== offset.value) return ''
  return formatTypingHeroesPeriod(p.kind, p.start, p.end, locale.value)
})

const currentPeriodBadge = computed(() => (offset.value === 0
  ? t(period.value === 'week' ? 'inputStats.heroes.currentWeek' : 'inputStats.heroes.currentMonth')
  : ''))

const me = computed(() => board.value?.me)
const ranked = computed(() => me.value?.status === 'ranked')
const activeMetric = computed<TypingHeroesMetric>(() => board.value?.metric ?? metric.value)

const tier = computed(() => typingHeroesTier(me.value?.rank, me.value?.beatPercent, board.value?.participants ?? 0))

const statusText = computed(() => {
  const m = me.value
  if (!m || m.status === 'ranked') return ''
  if (m.status === 'no_data') return t('inputStats.heroes.noData')
  if (activeMetric.value === 'speed') {
    return t('inputStats.heroes.speedNotEligible', {
      active: m.stats.activeMinutes,
      min: board.value?.speedMinActiveMinutes ?? 10,
    })
  }
  return t('inputStats.heroes.noChars')
})

const progressText = computed(() => {
  const m = me.value
  if (!m || m.status !== 'ranked') return ''
  if (m.rank === 1) return t('inputStats.heroes.topRank')
  if (m.gapToPrev != null && m.rank) {
    return t('inputStats.heroes.gapToPrev', {
      rank: m.rank - 1,
      gap: `${formatTypingHeroesValue(activeMetric.value, m.gapToPrev)} ${metricUnit(activeMetric.value)}`,
    })
  }
  return ''
})

const beatText = computed(() => {
  const m = me.value
  if (!m || m.status !== 'ranked') return ''
  if (m.beatPercent == null) return t('inputStats.heroes.onlyOne')
  return t('inputStats.heroes.beat', { p: m.beatPercent })
})

const metricTabs = computed(() => TYPING_HEROES_METRICS.map(m => {
  const stats = me.value?.status && me.value.status !== 'no_data' ? me.value.stats : undefined
  return {
    key: m,
    label: metricLabel(m),
    value: stats ? formatTypingHeroesValue(m, typingHeroesStatValue(stats, m)) : '—',
  }
}))

const entries = computed(() => board.value?.entries ?? [])
const listTitle = computed(() => t(ranked.value ? 'inputStats.heroes.nearby' : 'inputStats.heroes.leaders'))

function entryName(entry: TypingHeroesEntry) {
  if (entry.kind === 'self') {
    return board.value?.subject === 'identity' ? entry.name : t('inputStats.heroes.you')
  }
  return entry.name
}

function entryInitial(entry: TypingHeroesEntry) {
  const name = entry.kind === 'self' && board.value?.subject !== 'identity' ? t('inputStats.heroes.you') : entry.name
  return Array.from(name.trim())[0] || '?'
}

const identityOptions = computed(() => buildTypingHeroesIdentityOptions(identities.value, '—'))

const themeStyle = computed(() => ({
  '--th-naive-primary': themeVars.value.primaryColor,
  '--th-naive-warning': themeVars.value.warningColor,
}))
</script>

<template>
  <div class="typing-heroes" data-sc-ui="typing-heroes" :data-sc-typing-heroes-subject="subject" :style="themeStyle">
    <!-- 周期 / 主体 -->
    <div class="th-toolbar">
      <div class="th-toolbar__group">
        <n-radio-group :value="period" size="small" @update:value="setPeriod">
          <n-radio-button value="week">{{ t('inputStats.heroes.periodWeek') }}</n-radio-button>
          <n-radio-button value="month">{{ t('inputStats.heroes.periodMonth') }}</n-radio-button>
        </n-radio-group>
        <div class="th-period-nav">
          <n-button
            quaternary circle size="small"
            :disabled="offset >= MAX_OFFSET"
            :aria-label="t('inputStats.heroes.prevPeriod')"
            @click="shiftPeriod(1)"
          >
            <template #icon><n-icon :component="ChevronBack" /></template>
          </n-button>
          <span class="th-period-label">
            <span v-if="currentPeriodBadge" class="th-period-badge">{{ currentPeriodBadge }}</span>
            <span class="th-period-range">{{ periodLabel || '…' }}</span>
          </span>
          <n-button
            quaternary circle size="small"
            :disabled="offset === 0"
            :aria-label="t('inputStats.heroes.nextPeriod')"
            @click="shiftPeriod(-1)"
          >
            <template #icon><n-icon :component="ChevronForward" /></template>
          </n-button>
          <n-button v-if="offset > 0" text size="tiny" type="primary" @click="offset = 0">
            {{ t('inputStats.heroes.backToCurrent') }}
          </n-button>
        </div>
      </div>
      <div class="th-toolbar__group th-toolbar__group--subject">
        <n-radio-group :value="subject" size="small" @update:value="setSubject">
          <n-radio-button value="user">{{ t('inputStats.heroes.subjectUser') }}</n-radio-button>
          <n-radio-button value="identity">{{ t('inputStats.heroes.subjectIdentity') }}</n-radio-button>
        </n-radio-group>
        <n-select
          v-if="subject === 'identity'"
          class="th-identity-select"
          size="small"
          filterable
          :value="identityId || null"
          :options="identityOptions"
          :loading="identitiesLoading"
          :placeholder="t('inputStats.heroes.identityPlaceholder')"
          @update:value="selectIdentity"
        />
      </div>
    </div>

    <!-- 指标 -->
    <div class="th-metrics" role="tablist">
      <button
        v-for="tab in metricTabs"
        :key="tab.key"
        type="button"
        role="tab"
        class="th-metric"
        :class="{ 'is-active': metric === tab.key }"
        :aria-selected="metric === tab.key"
        @click="metric = tab.key"
      >
        <span class="th-metric__label">
          {{ tab.label }}
          <n-tooltip trigger="hover" placement="top" :style="{ maxWidth: '18rem' }">
            <template #trigger>
              <n-icon class="th-metric__hint" :component="InformationCircleOutline" @click.stop />
            </template>
            {{ metricHint(tab.key) }}
          </n-tooltip>
        </span>
        <span class="th-metric__value">{{ tab.value }}</span>
      </button>
    </div>

    <n-spin :show="loading || identitiesLoading">
      <div class="th-body">
        <div v-if="identityNotice" class="th-notice">{{ identityNotice }}</div>

        <div v-if="subject === 'identity' && identitiesLoaded && identities.length === 0" class="th-empty">
          {{ t('inputStats.heroes.identityEmpty') }}
        </div>
        <div v-else-if="loadError && !board" class="th-empty">
          <span>{{ loadError }}</span>
          <n-button size="small" secondary @click="loadBoard">{{ t('inputStats.heroes.retry') }}</n-button>
        </div>

        <template v-else-if="board">
          <!-- 我的成绩 -->
          <section class="th-hero" :data-sc-typing-heroes-status="board.me.status" :data-sc-typing-heroes-podium="ranked && (board.me.rank ?? 99) <= 3 ? board.me.rank : undefined">
            <div class="th-hero__rank">
              <span class="th-hero__rank-label">{{ t('inputStats.heroes.myResult') }}</span>
              <span class="th-hero__rank-value">{{ ranked ? `#${board.me.rank}` : '—' }}</span>
              <span class="th-hero__rank-total">{{ t('inputStats.heroes.participants', { n: board.participants }) }}</span>
            </div>
            <div class="th-hero__detail">
              <div class="th-hero__head">
                <span v-if="ranked" class="th-hero__tier">{{ t(`inputStats.heroes.tiers.${tier}`) }}</span>
                <span v-if="board.identity" class="th-hero__identity">
                  <span class="th-dot" :style="board.identity.color ? { '--th-dot-color': board.identity.color } : undefined" />
                  {{ board.identity.displayName }}
                  <span class="th-hero__channel"># {{ board.identity.channelName }}</span>
                </span>
              </div>
              <div class="th-hero__value">
                <strong>{{ formatTypingHeroesValue(activeMetric, board.me.value) }}</strong>
                <span>{{ metricUnit(activeMetric) }}</span>
              </div>
              <template v-if="ranked">
                <div class="th-beat">
                  <div class="th-beat__track">
                    <div class="th-beat__fill" :style="{ width: `${board.me.beatPercent ?? 100}%` }" />
                  </div>
                  <span class="th-beat__text">{{ beatText }}</span>
                </div>
                <div v-if="progressText" class="th-hero__progress">{{ progressText }}</div>
              </template>
              <div v-else class="th-hero__progress">{{ statusText }}</div>
            </div>
          </section>

          <!-- 附近排名 -->
          <section class="th-nearby">
            <div class="th-nearby__head">
              <h3>{{ listTitle }}</h3>
              <span>{{ board.subject === 'identity' ? t('inputStats.heroes.identityHint') : t('inputStats.heroes.anonymousHint') }}</span>
            </div>
            <div v-if="entries.length === 0" class="th-empty th-empty--compact">{{ t('inputStats.heroes.emptyBoard') }}</div>
            <ol v-else class="th-list">
              <li
                v-for="entry in entries"
                :key="entry.key"
                class="th-row"
                :class="{ 'is-self': entry.kind === 'self' }"
                :data-sc-typing-heroes-kind="entry.kind"
                :data-sc-typing-heroes-podium="entry.rank <= 3 ? entry.rank : undefined"
              >
                <span class="th-rank">{{ entry.rank }}</span>
                <span
                  class="th-avatar"
                  :style="entry.color ? { '--th-dot-color': entry.color } : undefined"
                  aria-hidden="true"
                >{{ entryInitial(entry) }}</span>
                <span class="th-row__main">
                  <span class="th-row__name">
                    <span class="th-row__name-text">{{ entryName(entry) }}</span>
                    <span v-if="entry.kind === 'public' && entry.username" class="th-row__username">@{{ entry.username }}</span>
                    <span v-if="entry.kind === 'self' && board.subject === 'identity'" class="th-row__you">{{ t('inputStats.heroes.you') }}</span>
                  </span>
                  <span class="th-row__bar"><span :style="{ width: `${typingHeroesBarPercent(entry.value, entries)}%` }" /></span>
                </span>
                <span class="th-row__value">
                  {{ formatTypingHeroesValue(activeMetric, entry.value) }}
                  <small>{{ metricUnit(activeMetric) }}</small>
                </span>
              </li>
            </ol>
          </section>
        </template>

        <div v-else-if="subject === 'identity' && !identityId && identitiesLoaded" class="th-empty">
          {{ t('inputStats.heroes.identityPlaceholder') }}
        </div>

        <!-- 公开设置与口径 -->
        <footer class="th-footer">
          <label v-if="subject === 'user' && board" class="th-public">
            <n-switch :value="board.isPublic" size="small" :loading="publicSaving" @update:value="togglePublic" />
            <span>
              <span class="th-public__title">{{ t('inputStats.heroes.publicSwitch') }}</span>
              <span class="th-public__desc">{{ board.isPublic ? t('inputStats.heroes.publicOn') : t('inputStats.heroes.publicOff') }}</span>
            </span>
          </label>
          <p class="th-rule">{{ t('inputStats.heroes.ruleNote') }}</p>
        </footer>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.typing-heroes {
  /* 主题包可通过 --sc-typing-heroes-* 覆盖；默认跟随主色与 Naive 语义色。 */
  --th-accent: var(--sc-typing-heroes-accent, var(--primary-color, var(--th-naive-primary)));
  --th-podium: var(--sc-typing-heroes-podium, var(--th-naive-warning));
  --th-card: var(--sc-typing-heroes-card, color-mix(in srgb, var(--th-accent) 10%, var(--sc-bg-elevated, transparent)));
  --th-row: var(--sc-typing-heroes-row, var(--sc-chip-bg, color-mix(in srgb, var(--sc-text-primary) 4%, transparent)));
  --th-self: var(--sc-typing-heroes-self, color-mix(in srgb, var(--th-accent) 14%, transparent));
  --th-track: color-mix(in srgb, var(--sc-text-primary) 10%, transparent);
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 20px 20px;
  color: var(--sc-text-primary);
  min-width: 0;
}

/* ===== Toolbar ===== */
.th-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
  align-items: center;
  justify-content: space-between;
}
.th-toolbar__group {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}
.th-toolbar__group--subject {
  flex: 1 1 260px;
  justify-content: flex-end;
}
.th-identity-select {
  flex: 1 1 180px;
  min-width: 0;
  max-width: 300px;
}
.th-period-nav {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
}
.th-period-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  min-width: 0;
}
.th-period-badge {
  padding: 1px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  color: var(--th-accent);
  background: color-mix(in srgb, var(--th-accent) 14%, transparent);
}
.th-period-range {
  color: var(--sc-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ===== Metric tabs ===== */
.th-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}
.th-metric {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  min-width: 0;
  padding: 8px 12px;
  border-radius: 10px;
  border: 1px solid var(--sc-border-mute);
  background: var(--th-row);
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.15s, background-color 0.15s;
}
.th-metric:hover {
  border-color: color-mix(in srgb, var(--th-accent) 45%, var(--sc-border-mute));
}
.th-metric:focus-visible {
  outline: 2px solid var(--th-accent);
  outline-offset: 2px;
}
.th-metric.is-active {
  border-color: var(--th-accent);
  background: var(--th-self);
}
.th-metric__label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--sc-text-secondary);
}
.th-metric.is-active .th-metric__label {
  color: var(--th-accent);
  font-weight: 600;
}
.th-metric__hint {
  font-size: 13px;
  opacity: 0.7;
}
.th-metric__value {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 15px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

/* ===== Body ===== */
.th-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 120px;
}
.th-notice {
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 12px;
  color: var(--sc-text-primary);
  background: color-mix(in srgb, var(--th-podium) 14%, transparent);
  border: 1px solid color-mix(in srgb, var(--th-podium) 35%, transparent);
}
.th-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 28px 12px;
  text-align: center;
  font-size: 13px;
  color: var(--sc-text-secondary);
}
.th-empty--compact {
  padding: 16px 12px;
}

/* ===== Hero card ===== */
.th-hero {
  display: grid;
  grid-template-columns: minmax(96px, auto) minmax(0, 1fr);
  gap: 20px;
  align-items: center;
  padding: 18px 20px;
  border-radius: 14px;
  border: 1px solid color-mix(in srgb, var(--th-accent) 28%, var(--sc-border-mute));
  background: var(--th-card);
}
.th-hero[data-sc-typing-heroes-podium] {
  border-color: color-mix(in srgb, var(--th-podium) 45%, var(--sc-border-mute));
}
.th-hero__rank {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding-right: 20px;
  border-right: 1px solid var(--sc-border-mute);
}
.th-hero__rank-label {
  font-size: 11px;
  letter-spacing: 0.5px;
  color: var(--sc-text-secondary);
}
.th-hero__rank-value {
  font-size: 40px;
  font-weight: 800;
  line-height: 1.1;
  color: var(--th-accent);
  font-variant-numeric: tabular-nums;
}
.th-hero[data-sc-typing-heroes-podium] .th-hero__rank-value {
  color: var(--th-podium);
}
.th-hero[data-sc-typing-heroes-status='no_data'] .th-hero__rank-value,
.th-hero[data-sc-typing-heroes-status='not_eligible'] .th-hero__rank-value {
  color: var(--sc-text-secondary);
}
.th-hero__rank-total {
  font-size: 12px;
  color: var(--sc-text-secondary);
  white-space: nowrap;
}
.th-hero__detail {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}
.th-hero__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 10px;
  min-width: 0;
}
.th-hero__tier {
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  color: var(--th-accent);
  background: color-mix(in srgb, var(--th-accent) 15%, transparent);
}
.th-hero[data-sc-typing-heroes-podium] .th-hero__tier {
  color: var(--sc-text-primary);
  background: color-mix(in srgb, var(--th-podium) 24%, transparent);
}
.th-hero__identity {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
}
.th-hero__channel {
  font-weight: 400;
  color: var(--sc-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.th-hero__value {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
}
.th-hero__value strong {
  font-size: 30px;
  font-weight: 800;
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
}
.th-hero__value span {
  font-size: 13px;
  color: var(--sc-text-secondary);
}
.th-beat {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.th-beat__track {
  flex: 1 1 auto;
  height: 6px;
  min-width: 60px;
  border-radius: 999px;
  background: var(--th-track);
  overflow: hidden;
}
.th-beat__fill {
  height: 100%;
  border-radius: inherit;
  background: var(--th-accent);
  transition: width 0.35s ease;
}
.th-beat__text {
  flex: 0 1 auto;
  font-size: 12px;
  color: var(--sc-text-secondary);
}
.th-hero__progress {
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.th-dot {
  width: 8px;
  height: 8px;
  flex: none;
  border-radius: 50%;
  background: var(--th-dot-color, var(--th-accent));
}

/* ===== Nearby list ===== */
.th-nearby {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}
.th-nearby__head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 12px;
}
.th-nearby__head h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--sc-text-secondary);
}
.th-nearby__head span {
  font-size: 11px;
  color: var(--sc-text-secondary);
  opacity: 0.8;
}
.th-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.th-row {
  display: grid;
  grid-template-columns: 2.25rem 1.75rem minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  padding: 7px 12px;
  border-radius: 10px;
  background: var(--th-row);
  border: 1px solid transparent;
}
.th-row.is-self {
  background: var(--th-self);
  border-color: color-mix(in srgb, var(--th-accent) 45%, transparent);
}
.th-rank {
  justify-self: center;
  min-width: 1.75rem;
  padding: 1px 6px;
  border-radius: 999px;
  text-align: center;
  font-size: 13px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--sc-text-secondary);
}
.th-row[data-sc-typing-heroes-podium] .th-rank {
  color: var(--sc-text-primary);
  background: color-mix(in srgb, var(--th-podium) 30%, transparent);
}
.th-row[data-sc-typing-heroes-podium='2'] .th-rank {
  background: color-mix(in srgb, var(--th-podium) 20%, transparent);
}
.th-row[data-sc-typing-heroes-podium='3'] .th-rank {
  background: color-mix(in srgb, var(--th-podium) 12%, transparent);
}
.th-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.75rem;
  height: 1.75rem;
  border-radius: 50%;
  font-size: 12px;
  font-weight: 700;
  color: var(--sc-text-primary);
  background: color-mix(in srgb, var(--th-dot-color, var(--sc-text-secondary)) 22%, transparent);
  border: 1px solid color-mix(in srgb, var(--th-dot-color, var(--sc-text-secondary)) 40%, transparent);
}
.th-row.is-self .th-avatar {
  background: color-mix(in srgb, var(--th-dot-color, var(--th-accent)) 30%, transparent);
  border-color: var(--th-dot-color, var(--th-accent));
}
.th-row__main {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.th-row__name {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  font-size: 13px;
}
.th-row__name-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.th-row.is-self .th-row__name-text {
  font-weight: 700;
}
.th-row__username {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: var(--sc-text-secondary);
}
.th-row__you {
  flex: none;
  padding: 0 6px;
  border-radius: 999px;
  font-size: 10px;
  color: var(--th-accent);
  background: color-mix(in srgb, var(--th-accent) 14%, transparent);
}
.th-row__bar {
  display: block;
  height: 4px;
  border-radius: 999px;
  background: var(--th-track);
  overflow: hidden;
}
.th-row__bar > span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: color-mix(in srgb, var(--sc-text-secondary) 55%, transparent);
}
.th-row.is-self .th-row__bar > span {
  background: var(--th-accent);
}
.th-row__value {
  font-size: 13px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  text-align: right;
  white-space: nowrap;
}
.th-row__value small {
  margin-left: 2px;
  font-size: 10px;
  font-weight: 400;
  color: var(--sc-text-secondary);
}

/* ===== Footer ===== */
.th-footer {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 10px;
  border-top: 1px solid var(--sc-border-mute);
}
.th-public {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  cursor: pointer;
}
.th-public > span {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.th-public__title {
  font-size: 13px;
}
.th-public__desc,
.th-rule {
  font-size: 11px;
  color: var(--sc-text-secondary);
}
.th-rule {
  margin: 0;
  opacity: 0.85;
}

@media (max-width: 640px) {
  .typing-heroes {
    padding: 10px 12px 16px;
  }
  .th-toolbar__group--subject {
    justify-content: flex-start;
  }
  .th-identity-select {
    max-width: none;
  }
  .th-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .th-hero {
    grid-template-columns: minmax(0, 1fr);
    gap: 12px;
    padding: 14px 16px;
  }
  .th-hero__rank {
    flex-direction: row;
    align-items: baseline;
    justify-content: flex-start;
    gap: 10px;
    padding: 0 0 10px;
    border-right: none;
    border-bottom: 1px solid var(--sc-border-mute);
  }
  .th-hero__rank-value {
    font-size: 32px;
  }
  .th-hero__value strong {
    font-size: 26px;
  }
  .th-row {
    grid-template-columns: 2rem minmax(0, 1fr) auto;
    gap: 8px;
    padding: 7px 10px;
  }
  .th-avatar {
    display: none;
  }
}
</style>
