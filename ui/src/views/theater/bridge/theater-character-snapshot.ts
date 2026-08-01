import type { ChatCharactersSnapshotPayload } from './theater-bridge-protocol'

export const getCharacterSnapshotContentSignature = (snapshot: ChatCharactersSnapshotPayload) => JSON.stringify({
  activeIdentityId: snapshot.activeIdentityId,
  characters: snapshot.characters.map(({ revision: _revision, updatedAt: _updatedAt, ...character }) => character),
})
