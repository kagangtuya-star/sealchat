import { api } from '@/stores/_config';
import type { GalleryCollection, GalleryItem, GallerySearchResponse, PaginationListResponse } from '@/types';

const prefix = 'api/v1/gallery';

export interface GalleryCollectionPayload {
  ownerType: 'user' | 'channel';
  ownerId: string;
  name: string;
  order?: number;
  collectionType?: string;
}

export interface GalleryItemUploadPayload {
  collectionId: string;
  items: Array<{
    attachmentId: string;
    thumbData: string;
    remark: string;
    order?: number;
  }>;
}

export function fetchCollections(ownerType: 'user' | 'channel', ownerId: string) {
  return api.get<{ items: GalleryCollection[] }>(`${prefix}/collections`, {
    params: { ownerType, ownerId }
  });
}

export function createCollection(payload: GalleryCollectionPayload) {
  return api.post<{ item: GalleryCollection }>(`${prefix}/collections`, payload);
}

export function updateCollection(id: string, payload: Partial<GalleryCollectionPayload>) {
  return api.patch<{ item: GalleryCollection }>(`${prefix}/collections/${id}`, payload);
}

export function deleteCollection(id: string) {
  return api.delete<{ message: string }>(`${prefix}/collections/${id}`);
}

export function fetchItems(collectionId: string, params: { page?: number; pageSize?: number; keyword?: string } = {}) {
  return api.get<PaginationListResponse<GalleryItem>>(`${prefix}/items`, {
    params: { collectionId, ...params }
  });
}

export function uploadItems(payload: GalleryItemUploadPayload) {
  return api.post<{ items: GalleryItem[] }>(`${prefix}/items/upload`, payload);
}

export function updateItem(id: string, payload: Partial<{ remark: string; collectionId: string; order: number }>) {
  return api.patch<{ item: GalleryItem }>(`${prefix}/items/${id}`, payload);
}

export function deleteItems(ids: string[]) {
  return api.post<{ message: string }>(`${prefix}/items/delete`, { ids });
}

export interface GallerySearchRequest {
  keyword: string;
  ownerId?: string;
  ownerType?: 'user' | 'channel';
}

export function searchGallery(params: GallerySearchRequest) {
  const normalized: GallerySearchRequest = {
    ownerType: 'user',
    ...params,
  };
  return api.get<GallerySearchResponse>(`${prefix}/search`, { params: normalized });
}

export function addEmojiToFavorites(attachmentId: string, remark?: string) {
  return api.post<{ item: GalleryItem }>('api/v1/user-emoji-add', { attachmentId, remark });
}

export function addEmojiToReactions(attachmentId: string, remark?: string) {
  return api.post<{ item: GalleryItem }>('api/v1/user-reaction-emoji-add', { attachmentId, remark });
}
