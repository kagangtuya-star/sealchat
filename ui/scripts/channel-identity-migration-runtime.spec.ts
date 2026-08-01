import assert from 'node:assert/strict'

import {
  normalizeIdentityExportFileForImport,
  remapDecorationsForImport,
  resolveIdentityAssetFetchUrl,
  type IdentityExportFile,
} from '../src/utils/channelIdentityMigration'

const versions = ['sealchat.channel-identity/v3', 'sealchat.channel-identity/v5']
const payload: IdentityExportFile = {
  version: 'sealchat.channel-identity/v3',
  generatedAt: new Date(0).toISOString(),
  items: [{
    sourceId: 'identity-1', displayName: 'Actor', color: '#123456', isDefault: true, sortOrder: 1,
    avatar: { attachmentId: 'source-avatar', hash: 'avatar', size: 6 },
    avatarDecorations: [{ enabled: true, resourceAttachmentId: 'id:source-decoration' }],
    theaterPresentation: {
      portrait: { media: { resourceAttachmentId: 'source-theater', fallbackAttachmentId: 'source-theater-fallback' } },
    },
  }],
  variants: [{
    sourceId: 'variant-1', identitySourceId: 'identity-1', selectorEmoji: 'V', keyword: 'variant', note: '', sortOrder: 1, enabled: true,
    appearance: {
      avatarAttachmentId: 'source-variant-avatar',
      theaterPresentation: {
        portrait: { media: { resourceAttachmentId: 'source-theater' } },
      },
    },
  }],
  assets: [
    { assetKey: 'asset-avatar', attachmentId: 'source-avatar', hash: 'avatar', size: 6, data: 'YXZhdGFy' },
    { assetKey: 'asset-decoration', attachmentId: 'source-decoration', hash: 'abc', size: 3, data: 'YWJj' },
    { assetKey: 'asset-theater', attachmentId: 'source-theater', hash: 'theater', size: 7, data: 'dGhlYXRlcg==' },
    { assetKey: 'asset-theater-fallback', attachmentId: 'source-theater-fallback', hash: 'fallback', size: 8, data: 'ZmFsbGJhY2s=' },
    { assetKey: 'asset-variant-avatar', attachmentId: 'source-variant-avatar', hash: 'variant', size: 7, data: 'dmFyaWFudA==' },
  ],
}

const normalized = normalizeIdentityExportFileForImport(payload, versions)
assert.equal(normalized.items[0].avatarAssetKey, 'asset-avatar')
assert.equal(normalized.items[0].avatarDecorations?.[0].resourceAssetKey, 'asset-decoration')
assert.equal(normalized.items[0].theaterPresentation?.portrait.media.resourceAssetKey, 'asset-theater')
assert.equal(normalized.items[0].theaterPresentation?.portrait.media.fallbackAssetKey, 'asset-theater-fallback')
assert.equal(normalized.variants?.[0].avatarAssetKey, 'asset-variant-avatar')
assert.equal(normalized.variants?.[0].theaterPresentation?.portrait.media.resourceAssetKey, 'asset-theater')
assert.equal(payload.items[0].avatarDecorations?.[0].resourceAssetKey, undefined, 'normalizer must not mutate parsed input')
assert.deepEqual(remapDecorationsForImport(normalized.items[0].avatarDecorations, new Map([['asset-decoration', 'target-decoration']])), [{
  enabled: true,
  resourceAttachmentId: 'target-decoration',
  resourceAssetKey: 'asset-decoration',
  fallbackAttachmentId: '',
  settings: undefined,
}])

assert.throws(() => remapDecorationsForImport([{ enabled: true, resourceAttachmentId: 'source-only' }], new Map()), /缺少可重建素材/)
assert.throws(() => remapDecorationsForImport([{ enabled: true, resourceAssetKey: 'missing' }], new Map()), /资源文件缺失/)
assert.throws(() => normalizeIdentityExportFileForImport({ ...payload, version: 'unknown' }, versions), /无法识别/)
assert.throws(() => normalizeIdentityExportFileForImport({
  ...payload,
  variants: [
    ...payload.variants || [],
    { sourceId: 'variant-2', identitySourceId: 'identity-1', selectorEmoji: 'W', keyword: 'VARIANT', note: '', sortOrder: 2, enabled: true },
  ],
}, versions), /重复差分快捷关键词/)
assert.equal(resolveIdentityAssetFetchUrl({
  normalizedId: 'uh6Isn0HHA8iFR47',
  externalUrl: 'id:uh6Isn0HHA8iFR47',
  urlBase: 'https://chat.example',
}), 'https://chat.example/api/v1/attachment/uh6Isn0HHA8iFR47')

console.log('channel identity migration runtime tests passed')
