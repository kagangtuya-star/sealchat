import { defineStore } from 'pinia'
import { api } from './_config'
import type {
  ChannelFolder,
  ChannelFolderNode,
  ChannelFolderMember,
  ChannelFolderListPayload,
  ChannelConfigSyncResult,
} from '@/types'

interface FolderCreatePayload {
  name: string
  parentId?: string
  description?: string
  sortOrder?: number
}

interface FolderAssignPayload {
  folderIds: string[]
  channelIds: string[]
  mode: 'replace' | 'append' | 'remove'
  includeChildren?: boolean
}

export const useChannelFolderStore = defineStore('channelFolders', {
  state: () => ({
    folders: [] as ChannelFolder[],
    members: [] as ChannelFolderMember[],
    favorites: [] as string[],
    loading: false,
    managerVisible: false,
    showFavoritesOnly: false,
    searchKeyword: '',
  }),
  getters: {
    folderTree(state): ChannelFolderNode[] {
      const map = new Map<string, ChannelFolderNode>()
      state.folders.forEach((folder) => {
        map.set(folder.id, { ...folder, children: [] })
      })
      const roots: ChannelFolderNode[] = []
      map.forEach((node) => {
        if (node.parentId && map.has(node.parentId)) {
          map.get(node.parentId)!.children!.push(node)
        } else {
          roots.push(node)
        }
      })
      return roots
    },
    folderMap(): Map<string, ChannelFolderNode> {
      const map = new Map<string, ChannelFolderNode>()
      const stack = [...(this.folderTree as ChannelFolderNode[])]
      while (stack.length) {
        const node = stack.pop()
        if (!node) continue
        map.set(node.id, node)
        if (node.children && node.children.length) {
          stack.push(...node.children)
        }
      }
      return map
    },
    favoriteFolderSet(state): Set<string> {
      return new Set(state.favorites)
    },
    favoriteChannelSet(state): Set<string> {
      const favoriteFolders = new Set(state.favorites)
      const channels = new Set<string>()
      state.members.forEach((member) => {
        if (favoriteFolders.has(member.folderId)) {
          channels.add(member.channelId)
        }
      })
      return channels
    },
    channelFolderMap(state): Map<string, string[]> {
      const map = new Map<string, string[]>()
      state.members.forEach((member) => {
        const list = map.get(member.channelId) || []
        list.push(member.folderId)
        map.set(member.channelId, list)
      })
      return map
    },
  },
  actions: {
    async fetchFolders() {
      this.loading = true
      try {
        const { data } = await api.get<ChannelFolderListPayload>('api/v1/channel-folders')
        this.folders = data?.folders || []
        this.members = data?.members || []
        this.favorites = data?.favorites || []
      } finally {
        this.loading = false
      }
    },
    async ensureLoaded() {
      if (this.loading) return
      if (this.folders.length === 0) {
        await this.fetchFolders()
      }
    },
    setManagerVisible(visible: boolean) {
      this.managerVisible = visible
      if (visible) {
        this.ensureLoaded()
      }
    },
    setShowFavoritesOnly(value: boolean) {
      this.showFavoritesOnly = value
      if (value) {
        this.ensureLoaded()
      }
    },
    setSearchKeyword(keyword: string) {
      this.searchKeyword = keyword
    },
    async toggleFavorite(folderId: string, favorite: boolean) {
      const { data } = await api.post<{ favorites: string[] }>(
        `api/v1/channel-folders/${folderId}/favorite`,
        { favorite },
      )
      this.favorites = data?.favorites || []
    },
    async createFolder(payload: FolderCreatePayload) {
      await api.post('api/v1/channel-folders', payload)
      await this.fetchFolders()
    },
    async updateFolder(folderId: string, payload: FolderCreatePayload) {
      await api.put(`api/v1/channel-folders/${folderId}`, payload)
      await this.fetchFolders()
    },
    async deleteFolder(folderId: string) {
      await api.delete(`api/v1/channel-folders/${folderId}`)
      await this.fetchFolders()
    },
    async assignChannels(payload: FolderAssignPayload) {
      await api.post('api/v1/channel-folders/assign', payload)
      await this.fetchFolders()
    },
    async syncChannelConfig(params: {
      sourceChannelId: string
      targetChannelIds: string[]
      scopes: string[]
    }) {
      const { data } = await api.post<ChannelConfigSyncResult>('api/v1/channel-config-sync', {
        sourceChannelId: params.sourceChannelId,
        targetChannelIds: params.targetChannelIds,
        scopes: params.scopes,
      })
      return data
    },
  },
})
