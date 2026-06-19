import { defineStore } from 'pinia';
import { api } from './_config';
import type { BattleReport, BattleReportDisplayChannel, BattleReportPayload } from '@/types';

interface BattleReportListResponse {
  items?: BattleReport[];
}

interface BattleReportItemResponse {
  item?: BattleReport;
}

interface BattleReportDisplayResponse {
  item?: BattleReportDisplayChannel | null;
}

interface BattleReportSummaryInputResponse {
  input?: string;
}

interface BattleReportState {
  itemsByChannel: Record<string, BattleReport[]>;
  detailById: Record<string, BattleReport>;
  displayByChannel: Record<string, BattleReportDisplayChannel | null>;
  loading: boolean;
  saving: boolean;
}

const normalizeItems = (items?: BattleReport[]): BattleReport[] => Array.isArray(items) ? items : [];

export const useBattleReportStore = defineStore('battleReport', {
  state: (): BattleReportState => ({
    itemsByChannel: Object.create(null),
    detailById: Object.create(null),
    displayByChannel: Object.create(null),
    loading: false,
    saving: false,
  }),
  actions: {
    upsertItem(item?: BattleReport) {
      if (!item?.id) {
        return;
      }
      const existingDetail = this.detailById[item.id];
      this.detailById[item.id] = {
        ...existingDetail,
        ...item,
        content: item.content !== undefined ? item.content : existingDetail?.content,
      };
      Object.keys(this.itemsByChannel).forEach((key) => {
        const current = this.itemsByChannel[key] || [];
        if (!current.some((candidate) => candidate.id === item.id)) {
          return;
        }
        this.itemsByChannel[key] = current.map((candidate) => candidate.id === item.id
          ? {
            ...candidate,
            ...item,
            content: item.content !== undefined ? item.content : candidate.content,
          }
          : candidate);
      });
      const channelId = item.channelId;
      if (!channelId) {
        return;
      }
      const current = this.itemsByChannel[channelId] || [];
      const index = current.findIndex((candidate) => candidate.id === item.id);
      const mergeListItem = (candidate: BattleReport) => ({
        ...candidate,
        ...item,
        content: item.content !== undefined ? item.content : candidate.content,
      });
      const next = index >= 0
        ? current.map((candidate) => candidate.id === item.id ? mergeListItem(candidate) : candidate)
        : [item, ...current];
      this.itemsByChannel[channelId] = next;
    },
    async list(channelId: string) {
      this.loading = true;
      try {
        const resp = await api.get<BattleReportListResponse>(`api/v1/channels/${channelId}/battle-reports`);
        const items = normalizeItems(resp.data?.items).map((item) => {
          const existingDetail = this.detailById[item.id];
          return {
            ...item,
            content: item.content !== undefined ? item.content : existingDetail?.content,
          };
        });
        this.itemsByChannel[channelId] = items;
        items.forEach((item) => this.upsertItem(item));
        return this.itemsByChannel[channelId] || items;
      } finally {
        this.loading = false;
      }
    },
    async get(reportId: string) {
      this.loading = true;
      try {
        const resp = await api.get<BattleReportItemResponse>(`api/v1/battle-reports/${reportId}`);
        const item = resp.data?.item;
        this.upsertItem(item);
        return item;
      } finally {
        this.loading = false;
      }
    },
    async create(channelId: string, payload: BattleReportPayload) {
      this.saving = true;
      try {
        const resp = await api.post<BattleReportItemResponse>(`api/v1/channels/${channelId}/battle-reports`, payload);
        const item = resp.data?.item;
        this.upsertItem(item);
        return item;
      } finally {
        this.saving = false;
      }
    },
    async update(reportId: string, payload: BattleReportPayload) {
      this.saving = true;
      try {
        const resp = await api.patch<BattleReportItemResponse>(`api/v1/battle-reports/${reportId}`, payload);
        const item = resp.data?.item;
        this.upsertItem(item);
        return item;
      } finally {
        this.saving = false;
      }
    },
    async delete(reportId: string) {
      this.saving = true;
      try {
        await api.delete(`api/v1/battle-reports/${reportId}`);
        delete this.detailById[reportId];
        Object.keys(this.itemsByChannel).forEach((channelId) => {
          this.itemsByChannel[channelId] = (this.itemsByChannel[channelId] || [])
            .filter((item) => item.id !== reportId);
        });
      } finally {
        this.saving = false;
      }
    },
    async reorder(channelId: string, ids: string[]) {
      await api.post(`api/v1/channels/${channelId}/battle-reports/reorder`, { ids });
    },
    async summarize(channelId: string, payload: BattleReportPayload) {
      this.saving = true;
      try {
        const resp = await api.post<BattleReportItemResponse>(`api/v1/channels/${channelId}/battle-reports/summarize`, payload);
        const item = resp.data?.item;
        this.upsertItem(item);
        return item;
      } finally {
        this.saving = false;
      }
    },
    async buildSummaryInput(channelId: string, payload: BattleReportPayload) {
      this.saving = true;
      try {
        const resp = await api.post<BattleReportSummaryInputResponse>(`api/v1/channels/${channelId}/battle-reports/summarize-input`, payload);
        return String(resp.data?.input || '');
      } finally {
        this.saving = false;
      }
    },
    async getDisplayChannel(channelId: string) {
      const resp = await api.get<BattleReportDisplayResponse>(`api/v1/channels/${channelId}/battle-report-display`);
      const item = resp.data?.item || null;
      this.displayByChannel[channelId] = item;
      if (item?.sourceChannelId) {
        this.displayByChannel[item.sourceChannelId] = item;
      }
      if (item?.displayChannelId) {
        this.displayByChannel[item.displayChannelId] = item;
      }
      return item;
    },
    async ensureDisplayChannel(channelId: string, displayName: string) {
      this.saving = true;
      try {
        const resp = await api.post<BattleReportDisplayResponse>(`api/v1/channels/${channelId}/battle-report-display`, { displayName });
        const item = resp.data?.item || null;
        this.displayByChannel[channelId] = item;
        if (item?.sourceChannelId) {
          this.displayByChannel[item.sourceChannelId] = item;
        }
        if (item?.displayChannelId) {
          this.displayByChannel[item.displayChannelId] = item;
        }
        return item;
      } finally {
        this.saving = false;
      }
    },
    async resyncDisplayChannel(channelId: string) {
      await api.post(`api/v1/channels/${channelId}/battle-report-display/resync`);
    },
    async disableDisplayChannel(channelId: string) {
      this.saving = true;
      try {
        await api.delete(`api/v1/channels/${channelId}/battle-report-display`);
        const existing = this.displayByChannel[channelId];
        if (existing?.sourceChannelId) {
          this.displayByChannel[existing.sourceChannelId] = null;
        }
        if (existing?.displayChannelId) {
          this.displayByChannel[existing.displayChannelId] = null;
        }
        this.displayByChannel[channelId] = null;
      } finally {
        this.saving = false;
      }
    },
    setChannelItems(channelId: string, items: BattleReport[]) {
      this.itemsByChannel[channelId] = items.map((item) => {
        const existingDetail = this.detailById[item.id];
        return {
          ...item,
          content: item.content !== undefined ? item.content : existingDetail?.content,
        };
      });
      items.forEach((item) => {
        if (item?.id) {
          this.upsertItem(item);
        }
      });
    },
  },
});
