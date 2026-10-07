<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { isAxiosError } from 'axios'
import { NAlert, NButton, NCard, NScrollbar, NSpace, NSpin } from 'naive-ui'
import { mcpOAuth, type MCPOAuthConsentRequest } from '@/api/mcp'
import { useUserStore } from '@/stores/user'

const route = useRoute()
const router = useRouter()
const user = useUserStore()
const consent = ref<MCPOAuthConsentRequest | null>(null)
const loading = ref(false)
const submitting = ref(false)
const error = ref('')
const requestId = computed(() => typeof route.query.request === 'string' ? route.query.request : '')
const expired = computed(() => !consent.value || Date.parse(consent.value.expiresAt) <= Date.now())

function risk(scope: string): string {
  if (scope === 'battle_report:generate') return '可能触发 AI 调用和额度消耗'
  if (scope === 'clue:publish') return '会改变其他用户看到的线索状态'
  if (scope.endsWith(':write')) return '允许修改对应数据'
  return ''
}

function signIn(id: string) {
  return router.replace({ name: 'user-signin', query: { mcpOAuthRequest: id } })
}

watch(requestId, async (id, _, onCleanup) => {
  let active = true
  onCleanup(() => { active = false })
  consent.value = null
  error.value = ''
  if (!/^[A-Za-z0-9_-]{43}$/.test(id)) {
    error.value = '授权请求无效，请从 ChatGPT 重新连接。'
    return
  }
  if (!user.token) { await signIn(id); return }
  loading.value = true
  try {
    await user.infoUpdate()
    const response = await mcpOAuth.request(id)
    if (active) consent.value = response.data
  } catch (e: unknown) {
    if (!active) return
    if (isAxiosError(e) && e.response?.status === 401) { await signIn(id); return }
    error.value = '授权请求已过期或无法读取，请从 ChatGPT 重新连接。'
  } finally {
    if (active) loading.value = false
  }
}, { immediate: true })

async function decide(decision: 'allow' | 'deny') {
  if (submitting.value || expired.value) return
  const id = requestId.value
  submitting.value = true
  error.value = ''
  try {
    const response = await mcpOAuth.decide(id, decision)
    if (id !== requestId.value) return
    window.location.assign(response.data.redirectUrl)
  } catch (e: unknown) {
    if (id !== requestId.value) return
    if (isAxiosError(e) && e.response?.status === 401) { await signIn(id); return }
    error.value = '授权未完成，请求可能已过期或权限已改变，请从 ChatGPT 重新连接。'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="oauth-consent">
    <NCard title="ChatGPT 请求连接 SealChat">
      <NSpin :show="loading">
        <NSpace vertical :size="16">
          <NAlert v-if="error" type="error">{{ error }}</NAlert>
          <template v-if="consent">
            <p>当前账号：{{ user.info.nick || user.info.username }}</p>
            <NSpace class="oauth-consent-actions">
              <NButton type="primary" :loading="submitting" :disabled="expired" @click="decide('allow')">允许</NButton>
              <NButton :disabled="submitting || expired" @click="decide('deny')">拒绝</NButton>
            </NSpace>
            <NScrollbar class="oauth-scope-scrollbar">
              <div class="oauth-scope-list">
                <p>请求权限</p>
                <ul v-if="consent.requestedScopes.length">
                  <li v-for="scope in consent.requestedScopes" :key="scope.id">
                    {{ scope.description }} <code>{{ scope.id }}</code>
                    <NAlert v-if="risk(scope.id)" type="warning" :show-icon="true">{{ risk(scope.id) }}</NAlert>
                  </li>
                </ul>
                <p v-else>仅访问账号与世界、频道的基础发现信息。</p>
              </div>
            </NScrollbar>
          </template>
        </NSpace>
      </NSpin>
    </NCard>
  </main>
</template>

<style scoped>
.oauth-consent { max-width: 560px; margin: 24px auto; padding: 0 16px 24px; box-sizing: border-box; }
.oauth-consent-actions { flex: none; }
.oauth-scope-scrollbar { max-height: calc(100dvh - 250px); }
.oauth-scope-list { padding-right: 8px; }
p { margin: 0 0 8px; }
ul { margin: 0; padding-left: 20px; }
li { margin-bottom: 16px; }
code { display: block; font-size: 12px; margin: 4px 0; }

@media (max-height: 560px) {
  .oauth-consent { margin-top: 12px; padding-bottom: 12px; }
  .oauth-scope-scrollbar { max-height: calc(100dvh - 210px); }
}
</style>
