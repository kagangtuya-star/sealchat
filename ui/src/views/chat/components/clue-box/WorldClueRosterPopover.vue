<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NBadge, NButton, NIcon, NInput, NPopover, NScrollbar, NSpin, useMessage } from 'naive-ui'
import { Plus, Search, UserMinus, UserPlus } from '@vicons/tabler'
import Avatar from '@/components/avatar.vue'
import { api } from '@/stores/_config'
import { useWorldClueStore } from '@/stores/worldClue'

interface WorldMemberCandidate {
  userId: string
  role: 'owner' | 'admin' | 'member' | 'spectator'
  username: string
  nickname: string
  avatar: string
}

const props = defineProps<{ worldId: string }>()
const store = useWorldClueStore()
const message = useMessage()
const show = ref(false)
const keyword = ref('')
const candidates = ref<WorldMemberCandidate[]>([])
const total = ref(0)
const loading = ref(false)
const submitting = ref('')
let searchTimer: ReturnType<typeof setTimeout> | undefined
let requestSeq = 0

const rosterIds = computed(() => new Set(store.roster.map(item => item.userId)))
const available = computed(() => candidates.value.filter(item => !rosterIds.value.has(item.userId)))

async function loadCandidates() {
  const worldId = props.worldId
  const seq = ++requestSeq
  loading.value = true
  try {
    const response = await api.get(`api/v1/worlds/${worldId}/members`, { params: { page: 1, pageSize: 100, keyword: keyword.value.trim() || undefined } })
    if (!show.value || props.worldId !== worldId || seq !== requestSeq) return
    candidates.value = response.data?.items || []
    total.value = Number(response.data?.total || candidates.value.length)
  } catch (error: any) {
    if (seq === requestSeq) message.error(error?.response?.data?.message || '世界成员加载失败')
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

watch(show, value => {
  if (!value) return
  void Promise.all([store.loadRoster(props.worldId), loadCandidates()]).catch(() => message.error('分发成员加载失败'))
})
watch(keyword, () => {
  if (!show.value) return
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => void loadCandidates(), 260)
})
onBeforeUnmount(() => { if (searchTimer) clearTimeout(searchTimer) })

async function add(userId: string) {
  submitting.value = userId
  try { await store.addRosterMember(props.worldId, userId) }
  catch (error: any) { message.error(error?.response?.data?.message || '添加失败') }
  finally { submitting.value = '' }
}
async function remove(userId: string) {
  submitting.value = userId
  try { await store.removeRosterMember(props.worldId, userId) }
  catch (error: any) { message.error(error?.response?.data?.message || '移除失败') }
  finally { submitting.value = '' }
}
const nameOf = (item: WorldMemberCandidate) => item.nickname || item.username || item.userId
</script>

<template>
  <NPopover v-model:show="show" trigger="click" placement="bottom-end" :width="360" :z-index="4300" raw>
    <template #trigger>
      <NBadge class="roster-count-badge" :value="store.roster.length" :show="store.roster.length > 0" :max="99" :offset="[-3, 4]">
        <NButton circle quaternary size="small" title="分发成员">
          <template #icon><NIcon><UserPlus /></NIcon></template>
        </NButton>
      </NBadge>
    </template>
    <div class="roster-popover" @pointerdown.stop>
      <header><strong>分发成员</strong><span>{{ store.roster.length }} 人</span></header>
      <NInput v-model:value="keyword" clearable placeholder="搜索世界成员...">
        <template #prefix><NIcon><Search /></NIcon></template>
      </NInput>
      <NScrollbar style="max-height: 420px">
        <section>
          <h4>已加入</h4>
          <div v-if="!store.roster.length" class="roster-empty">暂无成员</div>
          <div v-for="item in store.roster" :key="item.userId" class="roster-row">
            <Avatar :src="item.avatar" :fallback-text="nameOf(item)" :size="30" :border="false" use-text-fallback />
            <span><b>{{ nameOf(item) }}</b><small>{{ item.username || item.userId }}</small></span>
            <NButton text type="error" :loading="submitting === item.userId" @click="remove(item.userId)"><template #icon><NIcon><UserMinus /></NIcon></template>移除</NButton>
          </div>
        </section>
        <section>
          <h4>世界成员</h4>
          <NSpin :show="loading">
            <div v-if="!loading && !available.length" class="roster-empty">没有匹配的成员</div>
            <div v-for="item in available" :key="item.userId" class="roster-row">
              <Avatar :src="item.avatar" :fallback-text="nameOf(item)" :size="30" :border="false" use-text-fallback />
              <span><b>{{ nameOf(item) }}</b><small>{{ item.username || item.userId }}</small></span>
              <NButton text type="primary" :loading="submitting === item.userId" @click="add(item.userId)"><template #icon><NIcon><Plus /></NIcon></template>添加</NButton>
            </div>
          </NSpin>
          <div v-if="total > candidates.length" class="roster-limit">显示前 {{ candidates.length }} / {{ total }} 人，请搜索以缩小范围</div>
        </section>
      </NScrollbar>
    </div>
  </NPopover>
</template>

<style scoped>
.roster-popover { width: 360px; padding: 14px; color: var(--sc-text-primary); border: 1px solid var(--sc-border-strong); border-radius: 7px; background: color-mix(in srgb, var(--sc-bg-surface) 97%, transparent); box-shadow: 0 12px 36px #0005; backdrop-filter: blur(14px); }
.roster-popover header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.roster-popover header span, h4, .roster-empty, .roster-limit, .roster-row small { color: var(--sc-text-secondary); font-size: 12px; }
h4 { margin: 15px 0 6px; font-weight: 600; }
.roster-row { display: grid; grid-template-columns: 30px minmax(0, 1fr) auto; align-items: center; gap: 9px; min-height: 44px; padding: 5px 3px; border-bottom: 1px solid var(--sc-border-mute); }
.roster-row > span { min-width: 0; }
.roster-row b, .roster-row small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.roster-empty, .roster-limit { padding: 9px 3px; }
.roster-count-badge :deep(.n-badge-sup) { color: var(--sc-text-secondary); background: var(--sc-bg-elevated); border: 1px solid var(--sc-border-mute); box-shadow: none; font-size: 10px; }
@media (max-width: 420px) { .roster-popover { box-sizing: border-box; width: calc(100vw - 24px); } }
</style>
