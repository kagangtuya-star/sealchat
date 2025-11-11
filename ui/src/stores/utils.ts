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
      return resp
    },

    async botTokenList() {
      const user = useUserStore();
      const resp = await api.get('api/v1/admin/bot-token-list', {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async botTokenAdd(name: string) {
      const user = useUserStore();
      const resp = await api.post('api/v1/admin/bot-token-add', { name }, {
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

    async adminBotList() {
      const user = useUserStore();
      return api.get('api/v1/admin/bots', {
        headers: { 'Authorization': user.token }
      })
    },

    async adminBotCreate(payload: any) {
      const user = useUserStore();
      return api.post('api/v1/admin/bots', payload, {
        headers: { 'Authorization': user.token }
      })
    },

    async adminBotUpdate(id: string, payload: any) {
      const user = useUserStore();
      return api.patch(`api/v1/admin/bots/${id}`, payload, {
        headers: { 'Authorization': user.token }
      })
    },

    async adminBotDelete(id: string) {
      const user = useUserStore();
      return api.delete(`api/v1/admin/bots/${id}`, {
        headers: { 'Authorization': user.token }
      })
    },

    async adminBotTest(id: string) {
      const user = useUserStore();
      return api.post(`api/v1/admin/bots/${id}/test`, null, {
        headers: { 'Authorization': user.token }
      })
    },

    async botProfileOptions() {
      const user = useUserStore();
      return api.get('api/v1/bot-profiles', {
        headers: { 'Authorization': user.token }
      })
    },

    async channelBotSettingsGet(channelId: string) {
      const user = useUserStore();
      return api.get(`api/v1/channels/${channelId}/bot-settings`, {
        headers: { 'Authorization': user.token }
      })
    },

    async channelBotSettingsSave(channelId: string, payload: any) {
      const user = useUserStore();
      return api.patch(`api/v1/channels/${channelId}/bot-settings`, payload, {
        headers: { 'Authorization': user.token }
      })
    },

    async configSet(data: ServerConfig) {
      const user = useUserStore();
      const resp = await api.put('api/v1/config', data, {
        headers: { 'Authorization': user.token }
      })
      return resp
    },

    async adminUserList() {
      const user = useUserStore();
      const resp = await api.get('api/v1/admin/user-list', {
        headers: { 'Authorization': user.token }
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
