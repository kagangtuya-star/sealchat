export const THEATER_FLOATING_TAKEOVER_REQUEST = 'sealchat:theater-floating-takeover-request:v1' as const;
export const THEATER_FLOATING_TAKEOVER_ACK = 'sealchat:theater-floating-takeover-ack:v1' as const;
export const CHAT_FLOATING_TAKEOVER_REQUEST = 'sealchat:chat-floating-takeover-request:v1' as const;
export const CHAT_FLOATING_TAKEOVER_ACK = 'sealchat:chat-floating-takeover-ack:v1' as const;
export const THEATER_CHAT_FLOATING_OPEN_REQUEST = 'sealchat:theater-chat-floating-open-request:v1' as const;
export const THEATER_CHAT_FLOATING_OPEN_ACK = 'sealchat:theater-chat-floating-open-ack:v1' as const;

export interface TheaterFloatingResource {
  key: string;
  url: string;
  title: string;
  presentation?: {
    chrome?: 'default' | 'minimal';
    minimized?: boolean;
    avatarUrl?: string;
    width?: number;
    height?: number;
  };
}

export interface TheaterFloatingTakeoverRequest {
  type: typeof THEATER_FLOATING_TAKEOVER_REQUEST;
  requestId: string;
  resource: TheaterFloatingResource;
  /** Missing intent is treated as a drag transfer for backwards compatibility. */
  intent?: 'transfer' | 'open';
  clientX: number;
  clientY: number;
}

export interface TheaterFloatingTakeoverAck {
  type: typeof THEATER_FLOATING_TAKEOVER_ACK;
  requestId: string;
  accepted: boolean;
}

export interface ChatFloatingTakeoverRequest {
  type: typeof CHAT_FLOATING_TAKEOVER_REQUEST;
  requestId: string;
  resource: TheaterFloatingResource;
  clientX: number;
  clientY: number;
  offsetX: number;
  offsetY: number;
}

export interface ChatFloatingTakeoverAck {
  type: typeof CHAT_FLOATING_TAKEOVER_ACK;
  requestId: string;
  accepted: boolean;
}

export interface TheaterChatFloatingOpenRequest {
  type: typeof THEATER_CHAT_FLOATING_OPEN_REQUEST;
  requestId: string;
  channelId: string;
}

export interface TheaterChatFloatingOpenAck {
  type: typeof THEATER_CHAT_FLOATING_OPEN_ACK;
  requestId: string;
  accepted: boolean;
}

let requestCounter = 0;

export type TheaterFloatingCoordinateEvent = Pick<MouseEvent, 'clientX' | 'clientY'>;

export const isTheaterChatFrame = () => {
  if (typeof window === 'undefined' || window.parent === window) return false;
  const hash = window.location.hash;
  const queryIndex = hash.indexOf('?');
  if (!hash.startsWith('#/embed') || queryIndex < 0) return false;
  return new URLSearchParams(hash.slice(queryIndex + 1)).get('mode') === 'theater';
};

const resolveParentPoint = (event: TheaterFloatingCoordinateEvent) => {
  const frame = window.frameElement;
  if (!frame || typeof frame.getBoundingClientRect !== 'function') return null;
  const rect = frame.getBoundingClientRect();
  return {
    clientX: rect.left + event.clientX,
    clientY: rect.top + event.clientY,
  };
};

export const requestTheaterFloatingTakeover = (
  resource: TheaterFloatingResource,
  event: TheaterFloatingCoordinateEvent,
): Promise<boolean> => {
  if (!isTheaterChatFrame()) return Promise.resolve(false);
  const point = resolveParentPoint(event);
  if (!point) return Promise.resolve(false);

  const requestId = `theater-floating-${Date.now()}-${++requestCounter}`;
  const request: TheaterFloatingTakeoverRequest = {
    type: THEATER_FLOATING_TAKEOVER_REQUEST,
    requestId,
    resource,
    intent: 'transfer',
    ...point,
  };

  return new Promise<boolean>((resolve) => {
    let settled = false;
    const finish = (accepted: boolean) => {
      if (settled) return;
      settled = true;
      window.clearTimeout(timeoutId);
      window.removeEventListener('message', handleMessage);
      resolve(accepted);
    };
    const handleMessage = (message: MessageEvent<unknown>) => {
      if (message.origin !== window.location.origin || message.source !== window.parent) return;
      const data = message.data as Partial<TheaterFloatingTakeoverAck> | null;
      if (
        data?.type !== THEATER_FLOATING_TAKEOVER_ACK
        || data.requestId !== requestId
        || typeof data.accepted !== 'boolean'
      ) return;
      finish(data.accepted);
    };
    const timeoutId = window.setTimeout(() => finish(false), 800);
    window.addEventListener('message', handleMessage);
    window.parent.postMessage(request, window.location.origin);
  });
};

export const requestTheaterFloatingOpen = (
  resource: TheaterFloatingResource,
  event: TheaterFloatingCoordinateEvent,
): Promise<boolean> => {
  if (!isTheaterChatFrame()) return Promise.resolve(false);
  const point = resolveParentPoint(event);
  if (!point) return Promise.resolve(false);

  const requestId = `theater-floating-open-${Date.now()}-${++requestCounter}`;
  const request: TheaterFloatingTakeoverRequest = {
    type: THEATER_FLOATING_TAKEOVER_REQUEST,
    requestId,
    resource,
    intent: 'open',
    ...point,
  };

  return new Promise<boolean>((resolve) => {
    let settled = false;
    const finish = (accepted: boolean) => {
      if (settled) return;
      settled = true;
      window.clearTimeout(timeoutId);
      window.removeEventListener('message', handleMessage);
      resolve(accepted);
    };
    const handleMessage = (message: MessageEvent<unknown>) => {
      if (message.origin !== window.location.origin || message.source !== window.parent) return;
      const data = message.data as Partial<TheaterFloatingTakeoverAck> | null;
      if (
        data?.type !== THEATER_FLOATING_TAKEOVER_ACK
        || data.requestId !== requestId
        || typeof data.accepted !== 'boolean'
      ) return;
      finish(data.accepted);
    };
    const timeoutId = window.setTimeout(() => finish(false), 800);
    window.addEventListener('message', handleMessage);
    window.parent.postMessage(request, window.location.origin);
  });
};

export const requestTheaterChatFloatingOpen = (channelId: string): Promise<boolean> => {
  if (!isTheaterChatFrame()) return Promise.resolve(false);
  const normalizedChannelId = channelId.trim();
  if (!normalizedChannelId) return Promise.resolve(false);

  const requestId = `theater-chat-floating-open-${Date.now()}-${++requestCounter}`;
  const request: TheaterChatFloatingOpenRequest = {
    type: THEATER_CHAT_FLOATING_OPEN_REQUEST,
    requestId,
    channelId: normalizedChannelId,
  };

  return new Promise<boolean>((resolve) => {
    let settled = false;
    const finish = (accepted: boolean) => {
      if (settled) return;
      settled = true;
      window.clearTimeout(timeoutId);
      window.removeEventListener('message', handleMessage);
      resolve(accepted);
    };
    const handleMessage = (message: MessageEvent<unknown>) => {
      if (message.origin !== window.location.origin || message.source !== window.parent) return;
      const data = message.data as Partial<TheaterChatFloatingOpenAck> | null;
      if (
        data?.type !== THEATER_CHAT_FLOATING_OPEN_ACK
        || data.requestId !== requestId
        || typeof data.accepted !== 'boolean'
      ) return;
      finish(data.accepted);
    };
    const timeoutId = window.setTimeout(() => finish(false), 800);
    window.addEventListener('message', handleMessage);
    window.parent.postMessage(request, window.location.origin);
  });
};

export const requestChatFloatingTakeover = (
  resource: TheaterFloatingResource,
  event: PointerEvent,
  chatFrame: HTMLIFrameElement,
  offset: { x: number; y: number },
): Promise<boolean> => {
  const target = chatFrame.contentWindow;
  if (!target) return Promise.resolve(false);
  const rect = chatFrame.getBoundingClientRect();
  if (
    event.clientX < rect.left
    || event.clientX > rect.right
    || event.clientY < rect.top
    || event.clientY > rect.bottom
  ) return Promise.resolve(false);

  const requestId = `chat-floating-${Date.now()}-${++requestCounter}`;
  const request: ChatFloatingTakeoverRequest = {
    type: CHAT_FLOATING_TAKEOVER_REQUEST,
    requestId,
    resource,
    clientX: event.clientX - rect.left,
    clientY: event.clientY - rect.top,
    offsetX: offset.x,
    offsetY: offset.y,
  };

  return new Promise<boolean>((resolve) => {
    let settled = false;
    const finish = (accepted: boolean) => {
      if (settled) return;
      settled = true;
      window.clearTimeout(timeoutId);
      window.removeEventListener('message', handleMessage);
      resolve(accepted);
    };
    const handleMessage = (message: MessageEvent<unknown>) => {
      if (message.origin !== window.location.origin || message.source !== target) return;
      const data = message.data as Partial<ChatFloatingTakeoverAck> | null;
      if (
        data?.type !== CHAT_FLOATING_TAKEOVER_ACK
        || data.requestId !== requestId
        || typeof data.accepted !== 'boolean'
      ) return;
      finish(data.accepted);
    };
    const timeoutId = window.setTimeout(() => finish(false), 5000);
    window.addEventListener('message', handleMessage);
    target.postMessage(request, window.location.origin);
  });
};

export const isTheaterFloatingTakeoverRequest = (
  value: unknown,
): value is TheaterFloatingTakeoverRequest => {
  if (!value || typeof value !== 'object') return false;
  const request = value as Partial<TheaterFloatingTakeoverRequest>;
  const resource = request.resource as Partial<TheaterFloatingResource> | undefined;
  const presentation = resource?.presentation;
  const validPresentation = presentation === undefined || (
    !!presentation
    && typeof presentation === 'object'
    && (presentation.chrome === undefined || presentation.chrome === 'default' || presentation.chrome === 'minimal')
    && (presentation.minimized === undefined || typeof presentation.minimized === 'boolean')
    && (presentation.avatarUrl === undefined || typeof presentation.avatarUrl === 'string')
    && (presentation.width === undefined || (typeof presentation.width === 'number' && Number.isFinite(presentation.width)))
    && (presentation.height === undefined || (typeof presentation.height === 'number' && Number.isFinite(presentation.height)))
  );
  return request.type === THEATER_FLOATING_TAKEOVER_REQUEST
    && typeof request.requestId === 'string'
    && !!request.requestId
    && (request.intent === undefined || request.intent === 'transfer' || request.intent === 'open')
    && typeof request.clientX === 'number'
    && Number.isFinite(request.clientX)
    && typeof request.clientY === 'number'
    && Number.isFinite(request.clientY)
    && typeof resource?.key === 'string'
    && !!resource.key
    && typeof resource.url === 'string'
    && !!resource.url
    && typeof resource.title === 'string'
    && validPresentation;
};

export const isChatFloatingTakeoverRequest = (
  value: unknown,
): value is ChatFloatingTakeoverRequest => {
  if (!value || typeof value !== 'object') return false;
  const request = value as Partial<ChatFloatingTakeoverRequest>;
  if (request.type !== CHAT_FLOATING_TAKEOVER_REQUEST) return false;
  return typeof request.offsetX === 'number'
    && Number.isFinite(request.offsetX)
    && typeof request.offsetY === 'number'
    && Number.isFinite(request.offsetY)
    && isTheaterFloatingTakeoverRequest({
      ...request,
      type: THEATER_FLOATING_TAKEOVER_REQUEST,
    });
};

export const isTheaterChatFloatingOpenRequest = (
  value: unknown,
): value is TheaterChatFloatingOpenRequest => {
  if (!value || typeof value !== 'object') return false;
  const request = value as Partial<TheaterChatFloatingOpenRequest>;
  return request.type === THEATER_CHAT_FLOATING_OPEN_REQUEST
    && typeof request.requestId === 'string'
    && !!request.requestId
    && typeof request.channelId === 'string'
    && !!request.channelId.trim();
};
