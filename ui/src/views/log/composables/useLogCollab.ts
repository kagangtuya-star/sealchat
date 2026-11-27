// 设计协作：在线状态与消息编辑锁。每个打开工作台的连接在服务端登记一条 presence
// （所在来源片段、选中的消息、正在编辑的消息），状态变化时立即上报，平时按心跳续期；
// 服务端超时清理断线或停止心跳的连接，编辑锁随之释放。锁只用于协作提示与互斥打开编辑器，
// 消息写入仍由版本 CAS 兜底。
import { computed, onBeforeUnmount, ref, shallowRef, watch, type Ref } from 'vue';
import { chatEvent, useChatStore } from '@/stores/chat';
import type { LogPresence } from '@/types/log';

const HEARTBEAT_MS = 15000;
const STATE_DEBOUNCE_MS = 250;
const REQUEST_TIMEOUT_MS = 8000;

export type LogPresenceLockResult =
  | { granted: true }
  | { granted: false; holderName: string; cancelled?: false }
  | { granted: false; cancelled: true };

const cancelled: LogPresenceLockResult = { granted: false, cancelled: true };

interface PresenceResponse {
  sessionId: string;
  presences: LogPresence[] | null;
  lock?: { messageId: string; granted: boolean; holderUserId?: string; holderName?: string };
}

/** 同一用户总用同一颜色，便于在画布标记与在线列表之间对应。 */
export const presenceColor = (userId: string) => {
  let h = 0;
  for (let i = 0; i < userId.length; i++) h = (h * 31 + userId.charCodeAt(i)) >>> 0;
  return `hsl(${h % 360}, 62%, 42%)`;
};

export const useLogCollab = (options: {
  projectId: Ref<string>;
  enabled: () => boolean;
  /** 当前光标：所在来源片段与选中的消息（未选中时为空）。 */
  cursor: () => { sectionId: string; messageId: string };
  /** 心跳时发现编辑锁已被他人取得（例如本机休眠导致超时）。 */
  onLockLost?: (messageId: string, holderName: string) => void;
}) => {
  const chat = useChatStore();
  const all = shallowRef<LogPresence[]>([]);
  const sessionId = ref('');
  const editing = ref('');
  let disposed = false;
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;
  let heartbeat: ReturnType<typeof setInterval> | null = null;
  let sending: Promise<PresenceResponse | null> | null = null;
  let resend = false;
  let acquireSeq = 0;

  const active = () => !disposed && options.enabled() && !!options.projectId.value;

  const payload = () => {
    const c = options.cursor();
    return { projectId: options.projectId.value, sectionId: c.sectionId || '', messageId: c.messageId || '', editingMessageId: editing.value };
  };

  const adopt = (pid: string, resp: PresenceResponse | null, requested: string) => {
    if (!resp || !active() || pid !== options.projectId.value) return;
    if (resp.sessionId) sessionId.value = resp.sessionId;
    all.value = resp.presences || [];
    const lock = resp.lock;
    if (lock && !lock.granted && requested && editing.value === requested) {
      editing.value = '';
      options.onLockLost?.(requested, lock.holderName || '其他设计者');
    }
  };

  const sendUpdate = (data: ReturnType<typeof payload>, apply: (result: PresenceResponse | null) => void) => {
    const request = chat.sendAPI<{ data: PresenceResponse }>('log.presence.update', data, { timeoutMs: REQUEST_TIMEOUT_MS })
      .then((resp) => {
        const result = resp?.data || null;
        apply(result);
        return result;
      })
      .catch(() => null)
      .finally(() => {
        sending = null;
        if (resend) {
          resend = false;
          void push();
        }
      });
    sending = request;
    return request;
  };

  /** 心跳与编辑锁共用在途请求，续期不会覆盖刚授予的锁。 */
  const push = (): Promise<PresenceResponse | null> => {
    if (!active() || chat.connectState !== 'connected') return Promise.resolve(null);
    if (sending) {
      resend = true;
      return sending;
    }
    const data = payload();
    return sendUpdate(data, (result) => adopt(data.projectId, result, data.editingMessageId));
  };

  const schedulePush = () => {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      debounceTimer = null;
      void push();
    }, STATE_DEBOUNCE_MS);
  };

  /**
   * 申请编辑锁：服务端仲裁，同一消息同时只授予一位用户（同一用户的多个窗口不互斥）。
   * 连接不可用时视为未授予，调用方不应打开编辑器。
   */
  const acquire = async (messageId: string): Promise<LogPresenceLockResult> => {
    if (!active()) return cancelled;
    const pid = options.projectId.value;
    const seq = ++acquireSeq;
    const isCurrent = () => active() && pid === options.projectId.value && seq === acquireSeq;
    try {
      await chat.ensureConnectionReady({ timeoutMs: 5000 });
      while (sending && isCurrent()) await sending;
      if (!isCurrent()) return cancelled;
      const result = await sendUpdate({ ...payload(), editingMessageId: messageId }, (response) => {
        if (!active() || pid !== options.projectId.value) return;
        if (!isCurrent()) {
          // 已发出的申请仍可能改变服务端状态；重新上报当前持有的锁。
          resend = true;
          return;
        }
        if (!response || response.lock?.messageId !== messageId) return;
        sessionId.value = response.sessionId || sessionId.value;
        all.value = response.presences || [];
        if (response.lock.granted) editing.value = messageId;
        else if (editing.value === messageId) editing.value = '';
        else if (editing.value) resend = true;
      });
      if (!isCurrent()) return cancelled;
      const lock = result?.lock;
      if (!lock || lock.messageId !== messageId) {
        void push();
        throw new Error('编辑锁响应无效');
      }
      return lock.granted ? { granted: true } : { granted: false, holderName: lock.holderName || '其他设计者' };
    } catch (error) {
      if (!isCurrent()) return cancelled;
      throw error;
    }
  };

  const release = (messageId?: string) => {
    if (messageId && editing.value !== messageId) return;
    acquireSeq++;
    if (!editing.value) return;
    editing.value = '';
    void push();
  };

  const onPresence = (event: { projectId?: string; presences?: LogPresence[] | null }) => {
    if (disposed || !event || event.projectId !== options.projectId.value) return;
    all.value = event.presences || [];
  };
  // 重连后服务端已清除本连接的状态：重新登记，并重新申请仍在编辑的消息。
  const onConnected = () => {
    all.value = [];
    void push();
  };

  const leave = (pid: string) => {
    if (!pid || chat.connectState !== 'connected') return;
    void chat.sendAPI('log.presence.leave', { projectId: pid }, { timeoutMs: REQUEST_TIMEOUT_MS }).catch(() => undefined);
  };

  chatEvent.on('log-project-presence' as any, onPresence as any);
  chatEvent.on('connected', onConnected);
  heartbeat = setInterval(() => void push(), HEARTBEAT_MS);
  watch(() => {
    const c = options.cursor();
    return `${c.sectionId}\n${c.messageId}`;
  }, schedulePush);
  watch(options.projectId, (_, old) => {
    acquireSeq++;
    leave(old);
    all.value = [];
    editing.value = '';
    void push();
  });
  watch(() => options.enabled(), (on) => {
    if (on) void push();
    else {
      acquireSeq++;
      editing.value = '';
      all.value = [];
      leave(options.projectId.value);
    }
  });
  void push();

  onBeforeUnmount(() => {
    disposed = true;
    if (debounceTimer) clearTimeout(debounceTimer);
    if (heartbeat) clearInterval(heartbeat);
    chatEvent.off('log-project-presence' as any, onPresence as any);
    chatEvent.off('connected', onConnected);
    if (options.enabled()) leave(options.projectId.value);
  });

  /** 其他连接（含本人其他窗口）的状态；本连接自身不显示。 */
  const others = computed(() => all.value.filter((p) => p.sessionId !== sessionId.value));
  return { others, sessionId, editing, acquire, release, refresh: push };
};

export type LogCollabController = ReturnType<typeof useLogCollab>;
