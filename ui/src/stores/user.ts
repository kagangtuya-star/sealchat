import { defineStore } from "pinia"
import type { UserEmojiModel, UserInfo } from "@/types";
import Cookies from 'js-cookie';
// import router from "@/router";
import type { AxiosResponse } from "axios";
import { api } from "./_config";
import { useChatStore } from "./chat";
import { PermResult, type PermCheckKey } from "@/types-perm";
import type { SystemRolePermSheet } from "@/types-perm-system";

interface UserState {
  _accessToken: string
  info: UserInfo;
  lastCheckTime: number;
  emojiCount: number,

  permSysMap: SystemRolePermSheet;
}

export const useUserStore = defineStore({
  id: 'user',

  state: (): UserState => ({
    _accessToken: '',
    lastCheckTime: 0,
    emojiCount: 1,

    permSysMap: {} as any,
    // 这样比info?好的地方在于可以建立watch关联
    info: {
      id: "",
      createdAt: "",
      updatedAt: "",
      deletedAt: null,
      username: "",
      nick: '',
      avatar: '',
      brief: '',
      disabled: false,
    },
  }),

  getters: {
    token: (state) => {
      if (!state._accessToken) {
        state._accessToken = localStorage.getItem('accessToken') || '';
        Cookies.set('Authorization', state._accessToken);
        // state._accessToken = Cookies.get('accessToken') || '';
      }
      return state._accessToken;
    },
    /** 判断用户是否使用默认头像（avatar 为空或未设置） */
    hasDefaultAvatar: (state) => !state.info.avatar || state.info.avatar.trim() === '',
  },

  actions: {
    async changePassword(form: { password: string, passwordNew: string }) {
      const resp = await api.post('api/v1/user-password-change', {
        password: form.password, passwordNew: form.passwordNew
      }, {
        headers: { 'Authorization': this.token }
      })

      // 密码重置后，之前的所有token都会被重置
      const data = resp.data as { token: string, message: string };
      const accessToken = data.token;
      return resp;
    },

    async signIn(payload: { username: string; password: string; captchaId?: string; captchaValue?: string; turnstileToken?: string }) {
      // 在此处进行用户鉴权操作，获取 accessToken
      const resp = await api.post('api/v1/user-signin', {
        username: payload.username,
        password: payload.password,
        captchaId: payload.captchaId,
        captchaValue: payload.captchaValue,
        turnstileToken: payload.turnstileToken,
      })

      const data = resp.data as { token: string, message: string };
      const accessToken = data.token;

      // 将 accessToken 存入 localStorage 中
      // Cookies.set('accessToken', accessToken, { expires: 7 })
      localStorage.setItem('accessToken', accessToken);

      // 更新 state 中的 accessToken
      this._accessToken = accessToken;

      return resp;
    },

    async timelineList() {
      const resp = await api.get('api/v1/timeline-list', {
        headers: { 'Authorization': this.token }
      });
      return resp;
    },

    // 强制更新用户信息
    async infoUpdate() {
      const resp = await api.get('api/v1/user-info', {
        headers: { 'Authorization': this.token }
      })

      this.info = resp.data.user as UserInfo;

      let permSysMap: { [key: string]: number } = {};
      for (let i of resp.data.permSys) {
        permSysMap[i] = PermResult.ALLOWED;
      }
      this.permSysMap = permSysMap as any;
      return this.info;
    },

    async changeInfo(info: { nick: string, brief: string }) {
      const resp = await api.post('api/v1/user-info-update', info, {
        headers: { 'Authorization': this.token }
      })
      return resp;
    },

    async checkUserSession(options?: { force?: boolean }): Promise<'ok' | 'unauthenticated' | 'network-error'> {
      const now = Date.now();
      const hasToken = !!this.token;
      const shouldCheck = options?.force || !this.info?.id || now - this.lastCheckTime > 60 * 1000;

      if (!shouldCheck && hasToken) {
        return 'ok';
      }

      if (!hasToken) {
        return 'unauthenticated';
      }

      try {
        const firstTime = !this.info?.id;
        await this.infoUpdate();
        if (firstTime) {
          useChatStore().tryInit();
        }
        this.lastCheckTime = Date.now();
        return 'ok';
      } catch (e: any) {
        const statusCode = e?.response?.status;
        const networkError = e?.code === 'ERR_NETWORK' || !statusCode;
        if (networkError) {
          console.warn('checkUserSession network issue, keep session as-is');
          this.lastCheckTime = now;
          return 'network-error';
        }
        if (statusCode === 401 || statusCode === 403) {
          this.info.id = '';
          localStorage.removeItem('accessToken');
          this._accessToken = '';
        }
        this.lastCheckTime = 0;
        return 'unauthenticated';
      }
    },

    async signUp(form: { username: string; password: string; nickname: string; captchaId?: string; captchaValue?: string; turnstileToken?: string }) {
      try {
        // 在此处进行用户鉴权操作，获取 accessToken
        const resp = await api.post('api/v1/user-signup', {
          username: form.username,
          password: form.password,
          nickname: form.nickname,
          captchaId: form.captchaId,
          captchaValue: form.captchaValue,
          turnstileToken: form.turnstileToken,
        })

        const data = resp.data as { token: string, message: string };
        const accessToken = data.token;

        // 将 accessToken 存入 localStorage 中
        localStorage.setItem('accessToken', accessToken)
        // Cookies.set('accessToken', accessToken, { expires: 7 })

        // 更新 state 中的 accessToken
        this._accessToken = accessToken

        return ''
      } catch (err) {
        // console.error('Authentication failed:', err)
        return (err as any).response?.data?.message || '错误';
      }
    },

    logout() {
      // 将 accessToken 从 localStorage 中删除
      localStorage.removeItem('accessToken')
      this.info.id = ''
      // 更新 state 中的 accessToken
      this._accessToken = ''
    },

    async emojiAdd(attachmentId: string, remark?: string) {
      const user = useUserStore();
      const resp = await api.post('api/v1/user-emoji-add', { attachmentId, remark }, {
        headers: { 'Authorization': user.token }
      });
      this.emojiCount += 1;
      return resp;
    },

    async emojiDelete(ids: string[]) {
      const user = useUserStore();
      const resp = await api.post('api/v1/user-emoji-delete', { ids }, {
        headers: { 'Authorization': user.token }
      });
      this.emojiCount += 1;
      return resp;
    },

    async emojiUpdate(id: string, payload: { remark: string }) {
      const user = useUserStore();
      const resp = await api.patch(`api/v1/user-emoji/${id}`, payload, {
        headers: { 'Authorization': user.token }
      });
      this.emojiCount += 1;
      return resp;
    },

    async emojiList(): Promise<AxiosResponse<{ items: UserEmojiModel[] }, any>> {
      const user = useUserStore();
      const resp = await api.get('api/v1/user-emoji-list', {
        headers: { 'Authorization': user.token }
      });
      return resp;
    },

    // 满足任意一个即可，这个read是啥意思我也忘了
    checkPerm(...keys: Array<keyof SystemRolePermSheet>) {
      for (let key of keys) {
        if (this.permSysMap[key] === PermResult.ALLOWED) {
          return true;
        }
      }
    },

  },
})
