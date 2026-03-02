import { FONT_ASSET_LIMIT } from './fontUtils'
import type { FontAssetMeta, FontAssetRecord, FontAssetSaveResult } from './types'

const DB_NAME = 'sealchat_font_assets'
const DB_VERSION = 1
const STORE_NAME = 'font_assets'
const INDEX_UPDATED_AT = 'updatedAt'

let dbPromise: Promise<IDBDatabase> | null = null

const hasIndexedDb = (): boolean => typeof window !== 'undefined' && typeof window.indexedDB !== 'undefined'

const requestToPromise = <T = unknown>(request: IDBRequest<T>): Promise<T> =>
  new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error || new Error('IndexedDB 请求失败'))
  })

const transactionDone = (tx: IDBTransaction): Promise<void> =>
  new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error || new Error('IndexedDB 事务失败'))
    tx.onabort = () => reject(tx.error || new Error('IndexedDB 事务中止'))
  })

const openDb = async (): Promise<IDBDatabase> => {
  if (!hasIndexedDb()) {
    throw new Error('当前环境不支持 IndexedDB')
  }
  if (dbPromise) return dbPromise

  dbPromise = new Promise((resolve, reject) => {
    const request = window.indexedDB.open(DB_NAME, DB_VERSION)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        const store = db.createObjectStore(STORE_NAME, { keyPath: 'id' })
        store.createIndex(INDEX_UPDATED_AT, INDEX_UPDATED_AT, { unique: false })
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error || new Error('字体缓存数据库打开失败'))
  })

  return dbPromise
}

const toMeta = (record: FontAssetRecord): FontAssetMeta => ({
  id: record.id,
  family: record.family,
  sourceType: record.sourceType,
  mime: record.mime,
  size: record.size,
  createdAt: record.createdAt,
  updatedAt: record.updatedAt,
  sourceUrl: record.sourceUrl,
})

export const isFontAssetCacheAvailable = (): boolean => hasIndexedDb()

export const listFontAssetMeta = async (): Promise<FontAssetMeta[]> => {
  if (!hasIndexedDb()) return []
  const db = await openDb()
  const tx = db.transaction(STORE_NAME, 'readonly')
  const store = tx.objectStore(STORE_NAME)
  const rows = await requestToPromise(store.getAll() as IDBRequest<FontAssetRecord[]>)
  await transactionDone(tx)
  return rows
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .map(toMeta)
}

export const getFontAssetById = async (id: string): Promise<FontAssetRecord | null> => {
  if (!id || !hasIndexedDb()) return null
  const db = await openDb()
  const tx = db.transaction(STORE_NAME, 'readonly')
  const store = tx.objectStore(STORE_NAME)
  const row = await requestToPromise(store.get(id) as IDBRequest<FontAssetRecord | undefined>)
  await transactionDone(tx)
  return row || null
}

export const deleteFontAssetById = async (id: string): Promise<void> => {
  if (!id || !hasIndexedDb()) return
  const db = await openDb()
  const tx = db.transaction(STORE_NAME, 'readwrite')
  const store = tx.objectStore(STORE_NAME)
  await requestToPromise(store.delete(id))
  await transactionDone(tx)
}

export const touchFontAssetById = async (id: string): Promise<void> => {
  if (!id || !hasIndexedDb()) return
  const db = await openDb()
  const tx = db.transaction(STORE_NAME, 'readwrite')
  const store = tx.objectStore(STORE_NAME)
  const existing = await requestToPromise(store.get(id) as IDBRequest<FontAssetRecord | undefined>)
  if (!existing) {
    await transactionDone(tx)
    return
  }
  existing.updatedAt = Date.now()
  await requestToPromise(store.put(existing))
  await transactionDone(tx)
}

export const saveFontAsset = async (
  payload: Omit<FontAssetRecord, 'createdAt' | 'updatedAt'>,
): Promise<FontAssetSaveResult> => {
  if (!hasIndexedDb()) {
    throw new Error('当前环境不支持字体本地缓存')
  }
  const db = await openDb()
  const now = Date.now()
  const tx = db.transaction(STORE_NAME, 'readwrite')
  const store = tx.objectStore(STORE_NAME)

  const existing = await requestToPromise(store.get(payload.id) as IDBRequest<FontAssetRecord | undefined>)
  const nextRecord: FontAssetRecord = {
    ...payload,
    createdAt: existing?.createdAt ?? now,
    updatedAt: now,
  }
  await requestToPromise(store.put(nextRecord))

  const all = await requestToPromise(store.getAll() as IDBRequest<FontAssetRecord[]>)
  const overflow = all.length - FONT_ASSET_LIMIT
  const evictedIds: string[] = []
  if (overflow > 0) {
    const candidates = all
      .filter(item => item.id !== nextRecord.id)
      .sort((a, b) => a.updatedAt - b.updatedAt)
      .slice(0, overflow)
    for (const item of candidates) {
      await requestToPromise(store.delete(item.id))
      evictedIds.push(item.id)
    }
  }

  await transactionDone(tx)
  return {
    saved: toMeta(nextRecord),
    evictedIds,
  }
}

