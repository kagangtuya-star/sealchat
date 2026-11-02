<script setup lang="ts">
import { computed } from 'vue'
import { Shield } from '@vicons/tabler'
import { isChannelExtensionWriteCapability } from '@/bridge/channelExtensionCapabilities'
import { useUICustomizationTrustPrompt } from '@/services/ui-customization/runtime'

// Non-modal prompt for world scripts this browser has not decided on yet.
// Trust/deny persist per packageId + jsHash; "later" lasts until reload.
const prompt = useUICustomizationTrustPrompt()
const pending = prompt.pending
const ids = computed(() => pending.value.map(p => p.id))
const multiple = computed(() => pending.value.length > 1)
</script>

<template>
  <Transition name="sc-uic-trust">
    <div
      v-if="pending.length"
      class="sc-uic-trust"
      role="dialog"
      aria-live="polite"
      aria-labelledby="sc-uic-trust-title"
      data-sc-ui="ui-customization-trust"
    >
      <n-card size="small" :bordered="true" class="sc-uic-trust__card">
        <div class="sc-uic-trust__head">
          <n-icon size="18" class="sc-uic-trust__icon"><Shield /></n-icon>
          <span id="sc-uic-trust-title" class="sc-uic-trust__title">
            当前世界提供了{{ multiple ? ` ${pending.length} 个` : '' }}自定义脚本
          </span>
        </div>
        <p class="sc-uic-trust__text">
          脚本由世界管理员提供，会在当前 SealChat 页面中以你的登录身份运行，可以读取和改变页面内容。只信任你认可的代码。
          拒绝不影响基础聊天和界面样式；脚本内容或请求的频道能力改变后需要重新确认。
        </p>
        <ul class="sc-uic-trust__list">
          <li v-for="item in pending" :key="item.id" class="sc-uic-trust__item">
            <span class="sc-uic-trust__name" :title="item.name">{{ item.name }}</span>
            <span class="sc-uic-trust__hash" :title="`脚本版本 ${item.jsHash}`">{{ item.jsHash.slice(0, 8) }}</span>
            <template v-if="multiple">
              <n-button size="tiny" quaternary @click="prompt.deny([item.id])">拒绝</n-button>
              <n-button size="tiny" secondary type="primary" @click="prompt.trust([item.id])">信任</n-button>
            </template>
            <div v-if="item.channelCapabilities?.length" class="sc-uic-trust__capabilities">
              请求频道能力：<span v-for="capability in item.channelCapabilities" :key="capability" :class="{ 'sc-uic-trust__write': isChannelExtensionWriteCapability(capability) }">{{ capability }}{{ isChannelExtensionWriteCapability(capability) ? '（写）' : '' }} </span>
            </div>
          </li>
        </ul>
        <div class="sc-uic-trust__actions">
          <n-button size="small" quaternary @click="prompt.postpone(ids)">稍后</n-button>
          <n-button size="small" secondary @click="prompt.deny(ids)">{{ multiple ? '全部拒绝' : '拒绝' }}</n-button>
          <n-button size="small" type="primary" @click="prompt.trust(ids)">{{ multiple ? '全部信任' : '信任并运行' }}</n-button>
        </div>
      </n-card>
    </div>
  </Transition>
</template>

<style scoped>
.sc-uic-trust {
  position: fixed;
  right: 16px;
  bottom: 16px;
  z-index: 2100;
  width: min(380px, calc(100vw - 24px));
}
.sc-uic-trust__card { box-shadow: 0 10px 30px rgba(15, 23, 42, 0.18); border-radius: 12px; }
.sc-uic-trust__head { display: flex; align-items: center; gap: 8px; }
.sc-uic-trust__icon { color: #d97706; }
.sc-uic-trust__title { font-weight: 600; font-size: 14px; }
.sc-uic-trust__text { margin: 8px 0; font-size: 12px; line-height: 1.6; color: var(--sc-text-secondary, #64748b); }
.sc-uic-trust__list { list-style: none; margin: 0 0 10px; padding: 0; display: flex; flex-direction: column; gap: 4px; max-height: 160px; overflow-y: auto; }
.sc-uic-trust__item { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; font-size: 13px; }
.sc-uic-trust__capabilities { flex-basis: 100%; font-size: 12px; overflow-wrap: anywhere; }
.sc-uic-trust__write { color: #d97706; font-weight: 600; }
.sc-uic-trust__name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; display: -webkit-box; -webkit-line-clamp: 1; -webkit-box-orient: vertical; }
.sc-uic-trust__hash { font-family: var(--sc-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace); font-size: 11px; color: var(--sc-text-secondary, #94a3b8); }
.sc-uic-trust__actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 6px; }
.sc-uic-trust-enter-active, .sc-uic-trust-leave-active { transition: opacity 0.18s ease, transform 0.18s ease; }
.sc-uic-trust-enter-from, .sc-uic-trust-leave-to { opacity: 0; transform: translateY(8px); }
@media (max-width: 640px) {
  .sc-uic-trust { right: 12px; left: 12px; bottom: 12px; width: auto; }
}
</style>
