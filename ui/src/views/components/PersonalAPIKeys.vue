<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NPopconfirm, useMessage } from 'naive-ui'
import { useUserStore } from '@/stores/user'
import { mcpOAuth, personalAPIKeys, type MCPOAuthGrant, type PersonalAPIKey, type PersonalAPIKeyCatalog } from '@/api/mcp'

const emit = defineEmits<{ close: [] }>()
const user = useUserStore()
const message = useMessage()
const catalog = ref<PersonalAPIKeyCatalog | null>(null)
const grants = ref<MCPOAuthGrant[]>([])
const grantLoadFailed = ref(false)
const busy = ref(false)
const name = ref('')
const scopes = ref<string[]>([])
const neverExpires = ref(false)
const editingID = ref('')
const plainKey = ref('')
let epoch = 0
const endpoint = computed(() => catalog.value ? new URL(catalog.value.path, window.location.origin).href : '')
const resetSecret = () => { plainKey.value = '' }
const resetDraft = () => {
  name.value = ''; editingID.value = ''; neverExpires.value = false
  scopes.value = (catalog.value?.allowedScopes || []).filter(scope => scope.endsWith(':read'))
}
const reportError = (error: unknown) => {
  const text = (error as { response?: { data?: { message?: string } } })?.response?.data?.message
  message.error(text || '个人 Key 操作失败')
}
const load = async (generation: number) => {
  const [response, connections] = await Promise.allSettled([personalAPIKeys.list(), mcpOAuth.grants()])
  if (generation !== epoch) return
  if (response.status === 'fulfilled') catalog.value = response.value.data
  grantLoadFailed.value = connections.status === 'rejected'
  if (connections.status === 'fulfilled') grants.value = connections.value.data.items
  else reportError(connections.reason)
  // A failed OAuth list must not disable the existing PAT management area.
  if (response.status === 'rejected') throw response.reason
}
watch(() => [user.info.id, user.token], async () => {
  const generation = ++epoch
  resetSecret(); catalog.value = null; grants.value = []; grantLoadFailed.value = false; resetDraft(); busy.value = true
  try { await load(generation); if (generation === epoch) resetDraft() }
  catch (error) { if (generation === epoch) reportError(error) }
  finally { if (generation === epoch) busy.value = false }
}, { immediate: true })
onBeforeUnmount(() => { ++epoch; resetSecret() })
const save = async () => {
  const generation = epoch
  if (busy.value || !catalog.value?.enabled || !name.value.trim()) return
  busy.value = true; resetSecret()
  try {
    if (editingID.value) await personalAPIKeys.update(editingID.value, { name: name.value, scopes: [...scopes.value] })
    else {
      const response = await personalAPIKeys.create({ name: name.value, scopes: [...scopes.value], neverExpires: neverExpires.value })
      if (generation === epoch) plainKey.value = response.data.token
    }
    await load(generation)
    if (generation === epoch) resetDraft()
  } catch (error) { if (generation === epoch) reportError(error) }
  finally { if (generation === epoch) busy.value = false }
}
const edit = (key: PersonalAPIKey) => {
  if (busy.value || !catalog.value?.enabled) return
  resetSecret(); editingID.value = key.id; name.value = key.name; scopes.value = [...key.scopes]
}
const mutate = async (key: PersonalAPIKey, rotate: boolean) => {
  if (busy.value || (rotate && !catalog.value?.enabled)) return
  const generation = epoch
  busy.value = true; resetSecret()
  try {
    if (rotate) { const response = await personalAPIKeys.rotate(key.id); if (generation === epoch) plainKey.value = response.data.token }
    else await personalAPIKeys.revoke(key.id)
    await load(generation)
    if (generation === epoch && editingID.value === key.id) resetDraft()
  } catch (error) { if (generation === epoch) reportError(error) }
  finally { if (generation === epoch) busy.value = false }
}
const copy = async () => {
  try { await navigator.clipboard.writeText(plainKey.value); message.success('已复制') }
  catch { message.error('无法访问剪贴板，请选中下方 Key 手动复制') }
}
const revokeGrant = async (grant: MCPOAuthGrant) => {
  if (busy.value || grant.revokedAt) return
  const generation = epoch
  busy.value = true
  try {
    await mcpOAuth.revoke(grant.id)
    await load(generation)
  } catch (error) { if (generation === epoch) reportError(error) }
  finally { if (generation === epoch) busy.value = false }
}
const grantStatus = (grant: MCPOAuthGrant) => grant.revokedAt ? '已撤销' : new Date(grant.refreshExpiresAt).getTime() <= Date.now() ? '已到期' : '已连接'
const close = () => { ++epoch; resetSecret(); emit('close') }
const formatDate = (value: string | null) => value ? new Date(value).toLocaleDateString() : '不过期'
</script>

<template>
  <section class="personal-keys" aria-label="Personal API Key 管理">
    <div class="personal-keys-header"><h3>Personal API Keys</h3><n-button size="small" @click="close">关闭 Key 管理</n-button></div>
    <p v-if="catalog">MCP 地址：<span class="key-url">{{ endpoint }}</span></p>
    <n-alert v-if="catalog && !catalog.enabled" type="info">平台 MCP 已关闭。仍可查看和撤销已有 Key 和 OAuth 连接。</n-alert>
    <n-alert v-if="plainKey" type="warning" class="key-secret">
      <p>完整 Key 仅此次显示。关闭后无法再次读取；请保存在自己的凭证管理工具中。</p>
      <n-input :value="plainKey" readonly type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" aria-label="本次创建或轮换的完整 Key" />
      <n-space class="key-actions"><n-button @click="copy">复制 Key</n-button><n-button @click="resetSecret">清除明文</n-button></n-space>
    </n-alert>
    <n-spin :show="busy">
      <div v-if="catalog?.enabled" class="key-form">
        <label>Key 名称<n-input v-model:value="name" placeholder="例如：我的桌面客户端" :maxlength="100" :disabled="busy" autocomplete="off" /></label>
        <p>默认仅选择平台当前开放的只读权限。灰色项表示平台未开放，并非你的账号权限不足；写入与额外授权需要主动勾选，实际业务权限仍实时校验。</p>
        <n-checkbox-group v-model:value="scopes">
          <div class="key-scopes">
            <n-checkbox v-for="scope in catalog.catalog" :key="scope.id" :value="scope.id" :disabled="busy || (!catalog.allowedScopes.includes(scope.id) && (!editingID || !scopes.includes(scope.id)))">
              <span>{{ scope.description }}<small>{{ scope.id }}{{ scope.extra ? ' · 额外授权' : '' }}{{ !catalog.allowedScopes.includes(scope.id) ? ' · 平台未开放，当前不生效' : '' }}</small></span>
            </n-checkbox>
          </div>
        </n-checkbox-group>
        <p>battle_report:generate 会调用原生 AI，可能计费并创建世界共享战报；clue:publish 会改变其他用户可见的线索。</p>
        <n-checkbox v-if="!editingID" v-model:checked="neverExpires" :disabled="busy">明确选择不过期（默认有效期 90 天）</n-checkbox>
        <n-space class="key-actions">
          <n-button type="primary" :disabled="busy || !name.trim() || !catalog.enabled" @click="save">{{ editingID ? '保存名称和授权' : '创建 Key' }}</n-button>
          <n-button v-if="editingID" @click="resetDraft">取消编辑</n-button>
        </n-space>
      </div>
      <article v-for="key in catalog?.items || []" :key="key.id" class="key-entry">
        <strong>{{ key.name }}</strong><span> ···{{ key.tail }} · {{ key.revokedAt ? '已撤销' : `到期：${formatDate(key.expiresAt)}` }}</span>
        <small>{{ key.scopes.join('、') || '仅基础发现能力' }}</small>
        <small>创建：{{ formatDate(key.createdAt) }} · 最近使用：{{ key.lastUsedAt ? formatDate(key.lastUsedAt) : '尚未使用' }}</small>
        <n-space v-if="!key.revokedAt" class="key-actions">
          <n-button v-if="catalog?.enabled" size="small" :disabled="busy" @click="edit(key)">编辑</n-button>
          <n-button v-if="catalog?.enabled" size="small" :disabled="busy" @click="mutate(key, true)">轮换（旧 Key 立即失效）</n-button>
          <n-button size="small" type="error" :disabled="busy" @click="mutate(key, false)">撤销</n-button>
        </n-space>
      </article>
      <div class="oauth-connections" aria-label="OAuth 连接管理">
        <h3>OAuth 连接</h3>
        <small v-if="grantLoadFailed">OAuth 连接暂时无法读取</small>
        <small v-else-if="!grants.length">暂无 OAuth 连接</small>
        <article v-for="grant in grants" :key="grant.id" class="key-entry">
          <strong>{{ grant.client }}</strong><span> · {{ grantStatus(grant) }}</span>
          <small>{{ grant.scopes.join('、') || '仅基础发现能力' }}</small>
          <small>创建：{{ formatDate(grant.createdAt) }} · 最近使用：{{ grant.lastUsedAt ? formatDate(grant.lastUsedAt) : '尚未使用' }}</small>
          <small>刷新授权到期：{{ formatDate(grant.refreshExpiresAt) }}</small>
          <NPopconfirm v-if="!grant.revokedAt" @positive-click="revokeGrant(grant)">
            <template #trigger><n-button size="small" type="error" class="key-actions" :disabled="busy">撤销连接</n-button></template>
            撤销后此连接立即失效，需要在 ChatGPT 中重新连接并授权。
          </NPopconfirm>
        </article>
      </div>
    </n-spin>
  </section>
</template>

<style scoped>
.personal-keys { min-width: 0; max-width: 680px; margin: 16px 0; }
.personal-keys-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.personal-keys h3 { margin: 0; }
.personal-keys p { margin: 12px 0; }
.key-url, .key-entry small { overflow-wrap: anywhere; }
.key-form, .key-entry { padding: 16px 0; border-bottom: 1px solid var(--sc-border-color, #ddd); }
.key-scopes { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; }
.key-scopes small, .key-entry small { display: block; color: var(--sc-text-secondary); }
.key-actions { margin-top: 12px; }
.key-secret { margin: 16px 0; }
.oauth-connections { margin-top: 16px; }
</style>
