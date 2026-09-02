export const THEATER_FLOATING_TAKEOVER_REQUEST = 'sealchat:theater-floating-takeover-request:v1' as const;
export const THEATER_FLOATING_TAKEOVER_ACK = 'sealchat:theater-floating-takeover-ack:v1' as const;
export const CHAT_FLOATING_TAKEOVER_REQUEST = 'sealchat:chat-floating-takeover-request:v1' as const;
export const CHAT_FLOATING_TAKEOVER_ACK = 'sealchat:chat-floating-takeover-ack:v1' as const;

export interface TheaterFloatingResource {
  key: string;
  url: string;
  title: string;
  presentation?: {
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

let requestCounter = 0;

const isTheaterChatFrame = () => {
  if (typeof window === 'undefined' || window.parent === window) return false;
  const hash = window.location.hash;
  const queryIndex = hash.indexOf('?');
  if (!hash.startsWith('#/embed') || queryIndex < 0) return false;
  return new URLSearchParams(hash.slice(queryIndex + 1)).get('mode') === 'theater';
};

const resolveParentPoint = (event: PointerEvent) => {
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
  event: PointerEvent,
): Promise<boolean> => {
  if (!isTheaterChatFrame()) return Promise.resolve(false);
  const point = resolveParentPoint(event);
  if (!point) return Promise.resolve(false);

  const requestId = `theater-floating-${Date.now()}-${++requestCounter}`;
  const request: TheaterFloatingTakeoverRequest = {
    type: THEATER_FLOATING_TAKEOVER_REQUEST,
    requestId,
    resource,
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
    && (presentation.minimized === undefined || typeof presentation.minimized === 'boolean')
    && (presentation.avatarUrl === undefined || typeof presentation.avatarUrl === 'string')
    && (presentation.width === undefined || (typeof presentation.width === 'number' && Number.isFinite(presentation.width)))
    && (presentation.height === undefined || (typeof presentation.height === 'number' && Number.isFinite(presentation.height)))
  );
  return request.type === THEATER_FLOATING_TAKEOVER_REQUEST
    && typeof request.requestId === 'string'
    && !!request.requestId
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
