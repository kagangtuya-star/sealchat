// 布局编辑器使用的入口目录：仅用于在配置页展示名称、图标与可用条件说明。
// 运行时按钮的定义、事件、权限与可用性仍以 ChatActionRibbon 为准；
// 顺序与 ChatActionRibbon 中按钮的默认构建顺序一致，即“系统默认”布局。
// ChatActionRibbon 新增入口时请同步追加到这里，否则配置页无法排序它（运行时仍会按默认顺序追加显示）。
import type { Component } from 'vue'
import {
  Archive as ArchiveIcon,
  Book as LogLayoutIcon,
  Bookmark as MessageFavoritesIcon,
  Bolt as ShortcutCommandIcon,
  Bulb as ClueBoxIcon,
  Code as CustomUiIcon,
  Dice as Dice3DIcon,
  Download as DownloadIcon,
  Eye as ObserverLinkIcon,
  Heartbeat as BridgeStatusIcon,
  Id as CharacterCardIcon,
  LayoutBoard as ClueBoardIcon,
  LayoutBoardSplit as SplitIcon,
  Link as LinkIcon,
  Message2 as CharacterRemarkIcon,
  MoodSmile as EmojiIcon,
  Palette,
  Photo as PhotoIcon,
  Presentation as TheaterIcon,
  Star as StarIcon,
  Upload as UploadIcon,
  Users as UsersIcon,
  Volume as SpeechIcon,
} from '@vicons/tabler'
import { DocumentTextOutline, MailOutline, NotificationsOutline } from '@vicons/ionicons5'

export interface ActionRibbonCatalogItem {
  key: string
  label: string
  icon: Component
  /** 入口并非总会出现时的条件说明。 */
  condition?: string
}

export const ACTION_RIBBON_CATALOG: readonly ActionRibbonCatalogItem[] = Object.freeze([
  { key: 'display', label: '常规设置', icon: Palette },
  { key: 'identity', label: '角色管理', icon: UsersIcon },
  { key: 'character-card', label: '人物卡', icon: CharacterCardIcon, condition: '进入频道后可用' },
  { key: 'export', label: '导出记录', icon: DownloadIcon },
  { key: 'gallery', label: '表情资源', icon: EmojiIcon },
  { key: 'channel-images', label: '图片浏览', icon: PhotoIcon },
  { key: 'clue-box', label: '线索箱', icon: ClueBoxIcon, condition: '进入频道后可用' },
  { key: 'clue-board', label: '线索画板', icon: ClueBoardIcon, condition: '进入频道后可用' },
  { key: 'battle-summary', label: '战报总结', icon: DocumentTextOutline, condition: '需开启战报功能' },
  { key: 'favorites', label: '频道收藏', icon: StarIcon },
  { key: 'message-favorites', label: '消息收藏', icon: MessageFavoritesIcon, condition: '进入频道后可用' },
  { key: 'character-remark', label: '角色备注', icon: CharacterRemarkIcon },
  { key: 'import', label: '导入记录', icon: UploadIcon, condition: '仅有导入权限的成员可见' },
  { key: 'archive', label: '消息归档', icon: ArchiveIcon },
  { key: 'theater', label: '小剧场', icon: TheaterIcon, condition: '小剧场可用时显示' },
  { key: 'split', label: '分屏', icon: SplitIcon, condition: '仅普通聊天页显示' },
  { key: 'ic-ooc-split', label: '场内外分屏', icon: SplitIcon, condition: '仅普通聊天页显示' },
  { key: 'speech', label: '语音朗读', icon: SpeechIcon, condition: '需平台开启 AI 语音' },
  { key: 'sticky-note', label: '便签', icon: DocumentTextOutline, condition: '进入频道后可用' },
  { key: 'dice3d', label: '3D 骰子', icon: Dice3DIcon, condition: '旁观模式不显示' },
  { key: 'webhook', label: 'Webhook', icon: LinkIcon, condition: '需频道管理权限' },
  { key: 'glass-background', label: '玻璃背景', icon: Palette },
  { key: 'shortcut-command', label: '快捷命令', icon: ShortcutCommandIcon },
  { key: 'log-layout-management', label: 'LOG 排版', icon: LogLayoutIcon },
  { key: 'observer-link-management', label: '旁观链接', icon: ObserverLinkIcon },
  { key: 'custom-ui-management', label: '定制界面', icon: CustomUiIcon },
  { key: 'input-stats', label: '输入统计', icon: DocumentTextOutline },
  { key: 'app-notification-settings', label: 'APP 推送', icon: NotificationsOutline },
  { key: 'bridge-status', label: '桥接状态', icon: BridgeStatusIcon },
  { key: 'email-notification', label: '未读提醒', icon: MailOutline, condition: '需频道管理权限' },
])

export const ACTION_RIBBON_KEYS: readonly string[] = Object.freeze(ACTION_RIBBON_CATALOG.map(item => item.key))

const catalogMap = new Map(ACTION_RIBBON_CATALOG.map(item => [item.key, item]))

export const getActionRibbonCatalogItem = (key: string) => catalogMap.get(key) || null
