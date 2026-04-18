export interface ChannelIFormMediaOptions {
  autoPlay?: boolean;
  autoUnmute?: boolean;
  autoExpand?: boolean;
  allowAudio?: boolean;
  allowVideo?: boolean;
}

export interface ChannelIForm {
  id: string;
  channelId: string;
  sourceChannelId?: string;
  name: string;
  url?: string;
  embedCode?: string;
  defaultWidth: number;
  defaultHeight: number;
  defaultCollapsed: boolean;
  defaultFloating: boolean;
  allowPopout: boolean;
  orderIndex: number;
  createdBy?: string;
  updatedBy?: string;
  createdAt?: number;
  updatedAt?: number;
  worldShared?: boolean;
  sharedRef?: boolean;
  sharedWorldId?: string;
  readonly?: boolean;
  mediaOptions?: ChannelIFormMediaOptions;
}

export interface ChannelIFormStatePayload {
  formId: string;
  windowId?: string;
  floating?: boolean;
  collapsed?: boolean;
  width?: number;
  height?: number;
  x?: number;
  y?: number;
  minimized?: boolean;
  force?: boolean;
  autoPlay?: boolean;
  autoUnmute?: boolean;
}

export interface ChannelIFormEventPayload {
  forms?: ChannelIForm[];
  form?: ChannelIForm;
  states?: ChannelIFormStatePayload[];
  state?: ChannelIFormStatePayload;
  action?: 'snapshot' | 'push' | string;
  targetUserIds?: string[];
}
