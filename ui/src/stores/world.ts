import { defineStore } from 'pinia';
import { api } from './_config';

export interface WorldModel {
  id: string;
  name: string;
  slug: string;
  avatar?: string;
  banner?: string;
  description?: string;
  visibility?: string;
  joinPolicy?: string;
  memberCount?: number;
  isMember?: boolean;
  isOwner?: boolean;
  ownerId?: string;
}

export interface WorldMemberModel {
  id: string;
  worldId: string;
  userId: string;
  nickname?: string;
  state?: string;
  joinedAt?: string;
}

export interface WorldInviteModel {
  id: string;
  worldId: string;
  code: string;
  channelId?: string;
  createdBy?: string;
  maxUses?: number;
  usedCount?: number;
  expiredAt?: string;
  isSingleUse?: boolean;
  isRevoked?: boolean;
  createdAt?: string;
}

interface WorldState {
  currentWorldId: string;
  currentWorldSlug: string;
  list: WorldModel[];
  loading: boolean;
  members: WorldMemberModel[];
  invites: WorldInviteModel[];
  membersLoading: boolean;
  invitesLoading: boolean;
}

export const useWorldStore = defineStore('world', {
  state: (): WorldState => ({
    currentWorldId: localStorage.getItem('worldId') || '',
    currentWorldSlug: localStorage.getItem('worldSlug') || '',
    list: [],
    loading: false,
  members: [],
  invites: [],
  membersLoading: false,
  invitesLoading: false,
  }),
  getters: {
    currentWorld(state): WorldModel | undefined {
      return state.list.find((w) => w.id === state.currentWorldId || w.slug === state.currentWorldSlug);
    },
  },
  actions: {
    canAccessWorld(world?: Partial<WorldModel> | null) {
      if (!world) return false;
      if (world.isMember !== false) {
        return true;
      }
      const joinPolicy = (world.joinPolicy || '').toLowerCase();
      const visibility = (world.visibility || '').toLowerCase();
      return joinPolicy === 'open' && visibility !== 'private';
    },
    async fetchWorlds(query?: string) {
      this.loading = true;
      try {
        const resp = await api.get<{ data: WorldModel[] }>('/api/v1/worlds', {
          params: { q: query, limit: 50 },
        });
        this.list = resp.data?.data || [];
        const preferred = this.list.find((w) => w.id === this.currentWorldId && this.canAccessWorld(w));
        if (preferred) {
          this.setCurrentWorld(preferred);
        } else {
          const fallback = this.list.find((w) => this.canAccessWorld(w));
          if (fallback) {
            this.setCurrentWorld(fallback);
          } else {
            this.clearWorld();
          }
        }
      } finally {
        this.loading = false;
      }
    },
    async fetchWorldBySlug(slug: string) {
      if (!slug) return null;
      const resp = await api.get<{ world: WorldModel }>(`/api/v1/worlds/${slug}`);
      const world = resp.data?.world;
      if (world) {
        const exists = this.list.find((w) => w.id === world.id);
        if (!exists) {
          this.list = [world, ...this.list];
        }
        if (this.canAccessWorld(world)) {
          this.setCurrentWorld(world);
        }
      }
      return world;
    },
    setCurrentWorld(world: Pick<WorldModel, 'id'> & Partial<Pick<WorldModel, 'slug' | 'isMember'>>) {
      if (!world?.id) return;
      if (!this.canAccessWorld(world)) {
        return;
      }
      const slug = world.slug || this.currentWorldSlug || '';
      this.currentWorldId = world.id;
      this.currentWorldSlug = slug;
      localStorage.setItem('worldId', world.id);
      if (slug) {
        localStorage.setItem('worldSlug', slug);
      } else {
        localStorage.removeItem('worldSlug');
      }
    },
    clearWorld() {
      this.currentWorldId = '';
      this.currentWorldSlug = '';
      localStorage.removeItem('worldId');
      localStorage.removeItem('worldSlug');
    },
    async fetchMembers(worldId?: string) {
      const targetId = worldId || this.currentWorldId;
      if (!targetId) {
        this.members = [];
        return;
      }
      this.membersLoading = true;
      try {
        const resp = await api.get<{ members: WorldMemberModel[] }>(`/api/v1/worlds/${targetId}/members`);
        this.members = resp.data?.members || [];
      } finally {
        this.membersLoading = false;
      }
    },
    async fetchInvites(worldId?: string) {
      const targetId = worldId || this.currentWorldId;
      if (!targetId) {
        this.invites = [];
        return;
      }
      this.invitesLoading = true;
      try {
        const resp = await api.get<{ invites: WorldInviteModel[] }>(`/api/v1/worlds/${targetId}/invites`);
        this.invites = resp.data?.invites || [];
      } finally {
        this.invitesLoading = false;
      }
    },
    async createInvite(payload: { channelId?: string; maxUses?: number; expireHours?: number; isSingleUse?: boolean }) {
      if (!this.currentWorldId) return null;
      const body: Record<string, any> = {};
      if (payload.channelId) body.channelId = payload.channelId;
      if (typeof payload.maxUses === 'number') body.maxUses = payload.maxUses;
      if (typeof payload.expireHours === 'number') body.expireHours = payload.expireHours;
      if (typeof payload.isSingleUse === 'boolean') body.isSingleUse = payload.isSingleUse;
      const resp = await api.post<{ invite: WorldInviteModel }>(
        `/api/v1/worlds/${this.currentWorldId}/invites`,
        body,
      );
      await this.fetchInvites();
      return resp.data?.invite;
    },
    async removeMember(userId: string) {
      if (!this.currentWorldId) return;
      await api.delete(`/api/v1/worlds/${this.currentWorldId}/members/${userId}`);
      await this.fetchMembers();
      await this.fetchWorlds();
    },
    async updateWorld(payload: Omit<WorldModel, 'id'> & { visibility?: string; joinPolicy?: string }) {
      if (!this.currentWorldId) return null;
      const resp = await api.put<{ world: WorldModel }>(`/api/v1/worlds/${this.currentWorldId}`, payload);
      const updated = resp.data?.world;
      if (updated) {
        this.list = this.list.map((world) => (world.id === updated.id ? updated : world));
        this.setCurrentWorld(updated);
      }
      return updated;
    },
    async deleteWorld(worldId?: string) {
      const targetId = worldId || this.currentWorldId;
      if (!targetId) return;
      await api.delete(`/api/v1/worlds/${targetId}`);
      if (this.currentWorldId === targetId) {
        this.clearWorld();
      }
      await this.fetchWorlds();
    },
  },
});
