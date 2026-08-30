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
  templateRef?: string;
  templateOverrides?: ChannelIFormTemplateOverrides;
  templateOrigin?: 'builtin' | 'platform' | string;
  templateName?: string;
  templateMissing?: boolean;
  templateArchived?: boolean;
  mediaOptions?: ChannelIFormMediaOptions;
  bridgePolicy?: ChannelIFormBridgePolicy;
}

export interface ChannelIFormTemplateOverrides {
  name?: string;
  defaultWidth?: number;
  defaultHeight?: number;
  defaultCollapsed?: boolean;
  defaultFloating?: boolean;
  allowPopout?: boolean;
  mediaOptions?: ChannelIFormMediaOptions;
  bridgePolicy?: ChannelIFormBridgePolicy;
}

export interface ChannelIFormTemplateCatalogItem {
  ref: string;
  origin: 'builtin' | 'platform' | string;
  name: string;
  description?: string;
  installable: boolean;
  archived?: boolean;
  enabled?: boolean;
  editable?: boolean;
  readOnly?: boolean;
}

export interface ChannelIFormBridgePolicy {
  enabled: boolean;
  allowedOrigins?: string[];
  capabilities?: string[];
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
