import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { api } from './_config';
import { useUserStore } from './user';

export type CharacterCardTemplateMode = 'managed' | 'detached';

export interface CharacterCardTemplate {
  id: string;
  userId: string;
  name: string;
  sheetType: string;
  content: string;
  isGlobalDefault: boolean;
  isSheetDefault: boolean;
  access?: 'owner' | 'world_shared';
  readonly?: boolean;
  isSharedToCurrentWorld?: boolean;
  sharedWorldId?: string;
  sharedByUserId?: string;
  sharedByNickname?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface CharacterCardTemplateBinding {
  id: string;
  userId: string;
  channelId: string;
  externalCardId: string;
  cardName: string;
  sheetType: string;
  mode: CharacterCardTemplateMode;
  templateId: string;
  templateSnapshot: string;
  createdAt?: string;
  updatedAt?: string;
}

interface TemplatePayload {
  name: string;
  sheetType?: string;
  content: string;
  isGlobalDefault?: boolean;
  isSheetDefault?: boolean;
}

interface TemplateQueryOptions {
  sheetType?: string;
  worldId?: string;
}

interface BindingPayload {
  channelId: string;
  externalCardId: string;
  cardName?: string;
  sheetType?: string;
  mode: CharacterCardTemplateMode;
  templateId?: string;
  templateSnapshot?: string;
}

interface CharacterCardLite {
  id: string;
  name: string;
  sheetType: string;
}

const LOCAL_TEMPLATE_STORAGE_KEY = 'sealchat_character_sheet_templates';
const MIGRATION_FLAG_PREFIX = 'sealchat_template_migration_v1_done';

const normalizeSheetType = (value?: string) => (value || '').trim().toLowerCase();

const buildMigrationFlagKey = (userId?: string) => {
  if (!userId) return '';
  return `${MIGRATION_FLAG_PREFIX}:${userId}`;
};

const hashTemplateContent = (content: string) => {
  let hash = 0;
  for (let i = 0; i < content.length; i += 1) {
    hash = ((hash << 5) - hash + content.charCodeAt(i)) | 0;
  }
  return `h${Math.abs(hash)}`;
};

export const useCharacterCardTemplateStore = defineStore('characterCardTemplate', () => {
  const userStore = useUserStore();

  const templateMap = ref<Record<string, CharacterCardTemplate>>({});
  const bindingsByChannel = ref<Record<string, Record<string, CharacterCardTemplateBinding>>>({});
  const bindingsLoadedChannels = ref<Record<string, boolean>>({});
  const templatesLoaded = ref(false);
  const loading = ref(false);
  const migrating = ref(false);
  const loadedWorldId = ref('');

  const templates = computed(() => Object.values(templateMap.value));

  const getTemplateById = (templateId?: string) => {
    if (!templateId) return null;
    return templateMap.value[templateId] || null;
  };

  const getBinding = (channelId: string, externalCardId: string) => {
    return bindingsByChannel.value[channelId]?.[externalCardId] || null;
  };

  const getTemplatesBySheetType = (sheetType?: string) => {
    const normalized = normalizeSheetType(sheetType);
    return templates.value.filter(item => {
      const current = normalizeSheetType(item.sheetType);
      if (!normalized) return true;
      return !current || current === normalized;
    });
  };

  const getSheetDefaultTemplate = (sheetType?: string) => {
    const normalized = normalizeSheetType(sheetType);
    if (!normalized) return null;
    return templates.value.find(item => !item.readonly && item.isSheetDefault && normalizeSheetType(item.sheetType) === normalized) || null;
  };

  const getGlobalDefaultTemplate = () => {
    return templates.value.find(item => item.isGlobalDefault && !item.readonly) || null;
  };

  const resolveDefaultTemplate = (sheetType?: string, fallback = '') => {
    const sheetDefault = getSheetDefaultTemplate(sheetType);
    if (sheetDefault?.content) return sheetDefault.content;
    const globalDefault = getGlobalDefaultTemplate();
    if (globalDefault?.content) return globalDefault.content;
    return fallback;
  };

  const resolveCardTemplate = (
    channelId: string,
    externalCardId: string,
    sheetType?: string,
    fallback = '',
  ) => {
    const binding = getBinding(channelId, externalCardId);
    if (binding?.mode === 'managed') {
      const managedTemplate = getTemplateById(binding.templateId);
      if (managedTemplate?.content) return managedTemplate.content;
    }
    if (binding?.mode === 'detached' && binding.templateSnapshot) {
      return binding.templateSnapshot;
    }
    return resolveDefaultTemplate(sheetType, fallback);
  };

  const loadTemplates = async (options?: TemplateQueryOptions) => {
    loading.value = true;
    try {
      const sheetType = options?.sheetType;
      const worldId = String(options?.worldId ?? loadedWorldId.value ?? '').trim();
      const resp = await api.get('/api/v1/character-card-templates', {
        params: {
          ...(sheetType ? { sheetType } : {}),
          ...(worldId ? { worldId } : {}),
        },
      });
      const items = Array.isArray(resp.data?.items) ? resp.data.items as CharacterCardTemplate[] : [];
      const nextMap: Record<string, CharacterCardTemplate> = {};
      items.forEach(item => {
        if (item?.id) {
          nextMap[item.id] = item;
        }
      });
      templateMap.value = nextMap;
      templatesLoaded.value = true;
      loadedWorldId.value = worldId;
      return items;
    } finally {
      loading.value = false;
    }
  };

  const ensureTemplatesLoaded = async (options?: TemplateQueryOptions) => {
    const worldId = String(options?.worldId ?? loadedWorldId.value ?? '').trim();
    if (templatesLoaded.value && loadedWorldId.value === worldId) return;
    await loadTemplates({ ...options, worldId: worldId || undefined });
  };

  const createTemplate = async (payload: TemplatePayload) => {
    const resp = await api.post('/api/v1/character-card-templates', payload);
    const item = resp.data?.item as CharacterCardTemplate | undefined;
    if (item?.id) {
      templateMap.value = { ...templateMap.value, [item.id]: item };
      if (item.isGlobalDefault || item.isSheetDefault) {
        await loadTemplates();
      }
    }
    return item || null;
  };

  const updateTemplate = async (templateId: string, payload: Partial<TemplatePayload>) => {
    const resp = await api.put(`/api/v1/character-card-templates/${templateId}`, payload);
    const item = resp.data?.item as CharacterCardTemplate | undefined;
    if (item?.id) {
      templateMap.value = { ...templateMap.value, [item.id]: item };
      if (item.isGlobalDefault || item.isSheetDefault || payload.isGlobalDefault !== undefined || payload.isSheetDefault !== undefined) {
        await loadTemplates();
      }
    }
    return item || null;
  };

  const deleteTemplate = async (templateId: string) => {
    await api.delete(`/api/v1/character-card-templates/${templateId}`);
    const nextMap = { ...templateMap.value };
    delete nextMap[templateId];
    templateMap.value = nextMap;
    await loadTemplates();
  };

  const setTemplateDefault = async (templateId: string, scope: 'global' | 'sheet') => {
    const resp = await api.post(`/api/v1/character-card-templates/${templateId}/set-default`, { scope });
    const item = resp.data?.item as CharacterCardTemplate | undefined;
    await loadTemplates();
    return item || null;
  };

  const shareTemplateToWorld = async (worldId: string, templateId: string) => {
    await api.post(`/api/v1/worlds/${worldId}/character-card-templates/${templateId}/share`);
    await loadTemplates({ worldId });
  };

  const unshareTemplateFromWorld = async (worldId: string, templateId: string) => {
    await api.delete(`/api/v1/worlds/${worldId}/character-card-templates/${templateId}/share`);
    await loadTemplates({ worldId });
  };

  const loadBindings = async (channelId: string) => {
    if (!channelId) return [];
    const resp = await api.get('/api/v1/character-card-template-bindings', { params: { channelId } });
    const items = Array.isArray(resp.data?.items) ? resp.data.items as CharacterCardTemplateBinding[] : [];
    const channelMap: Record<string, CharacterCardTemplateBinding> = {};
    items.forEach(item => {
      if (item?.externalCardId) {
        channelMap[item.externalCardId] = item;
      }
    });
    bindingsByChannel.value = {
      ...bindingsByChannel.value,
      [channelId]: channelMap,
    };
    bindingsLoadedChannels.value = {
      ...bindingsLoadedChannels.value,
      [channelId]: true,
    };
    return items;
  };

  const ensureBindingsLoaded = async (channelId: string) => {
    if (!channelId) return;
    if (bindingsLoadedChannels.value[channelId]) return;
    await loadBindings(channelId);
  };

  const upsertBinding = async (payload: BindingPayload) => {
    const resp = await api.post('/api/v1/character-card-template-bindings/upsert', payload);
    const item = resp.data?.item as CharacterCardTemplateBinding | undefined;
    if (item?.externalCardId && item.channelId) {
      const channelMap = {
        ...(bindingsByChannel.value[item.channelId] || {}),
        [item.externalCardId]: item,
      };
      bindingsByChannel.value = {
        ...bindingsByChannel.value,
        [item.channelId]: channelMap,
      };
      bindingsLoadedChannels.value = {
        ...bindingsLoadedChannels.value,
        [item.channelId]: true,
      };
    }
    return item || null;
  };

  const bindCardToTemplate = async (payload: {
    channelId: string;
    externalCardId: string;
    cardName?: string;
    sheetType?: string;
    templateId: string;
  }) => {
    return upsertBinding({
      channelId: payload.channelId,
      externalCardId: payload.externalCardId,
      cardName: payload.cardName,
      sheetType: payload.sheetType,
      mode: 'managed',
      templateId: payload.templateId,
      templateSnapshot: '',
    });
  };

  const bindCardToDetachedTemplate = async (payload: {
    channelId: string;
    externalCardId: string;
    cardName?: string;
    sheetType?: string;
    templateSnapshot: string;
  }) => {
    return upsertBinding({
      channelId: payload.channelId,
      externalCardId: payload.externalCardId,
      cardName: payload.cardName,
      sheetType: payload.sheetType,
      mode: 'detached',
      templateId: '',
      templateSnapshot: payload.templateSnapshot,
    });
  };

  const ensureCardBinding = async (payload: {
    channelId: string;
    externalCardId: string;
    cardName: string;
    sheetType: string;
    fallbackTemplate: string;
  }) => {
    if (!payload.channelId || !payload.externalCardId) return null;
    await ensureTemplatesLoaded();
    await ensureBindingsLoaded(payload.channelId);

    const existing = getBinding(payload.channelId, payload.externalCardId);
    if (existing) return existing;

    const sheetDefault = getSheetDefaultTemplate(payload.sheetType);
    if (sheetDefault?.id) {
      return bindCardToTemplate({
        channelId: payload.channelId,
        externalCardId: payload.externalCardId,
        cardName: payload.cardName,
        sheetType: payload.sheetType,
        templateId: sheetDefault.id,
      });
    }

    const globalDefault = getGlobalDefaultTemplate();
    if (globalDefault?.id) {
      return bindCardToTemplate({
        channelId: payload.channelId,
        externalCardId: payload.externalCardId,
        cardName: payload.cardName,
        sheetType: payload.sheetType,
        templateId: globalDefault.id,
      });
    }

    return bindCardToDetachedTemplate({
      channelId: payload.channelId,
      externalCardId: payload.externalCardId,
      cardName: payload.cardName,
      sheetType: payload.sheetType,
      templateSnapshot: payload.fallbackTemplate,
    });
  };

  const migrateLocalTemplatesIfNeeded = async (channelId: string, cards: CharacterCardLite[]) => {
    if (migrating.value) return;
    const userId = userStore.info?.id;
    if (!userId || !channelId || !Array.isArray(cards) || cards.length === 0) {
      return;
    }
    const key = buildMigrationFlagKey(userId);
    if (!key) return;
    if (typeof window === 'undefined') return;
    if (localStorage.getItem(key) === '1') {
      return;
    }

    let localTemplates: Record<string, string> = {};
    try {
      const raw = localStorage.getItem(LOCAL_TEMPLATE_STORAGE_KEY);
      if (!raw) {
        localStorage.setItem(key, '1');
        return;
      }
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== 'object') {
        localStorage.setItem(key, '1');
        return;
      }
      localTemplates = parsed;
    } catch (e) {
      console.warn('Failed to parse local character templates for migration', e);
      return;
    }

    migrating.value = true;
    try {
      await ensureTemplatesLoaded();
      await ensureBindingsLoaded(channelId);
      const contentIndex = new Map<string, CharacterCardTemplate>();
      templates.value.forEach(item => {
        const idxKey = `${normalizeSheetType(item.sheetType)}::${item.content}`;
        if (!contentIndex.has(idxKey)) {
          contentIndex.set(idxKey, item);
        }
      });

      for (const card of cards) {
        const localTemplate = String(localTemplates[card.id] || '').trim();
        if (!localTemplate) continue;

        const existingBinding = getBinding(channelId, card.id);
        if (existingBinding) continue;

        const idxKey = `${normalizeSheetType(card.sheetType)}::${localTemplate}`;
        let template = contentIndex.get(idxKey) || null;
        if (!template) {
          const suffix = hashTemplateContent(localTemplate).slice(-6);
          const created = await createTemplate({
            name: `${card.name || '人物卡'}-迁移-${suffix}`,
            sheetType: card.sheetType,
            content: localTemplate,
          });
          if (created) {
            template = created;
            contentIndex.set(idxKey, created);
          }
        }

        if (template?.id) {
          await bindCardToTemplate({
            channelId,
            externalCardId: card.id,
            cardName: card.name,
            sheetType: card.sheetType,
            templateId: template.id,
          });
        } else {
          await bindCardToDetachedTemplate({
            channelId,
            externalCardId: card.id,
            cardName: card.name,
            sheetType: card.sheetType,
            templateSnapshot: localTemplate,
          });
        }
      }

      localStorage.setItem(key, '1');
    } catch (e) {
      console.warn('Failed to migrate local character templates', e);
    } finally {
      migrating.value = false;
    }
  };

  return {
    loading,
    migrating,
    templates,
    templateMap,
    bindingsByChannel,
    getTemplateById,
    getBinding,
    getTemplatesBySheetType,
    getSheetDefaultTemplate,
    getGlobalDefaultTemplate,
    resolveDefaultTemplate,
    resolveCardTemplate,
    loadTemplates,
    ensureTemplatesLoaded,
    createTemplate,
    updateTemplate,
    deleteTemplate,
    setTemplateDefault,
    shareTemplateToWorld,
    unshareTemplateFromWorld,
    loadBindings,
    ensureBindingsLoaded,
    upsertBinding,
    bindCardToTemplate,
    bindCardToDetachedTemplate,
    ensureCardBinding,
    migrateLocalTemplatesIfNeeded,
  };
});
