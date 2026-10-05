<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { cloneDeep } from 'lodash-es'
import { useUtilsStore } from '@/stores/utils'
import type { MCPConfig, MCPMode } from '@/api/mcp'

const utils = useUtilsStore()
const message = useMessage()
const draft = ref<MCPConfig>({ enabled: false, chat: 'off', search: 'off', battleReport: 'off', clue: 'off', glossary: 'off', identity: 'off', audio: 'off', note: 'off', file: 'off', battleReportGenerate: false, cluePublish: false, callsPerMinute: 120, writesPerMinute: 30 })
const initial = ref('')
const loading = ref(true)
const modules: { key: 'chat' | 'search' | 'battleReport' | 'clue' | 'glossary' | 'identity' | 'audio' | 'note' | 'file'; label: string; scopes: string; modes: MCPMode[] }[] = [
  { key: 'chat', label: '聊天记录', scopes: 'chat:read', modes: ['off', 'read'] },
  { key: 'search', label: '消息搜索', scopes: 'search:read', modes: ['off', 'read'] },
  { key: 'battleReport', label: '战报总结', scopes: 'battle_report:read / battle_report:write', modes: ['off', 'read', 'write'] },
  { key: 'clue', label: '线索箱', scopes: 'clue:read / clue:write', modes: ['off', 'read', 'write'] },
  { key: 'glossary', label: '世界术语', scopes: 'glossary:read / glossary:write', modes: ['off', 'read', 'write'] },
  { key: 'identity', label: '频道角色', scopes: 'identity:read / identity:write', modes: ['off', 'read', 'write'] },
  { key: 'audio', label: '音频工作台', scopes: 'audio:read / audio:write', modes: ['off', 'read', 'write'] },
  { key: 'note', label: '频道便签', scopes: 'note:read / note:write', modes: ['off', 'read', 'write'] },
  { key: 'file', label: '基础文件上传', scopes: 'file:write', modes: ['off', 'write'] },
]
const modeLabels = { off: '关闭', read: '只读', write: '读写' }
const endpoint = computed(() => {
  const config = utils.config
  const domain = config?.domain || window.location.origin
  const origin = domain.includes('://') ? new URL(domain).origin : `${window.location.protocol}//${domain}`
  const base = (config?.webUrl || '').replace(/^\/+|\/+$/g, '')
  return `${origin}/${base ? `${base}/` : ''}mcp`
})
onMounted(async () => {
  try { await utils.configGet(); Object.assign(draft.value, utils.config?.mcp); initial.value = JSON.stringify(draft.value) }
  catch { message.error('读取 MCP 配置失败') }
  finally { loading.value = false }
})
const isModified = () => !loading.value && initial.value !== JSON.stringify(draft.value)
const save = async () => {
  if (loading.value) return
  loading.value = true
  try {
    await utils.configGet()
    if (!utils.config) throw new Error('配置不可用')
    const config = cloneDeep(utils.config)
    config.mcp = cloneDeep(draft.value)
    await utils.configSet(config)
    initial.value = JSON.stringify(draft.value)
    message.success('MCP 配置已保存')
  } catch { message.error('保存失败，请检查限流数值和配置存储状态') }
  finally { loading.value = false }
}
defineExpose({ save, isModified })
</script>

<template>
  <div class="mcp-settings">
    <n-spin :show="loading">
      <n-form label-placement="top">
        <n-form-item label="平台 MCP 接入"><n-switch v-model:value="draft.enabled" /></n-form-item>
        <n-form-item label="接入地址"><n-input :value="endpoint" readonly /></n-form-item>
        <p>用户在个人信息中自行创建 Personal API Key。权限始终取平台能力、个人授权与当前业务权限的交集。</p>
        <div v-for="module in modules" :key="module.key" class="mcp-module">
          <div><strong>{{ module.label }}</strong><small>{{ module.scopes }}</small></div>
          <n-select v-model:value="draft[module.key]" :options="module.modes.map(value => ({ value, label: value === 'write' && module.key === 'file' ? '上传' : modeLabels[value] }))" />
        </div>
        <p>“读写”只展开为此处列出的读取和写入 scopes，不自动包含未来能力。</p>
        <n-form-item label="额外授权（默认关闭）">
          <n-space vertical>
            <n-checkbox v-model:checked="draft.battleReportGenerate" :disabled="draft.battleReport !== 'write'">battle_report:generate：调用原生 AI，可能计费，并创建世界内可见战报</n-checkbox>
            <n-checkbox v-model:checked="draft.cluePublish" :disabled="draft.clue !== 'write'">clue:publish：发布、揭示或隐藏线索，会影响其他用户</n-checkbox>
          </n-space>
        </n-form-item>
        <div class="mcp-limits">
          <n-form-item label="每用户每分钟调用上限"><n-input-number v-model:value="draft.callsPerMinute" :min="1" :max="10000" :show-button="false" /></n-form-item>
          <n-form-item label="其中写入上限"><n-input-number v-model:value="draft.writesPerMinute" :min="1" :max="draft.callsPerMinute" :show-button="false" /></n-form-item>
        </div>
        <p>停用模块暂时使已有授权失效，保留 Key 保存的 scopes。关闭平台后仍可查看和撤销 Key。公网连接须使用 HTTPS。</p>
      </n-form>
    </n-spin>
  </div>
</template>

<style scoped>
.mcp-settings {
  max-height: 61vh;
  overflow-x: hidden;
  overflow-y: auto;
  padding: 12px 8px 24px 4px;
  scrollbar-gutter: stable;
}
.mcp-settings :deep(.n-spin-content) { max-width: 760px; }
.mcp-settings p { margin: 8px 0 20px; color: var(--sc-text-secondary); }
.mcp-module { display: grid; grid-template-columns: minmax(0, 1fr) 120px; gap: 16px; align-items: center; margin-bottom: 16px; }
.mcp-module small { display: block; overflow-wrap: anywhere; color: var(--sc-text-secondary); }
.mcp-limits { display: flex; flex-wrap: wrap; gap: 20px; }
</style>
