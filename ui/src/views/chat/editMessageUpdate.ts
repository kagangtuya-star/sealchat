export interface EditMessageUpdateSnapshot {
  isWhisper: boolean;
  whisperTargetIds: string[];
  icMode: 'ic' | 'ooc';
  identityId: string | null;
  identityVariantId: string | null;
  initialIdentityId: string | null;
  initialIdentityVariantId: string | null;
}

export interface EditMessageUpdateOptions {
  icMode: 'ic' | 'ooc';
  identityId?: string | null;
  identityVariantId?: string | null;
  whisperTargetIds?: string[];
}

export const buildEditMessageUpdateOptions = (
  snapshot: EditMessageUpdateSnapshot,
): EditMessageUpdateOptions => {
  const options: EditMessageUpdateOptions = {
    icMode: snapshot.icMode === 'ooc' ? 'ooc' : 'ic',
  };
  if (
    snapshot.identityId !== snapshot.initialIdentityId
    || snapshot.identityVariantId !== snapshot.initialIdentityVariantId
  ) {
    options.identityId = snapshot.identityId ?? null;
    options.identityVariantId = snapshot.identityVariantId ?? null;
  }
  if (snapshot.isWhisper) {
    options.whisperTargetIds = Array.from(new Set(
      (snapshot.whisperTargetIds || []).map((id) => String(id || '').trim()).filter(Boolean),
    ));
  }
  return options;
};
