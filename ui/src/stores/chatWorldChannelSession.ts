import { findChannelByIdFromTree, findFirstEnterableChannel } from './chatChannelSelection';

type ChannelLike = {
  id?: string;
  type?: number;
  isPrivate?: boolean;
  children?: ChannelLike[];
};

export type LastChannelByWorldMap = Record<string, string>;

export const normalizeLastChannelByWorldMap = (raw: unknown): LastChannelByWorldMap => {
  if (!raw || typeof raw !== 'object') {
    return {};
  }
  const normalized: LastChannelByWorldMap = {};
  Object.entries(raw as Record<string, unknown>).forEach(([worldId, channelId]) => {
    if (typeof channelId !== 'string') {
      return;
    }
    const normalizedWorldId = String(worldId || '').trim();
    const normalizedChannelId = channelId.trim();
    if (!normalizedWorldId || !normalizedChannelId) {
      return;
    }
    normalized[normalizedWorldId] = normalizedChannelId;
  });
  return normalized;
};

export const parseLastChannelByWorldMap = (raw: string | null | undefined): LastChannelByWorldMap => {
  if (!raw) {
    return {};
  }
  try {
    return normalizeLastChannelByWorldMap(JSON.parse(raw));
  } catch {
    return {};
  }
};

export const updateLastChannelByWorldMap = (
  current: LastChannelByWorldMap,
  worldId: string,
  channelId: string,
): LastChannelByWorldMap => {
  const normalizedWorldId = String(worldId || '').trim();
  const normalizedChannelId = String(channelId || '').trim();
  if (!normalizedWorldId || !normalizedChannelId) {
    return normalizeLastChannelByWorldMap(current);
  }
  return {
    ...normalizeLastChannelByWorldMap(current),
    [normalizedWorldId]: normalizedChannelId,
  };
};

export const resolvePreferredChannelForWorld = <T extends ChannelLike>(options: {
  worldId: string;
  tree: T[];
  defaultChannelId?: string;
  lastChannelByWorld?: LastChannelByWorldMap;
  fallbackLastChannel?: string;
}) => {
  const worldId = String(options.worldId || '').trim();
  const tree = Array.isArray(options.tree) ? options.tree : [];
  const preferredWorldChannel = worldId
    ? String(options.lastChannelByWorld?.[worldId] || '').trim()
    : '';
  const defaultChannelId = String(options.defaultChannelId || '').trim();
  const fallbackLastChannel = String(options.fallbackLastChannel || '').trim();
  const candidates = [preferredWorldChannel, defaultChannelId, fallbackLastChannel];
  for (const candidate of candidates) {
    if (!candidate) {
      continue;
    }
    const found = findChannelByIdFromTree(tree, candidate);
    if (found?.id) {
      return found.id;
    }
  }
  return findFirstEnterableChannel(tree)?.id || '';
};
