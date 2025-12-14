import { defineStore } from "pinia"
import type { ServerConfig, UserInfo } from "@/types";
import { Howl, Howler } from 'howler';

import axiosFactory from "axios"
import { cloneDeep } from "lodash-es";
import { useWindowSize } from '@vueuse/core'

import type { AxiosResponse } from "axios";
import { api } from "./_config";
import { useChatStore } from "./chat";
import { useUserStore } from "./user";

export const DEFAULT_PAGE_TITLE = '海豹尬聊 SealChat';
export const applyPageTitle = (title?: string | null) => {
  if (typeof document === 'undefined') return;
  const trimmed = title?.trim() || '';
  document.title = trimmed.length > 0 ? trimmed : DEFAULT_PAGE_TITLE;
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
      const resp = await api.get('api/v1/config', {
        headers: { 'Authorization': user.token }
      })
      this.config = resp.data as ServerConfig;
      applyPageTitle(this.config?.pageTitle);
      return resp
    },

    async botTokenList() {
      const user = useUserStore();
      const resp = await api.get('api/v1/admin/bot-token-list', {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async botTokenAdd(input: string | { name: string; avatar?: string; nickColor?: string }) {
      const user = useUserStore();
      const payload = typeof input === 'string' ? { name: input } : input;
      const resp = await api.post('api/v1/admin/bot-token-add', payload, {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async botTokenUpdate(payload: { id: string; name?: string; avatar?: string; nickColor?: string }) {
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

    async configSet(data: ServerConfig) {
      const user = useUserStore();
      const resp = await api.put('api/v1/config', data, {
        headers: { 'Authorization': user.token }
      })
      this.config = cloneDeep(data);
      applyPageTitle(this.config?.pageTitle);
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
