export interface GlassBackgroundChannelNode {
  id: string;
  name?: string;
  type?: number;
  isPrivate?: boolean;
  permType?: string;
  children?: GlassBackgroundChannelNode[];
}
