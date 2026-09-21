<script setup lang="ts">
import { useUtilsStore } from '@/stores/utils'
import type { CertificateChallenge, CertificateConfig, CertificateIssuer, CertificateLogEntry, CertificateStatus } from '@/types'
import { cloneDeep } from 'lodash-es'
import { useDialog, useMessage } from 'naive-ui'
import { computed, onMounted, ref, watch } from 'vue'

type CertificateConfigResponse = {
  config: CertificateConfig
  restartRequired?: boolean
}

const utils = useUtilsStore()
const message = useMessage()
const dialog = useDialog()

const defaultConfig = (): CertificateConfig => ({
  enabled: false,
  subjectIp: '',
  issuer: 'letsencrypt_shortlived',
  challenge: 'http-01',
  email: '',
  storageDir: './data/certmagic',
  httpsServeAt: '',
  forceHTTPS: true,
  redirectHTTP: true,
  checkIntervalMinutes: 360,
  renewBeforeDays: 3,
  retryInitialMinutes: 5,
  retryMaxMinutes: 240,
  zeroSSLAPIKey: '',
  zeroSSLEABKeyID: '',
  zeroSSLEABMACKey: '',
  staging: false,
})

const model = ref<CertificateConfig>(defaultConfig())
const status = ref<CertificateStatus | null>(null)
const logs = ref<CertificateLogEntry[]>([])
const loading = ref(false)
const saving = ref(false)
const obtaining = ref(false)
const originalSnapshot = ref('')
const restartRequired = ref(false)
const clearZeroSSLAPIKey = ref(false)
const clearZeroSSLEABMACKey = ref(false)

const issuerOptions: { label: string; value: CertificateIssuer }[] = [
  { label: 'Let’s Encrypt 短期证书', value: 'letsencrypt_shortlived' },
  { label: 'ZeroSSL 90 天证书', value: 'zerossl_90d' },
]

const challengeOptions = computed<{ label: string; value: CertificateChallenge }[]>(() => (
  model.value.issuer === 'zerossl_90d'
    ? [{ label: 'HTTP-01（使用 80 端口）', value: 'http-01' }]
    : [
        { label: 'HTTP-01（使用 80 端口）', value: 'http-01' },
        { label: 'TLS-ALPN-01（使用 443 端口）', value: 'tls-alpn-01' },
      ]
))

const minimumRenewBeforeDays = computed(() => (
  model.value.issuer === 'letsencrypt_shortlived' ? 3 : 1
))
const maximumRenewBeforeDays = computed(() => (
  model.value.issuer === 'letsencrypt_shortlived' ? 6 : undefined
))

const normalizeConfig = (value?: Partial<CertificateConfig> | null): CertificateConfig => ({
  ...defaultConfig(),
  ...(value || {}),
  issuer: (value?.issuer === 'zerossl_90d' ? 'zerossl_90d' : 'letsencrypt_shortlived'),
  challenge: (value?.challenge === 'tls-alpn-01' ? 'tls-alpn-01' : 'http-01'),
})

const snapshotOf = (value: CertificateConfig) => JSON.stringify(value)
const isModified = computed(() => (
  snapshotOf(model.value) !== originalSnapshot.value ||
  clearZeroSSLAPIKey.value ||
  clearZeroSSLEABMACKey.value
))

const formatCertificateTime = (value?: string | null) => {
  if (!value || value.startsWith('0001-01-01')) return '未知'
  return new Date(value).toLocaleString()
}

watch(() => model.value.issuer, (issuer, previousIssuer) => {
  if (issuer === 'zerossl_90d') {
    model.value.challenge = 'http-01'
    if (previousIssuer === 'letsencrypt_shortlived') {
      model.value.renewBeforeDays = 14
    }
  } else if (previousIssuer === 'zerossl_90d') {
    model.value.renewBeforeDays = 3
  }
  if (model.value.renewBeforeDays < minimumRenewBeforeDays.value) {
    model.value.renewBeforeDays = minimumRenewBeforeDays.value
  }
})

const statusRows = computed(() => {
  const item = status.value
  if (!item) return []
  return [
    ['运行状态', item.runtimeActive ? '已启用' : (item.enabled ? '等待重启生效' : '未启用')],
    ['公网 IP', item.subjectIp || '未配置'],
    ['签发方', item.issuer || '未配置'],
    ['验证方式', item.challenge || '未配置'],
    ['证书缓存', item.certificatePresent ? '已找到' : '未找到'],
    ['剩余天数', item.certificatePresent ? `${item.remainingDays} 天` : '未知'],
    ['生效时间', formatCertificateTime(item.notBefore)],
    ['过期时间', formatCertificateTime(item.notAfter)],
    ['最近检查', formatCertificateTime(item.lastCheckAt)],
    ['最近成功', formatCertificateTime(item.lastSuccessAt)],
    ['下次检查', formatCertificateTime(item.nextCheckAt)],
    ['重试状态', item.retrying ? '重试中' : '正常周期'],
    ['重试次数', `${item.retryCount ?? 0}`],
    ['检查周期', `${item.checkIntervalMinutes ?? 0} 分钟`],
    ['续期阈值', `${item.renewBeforeDays ?? 0} 天`],
    ['重试初始间隔', `${item.retryInitialMinutes ?? 0} 分钟`],
    ['重试最大间隔', `${item.retryMaxMinutes ?? 0} 分钟`],
  ]
})

const load = async () => {
  loading.value = true
  try {
    const [configResp, statusResp, logsResp] = await Promise.all([
      utils.adminCertificateConfigGet(),
      utils.adminCertificateStatus(),
      utils.adminCertificateLogs(100),
    ])
    model.value = normalizeConfig((configResp.data as CertificateConfigResponse).config)
    originalSnapshot.value = snapshotOf(model.value)
    restartRequired.value = Boolean((configResp.data as CertificateConfigResponse).restartRequired)
    status.value = statusResp.data?.status || null
    restartRequired.value = Boolean(statusResp.data?.restartRequired ?? restartRequired.value)
    logs.value = logsResp.data?.items || []
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '加载证书配置失败')
  } finally {
    loading.value = false
  }
}

const save = async () => {
  saving.value = true
  try {
    const payload = cloneDeep(model.value)
    const resp = await utils.adminCertificateConfigUpdate(payload, {
      clearZeroSSLAPIKey: clearZeroSSLAPIKey.value,
      clearZeroSSLEABMACKey: clearZeroSSLEABMACKey.value,
    })
    model.value = normalizeConfig((resp.data as CertificateConfigResponse).config)
    originalSnapshot.value = snapshotOf(model.value)
    restartRequired.value = Boolean(resp.data?.restartRequired)
    clearZeroSSLAPIKey.value = false
    clearZeroSSLEABMACKey.value = false
    message.success(resp.data?.restartRequired ? '已保存，重启服务后生效' : '已保存')
    await refreshStatus()
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '保存证书配置失败')
  } finally {
    saving.value = false
  }
}

const refreshStatus = async () => {
  try {
    const [statusResp, logsResp] = await Promise.all([
      utils.adminCertificateStatus(),
      utils.adminCertificateLogs(100),
    ])
    status.value = statusResp.data?.status || null
    restartRequired.value = Boolean(statusResp.data?.restartRequired)
    logs.value = logsResp.data?.items || []
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '刷新证书状态失败')
  }
}

const obtainNow = async (force = false) => {
  obtaining.value = true
  try {
    const resp = await utils.adminCertificateObtain(force)
    status.value = resp.data?.status || status.value
    message.success(force ? '已强制触发证书重新申请' : '已完成证书检查与续期')
    await refreshStatus()
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '触发证书检查失败')
  } finally {
    obtaining.value = false
  }
}

const confirmForceRenew = () => {
  dialog.warning({
    title: '强制重新申请证书',
    content: '此操作会向 CA 重新签发证书，频繁操作可能触发签发频率限制。确定继续吗？',
    positiveText: '强制重新申请',
    negativeText: '取消',
    onPositiveClick: () => obtainNow(true),
  })
}

onMounted(load)

defineExpose({
  save,
  isModified: () => isModified.value,
})
</script>

<template>
  <div class="admin-certificate">
    <n-alert type="warning" title="公网 IP 证书签发限制" class="admin-certificate__notice">
      仅支持单个公网 IP。HTTPS 监听地址留空时会在原服务端口上同时支持 HTTPS；本地 HTTP 访问仍保留，保存后需要重启服务。
    </n-alert>
    <n-alert v-if="restartRequired" type="warning" title="配置已变更，重启后生效" class="admin-certificate__notice" />

    <div class="admin-certificate__grid">
      <n-card title="证书配置" :bordered="false" class="admin-certificate__card admin-certificate__config-card">
        <n-spin :show="loading || saving">
          <n-form label-placement="left" label-width="112">
            <n-form-item label="启用 IP 证书">
              <n-switch v-model:value="model.enabled" />
            </n-form-item>
            <n-form-item label="公网 IP" feedback="必须是公网 IPv4 或 IPv6，不能是内网、回环或保留地址。">
              <n-input v-model:value="model.subjectIp" placeholder="例如 8.8.8.8" />
            </n-form-item>
            <n-form-item label="签发方">
              <n-select v-model:value="model.issuer" :options="issuerOptions" />
            </n-form-item>
            <n-form-item label="验证方式">
              <n-select v-model:value="model.challenge" :options="challengeOptions" />
            </n-form-item>
            <n-form-item label="联系邮箱">
              <n-input v-model:value="model.email" placeholder="admin@example.com" />
            </n-form-item>
            <n-form-item label="证书缓存目录">
              <n-input v-model:value="model.storageDir" placeholder="./data/certmagic" />
            </n-form-item>
            <n-form-item label="HTTPS 监听地址" feedback="留空复用服务地址端口，例如 https://公网IP:3212；也可填 :443 或 :8443 使用独立 HTTPS 端口。">
              <n-input v-model:value="model.httpsServeAt" placeholder="留空复用服务地址，或填 :443 / :8443" />
            </n-form-item>
            <n-form-item label="强制 HTTPS">
              <n-switch v-model:value="model.forceHTTPS" />
            </n-form-item>
            <n-form-item label="HTTP 重定向">
              <n-switch v-model:value="model.redirectHTTP" />
            </n-form-item>
            <n-form-item label="检查周期（分钟）" feedback="自动续期守护完成一轮成功检查后，按此周期进入下一轮。">
              <n-input-number v-model:value="model.checkIntervalMinutes" :min="1" />
            </n-form-item>
            <n-form-item label="最小续期阈值（天）" feedback="剩余天数小于等于该值时，自动续期守护会主动触发证书检查。Let’s Encrypt 短期证书允许 3 至 6 天。">
              <n-input-number v-model:value="model.renewBeforeDays" :min="minimumRenewBeforeDays" :max="maximumRenewBeforeDays" />
            </n-form-item>
            <n-form-item label="重试初始间隔（分钟）" feedback="自动续期检查失败后，从此间隔开始指数退避重试。">
              <n-input-number v-model:value="model.retryInitialMinutes" :min="1" />
            </n-form-item>
            <n-form-item label="重试最大间隔（分钟）" feedback="指数退避重试不会超过此上限。">
              <n-input-number v-model:value="model.retryMaxMinutes" :min="1" />
            </n-form-item>

            <template v-if="model.issuer === 'zerossl_90d'">
              <n-divider>ZeroSSL 凭据</n-divider>
              <n-form-item label="API Key" feedback="已保存的密钥不会回显；留空表示保留旧值。完整 EAB 凭据优先于 API Key。">
                <n-input v-model:value="model.zeroSSLAPIKey" type="password" show-password-on="click" placeholder="ZeroSSL API Key" />
              </n-form-item>
              <n-form-item label="清除 API Key">
                <n-checkbox v-model:checked="clearZeroSSLAPIKey">保存时清除已保存的 API Key</n-checkbox>
              </n-form-item>
              <n-form-item label="EAB Key ID">
                <n-input v-model:value="model.zeroSSLEABKeyID" placeholder="ZeroSSL EAB Key ID" />
              </n-form-item>
              <n-form-item label="EAB MAC Key" feedback="已保存的密钥不会回显；留空表示保留旧值。">
                <n-input v-model:value="model.zeroSSLEABMACKey" type="password" show-password-on="click" placeholder="ZeroSSL EAB MAC Key" />
              </n-form-item>
              <n-form-item label="清除 EAB MAC">
                <n-checkbox v-model:checked="clearZeroSSLEABMACKey">保存时清除已保存的 EAB MAC Key</n-checkbox>
              </n-form-item>
            </template>
            <n-form-item v-if="model.issuer === 'letsencrypt_shortlived'" label="测试环境">
              <n-switch v-model:value="model.staging" />
            </n-form-item>
          </n-form>
        </n-spin>
      </n-card>

      <div class="admin-certificate__side">
        <n-card title="当前状态" :bordered="false" class="admin-certificate__card">
          <div class="admin-certificate__actions">
            <n-button size="small" @click="refreshStatus">刷新</n-button>
            <n-button size="small" type="primary" :loading="obtaining" :disabled="restartRequired" @click="obtainNow(false)">检查与续期</n-button>
            <n-button size="small" type="error" secondary :disabled="restartRequired || obtaining" @click="confirmForceRenew">强制重新申请</n-button>
          </div>
          <div v-if="status?.lastError" class="admin-certificate__error">{{ status.lastError }}</div>
          <div v-for="row in statusRows" :key="row[0]" class="admin-certificate__status-row">
            <span>{{ row[0] }}</span>
            <strong>{{ row[1] }}</strong>
          </div>
        </n-card>

        <n-card title="最近日志" :bordered="false" class="admin-certificate__card admin-certificate__logs">
          <n-empty v-if="!logs.length" description="暂无证书日志" />
          <div v-for="item in logs" :key="`${item.time}-${item.event}-${item.message}`" class="admin-certificate__log">
            <div class="admin-certificate__log-meta">
              <n-tag size="small" :type="item.level === 'error' ? 'error' : 'info'">{{ item.level }}</n-tag>
              <span>{{ item.event }}</span>
              <time>{{ new Date(item.time).toLocaleString() }}</time>
            </div>
            <p>{{ item.message }}</p>
          </div>
        </n-card>
      </div>
    </div>
  </div>
</template>

<style scoped>
.admin-certificate {
  height: auto;
  min-height: 0;
  max-height: 61vh;
  overflow-x: hidden;
  overflow-y: auto;
  padding: 4px 8px 18px 4px;
  scrollbar-gutter: stable;
}

.admin-certificate__notice {
  margin-bottom: 12px;
}

.admin-certificate__grid {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(320px, 0.8fr);
  gap: 14px;
  min-height: 0;
  align-items: start;
}

.admin-certificate__card {
  background: var(--n-color);
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.08);
}

.admin-certificate__config-card :deep(.n-card__content) {
  padding-bottom: 10px;
}

.admin-certificate__config-card :deep(.n-form-item) {
  margin-bottom: 14px;
}

.admin-certificate__config-card :deep(.n-form-item-feedback-wrapper) {
  min-height: 18px;
}

.admin-certificate__side {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
}

.admin-certificate__actions {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}

.admin-certificate__error {
  margin-bottom: 12px;
  color: #dc2626;
  font-size: 13px;
}

.admin-certificate__status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid rgba(148, 163, 184, 0.22);
}

.admin-certificate__status-row span {
  color: var(--n-text-color-3);
}

.admin-certificate__logs {
  max-height: min(360px, 34vh);
  overflow: hidden;
}

.admin-certificate__logs :deep(.n-card__content) {
  max-height: min(300px, 28vh);
  overflow-y: auto;
  scrollbar-gutter: stable;
}

.admin-certificate__log {
  padding: 10px 0;
  border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.admin-certificate__log-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--n-text-color-3);
  font-size: 12px;
}

.admin-certificate__log p {
  margin: 6px 0 0;
  word-break: break-word;
}

@media (max-width: 960px) {
  .admin-certificate__grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .admin-certificate {
    padding-right: 4px;
  }

  .admin-certificate__actions {
    flex-wrap: wrap;
  }

  .admin-certificate__status-row {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }
}
</style>
