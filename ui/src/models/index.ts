import Dexie, { type Table } from 'dexie';
import { urlBase } from '@/stores/_config';
import type { UserEmojiModel } from '@/types';

export interface Thumb {
  id?: string;
  filename: string;
  recentUsed: number;
  data: string | ArrayBuffer | null;
  mimeType: string;
}

export interface ChatMessageCacheRecord {
  key: string;
  channelId: string;
  filterSignature: string;
  showArchived: boolean;
  updatedAt: number;
  rows: any[];
  beforeCursor: string;
  afterCursor: string;
  earliestTimestamp: number | null;
  latestTimestamp: number | null;
  hasReachedStart: boolean;
  hasReachedLatest: boolean;
  beforeCursorExhausted: boolean;
  viewMode: 'live' | 'history';
  lockedHistory: boolean;
  anchorMessageId: string | null;
  scrollTop: number;
  nearBottom: boolean;
}

export class MySubClassedDexie extends Dexie {
  // 'friends' is added by dexie when declaring the stores()
  // We just tell the typing system this is the case
  thumbs!: Table<Thumb>;
  chatMessageCache!: Table<ChatMessageCacheRecord>;

  constructor() {
    super('myDatabase');
    this.version(1).stores({
      thumbs: '++id, recentUsed, filename, data, mimeType' // Primary key and indexed props
    });
    this.version(2).stores({
      thumbs: '++id, recentUsed, filename, data, mimeType',
      chatMessageCache: 'key, updatedAt, channelId'
    });
  }
}


export function getSrcThumb(i: Thumb) {
  if (i.data) {
    let URL = window.URL || window.webkitURL
    if (URL && URL.createObjectURL) {
      const b = new Blob([i.data as any], { type: i.mimeType })
      return URL.createObjectURL(b)
    }
  } else {
      return `${urlBase}/api/v1/attachment/${i.id}`;
  }
}

export function getSrc(i: UserEmojiModel) {
  const id = i.attachmentId || i.id;
  return `${urlBase}/api/v1/attachment/${id}`;
}

export const db = new MySubClassedDexie();
