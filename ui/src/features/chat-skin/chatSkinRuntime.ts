// 聊天装扮的运行时接线：附件 URL 解析 + 内置纹样解析，供聊天消息、编辑器预览共用。

import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { createChatSkinRefResolver } from './chatSkinOrnaments'
import { buildChatSkinRender, type ChatSkinRenderContext } from './chatSkinRender'
import type { ChatSkin } from './chatSkinSchema'

export const chatSkinRefResolver = createChatSkinRefResolver((ref) => resolveAttachmentUrl(ref))

export const buildChatSkinRenderWithAssets = (
  skin: ChatSkin | null | undefined,
  ctx: Omit<ChatSkinRenderContext, 'resolveRef'>,
) => buildChatSkinRender(skin, { ...ctx, resolveRef: chatSkinRefResolver })
