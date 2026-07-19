import { z } from 'zod'
import {
  theaterPresentationPatchSchema,
  theaterPresentationSchema,
} from '../../../types/theaterPresentation'
import { isSafeStageImageUrl } from '../shared/stage-types'

export const THEATER_BRIDGE_PROTOCOL = 'sealchat.theater' as const
export const THEATER_BRIDGE_VERSION = '1.0' as const
export const THEATER_BRIDGE_MAX_MESSAGE_BYTES = 256 * 1024

export const THEATER_CHAT_MESSAGE_EVENT_NAMES = [
  'chat.message.created',
  'chat.message.updated',
  'chat.message.removed',
] as const

export const THEATER_STAGE_CAPABILITIES = [
  'stage.scene.read',
  'stage.scene.apply',
  'stage.action.trigger',
  ...THEATER_CHAT_MESSAGE_EVENT_NAMES,
] as const

export const THEATER_CHAT_CAPABILITIES = [
  'stage.scene.applied',
  'chat.message.send',
  'chat.composer.insert',
  'chat.character.read',
  'chat.character.subscribe',
  'chat.character.select',
  'chat.character.variant.select',
  'chat.character.updated',
  'chat.character.selected',
  'chat.character.appearance.updated',
  'chat.character.variant.selected',
  ...THEATER_CHAT_MESSAGE_EVENT_NAMES,
] as const

export const bridgeEndpointSchema = z.enum(['host', 'stage', 'chat', 'plugin', 'broadcast'])
export const bridgeKindSchema = z.enum(['system', 'command', 'result', 'event'])

const nonEmptyIdSchema = z.string().trim().min(1).max(256)
const capabilitySchema = z.string().trim().min(1).max(128)
const theaterChatMessageEventNameSet = new Set<string>(THEATER_CHAT_MESSAGE_EVENT_NAMES)
const optionalSafeImageUrlSchema = z.string().max(8_192).refine(
  (value) => !value || isSafeStageImageUrl(value),
  'unsafe stage image URL',
)

export const bridgeMessageEnvelopeSchema = z.strictObject({
  protocol: z.literal(THEATER_BRIDGE_PROTOCOL),
  version: z.literal(THEATER_BRIDGE_VERSION),
  id: nonEmptyIdSchema,
  correlationId: nonEmptyIdSchema.optional(),
  kind: bridgeKindSchema,
  source: bridgeEndpointSchema,
  target: bridgeEndpointSchema,
  worldId: nonEmptyIdSchema,
  channelId: nonEmptyIdSchema,
  sessionId: nonEmptyIdSchema,
  timestamp: z.number().int().nonnegative(),
  name: z.string().trim().min(1).max(128),
  payload: z.unknown(),
}).superRefine((message, context) => {
  if (message.kind === 'result' && !message.correlationId) {
    context.addIssue({
      code: 'custom',
      path: ['correlationId'],
      message: 'result message requires correlationId',
    })
  }
  if (message.kind === 'event' && theaterChatMessageEventNameSet.has(message.name)) {
    if (message.source !== 'chat') {
      context.addIssue({
        code: 'custom',
        path: ['source'],
        message: 'chat message events require chat source',
      })
    }
    if (message.target !== 'stage') {
      context.addIssue({
        code: 'custom',
        path: ['target'],
        message: 'chat message events require stage target',
      })
    }
  }
})

const stageObjectTransformSchema = z.strictObject({
  x: z.number().finite(),
  y: z.number().finite(),
  width: z.number().finite(),
  height: z.number().finite(),
  rotation: z.number().finite(),
  scale: z.number().finite().positive().max(100).optional(),
  scaleX: z.number().finite().positive().max(100).optional(),
  scaleY: z.number().finite().positive().max(100).optional(),
  z: z.number().finite(),
  order: z.number().finite(),
}).transform(({ scale, ...transform }) => ({
  ...transform,
  scaleX: transform.scaleX ?? scale ?? 1,
  scaleY: transform.scaleY ?? scale ?? 1,
}))

const stageImageRefSchema = z.strictObject({
  resourceId: nonEmptyIdSchema,
  url: z.string().trim().min(1).max(8_192).refine(isSafeStageImageUrl, 'unsafe stage image URL'),
  alt: z.string().max(2_048).optional(),
  mimeType: z.string().trim().min(1).max(256).optional(),
  animated: z.boolean().optional(),
})

const characterDecorationSchema = z.strictObject({
  id: nonEmptyIdSchema,
  resource: stageImageRefSchema,
  enabled: z.boolean(),
  zIndex: z.number().finite(),
  settings: z.record(z.string(), z.unknown()),
  extensions: z.record(z.string(), z.unknown()),
})

export const characterAppearanceSchema = z.strictObject({
  displayName: z.string().max(512),
  color: z.string().max(256),
  avatar: stageImageRefSchema.nullable(),
  decorations: z.array(characterDecorationSchema).max(64),
  theaterPresentation: theaterPresentationSchema.nullable().optional(),
  extensions: z.record(z.string(), z.unknown()),
})

const characterAppearancePatchSchema = z.strictObject({
  displayName: z.string().max(512).optional(),
  color: z.string().max(256).optional(),
  avatar: stageImageRefSchema.nullable().optional(),
  decorations: z.array(characterDecorationSchema).max(64).optional(),
  theaterPresentation: theaterPresentationPatchSchema.nullable().optional(),
  extensions: z.record(z.string(), z.unknown()).optional(),
})

const chatCharacterVariantSchema = z.strictObject({
  variantId: nonEmptyIdSchema,
  keyword: z.string().max(512),
  selectorEmoji: z.string().max(2_048),
  note: z.string().max(4_096),
  enabled: z.boolean(),
  appearancePatch: characterAppearancePatchSchema,
  extensions: z.record(z.string(), z.unknown()),
})

const chatCharacterSnapshotSchema = z.strictObject({
  identityId: nonEmptyIdSchema,
  displayName: z.string().max(512),
  color: z.string().max(256),
  avatarUrl: optionalSafeImageUrlSchema,
  isTemporary: z.boolean(),
  icOocOnActivate: z.enum(['', 'ic', 'ooc']).optional(),
  activeVariantId: nonEmptyIdSchema.nullable(),
  activeVariantDisplayName: z.string().max(512).optional(),
  activeVariantColor: z.string().max(256).optional(),
  activeVariantAvatarUrl: optionalSafeImageUrlSchema.optional(),
  isActive: z.boolean(),
  revision: z.number().int().nonnegative(),
  updatedAt: z.number().int().nonnegative(),
  baseAppearance: characterAppearanceSchema,
  variants: z.array(chatCharacterVariantSchema).max(256),
  resolvedAppearance: characterAppearanceSchema,
  extensions: z.record(z.string(), z.unknown()),
})

const chatCharactersSnapshotPayloadShape = {
  revision: z.number().int().nonnegative(),
  updatedAt: z.number().int().nonnegative(),
  activeIdentityId: nonEmptyIdSchema.nullable(),
  characters: z.array(chatCharacterSnapshotSchema).max(512),
}
const chatCharacterSnapshotBaseSchema = z.strictObject(chatCharactersSnapshotPayloadShape)

const validateCharacterSnapshot = (
  snapshot: z.infer<typeof chatCharacterSnapshotBaseSchema>,
  context: z.RefinementCtx,
) => {
  const identityIds = new Set<string>()
  let activeCount = 0
  snapshot.characters.forEach((character, index) => {
    if (identityIds.has(character.identityId)) {
      context.addIssue({
        code: 'custom',
        path: ['characters', index, 'identityId'],
        message: 'duplicate character identityId',
      })
    }
    identityIds.add(character.identityId)
    if (character.revision !== snapshot.revision || character.updatedAt !== snapshot.updatedAt) {
      context.addIssue({
        code: 'custom',
        path: ['characters', index],
        message: 'character revision does not match snapshot',
      })
    }
    if (character.isActive) {
      activeCount += 1
      if (snapshot.activeIdentityId !== character.identityId) {
        context.addIssue({
          code: 'custom',
          path: ['characters', index, 'isActive'],
          message: 'active character does not match activeIdentityId',
        })
      }
    }
  })
  if ((snapshot.activeIdentityId === null && activeCount !== 0) || (snapshot.activeIdentityId !== null && activeCount !== 1)) {
    context.addIssue({
      code: 'custom',
      path: ['activeIdentityId'],
      message: 'snapshot must contain exactly one matching active character',
    })
  }
}

export const chatCharactersSnapshotPayloadSchema = chatCharacterSnapshotBaseSchema.superRefine(validateCharacterSnapshot)

const chatSendActionSchema = z.strictObject({
  id: nonEmptyIdSchema,
  type: z.literal('chat.send'),
  payload: z.strictObject({
    content: z.string().min(1).max(10_000),
    channelId: nonEmptyIdSchema.optional(),
    characterId: nonEmptyIdSchema.optional(),
  }),
})

const chatInsertActionSchema = z.strictObject({
  id: nonEmptyIdSchema,
  type: z.literal('chat.insert'),
  payload: z.strictObject({
    content: z.string().min(1).max(10_000),
  }),
})

const sceneApplyActionSchema = z.strictObject({
  id: nonEmptyIdSchema,
  type: z.literal('scene.apply'),
  payload: z.strictObject({ sceneId: nonEmptyIdSchema }),
})

const objectToggleActionSchema = z.strictObject({
  id: nonEmptyIdSchema,
  type: z.literal('object.toggle'),
  payload: z.strictObject({ objectId: nonEmptyIdSchema }),
})

export const stageActionSchema = z.discriminatedUnion('type', [
  chatSendActionSchema,
  chatInsertActionSchema,
  sceneApplyActionSchema,
  objectToggleActionSchema,
])

const stageDrawingSchema = z.strictObject({
  tool: z.enum(['pen', 'highlighter', 'line', 'arrow', 'rectangle', 'ellipse', 'triangle', 'polygon']),
  style: z.strictObject({
    stroke: z.string().max(256),
    strokeWidth: z.number().finite().min(1).max(128),
    opacity: z.number().finite().min(0.05).max(1),
    fill: z.string().max(256).nullable(),
    dash: z.enum(['solid', 'dashed', 'dotted']),
  }),
  points: z.array(z.number().finite()).max(2_000).optional(),
  sides: z.number().int().min(5).max(12).optional(),
  smoothing: z.number().finite().min(0).max(1).optional(),
})

const stageObjectSchema = z.strictObject({
  id: nonEmptyIdSchema,
  parentId: nonEmptyIdSchema.nullable(),
  type: z.enum(['group', 'drawing', 'text', 'image', 'button', 'effect']),
  name: z.string().max(512),
  transform: stageObjectTransformSchema,
  visible: z.boolean(),
  locked: z.boolean(),
  aspectRatioLocked: z.boolean(),
  interactive: z.boolean(),
  editable: z.boolean(),
  fill: z.string().max(256),
  drawing: stageDrawingSchema.optional(),
  text: z.string().max(100_000).optional(),
  image: stageImageRefSchema.optional(),
  content: z.record(z.string(), z.unknown()).optional(),
  ownerUserId: nonEmptyIdSchema.nullable().optional(),
  characterIdentityId: nonEmptyIdSchema.nullable().optional(),
  actions: z.array(stageActionSchema).max(32),
  metadata: z.record(z.string(), z.unknown()),
}).superRefine((object, context) => {
  if (object.type !== 'drawing') return
  if (!object.drawing) {
    context.addIssue({ code: z.ZodIssueCode.custom, path: ['drawing'], message: '绘制对象缺少 drawing' })
    return
  }
  if (
    (object.drawing.tool === 'pen' || object.drawing.tool === 'highlighter')
    && (!object.drawing.points || object.drawing.points.length < 2 || object.drawing.points.length % 2 !== 0)
  ) context.addIssue({ code: z.ZodIssueCode.custom, path: ['drawing', 'points'], message: '自由笔迹 points 无效' })
})

const stageSurfaceStyleSchema = z.strictObject({
  brightness: z.number().finite().min(0).max(2),
  blurPx: z.number().finite().min(0).max(40),
  opacity: z.number().finite().min(0).max(1),
  zoom: z.number().finite().min(0.1).max(5),
  fit: z.enum(['fill', 'cover', 'contain', 'tile', 'center']),
  overlay: z.strictObject({
    enabled: z.boolean(),
    color: z.string().trim().min(1).max(64),
    opacity: z.number().finite().min(0).max(1),
  }),
})

const stageSceneStateSchema = z.strictObject({
  background: stageImageRefSchema.nullable(),
  foreground: stageImageRefSchema.nullable(),
  surfaceStyles: z.strictObject({
    background: stageSurfaceStyleSchema,
    foreground: stageSurfaceStyleSchema,
  }),
  backgroundColor: z.string().max(256),
  fieldWidth: z.number().finite().positive(),
  fieldHeight: z.number().finite().positive(),
  fieldObjectFit: z.enum(['fill', 'cover', 'contain']),
  displayGrid: z.boolean(),
  gridSize: z.number().finite().positive(),
  alignWithGrid: z.boolean(),
  sceneObjects: z.record(z.string(), stageObjectSchema),
  transition: z.strictObject({
    type: z.enum(['none', 'crossfade']),
    durationMs: z.number().int().min(0).max(60_000),
  }),
  serverState: z.record(z.string(), z.unknown()).optional(),
})

const stageSceneSchema = z.strictObject({
  id: nonEmptyIdSchema,
  name: z.string().max(512),
  order: z.number().finite(),
  locked: z.boolean(),
  state: stageSceneStateSchema,
})

const stageWorkspaceStateSchema = z.strictObject({
  activeSceneId: nonEmptyIdSchema,
  liveState: stageSceneStateSchema,
  scenes: z.record(z.string(), stageSceneSchema),
  persistentObjects: z.record(z.string(), stageObjectSchema),
  camera: z.strictObject({
    x: z.number().finite(),
    y: z.number().finite(),
    zoom: z.number().finite().positive(),
  }),
  selectedObjectId: nonEmptyIdSchema.nullable(),
})

export const readyPayloadSchema = z.strictObject({
  endpoint: z.enum(['stage', 'chat']),
  supportedVersions: z.array(z.string().trim().min(1).max(32)).min(1).max(16),
  capabilities: z.array(capabilitySchema).max(256),
})

export const initializePayloadSchema = z.strictObject({
  selectedVersion: z.literal(THEATER_BRIDGE_VERSION),
  worldId: nonEmptyIdSchema,
  channelId: nonEmptyIdSchema,
  userId: z.string().max(256),
  permissions: z.array(capabilitySchema).max(256),
  capabilities: z.array(capabilitySchema).max(256),
  initialContext: z.strictObject({
    activeSceneId: nonEmptyIdSchema.nullable().optional(),
    activeCharacterId: nonEmptyIdSchema.nullable().optional(),
  }),
})

export const initializedPayloadSchema = z.strictObject({
  endpoint: z.enum(['stage', 'chat']),
  selectedVersion: z.literal(THEATER_BRIDGE_VERSION),
  capabilities: z.array(capabilitySchema).max(256),
})

export const bridgeErrorResultSchema = z.strictObject({
  ok: z.literal(false),
  error: z.strictObject({
    code: z.string().trim().min(1).max(128),
    message: z.string().max(2048),
    details: z.unknown().optional(),
  }),
})

export const stageSceneReadPayloadSchema = z.strictObject({})
export const stageSceneReadResultSchema = z.union([
  z.strictObject({
    ok: z.literal(true),
    state: stageWorkspaceStateSchema,
  }),
  bridgeErrorResultSchema,
])

export const applyScenePayloadSchema = z.strictObject({
  sceneId: nonEmptyIdSchema,
  transition: z.strictObject({
    type: z.enum(['none', 'crossfade']),
    durationMs: z.number().int().min(0).max(60_000).optional(),
  }).optional(),
})

export const applySceneResultSchema = z.union([
  z.strictObject({
    ok: z.literal(true),
    sceneId: nonEmptyIdSchema,
  }),
  bridgeErrorResultSchema,
])

export const sceneAppliedPayloadSchema = z.strictObject({
  sceneId: nonEmptyIdSchema,
  previousSceneId: nonEmptyIdSchema.nullable(),
  transition: z.strictObject({
    type: z.enum(['none', 'crossfade']),
    durationMs: z.number().int().min(0).max(60_000).optional(),
  }).optional(),
})

export const stageActionTriggeredPayloadSchema = z.strictObject({
  objectId: nonEmptyIdSchema,
  actionId: nonEmptyIdSchema,
  action: stageActionSchema,
  pointer: z.strictObject({
    x: z.number().finite(),
    y: z.number().finite(),
  }).optional(),
})

export const chatMessageSendPayloadSchema = z.strictObject({
  content: z.string().min(1).max(10_000),
  channelId: nonEmptyIdSchema.optional(),
  characterId: nonEmptyIdSchema.optional(),
})

export const chatMessageSendResultSchema = z.union([
  z.strictObject({
    ok: z.literal(true),
    messageId: nonEmptyIdSchema,
  }),
  bridgeErrorResultSchema,
])

export const chatComposerInsertPayloadSchema = z.strictObject({
  content: z.string().min(1).max(10_000),
})

export const chatComposerInsertResultSchema = z.union([
  z.strictObject({ ok: z.literal(true) }),
  bridgeErrorResultSchema,
])

const theaterDialogueActorAppearanceSchema = z.strictObject({
  displayName: z.string().max(512),
  color: z.string().max(256),
  avatar: stageImageRefSchema.nullable(),
  decorations: z.array(characterDecorationSchema).max(64),
  theaterPresentation: theaterPresentationSchema.nullable(),
  extensions: z.record(z.string(), z.unknown()),
})

export const theaterDialogueMessagePayloadSchema = z.strictObject({
  messageId: nonEmptyIdSchema,
  createdAt: z.number().int().nonnegative(),
  displayOrder: z.number().finite().optional(),
  icMode: z.enum(['ic', 'ooc']),
  isWhisper: z.boolean(),
  isArchived: z.boolean(),
  isDeleted: z.boolean(),
  contentText: z.string().max(200_000),
  contentRichText: z.string().max(200_000).optional(),
  hasPerformanceContent: z.boolean().optional(),
  actor: z.strictObject({
    userId: nonEmptyIdSchema.nullable(),
    identityId: nonEmptyIdSchema.nullable(),
    variantId: nonEmptyIdSchema.nullable(),
    displayName: z.string().max(512),
    color: z.string().max(256),
    appearance: theaterDialogueActorAppearanceSchema,
  }),
})

export const theaterDialogueMessageRemovedPayloadSchema = z.strictObject({
  messageId: nonEmptyIdSchema,
})

export const chatCharacterReadPayloadSchema = z.strictObject({})
export const chatCharacterReadResultSchema = z.union([
  z.strictObject({
    ok: z.literal(true),
    snapshot: chatCharactersSnapshotPayloadSchema,
  }),
  bridgeErrorResultSchema,
])

export const selectCharacterPayloadSchema = z.strictObject({
  identityId: nonEmptyIdSchema,
})

export const selectCharacterVariantPayloadSchema = z.strictObject({
  identityId: nonEmptyIdSchema,
  variantId: nonEmptyIdSchema.nullable(),
})

export const selectCharacterResultSchema = z.union([
  z.strictObject({
    ok: z.literal(true),
    snapshot: chatCharactersSnapshotPayloadSchema,
  }),
  bridgeErrorResultSchema,
])

export const characterSelectedPayloadSchema = z.strictObject({
  ...chatCharactersSnapshotPayloadShape,
  identityId: nonEmptyIdSchema,
}).superRefine(validateCharacterSnapshot)

export const characterAppearanceUpdatedPayloadSchema = z.strictObject({
  ...chatCharactersSnapshotPayloadShape,
  identityId: nonEmptyIdSchema.nullable(),
}).superRefine(validateCharacterSnapshot)

export const characterVariantSelectedPayloadSchema = z.strictObject({
  ...chatCharactersSnapshotPayloadShape,
  identityId: nonEmptyIdSchema,
  variantId: nonEmptyIdSchema.nullable(),
}).superRefine(validateCharacterSnapshot)

const payloadSchemas = new Map<string, z.ZodType>([
  ['system:system.ready', readyPayloadSchema],
  ['system:system.initialize', initializePayloadSchema],
  ['system:system.initialized', initializedPayloadSchema],
  ['command:stage.scene.read', stageSceneReadPayloadSchema],
  ['result:stage.scene.read.result', stageSceneReadResultSchema],
  ['command:stage.scene.apply', applyScenePayloadSchema],
  ['result:stage.scene.apply.result', applySceneResultSchema],
  ['event:stage.scene.applied', sceneAppliedPayloadSchema],
  ['event:stage.action.triggered', stageActionTriggeredPayloadSchema],
  ['command:chat.message.send', chatMessageSendPayloadSchema],
  ['result:chat.message.send.result', chatMessageSendResultSchema],
  ['command:chat.composer.insert', chatComposerInsertPayloadSchema],
  ['result:chat.composer.insert.result', chatComposerInsertResultSchema],
  ['command:chat.character.read', chatCharacterReadPayloadSchema],
  ['result:chat.character.read.result', chatCharacterReadResultSchema],
  ['command:chat.character.select', selectCharacterPayloadSchema],
  ['result:chat.character.select.result', selectCharacterResultSchema],
  ['command:chat.character.variant.select', selectCharacterVariantPayloadSchema],
  ['result:chat.character.variant.select.result', selectCharacterResultSchema],
  ['event:chat.character.updated', chatCharactersSnapshotPayloadSchema],
  ['event:chat.character.selected', characterSelectedPayloadSchema],
  ['event:chat.character.appearance.updated', characterAppearanceUpdatedPayloadSchema],
  ['event:chat.character.variant.selected', characterVariantSelectedPayloadSchema],
  ['event:chat.message.created', theaterDialogueMessagePayloadSchema],
  ['event:chat.message.updated', theaterDialogueMessagePayloadSchema],
  ['event:chat.message.removed', theaterDialogueMessageRemovedPayloadSchema],
])

export type BridgeEndpoint = z.infer<typeof bridgeEndpointSchema>
export type BridgeKind = z.infer<typeof bridgeKindSchema>
export type ReadyPayload = z.infer<typeof readyPayloadSchema>
export type InitializePayload = z.infer<typeof initializePayloadSchema>
export type InitializedPayload = z.infer<typeof initializedPayloadSchema>
export type BridgeErrorResult = z.infer<typeof bridgeErrorResultSchema>
export type StageSceneReadResult = z.infer<typeof stageSceneReadResultSchema>
export type ApplyScenePayload = z.infer<typeof applyScenePayloadSchema>
export type ApplySceneResult = z.infer<typeof applySceneResultSchema>
export type SceneAppliedPayload = z.infer<typeof sceneAppliedPayloadSchema>
export type StageAction = z.infer<typeof stageActionSchema>
export type StageActionTriggeredPayload = z.infer<typeof stageActionTriggeredPayloadSchema>
export type ChatMessageSendPayload = z.infer<typeof chatMessageSendPayloadSchema>
export type ChatMessageSendResult = z.infer<typeof chatMessageSendResultSchema>
export type ChatComposerInsertPayload = z.infer<typeof chatComposerInsertPayloadSchema>
export type ChatComposerInsertResult = z.infer<typeof chatComposerInsertResultSchema>
export type TheaterDialogueMessagePayload = z.infer<typeof theaterDialogueMessagePayloadSchema>
export type TheaterDialogueMessageRemovedPayload = z.infer<typeof theaterDialogueMessageRemovedPayloadSchema>
export type CharacterAppearance = z.infer<typeof characterAppearanceSchema>
export type ChatCharacterVariant = z.infer<typeof chatCharacterVariantSchema>
export type ChatCharacterSnapshot = z.infer<typeof chatCharacterSnapshotSchema>
export type ChatCharactersSnapshotPayload = z.infer<typeof chatCharactersSnapshotPayloadSchema>
export type ChatCharacterReadResult = z.infer<typeof chatCharacterReadResultSchema>
export type SelectCharacterPayload = z.infer<typeof selectCharacterPayloadSchema>
export type SelectCharacterVariantPayload = z.infer<typeof selectCharacterVariantPayloadSchema>
export type SelectCharacterResult = z.infer<typeof selectCharacterResultSchema>
export type CharacterSelectedPayload = z.infer<typeof characterSelectedPayloadSchema>
export type CharacterAppearanceUpdatedPayload = z.infer<typeof characterAppearanceUpdatedPayloadSchema>
export type CharacterVariantSelectedPayload = z.infer<typeof characterVariantSelectedPayloadSchema>

export const isNewerCharacterSnapshot = (
  candidate: Pick<ChatCharactersSnapshotPayload, 'revision' | 'updatedAt'>,
  current: Pick<ChatCharactersSnapshotPayload, 'revision' | 'updatedAt'>,
) => candidate.updatedAt > current.updatedAt
  || (candidate.updatedAt === current.updatedAt && candidate.revision > current.revision)

export interface TheaterBridgeMessage<T = unknown> {
  protocol: typeof THEATER_BRIDGE_PROTOCOL
  version: typeof THEATER_BRIDGE_VERSION
  id: string
  correlationId?: string
  kind: BridgeKind
  source: BridgeEndpoint
  target: BridgeEndpoint
  worldId: string
  channelId: string
  sessionId: string
  timestamp: number
  name: string
  payload: T
}

export interface TheaterBridgeContext {
  worldId: string
  channelId: string
  sessionId: string
}

export const parseTheaterBridgeMessage = (input: unknown): TheaterBridgeMessage => {
  const message = bridgeMessageEnvelopeSchema.parse(input) as TheaterBridgeMessage
  const payloadSchema = payloadSchemas.get(`${message.kind}:${message.name}`)
  if (!payloadSchema) {
    throw new Error(`Unsupported theater bridge message: ${message.kind}:${message.name}`)
  }
  return {
    ...message,
    payload: payloadSchema.parse(message.payload),
  }
}

export const createTheaterBridgeId = (prefix = 'msg') => {
  const value = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `${prefix}_${value}`
}

export const createTheaterBridgeMessage = <T>(
  context: TheaterBridgeContext,
  fields: Pick<TheaterBridgeMessage<T>, 'kind' | 'source' | 'target' | 'name' | 'payload'>
    & Partial<Pick<TheaterBridgeMessage<T>, 'id' | 'correlationId'>>,
): TheaterBridgeMessage<T> => ({
  protocol: THEATER_BRIDGE_PROTOCOL,
  version: THEATER_BRIDGE_VERSION,
  id: fields.id || createTheaterBridgeId(),
  correlationId: fields.correlationId,
  kind: fields.kind,
  source: fields.source,
  target: fields.target,
  worldId: context.worldId,
  channelId: context.channelId,
  sessionId: context.sessionId,
  timestamp: Date.now(),
  name: fields.name,
  payload: fields.payload,
})
