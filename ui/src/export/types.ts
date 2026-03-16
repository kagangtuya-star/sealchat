export interface ExportMessage {
  id: string
  sender_id: string
  sender_identity_id?: string
  sender_name: string
  sender_color?: string
  sender_avatar?: string
  is_merged?: boolean
  ic_mode: string
  is_whisper: boolean
  is_archived: boolean
  is_bot: boolean
  created_at: string | number | Date
  content: string
  content_html?: string
  whisper_targets?: string[]
}

export interface DisplayOptions {
  layout?: string
  palette?: string
  showAvatar?: boolean
  mergeNeighbors?: boolean
}

export interface ExportPayload {
  channel_id: string
  channel_name: string
  generated_at: string
  start_time?: string
  end_time?: string
  slice_start?: string
  slice_end?: string
  part_index?: number
  part_total?: number
  display_options?: DisplayOptions
  messages: ExportMessage[]
  count: number
  without_timestamp?: boolean
}

export interface ViewerManifestPart {
  file: string
  part_index: number
  part_total: number
  messages: number
  slice_start?: string
  slice_end?: string
  sha256?: string
}

export interface ViewerManifest {
  channel_id: string
  channel_name: string
  generated_at: string
  display_options?: DisplayOptions
  slice_limit: number
  max_concurrency: number
  part_total: number
  total_messages: number
  parts: ViewerManifestPart[]
}

declare global {
  interface Window {
    __EXPORT_DATA__?: ExportPayload
    __EXPORT_INDEX__?: ViewerManifest
  }
}
