import { normalizeBotCommandPrefixes } from './botCommand'

export interface BotNicknameSyncChannelLike {
  botFeatureEnabled?: boolean;
  botCommandPrefixes?: string[];
  friendInfo?: {
    userInfo?: {
      is_bot?: boolean;
    } | null;
  } | null;
}

export interface BotNicknameSyncNameInput {
  identityName?: string | null;
  boundCardName?: string | null;
  explicitCardName?: string | null;
}

const normalizeName = (value?: string | null) => String(value || '')
  .replace(/[\r\n]+/g, ' ')
  .trim();

export const resolveBotNicknameSyncName = (input: BotNicknameSyncNameInput) => {
  const explicitCardName = normalizeName(input.explicitCardName);
  if (explicitCardName) {
    return explicitCardName;
  }
  const boundCardName = normalizeName(input.boundCardName);
  if (boundCardName) {
    return boundCardName;
  }
  return normalizeName(input.identityName);
};

export const buildBotNicknameSyncCommand = (name?: string | null, prefixes?: string[] | null) => {
  const normalized = normalizeName(name);
  if (!normalized) {
    return '';
  }
  const commandPrefix = normalizeBotCommandPrefixes(prefixes)[0] || '.';
  return `${commandPrefix}nn ${normalized}`;
};

export const shouldEnableBotNicknameSyncForChannel = (channel?: BotNicknameSyncChannelLike | null) => {
  if (!channel) {
    return false;
  }
  if (channel.botFeatureEnabled === true) {
    return true;
  }
  return channel.friendInfo?.userInfo?.is_bot === true;
};
