import { defineStore } from "pinia"
import type { BotOneBotConfig, ServerConfig, UserInfo } from "@/types";
import { Howl, Howler } from 'howler';

import axiosFactory from "axios"
import { cloneDeep } from "lodash-es";
import { useWindowSize } from '@vueuse/core'

import type { AxiosResponse } from "axios";
import { api, urlBase } from "./_config";
import { useChatStore } from "./chat";
import { useDisplayStore } from "./display";
import { useUserStore } from "./user";

const resolveDefaultPageTitle = () => {
  if (typeof document === 'undefined') {
    return '海豹尬聊 SealChat';
  }
  const trimmed = document.title?.trim();
  return trimmed && trimmed.length > 0 ? trimmed : '海豹尬聊 SealChat';
};

export const DEFAULT_PAGE_TITLE = resolveDefaultPageTitle();
export const applyPageTitle = (title?: string | null) => {
  if (typeof document === 'undefined') return;
  const trimmed = title?.trim() || '';
  document.title = trimmed.length > 0 ? trimmed : DEFAULT_PAGE_TITLE;
};

const DEFAULT_FAVICON_HREF = `${urlBase}/favicon.ico?v=default`;

const normalizeFaviconAttachmentId = (attachmentId?: string | null) => {
  const trimmed = attachmentId?.trim() || '';
  if (!trimmed) return '';
  return trimmed.startsWith('id:') ? trimmed.slice(3) : trimmed;
};

const upsertFaviconLink = (rel: string, href: string) => {
  if (typeof document === 'undefined') return;
  let link = document.head.querySelector<HTMLLinkElement>(`link[rel="${rel}"]`);
  if (!link) {
    link = document.createElement('link');
    link.rel = rel;
    document.head.appendChild(link);
  }
  link.href = href;
};

export const applyPageFavicon = (attachmentId?: string | null) => {
  if (typeof document === 'undefined') return;
  const normalized = normalizeFaviconAttachmentId(attachmentId);
  const href = normalized
    ? `${urlBase}/api/v1/attachment/${encodeURIComponent(normalized)}?v=${encodeURIComponent(normalized)}`
    : DEFAULT_FAVICON_HREF;
  upsertFaviconLink('icon', href);
  upsertFaviconLink('shortcut icon', href);
};

// 未读消息数量标题通知
let _unreadCount = 0;
let _currentChannelName = ''; // 当前频道名字（作为默认标题）

// 设置当前频道名字作为默认标题
export const setChannelTitle = (channelName: string) => {
  if (typeof document === 'undefined') return;
  _currentChannelName = channelName;
  // 只有在没有未读消息时才更新标题
  if (_unreadCount === 0) {
    document.title = channelName || DEFAULT_PAGE_TITLE;
  }
};

export const updateUnreadTitleNotification = (count: number, channelName: string) => {
  if (typeof document === 'undefined') return;
  _unreadCount = count;

  if (count > 0 && channelName) {
    document.title = `有${count}条新消息 | ${channelName}`;
  } else {
    // 恢复为当前频道名字
    document.title = _currentChannelName || DEFAULT_PAGE_TITLE;
  }
};

export const clearUnreadTitleNotification = () => {
  if (typeof document === 'undefined') return;
  _unreadCount = 0;
  // 恢复为当前频道名字
  document.title = _currentChannelName || DEFAULT_PAGE_TITLE;
};

interface SoundItem {
  sound: Howl;
  time: number;
  playing: boolean;
}

interface UtilsState {
  config: ServerConfig | null;
  botCommands: { [key: string]: any };
  sounds: Map<string, SoundItem>;
  soundsTimer: any;
  pageWidth: any;
}

export const useUtilsStore = defineStore({
  id: 'utils',

  state: (): UtilsState => ({
    config: null,
    botCommands: {} as any,
    sounds: new Map<string, SoundItem>(),
    soundsTimer: null,
    pageWidth: useWindowSize().width,
  }),

  getters: {
    fileSizeLimit: (state) => {
      if (state.config) {
        return state.config.imageSizeLimit * 1024;
      }
      return 2 * 1024 * 1024
    },

    isSmallPage: (state) => {
      if (state.pageWidth < 700) {
        return true;
      }
      return false;
    }
  },

  actions: {
    async soundsTryInit() {
      if (this.soundsTimer) return;
      this.soundsTimer = setInterval(() => {
        for (let [k, v] of this.sounds.entries()) {
          v.time = v.sound.seek();
        }
      }, 1000);
    },

    async configGet() {
      const user = useUserStore();
      const display = useDisplayStore();
      const resp = await api.get('api/v1/config', {
        headers: { 'Authorization': user.token }
      })
      this.config = resp.data as ServerConfig;
      applyPageTitle(this.config?.pageTitle);
      applyPageFavicon(this.config?.faviconAttachmentId);
      display.syncPlatformThemeManagement(this.config?.themeManagement);
      return resp
    },

    async botTokenList(params?: { keyword?: string; scope?: 'manual' | 'system' | 'all' }) {
      const user = useUserStore();
      const resp = await api.get('api/v1/admin/bot-token-list', {
        headers: { 'Authorization': user.token },
        params,
      })
      return resp
    },

    async botTokenAdd(input: string | { name: string; avatar?: string; nickColor?: string; onebotConfig?: BotOneBotConfig }) {
      const user = useUserStore();
      const payload = typeof input === 'string' ? { name: input } : input;
      const resp = await api.post('api/v1/admin/bot-token-add', payload, {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async botTokenUpdate(payload: { id: string; name?: string; avatar?: string; nickColor?: string; onebotConfig?: BotOneBotConfig }) {
      const user = useUserStore();
      const resp = await api.post('api/v1/admin/bot-token-update', payload, {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async botTokenDelete(id: string) {
      const user = useUserStore();
      const resp = await api.post(`api/v1/admin/bot-token-delete`, {}, {
        headers: { 'Authorization': user.token },
        params: { id },
      })
      return resp
    },

    async botTokenBatchDelete(ids: string[]) {
      const user = useUserStore();
      const resp = await api.post('api/v1/admin/bot-token-batch-delete', { ids }, {
        headers: { 'Authorization': user.token },
      })
      return resp
    },

    async cleanupOrphanSystemBots() {
      const user = useUserStore();
      const resp = await api.post('api/v1/admin/system-bots/cleanup-orphaned', {}, {
        headers: { 'Authorization': user.token },
      })
      return resp
    },

    async configSet(data: ServerConfig) {
      const user = useUserStore();
      const display = useDisplayStore();
      const resp = await api.put('api/v1/config', data, {
        headers: { 'Authorization': user.token }
      })
      this.config = cloneDeep(data);
      applyPageTitle(this.config?.pageTitle);
      applyPageFavicon(this.config?.faviconAttachmentId);
      display.syncPlatformThemeManagement(this.config?.themeManagement);
      return resp
    },

    async adminUserList(params?: {
      page?: number;
      pageSize?: number;
      keyword?: string;
      type?: string
    }) {
      const user = useUserStore();
      const resp = await api.get('api/v1/admin/user-list', {
        headers: { 'Authorization': user.token },
        params: params
      })
      return resp
    },

    async adminUpdateStatus() {
      const user = useUserStore();
      const resp = await api.get('api/v1/admin/update-status', {
        headers: { 'Authorization': user.token },
      });
      return resp;
    },

    async adminUpdateCheck() {
      const user = useUserStore();
      const resp = await api.post('api/v1/admin/update-check', null, {
        headers: { 'Authorization': user.token },
      });
      return resp;
    },

    async adminUpdateVersion(currentVersion: string) {
      const user = useUserStore();
      const resp = await api.post('api/v1/admin/update-version', {
        currentVersion: currentVersion,
      }, {
        headers: { 'Authorization': user.token },
      });
      return resp;
    },

    async userResetPassword(id: string) {
      const user = useUserStore();
      const resp = await api.post(`api/v1/admin/user-password-reset`, null, {
        headers: { 'Authorization': user.token },
        params: { id },
      })
      return resp
    },

    async userEnable(id: string) {
      const user = useUserStore();
      const resp = await api.post(`api/v1/admin/user-enable`, null, {
        headers: { 'Authorization': user.token },
        params: { id },
      })
      return resp
    },

    async userDisable(id: string) {
      const user = useUserStore();
      const resp = await api.post(`api/v1/admin/user-disable`, null, {
        headers: { 'Authorization': user.token },
        params: { id },
      })
      return resp
    },

    // 添加用户角色
    async userRoleLinkByUserId(userId: string, roleIds: string[]) {
      const user = useUserStore();
      const resp = await api.post<{ data: boolean }>('api/v1/admin/user-role-link-by-user-id', { userId, roleIds }, {
        headers: { 'Authorization': user.token },
      });
      return resp?.data;
    },

    // 移除用户角色
    async userRoleUnlinkByUserId(userId: string, roleIds: string[]) {
      const resp = await api.post<{ data: boolean }>('api/v1/admin/user-role-unlink-by-user-id', { userId, roleIds });
      return resp?.data;
    },

    // 创建用户
    async adminUserCreate(data: {
      username: string;
      nickname: string;
      password: string;
      roleIds?: string[];
      disabled?: boolean;
    }) {
      const user = useUserStore();
      const resp = await api.post('api/v1/admin/user-create', data, {
        headers: { 'Authorization': user.token },
      });
      return resp;
    },

    // 检查用户名是否可用
    async adminCheckUsername(username: string) {
      const user = useUserStore();
      const resp = await api.get<{ available: boolean }>('api/v1/admin/user-check-username', {
        headers: { 'Authorization': user.token },
        params: { username },
      });
      return resp.data;
    },

    // 获取批量导入模板下载URL
    getImportTemplateUrl() {
      return `${api.defaults.baseURL}api/v1/admin/user-import-template`;
    },

    // 批量导入用户
    async adminUserBatchCreate(file: File) {
      const user = useUserStore();
      const formData = new FormData();
      formData.append('file', file);
      const resp = await api.post('api/v1/admin/user-batch-create', formData, {
        headers: {
          'Authorization': user.token,
          'Content-Type': 'multipart/form-data',
        },
      });
      return resp;
    },

    async adminBackupList() {
      const user = useUserStore();
      const resp = await api.get('api/v1/admin/backup/list', {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async adminBackupExecute() {
      const user = useUserStore();
      const resp = await api.post('api/v1/admin/backup/execute', {}, {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async adminBackupDelete(filename: string) {
      const user = useUserStore();
      const resp = await api.post('api/v1/admin/backup/delete', { filename }, {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async adminSQLiteVacuumExecute() {
      const user = useUserStore();
      const resp = await api.post('api/v1/admin/sqlite/vacuum', {}, {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async adminSQLiteVacuumStatus() {
      const user = useUserStore();
      const resp = await api.get('api/v1/admin/sqlite/vacuum/status', {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async commandsRefresh() {
      const user = useUserStore();
      const resp = await api.get(`api/v1/commands`, {
        headers: { 'Authorization': user.token }
      })
      this.botCommands = resp.data as any;
      return resp
    },
  },
})
