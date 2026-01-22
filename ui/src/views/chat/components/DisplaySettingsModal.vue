<script setup lang="ts">
import { reactive, watch, computed, ref } from 'vue'
import { createDefaultDisplaySettings, useDisplayStore, type DisplaySettings } from '@/stores/display'
import { useOnboardingStore } from '@/stores/onboarding'
import ShortcutSettingsPanel from './ShortcutSettingsPanel.vue'
import IcOocRoleConfigPanel from './IcOocRoleConfigPanel.vue'
import CustomThemePanel from './CustomThemePanel.vue'
import AvatarStylePanel from './AvatarStylePanel.vue'

interface Props {
  visible: boolean
  settings: DisplaySettings
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'save', value: DisplaySettings): void
}>()

const draft = reactive<DisplaySettings>(createDefaultDisplaySettings())
const shortcutPanelVisible = ref(false)
const roleConfigPanelVisible = ref(false)
const customThemePanelVisible = ref(false)
const avatarStylePanelVisible = ref(false)
const display = useDisplayStore()
const onboarding = useOnboardingStore()
const timestampFormatOptions = [
  { label: '相对时间（2 分钟前）', value: 'relative' },
  { label: '仅时间（14:35）', value: 'time' },
  { label: '日期 + 时间（2024-05-30 14:35）', value: 'datetime' },
  { label: '日期 + 时间（含秒）', value: 'datetimeSeconds' },
]

const syncFavoriteBar = (source?: DisplaySettings) => {
  if (!source) return
  draft.favoriteChannelBarEnabled = source.favoriteChannelBarEnabled
}

// Sync avatar settings when AvatarStylePanel closes (it saves directly to store)
watch(avatarStylePanelVisible, (visible) => {
  if (!visible) {
    draft.avatarSize = display.settings.avatarSize
    draft.avatarBorderRadius = display.settings.avatarBorderRadius
  }
})

watch(
  () => props.settings,
  (value) => {
    if (!value) return
    draft.layout = value.layout
    draft.palette = value.palette
    draft.showAvatar = value.showAvatar
    draft.showInputPreview = value.showInputPreview
    draft.autoScrollTypingPreview = value.autoScrollTypingPreview
    draft.mergeNeighbors = value.mergeNeighbors
    draft.alwaysShowTimestamp = value.alwaysShowTimestamp
    draft.timestampFormat = value.timestampFormat
    draft.maxExportMessages = value.maxExportMessages
    draft.maxExportConcurrency = value.maxExportConcurrency
    draft.fontSize = value.fontSize
    draft.lineHeight = value.lineHeight
    draft.letterSpacing = value.letterSpacing
    draft.bubbleGap = value.bubbleGap
    draft.compactBubbleGap = value.compactBubbleGap
    draft.paragraphSpacing = value.paragraphSpacing
  draft.messagePaddingX = value.messagePaddingX
  draft.messagePaddingY = value.messagePaddingY
  draft.sendShortcut = value.sendShortcut
  draft.enableIcToggleHotkey = value.enableIcToggleHotkey
  syncFavoriteBar(value)
  draft.worldKeywordHighlightEnabled = value.worldKeywordHighlightEnabled
  draft.worldKeywordUnderlineOnly = value.worldKeywordUnderlineOnly
  draft.worldKeywordTooltipEnabled = value.worldKeywordTooltipEnabled
  draft.worldKeywordTooltipTextIndent = value.worldKeywordTooltipTextIndent
  draft.worldKeywordQuickInputEnabled = value.worldKeywordQuickInputEnabled
  draft.worldKeywordQuickInputTrigger = value.worldKeywordQuickInputTrigger
  draft.toolbarHotkeys = value.toolbarHotkeys
  draft.autoSwitchRoleOnIcOocToggle = value.autoSwitchRoleOnIcOocToggle
  draft.showDragIndicator = value.showDragIndicator
  draft.disableContextMenu = value.disableContextMenu
  draft.avatarSize = value.avatarSize
  draft.avatarBorderRadius = value.avatarBorderRadius
  draft.characterCardBadgeEnabled = value.characterCardBadgeEnabled
  // Custom theme fields are managed directly by store actions, not by draft
  },
  { deep: true, immediate: true },
)

const previewClasses = computed(() => [
  'display-preview',
  `display-preview--${draft.palette}`,
  `display-preview--${draft.layout}`,
])

const previewStyleVars = computed(() => ({
  '--chat-font-size': `${draft.fontSize / 16}rem`,
  '--chat-line-height': `${draft.lineHeight}`,
  '--chat-letter-spacing': `${draft.letterSpacing}px`,
  '--chat-bubble-gap': `${draft.bubbleGap}px`,
  '--chat-compact-gap': `${draft.compactBubbleGap}px`,
  '--chat-paragraph-spacing': `${draft.paragraphSpacing}px`,
  '--chat-message-padding-x': `${draft.messagePaddingX}px`,
  '--chat-message-padding-y': `${draft.messagePaddingY}px`,
}))

const formatPxTooltip = (value: number) => `${Math.round(value)}px`
const formatLetterSpacingTooltip = (value: number) => `${value.toFixed(1)}px`
const formatLineHeightTooltip = (value: number) => value.toFixed(2)
type NumericSettingKey =
  | 'fontSize'
  | 'lineHeight'
  | 'letterSpacing'
  | 'bubbleGap'
  | 'compactBubbleGap'
  | 'paragraphSpacing'
  | 'messagePaddingX'
  | 'messagePaddingY'
const handleNumericInput = (key: NumericSettingKey, value: number | null) => {
  if (value === null) return
  draft[key] = value as DisplaySettings[NumericSettingKey]
}

const handleRestoreDefaults = () => {
  const defaults = createDefaultDisplaySettings()
  Object.assign(draft, defaults)
  syncFavoriteBar(props.settings)
}

const handleClose = () => emit('update:visible', false)
const handleConfirm = () => {
  // Exclude custom theme fields - they are managed directly by store actions
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const { customThemeEnabled, customThemes, activeCustomThemeId, ...rest } = draft
  emit('save', rest as any)
}

const handleOpenTutorialHub = () => {
  onboarding.restart()
  emit('update:visible', false)
}
</script>

<template>
  <n-modal
    class="display-settings-modal"
    preset="card"
    :show="props.visible"
    title="显示模式"
    :style="{ width: 'min(880px, 96vw)' }"
    @update:show="emit('update:visible', $event)"
  >
    <div class="display-settings">
      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">版式</p>
            <p class="section-desc">气泡模式强调对话气泡，紧凑模式更接近论坛流</p>
          </div>
        </header>
        <n-radio-group v-model:value="draft.layout" size="large">
          <n-radio-button value="bubble">气泡模式</n-radio-button>
          <n-radio-button value="compact">紧凑模式</n-radio-button>
        </n-radio-group>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">主题</p>
            <p class="section-desc">在日间/夜间之间切换沉浸背景</p>
          </div>
        </header>
        <n-radio-group v-model:value="draft.palette" size="large">
          <n-radio-button value="day">日间模式</n-radio-button>
          <n-radio-button value="night">夜间模式</n-radio-button>
        </n-radio-group>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">自定义主题</p>
            <p class="section-desc">创建个性化配色方案，覆盖系统日夜主题</p>
          </div>
        </header>
        <div class="custom-theme-row">
          <n-switch
            :value="display.settings.customThemeEnabled"
            @update:value="display.setCustomThemeEnabled">
            <template #checked>已启用</template>
            <template #unchecked>已关闭</template>
          </n-switch>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button
                circle
                size="tiny"
                quaternary
                :disabled="!display.settings.customThemeEnabled"
                @click="customThemePanelVisible = true"
              >
                <template #icon>
                  <n-icon size="16">
                    <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M12 20a8 8 0 1 0 0-16 8 8 0 0 0 0 16Z"></path>
                      <path d="M12 14a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z"></path>
                      <path d="M12 2v2"></path>
                      <path d="M12 22v-2"></path>
                      <path d="m17 20.66-1-1.73"></path>
                      <path d="M11 10.27 7 3.34"></path>
                      <path d="m20.66 17-1.73-1"></path>
                      <path d="m3.34 7 1.73 1"></path>
                      <path d="M14 12h8"></path>
                      <path d="M2 12h2"></path>
                      <path d="m20.66 7-1.73 1"></path>
                      <path d="m3.34 17 1.73-1"></path>
                      <path d="m17 3.34-1 1.73"></path>
                      <path d="m11 13.73-4 6.93"></path>
                    </svg>
                  </n-icon>
                </template>
              </n-button>
            </template>
            配置自定义主题颜色
          </n-tooltip>
          <span v-if="display.settings.customThemeEnabled && display.getActiveCustomTheme()" class="active-theme-name">
            当前：{{ display.getActiveCustomTheme()?.name }}
          </span>
        </div>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">头像显示</p>
            <p class="section-desc">隐藏头像可获得更紧凑的布局</p>
          </div>
        </header>
        <div class="avatar-display-row">
          <n-switch v-model:value="draft.showAvatar">
            <template #checked>显示头像</template>
            <template #unchecked>隐藏头像</template>
          </n-switch>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button
                circle
                size="tiny"
                quaternary
                :disabled="!draft.showAvatar"
                @click="avatarStylePanelVisible = true"
              >
                <template #icon>
                  <n-icon size="16">
                    <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M12 20a8 8 0 1 0 0-16 8 8 0 0 0 0 16Z"></path>
                      <path d="M12 14a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z"></path>
                      <path d="M12 2v2"></path>
                      <path d="M12 22v-2"></path>
                      <path d="m17 20.66-1-1.73"></path>
                      <path d="M11 10.27 7 3.34"></path>
                      <path d="m20.66 17-1.73-1"></path>
                      <path d="m3.34 7 1.73 1"></path>
                      <path d="M14 12h8"></path>
                      <path d="M2 12h2"></path>
                      <path d="m20.66 7-1.73 1"></path>
                      <path d="m3.34 17 1.73-1"></path>
                      <path d="m17 3.34-1 1.73"></path>
                      <path d="m11 13.73-4 6.93"></path>
                    </svg>
                  </n-icon>
                </template>
              </n-button>
            </template>
            样式设定
          </n-tooltip>
        </div>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">合并连续消息</p>
            <p class="section-desc">相邻同角色消息视作一段，拖动可拆分</p>
          </div>
        </header>
        <n-switch v-model:value="draft.mergeNeighbors">
          <template #checked>已启用</template>
          <template #unchecked>已关闭</template>
        </n-switch>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">拖拽指示线</p>
            <p class="section-desc">拖动消息排序时显示蓝色指示线</p>
          </div>
        </header>
        <n-switch v-model:value="draft.showDragIndicator">
          <template #checked>显示指示线</template>
          <template #unchecked>隐藏指示线</template>
        </n-switch>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">实时预览</p>
            <p class="section-desc">开启后，输入内容会在聊天框上方即时渲染成消息预览</p>
          </div>
        </header>
        <n-switch v-model:value="draft.showInputPreview">
          <template #checked>预览开启</template>
          <template #unchecked>预览关闭</template>
        </n-switch>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">输入时自动滚动</p>
            <p class="section-desc">关闭时仅在非历史模式且已在底部时保持可见，开启后输入时总是滚到底部</p>
          </div>
        </header>
        <n-switch v-model:value="draft.autoScrollTypingPreview">
          <template #checked>输入时滚动到底部</template>
          <template #unchecked>输入时不滚动到底部</template>
        </n-switch>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">时间戳显示</p>
            <p class="section-desc">默认悬停延迟显示，可切换为始终展示并选择格式</p>
          </div>
        </header>
        <div class="timestamp-settings">
          <n-switch v-model:value="draft.alwaysShowTimestamp">
            <template #checked>始终显示时间</template>
            <template #unchecked>悬停后显示</template>
          </n-switch>
          <n-select
            v-model:value="draft.timestampFormat"
            :options="timestampFormatOptions"
            size="small"
            :consistent-menu-width="false"
            style="min-width: 220px"
          />
        </div>
        <p class="control-desc control-desc--hint">鼠标移入消息约 2 秒后会临时显示时间戳。</p>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">快捷键管理</p>
            <p class="section-desc">自定义工具栏各功能的快捷键绑定，包括场内/场外切换、悄悄话、上传等</p>
          </div>
        </header>
        <n-button secondary size="small" @click="shortcutPanelVisible = true">
          配置快捷键
        </n-button>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">禁用浏览器右键菜单</p>
            <p class="section-desc">避免应用内右键功能菜单与浏览器默认右键菜单冲突</p>
          </div>
        </header>
        <n-switch v-model:value="draft.disableContextMenu">
          <template #checked>已禁用</template>
          <template #unchecked>允许</template>
        </n-switch>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">输入与发送</p>
            <p class="section-desc">选择回车发送方式，另一组合则换行</p>
          </div>
        </header>
        <n-radio-group v-model:value="draft.sendShortcut" size="large">
          <n-radio-button value="enter">Enter 直接发送</n-radio-button>
          <n-radio-button value="ctrlEnter">Ctrl / Cmd + Enter 发送</n-radio-button>
        </n-radio-group>
        <p class="control-desc control-desc--hint">Shift + Enter 始终换行</p>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">场内场外自动切换</p>
            <p class="section-desc">切换IC/OOC模式时，自动切换到预设的频道角色</p>
          </div>
        </header>
        <div style="display: flex; align-items: center; gap: 0.75rem;">
          <n-switch v-model:value="draft.autoSwitchRoleOnIcOocToggle">
            <template #checked>已启用</template>
            <template #unchecked>已关闭</template>
          </n-switch>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button
                circle
                size="tiny"
                quaternary
                @click="roleConfigPanelVisible = true"
              >
                <template #icon>
                  <n-icon size="16">
                    <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M12 20a8 8 0 1 0 0-16 8 8 0 0 0 0 16Z"></path>
                      <path d="M12 14a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z"></path>
                      <path d="M12 2v2"></path>
                      <path d="M12 22v-2"></path>
                      <path d="m17 20.66-1-1.73"></path>
                      <path d="M11 10.27 7 3.34"></path>
                      <path d="m20.66 17-1.73-1"></path>
                      <path d="m3.34 7 1.73 1"></path>
                      <path d="M14 12h8"></path>
                      <path d="M2 12h2"></path>
                      <path d="m20.66 7-1.73 1"></path>
                      <path d="m3.34 17 1.73-1"></path>
                      <path d="m17 3.34-1 1.73"></path>
                      <path d="m11 13.73-4 6.93"></path>
                    </svg>
                  </n-icon>
                </template>
              </n-button>
            </template>
            配置默认场内/场外角色
          </n-tooltip>
        </div>
        <p class="control-desc control-desc--hint">频道角色配置独立保存，切换频道时自动加载对应配置</p>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">术语高亮</p>
            <p class="section-desc">控制世界术语的高亮样式与释义气泡</p>
          </div>
        </header>
        <div class="keyword-settings">
          <n-switch v-model:value="draft.worldKeywordHighlightEnabled">
            <template #checked>已启用</template>
            <template #unchecked>已关闭</template>
          </n-switch>
          <n-switch v-model:value="draft.worldKeywordUnderlineOnly" :disabled="!draft.worldKeywordHighlightEnabled">
            <template #checked>仅下划线</template>
            <template #unchecked>背景 + 下划线</template>
          </n-switch>
          <n-switch v-model:value="draft.worldKeywordTooltipEnabled" :disabled="!draft.worldKeywordHighlightEnabled">
            <template #checked>启用释义气泡</template>
            <template #unchecked>禁用释义气泡</template>
          </n-switch>
          <n-switch v-model:value="draft.worldKeywordDeduplicateEnabled" :disabled="!draft.worldKeywordHighlightEnabled">
            <template #checked>术语去重</template>
            <template #unchecked>允许重复</template>
          </n-switch>
        </div>
        <div class="keyword-quick-input-row">
          <n-switch v-model:value="draft.worldKeywordQuickInputEnabled">
            <template #checked>术语快捷输入已开启</template>
            <template #unchecked>术语快捷输入已关闭</template>
          </n-switch>
          <span class="quick-input-hint">触发字符</span>
          <n-input
            v-model:value="draft.worldKeywordQuickInputTrigger"
            size="small"
            :maxlength="1"
            :disabled="!draft.worldKeywordQuickInputEnabled"
            style="width: 50px; text-align: center"
            placeholder="/"
          />
          <span class="quick-input-hint">输入该字符后可快速搜索并插入世界术语</span>
        </div>
        <div class="keyword-indent-settings">
          <span class="indent-label">多段首行缩进</span>
          <n-input-number
            v-model:value="draft.worldKeywordTooltipTextIndent"
            size="small"
            :min="0"
            :max="4"
            :step="0.5"
            :disabled="!draft.worldKeywordHighlightEnabled || !draft.worldKeywordTooltipEnabled"
            style="width: 90px"
          />
          <span class="indent-unit">em</span>
          <span class="indent-hint">（0 为关闭）</span>
        </div>
        <div class="keyword-preview">
          <span
            class="keyword-preview__text"
            :class="{
              'keyword-preview__text--underline': draft.worldKeywordUnderlineOnly,
              'keyword-preview__text--disabled': !draft.worldKeywordHighlightEnabled,
            }"
          >
            阿瓦隆勇者
          </span>
          <span> 穿越黑森林。</span>
        </div>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">人物卡徽章</p>
            <p class="section-desc">在消息发送者名称后显示角色属性（如 HP/SAN）</p>
          </div>
        </header>
        <n-switch v-model:value="draft.characterCardBadgeEnabled">
          <template #checked>已启用</template>
          <template #unchecked>已关闭</template>
        </n-switch>
        <p class="control-desc control-desc--hint">徽章模板可在世界设置中自定义，格式如 "HP {hp}/{hpmax} SAN {san}"</p>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">排版（字号 / 行距 / 字距）</p>
            <p class="section-desc">控制阅读密度，满足不同屏幕与视力偏好</p>
          </div>
        </header>
        <div class="display-settings__controls">
          <div class="control-field">
            <div>
              <p class="control-title">字号</p>
              <p class="control-desc">影响聊天内容与预览文本大小</p>
            </div>
            <div class="control-input">
              <n-slider v-model:value="draft.fontSize" :min="12" :max="22" :step="1" :format-tooltip="formatPxTooltip" />
              <n-input-number
                v-model:value="draft.fontSize"
                size="small"
                :min="12"
                :max="22"
                @update:value="(v) => handleNumericInput('fontSize', v)"
              />
            </div>
          </div>
          <div class="control-field">
            <div>
              <p class="control-title">行距</p>
              <p class="control-desc">控制段落纵向密度</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.lineHeight"
                :min="1.2"
                :max="2"
                :step="0.05"
                :format-tooltip="formatLineHeightTooltip"
              />
              <n-input-number
                v-model:value="draft.lineHeight"
                size="small"
                :min="1.2"
                :max="2"
                :step="0.05"
                @update:value="(v) => handleNumericInput('lineHeight', v)"
              />
            </div>
          </div>
          <div class="control-field">
            <div>
              <p class="control-title">字距</p>
              <p class="control-desc">微调字符间隔，提升可读性</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.letterSpacing"
                :min="-1"
                :max="2"
                :step="0.1"
                :format-tooltip="formatLetterSpacingTooltip"
              />
              <n-input-number
                v-model:value="draft.letterSpacing"
                size="small"
                :min="-1"
                :max="2"
                :step="0.1"
                @update:value="(v) => handleNumericInput('letterSpacing', v)"
              />
            </div>
          </div>
        </div>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">气泡与段落间距</p>
            <p class="section-desc">调节消息块之间、段落之间的空白</p>
          </div>
        </header>
        <div class="display-settings__controls">
          <div class="control-field">
            <div>
              <p class="control-title">气泡间距</p>
              <p class="control-desc">作用于消息行之间的上下内间距</p>
            </div>
            <div class="control-input">
              <n-slider v-model:value="draft.bubbleGap" :min="4" :max="48" :step="2" :format-tooltip="formatPxTooltip" />
              <n-input-number
                v-model:value="draft.bubbleGap"
                size="small"
                :min="4"
                :max="48"
                :step="2"
                @update:value="(v) => handleNumericInput('bubbleGap', v)"
              />
            </div>
          </div>
          <div class="control-field">
            <div>
              <p class="control-title">紧凑间距</p>
              <p class="control-desc">仅作用于紧凑模式消息块之间的上下内间距</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.compactBubbleGap"
                :min="0"
                :max="24"
                :step="1"
                :format-tooltip="formatPxTooltip"
              />
              <n-input-number
                v-model:value="draft.compactBubbleGap"
                size="small"
                :min="0"
                :max="24"
                :step="1"
                @update:value="(v) => handleNumericInput('compactBubbleGap', v)"
              />
            </div>
          </div>
          <div class="control-field">
            <div>
              <p class="control-title">段落间距</p>
              <p class="control-desc">连续段落之间的外边距</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.paragraphSpacing"
                :min="0"
                :max="24"
                :step="1"
                :format-tooltip="formatPxTooltip"
              />
              <n-input-number
                v-model:value="draft.paragraphSpacing"
                size="small"
                :min="0"
                :max="24"
                @update:value="(v) => handleNumericInput('paragraphSpacing', v)"
              />
            </div>
          </div>
        </div>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">气泡内边距</p>
            <p class="section-desc">对齐不同设备的左右/上下空白</p>
          </div>
        </header>
        <div class="display-settings__controls">
          <div class="control-field">
            <div>
              <p class="control-title">左右内边距</p>
              <p class="control-desc">默认 18px，可适配窄屏</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.messagePaddingX"
                :min="8"
                :max="48"
                :step="1"
                :format-tooltip="formatPxTooltip"
              />
              <n-input-number
                v-model:value="draft.messagePaddingX"
                size="small"
                :min="8"
                :max="48"
                @update:value="(v) => handleNumericInput('messagePaddingX', v)"
              />
            </div>
          </div>
          <div class="control-field">
            <div>
              <p class="control-title">上下内边距</p>
              <p class="control-desc">默认 14px，影响气泡高度</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.messagePaddingY"
                :min="4"
                :max="32"
                :step="1"
                :format-tooltip="formatPxTooltip"
              />
              <n-input-number
                v-model:value="draft.messagePaddingY"
                size="small"
                :min="4"
                :max="32"
                @update:value="(v) => handleNumericInput('messagePaddingY', v)"
              />
            </div>
          </div>
        </div>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">功能教程</p>
            <p class="section-desc">重新学习平台核心功能，选择性查看各功能模块</p>
          </div>
        </header>
        <n-button secondary size="small" @click="handleOpenTutorialHub">
          📚 打开教程中心
        </n-button>
      </section>



      <section class="display-settings__section">
        <header class="preview-header">
          <div>
            <p class="section-title">实时预览</p>
            <p class="section-desc">排版参数实时映射至聊天气泡</p>
          </div>
        </header>
        <div :class="previewClasses" :style="previewStyleVars">
          <div class="preview-card">
            <div class="preview-avatar" />
            <div>
              <p class="preview-name">晨星角色 · 场内</p>
              <p class="preview-body">采用 {{ draft.layout === 'bubble' ? '气泡' : '紧凑' }} 模式展示。</p>
            </div>
          </div>
          <div class="preview-card preview-card--ooc">
            <div class="preview-avatar" />
            <div>
              <p class="preview-name">旁白 · 场外</p>
              <p class="preview-body">日夜模式在此同步变化。</p>
            </div>
          </div>
          <div class="preview-card preview-card--preview">
            <div>
              <p class="preview-name">实时预览</p>
              <p class="preview-body">无气泡，使用密排圆点背景。</p>
            </div>
          </div>
        </div>
      </section>

      <n-space justify="space-between" align="center" class="display-settings__footer">
        <n-space size="small">
          <n-button quaternary size="small" text-color="#fff" @click="handleClose">取消</n-button>
          <n-button tertiary size="small" text-color="#fff" @click="handleRestoreDefaults">恢复默认</n-button>
        </n-space>
        <n-button type="primary" size="small" @click="handleConfirm">应用设置</n-button>
      </n-space>
    </div>
  </n-modal>
  <ShortcutSettingsPanel v-model:show="shortcutPanelVisible" />
  <IcOocRoleConfigPanel v-model:show="roleConfigPanelVisible" />
  <CustomThemePanel v-model:show="customThemePanelVisible" />
  <AvatarStylePanel v-model:show="avatarStylePanelVisible" />
</template>

<style scoped lang="scss">
.display-settings-modal :global(.n-card) {
  background-color: var(--sc-bg-elevated);
  border: 1px solid var(--sc-border-strong);
  color: var(--sc-text-primary);
}

.display-settings-modal :global(.n-card__content) {
  max-width: 100%;
}

.display-settings {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  color: var(--sc-text-primary);
}

.display-settings__controls {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.control-field {
  display: flex;
  justify-content: space-between;
  gap: 1.25rem;
  align-items: flex-start;
  flex-wrap: wrap;
}

.control-field > div:first-child {
  flex: 0 0 220px;
}

.control-title {
  font-size: 0.85rem;
  font-weight: 600;
}

.control-desc {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  margin-top: 0.15rem;
}
.control-desc--hint {
  margin-top: 0.35rem;
}

.control-input {
  flex: 1;
  min-width: 280px;
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 0.6rem;
  align-items: center;
}

.control-input :deep(.n-slider) {
  margin: 0;
}

.control-input :deep(.n-input-number) {
  min-width: 120px;
}

.display-settings__section header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.45rem;
}

.display-settings :deep(.n-radio-group),
.display-settings :deep(.n-radio-button-group) {
  --n-button-color: transparent !important;
  --n-button-color-active: var(--sc-bg-elevated) !important;
  background-color: transparent !important;
}

.display-settings :deep(.n-radio-button) {
  background-color: transparent !important;
}

.display-settings :deep(.n-radio-button--checked) {
  background-color: var(--sc-bg-elevated) !important;
}

.section-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--sc-text-primary);
}

.section-desc {
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
  margin-top: 0.15rem;
}

.display-preview {
  border-radius: 0.9rem;
  padding: 0.9rem;
  display: flex;
  flex-direction: column;
  gap: var(--chat-bubble-gap, 0.65rem);
  border: 1px solid var(--sc-border-mute);
  background: linear-gradient(135deg, var(--sc-bg-surface), var(--sc-bg-elevated));
}

.display-preview--night {
  background: linear-gradient(135deg, var(--sc-bg-header), var(--sc-bg-elevated));
  border-color: var(--sc-border-strong);
}

.display-preview .preview-card {
  display: flex;
  gap: 0.75rem;
  padding: var(--chat-message-padding-y, 0.65rem) var(--chat-message-padding-x, 0.75rem);
  border-radius: var(--preview-radius, 1rem);
  background-color: var(--custom-chat-ic-bg, var(--chat-ic-bg, var(--sc-bg-surface)));
  border: 1px solid var(--sc-border-mute);
}

.display-preview--night .preview-card {
  background-color: var(--custom-chat-ic-bg, var(--chat-ic-bg, var(--sc-bg-input)));
  color: var(--sc-text-primary);
}

.display-preview--night .preview-card--ooc {
  background-color: var(--custom-chat-ooc-bg, var(--chat-ooc-bg, var(--sc-bg-input)));
}

.display-preview--night .preview-card--preview {
  background-image: radial-gradient(var(--custom-chat-preview-dot, var(--chat-preview-dot, rgba(148, 163, 184, 0.35))) 1px, transparent 1px);
  background-color: var(--custom-chat-preview-bg, var(--chat-preview-bg, var(--sc-bg-surface)));
  background-size: 10px 10px;
}

.display-preview--night .preview-name {
  color: var(--sc-text-primary);
}

.display-preview--night .preview-body {
  color: var(--sc-text-secondary);
}

.preview-card--ooc {
  background-color: var(--custom-chat-ooc-bg, var(--chat-ooc-bg, var(--sc-bg-surface)));
}

.preview-card--preview {
  flex-direction: column;
  background-color: var(--custom-chat-preview-bg, var(--chat-preview-bg, var(--sc-bg-surface)));
  background-image: radial-gradient(var(--custom-chat-preview-dot, var(--chat-preview-dot, rgba(148, 163, 184, 0.35))) 1px, transparent 1px);
  background-size: 10px 10px;
}

.preview-avatar {
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 0.75rem;
  background: linear-gradient(135deg, #f87171, #fbbf24);
  border: 1px solid var(--sc-border-mute);
}

.preview-name {
  font-size: calc(var(--chat-font-size, 0.95rem) - 0.05rem);
  font-weight: 600;
  color: var(--sc-text-primary);
}

.preview-body {
  font-size: var(--chat-font-size, 0.95rem);
  line-height: var(--chat-line-height, 1.6);
  letter-spacing: var(--chat-letter-spacing, 0px);
  color: var(--sc-text-secondary);
}

.display-preview--compact {
  --preview-radius: 0.75rem;
  gap: var(--chat-compact-gap, calc(var(--chat-bubble-gap, 0.65rem) * 0.35));
}

.keyword-settings {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  margin-bottom: 12px;
}

.keyword-indent-settings {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin-bottom: 12px;
}

.keyword-quick-input-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  margin-bottom: 12px;
}

.quick-input-hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.indent-label {
  font-size: 0.85rem;
  color: var(--sc-text-primary);
}

.indent-unit {
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
}

.indent-hint {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  opacity: 0.85;
}

.timestamp-settings {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  align-items: center;
}

.keyword-preview {
  border: 1px dashed rgba(148, 163, 184, 0.4);
  padding: 10px 12px;
  border-radius: 8px;
  font-size: 14px;
  color: var(--sc-text-secondary);
}

.keyword-preview__text {
  display: inline-flex;
  padding: 0 4px;
  margin-right: 2px;
  border-bottom: 1px dashed rgba(168, 108, 0, 0.85);
  background: rgba(255, 230, 150, 0.85);
  border-radius: 2px;
}

.keyword-preview__text--underline {
  background: transparent;
  border-bottom-style: dotted;
}

.keyword-preview__text--disabled {
  opacity: 0.5;
}

.custom-theme-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.avatar-display-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.active-theme-name {
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
  padding: 0.2rem 0.5rem;
  background: rgba(51, 136, 222, 0.1);
  border-radius: 4px;
}

.display-settings__footer {
  margin-top: 0.5rem;
}

@media (max-width: 720px) {
  .control-field {
    flex-direction: column;
  }

  .control-field > div:first-child {
    flex: 1;
    width: 100%;
  }

  .control-input {
    width: 100%;
    min-width: 0;
    grid-template-columns: 1fr;
    gap: 0.4rem;
  }

  .control-input :deep(.n-input-number) {
    width: 100%;
  }
}
</style>
