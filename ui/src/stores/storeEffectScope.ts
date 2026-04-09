import { effectScope, type EffectScope } from 'vue';

export const ensureDetachedEffectScope = (
  scope: EffectScope | null | undefined,
  runner: () => void,
): EffectScope => {
  if (scope?.active) {
    return scope;
  }
  const nextScope = effectScope(true);
  nextScope.run(() => {
    runner();
  });
  return nextScope;
};
