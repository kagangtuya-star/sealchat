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

export class MySubClassedDexie extends Dexie {
  // 'friends' is added by dexie when declaring the stores()
  // We just tell the typing system this is the case
  thumbs!: Table<Thumb>;

  constructor() {
    super('myDatabase');
    this.version(1).stores({
      thumbs: '++id, recentUsed, filename, data, mimeType' // Primary key and indexed props
    });
    this.version(2)
      .stores({
        thumbs: '++id, recentUsed, filename, data, mimeType'
      })
      .upgrade((tx) => {
        // 历史版本会把上传原图长期写入 thumbs，这里主动清空旧数据回收磁盘空间。
        return tx.table('thumbs').clear();
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
