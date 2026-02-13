import { defineStore } from "pinia";
import { api } from "./_config";

export interface ChannelImageLayoutItem {
  attachmentId: string;
  width: number;
  height: number;
  updatedAt?: number;
}

interface ChannelImageLayoutState {
  layoutsByChannel: Record<string, Record<string, ChannelImageLayoutItem>>;
  pendingFetchKeys: Record<string, boolean>;
  knownMissingByChannel: Record<string, Record<string, boolean>>;
}

const normalizeAttachmentId = (raw?: string): string => {
  const value = String(raw || "").trim();
  if (!value) {
    return "";
  }
  if (value.startsWith("id:")) {
    return value.slice(3);
  }
  return value;
};

const toPendingKey = (channelId: string, attachmentId: string) => channelId + "::" + attachmentId;

const normalizeLayoutItems = (rawItems: any[]): ChannelImageLayoutItem[] => {
  if (!Array.isArray(rawItems)) {
    return [];
  }
  const out: ChannelImageLayoutItem[] = [];
  for (const raw of rawItems) {
    const attachmentId = normalizeAttachmentId(raw?.attachmentId || raw?.attachment_id);
    const width = Number(raw?.width || 0);
    const height = Number(raw?.height || 0);
    if (!attachmentId || !Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
      continue;
    }
    const updatedAtRaw = Number(raw?.updatedAt || raw?.updated_at || 0);
    out.push({
      attachmentId,
      width: Math.round(width),
      height: Math.round(height),
      updatedAt: Number.isFinite(updatedAtRaw) ? Math.round(updatedAtRaw) : 0,
    });
  }
  return out;
};

export const useChannelImageLayoutStore = defineStore("channelImageLayout", {
  state: (): ChannelImageLayoutState => ({
    layoutsByChannel: {},
    pendingFetchKeys: {},
    knownMissingByChannel: {},
  }),

  actions: {
    getLayout(channelId: string, attachmentId: string): ChannelImageLayoutItem | null {
      const normalizedChannelId = String(channelId || "").trim();
      const normalizedAttachmentId = normalizeAttachmentId(attachmentId);
      if (!normalizedChannelId || !normalizedAttachmentId) {
        return null;
      }
      return this.layoutsByChannel[normalizedChannelId]?.[normalizedAttachmentId] || null;
    },

    mergeLayouts(channelId: string, items: ChannelImageLayoutItem[]) {
      const normalizedChannelId = String(channelId || "").trim();
      if (!normalizedChannelId || !Array.isArray(items) || items.length === 0) {
        return;
      }
      const prevChannelLayouts = this.layoutsByChannel[normalizedChannelId] || {};
      const nextChannelLayouts: Record<string, ChannelImageLayoutItem> = { ...prevChannelLayouts };
      const prevMissing = this.knownMissingByChannel[normalizedChannelId] || {};
      const nextMissing: Record<string, boolean> = { ...prevMissing };
      let changed = false;

      for (const item of items) {
        const attachmentId = normalizeAttachmentId(item?.attachmentId);
        if (!attachmentId) {
          continue;
        }
        const next = {
          attachmentId,
          width: Math.round(item.width),
          height: Math.round(item.height),
          updatedAt: item.updatedAt || Date.now(),
        };
        const prev = prevChannelLayouts[attachmentId];
        if (!prev || prev.width !== next.width || prev.height !== next.height || (prev.updatedAt || 0) !== (next.updatedAt || 0)) {
          nextChannelLayouts[attachmentId] = next;
          changed = true;
        }
        if (nextMissing[attachmentId]) {
          delete nextMissing[attachmentId];
          changed = true;
        }
      }

      if (!changed) {
        return;
      }
      this.layoutsByChannel = {
        ...this.layoutsByChannel,
        [normalizedChannelId]: nextChannelLayouts,
      };
      this.knownMissingByChannel = {
        ...this.knownMissingByChannel,
        [normalizedChannelId]: nextMissing,
      };
    },

    async ensureLayouts(channelId: string, attachmentIds: string[]) {
      const normalizedChannelId = String(channelId || "").trim();
      if (!normalizedChannelId || !Array.isArray(attachmentIds) || attachmentIds.length === 0) {
        return;
      }
      const channelLayouts = this.layoutsByChannel[normalizedChannelId] || {};
      const channelMissing = this.knownMissingByChannel[normalizedChannelId] || {};
      const pending = this.pendingFetchKeys;
      const targets: string[] = [];

      for (const rawId of attachmentIds) {
        const attachmentId = normalizeAttachmentId(rawId);
        if (!attachmentId) {
          continue;
        }
        if (channelLayouts[attachmentId]) {
          continue;
        }
        if (channelMissing[attachmentId]) {
          continue;
        }
        const pendingKey = toPendingKey(normalizedChannelId, attachmentId);
        if (pending[pendingKey]) {
          continue;
        }
        targets.push(attachmentId);
      }

      if (targets.length === 0) {
        return;
      }

      const nextPending = { ...pending };
      targets.forEach((id) => {
        nextPending[toPendingKey(normalizedChannelId, id)] = true;
      });
      this.pendingFetchKeys = nextPending;

      try {
        const resp = await api.get("api/v1/channels/" + normalizedChannelId + "/image-layouts", {
          params: {
            attachmentIds: targets.join(","),
          },
        });
        const items = normalizeLayoutItems(resp.data?.items || []);
        this.mergeLayouts(normalizedChannelId, items);

        const returned = new Set(items.map((item) => item.attachmentId));
        const missingPatch = { ...(this.knownMissingByChannel[normalizedChannelId] || {}) };
        let missingChanged = false;
        targets.forEach((id) => {
          if (!returned.has(id)) {
            missingPatch[id] = true;
            missingChanged = true;
          }
        });
        if (missingChanged) {
          this.knownMissingByChannel = {
            ...this.knownMissingByChannel,
            [normalizedChannelId]: missingPatch,
          };
        }
      } catch (error) {
        console.error("加载图片尺寸失败", error);
      } finally {
        const clearPending = { ...this.pendingFetchKeys };
        targets.forEach((id) => {
          delete clearPending[toPendingKey(normalizedChannelId, id)];
        });
        this.pendingFetchKeys = clearPending;
      }
    },

    async saveMessageLayouts(channelId: string, messageId: string, items: Array<{ attachmentId: string; width: number; height: number }>) {
      const normalizedChannelId = String(channelId || "").trim();
      const normalizedMessageId = String(messageId || "").trim();
      if (!normalizedChannelId || !normalizedMessageId || !Array.isArray(items) || items.length === 0) {
        return [] as ChannelImageLayoutItem[];
      }
      const payloadItems = items
        .map((item) => ({
          attachmentId: normalizeAttachmentId(item?.attachmentId),
          width: Math.round(Number(item?.width || 0)),
          height: Math.round(Number(item?.height || 0)),
        }))
        .filter((item) => item.attachmentId && item.width > 0 && item.height > 0);
      if (payloadItems.length === 0) {
        return [] as ChannelImageLayoutItem[];
      }

      const resp = await api.post(
        "api/v1/channels/" + normalizedChannelId + "/messages/" + normalizedMessageId + "/image-layouts",
        { items: payloadItems },
      );
      const merged = normalizeLayoutItems(resp.data?.items || payloadItems);
      this.mergeLayouts(normalizedChannelId, merged);
      return merged;
    },

    applyRealtimeUpdate(payload: any) {
      const channelId = String(payload?.channelId || payload?.channel_id || "").trim();
      if (!channelId) {
        return;
      }
      const items = normalizeLayoutItems(payload?.items || []);
      if (items.length === 0) {
        return;
      }
      this.mergeLayouts(channelId, items);
    },

    resetChannel(channelId: string) {
      const normalizedChannelId = String(channelId || "").trim();
      if (!normalizedChannelId) {
        return;
      }
      const layouts = { ...this.layoutsByChannel };
      const missing = { ...this.knownMissingByChannel };
      delete layouts[normalizedChannelId];
      delete missing[normalizedChannelId];
      this.layoutsByChannel = layouts;
      this.knownMissingByChannel = missing;

      const pending = { ...this.pendingFetchKeys };
      Object.keys(pending).forEach((key) => {
        if (key.startsWith(normalizedChannelId + "::")) {
          delete pending[key];
        }
      });
      this.pendingFetchKeys = pending;
    },
  },
});
