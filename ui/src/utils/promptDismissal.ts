type Prompt = 'avatar' | 'identitySync';

// Keep choices for this page session even when browser storage is unavailable.
const dismissed = new Set<string>();

const promptKey = (prompt: Prompt, userId: string, worldId?: string): string => {
  if (!userId || (prompt === 'identitySync' && !worldId)) return '';
  const scope = prompt === 'identitySync' && worldId ? `:${encodeURIComponent(worldId)}` : '';
  return `sealchat.${prompt}PromptDismissed:${encodeURIComponent(userId)}${scope}`;
};

export const isPromptDismissed = (prompt: Prompt, userId: string, worldId?: string): boolean => {
  const key = promptKey(prompt, userId, worldId);
  if (!key) return false;
  if (dismissed.has(key)) return true;
  try {
    return localStorage.getItem(key) === '1';
  } catch {
    return false;
  }
};

export const dismissPrompt = (prompt: Prompt, userId: string, worldId?: string): void => {
  const key = promptKey(prompt, userId, worldId);
  if (!key) return;
  dismissed.add(key);
  try {
    localStorage.setItem(key, '1');
  } catch {
    // The in-memory choice still prevents repeated prompts in this session.
  }
};
