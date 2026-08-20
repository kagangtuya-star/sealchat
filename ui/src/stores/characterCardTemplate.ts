import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import cocTemplateHtml from '../../../doc/template/sealchat-default-template-v3-coc7th.html?raw';
import shinobigamiTemplateHtml from '../../../doc/template/sealchat-shinobigami-template-v1.html?raw';
import { api } from './_config';
import { useUserStore } from './user';

export type CharacterCardTemplateMode = 'managed' | 'detached';
export type CharacterCardTemplateAccess = 'owner' | 'world_shared' | 'platform';

export const PLATFORM_CHARACTER_CARD_TEMPLATE_REF_PREFIX = 'platform:';

export const parseCharacterCardTemplateRef = (value?: string) => {
  const ref = String(value || '').trim();
  if (!ref) return null;
  if (ref.startsWith(PLATFORM_CHARACTER_CARD_TEMPLATE_REF_PREFIX)) {
    const id = ref.slice(PLATFORM_CHARACTER_CARD_TEMPLATE_REF_PREFIX.length).trim();
    if (!id || id.includes(':')) return null;
    return { source: 'platform' as const, id, ref: `${PLATFORM_CHARACTER_CARD_TEMPLATE_REF_PREFIX}${id}` };
  }
  return { source: 'user' as const, id: ref, ref };
};

export const isPlatformCharacterCardTemplateRef = (value?: string) => (
  parseCharacterCardTemplateRef(value)?.source === 'platform'
);

export interface CharacterCardTemplate {
  id: string;
  userId: string;
  name: string;
  sheetType: string;
  content: string;
  defaultBadgeTemplate: string;
  isGlobalDefault: boolean;
  isSheetDefault: boolean;
  ref?: string;
  origin?: 'user' | 'platform';
  enabled?: boolean;
  badgeTemplateOverride?: string;
  theaterOverlayTemplateJson?: string;
  access?: CharacterCardTemplateAccess;
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
  defaultBadgeTemplate?: string;
  isGlobalDefault?: boolean;
  isSheetDefault?: boolean;
  isBuiltin?: boolean;
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

interface BuiltinCharacterCardTemplate {
  name: string;
  sheetType: string;
  content: string;
  legacyMarkers: string[];
}

const LOCAL_TEMPLATE_STORAGE_KEY = 'sealchat_character_sheet_templates';
const MIGRATION_FLAG_PREFIX = 'sealchat_template_migration_v1_done';
const BUILTIN_SHEET_TYPES = new Set(['coc7', 'coc', 'dnd5e', 'dnd5', 'dnd', 'shinobigami']);
const BUILTIN_CHARACTER_CARD_TEMPLATES: BuiltinCharacterCardTemplate[] = [
  {
    name: 'coc默认',
    sheetType: 'coc7',
    content: cocTemplateHtml.trim(),
    legacyMarkers: [
      'sealchat-default-template:v2-coc-dark',
      'sealchat-default-template:v2-coc7th',
    ],
  },
  {
    name: '忍神人物卡模板',
    sheetType: '忍神',
    content: shinobigamiTemplateHtml.trim(),
    legacyMarkers: ['sealchat-shinobigami-template:v1'],
  },
];

const normalizeSheetType = (value?: string) => {
  const normalized = (value || '').trim().toLowerCase();
  if (normalized === 'shinobigami' || normalized === '忍神') {
    return 'shinobigami';
  }
  if (normalized === 'coc') return 'coc7';
  if (normalized === 'dnd5' || normalized === 'dnd') return 'dnd5e';
  return normalized;
};
const isBuiltInSheetType = (value?: string) => BUILTIN_SHEET_TYPES.has(normalizeSheetType(value));

const isLegacyBuiltinTemplate = (
  template: CharacterCardTemplate,
  builtin: BuiltinCharacterCardTemplate,
) => (
  !template.readonly
  && normalizeSheetType(template.sheetType) === normalizeSheetType(builtin.sheetType)
  && builtin.legacyMarkers.some(marker => template.content.includes(marker))
);

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
  const bindingsMutationVersions = ref<Record<string, number>>({});
  const bindingsLoadPromises: Record<string, Promise<CharacterCardTemplateBinding[]> | undefined> = {};
  const templatesLoaded = ref(false);
  const loading = ref(false);
  const migrating = ref(false);
  const loadedWorldId = ref('');
  const builtinTemplatesEnsured = ref(false);
  let builtinTemplatesEnsurePromise: Promise<void> | null = null;

  const templates = computed(() => Object.values(templateMap.value));

  const getTemplateById = (templateId?: string) => {
    if (!templateId) return null;
    const parsed = parseCharacterCardTemplateRef(templateId);
    return templateMap.value[parsed?.ref || templateId] || null;
  };

  const getTemplateRef = (template?: CharacterCardTemplate | null) => {
    if (!template) return '';
    return String(template.ref || template.id || '').trim();
  };

  const getBinding = (channelId: string, externalCardId: string) => {
    return bindingsByChannel.value[channelId]?.[externalCardId] || null;
  };

  const getBindingsMutationVersion = (channelId: string) => (
    bindingsMutationVersions.value[channelId] || 0
  );

  const markBindingsMutated = (channelId: string) => {
    bindingsMutationVersions.value = {
      ...bindingsMutationVersions.value,
      [channelId]: getBindingsMutationVersion(channelId) + 1,
    };
  };

  const getTemplatesBySheetType = (sheetType?: string) => {
    const normalized = normalizeSheetType(sheetType);
    const builtin = BUILTIN_CHARACTER_CARD_TEMPLATES.find(item => normalizeSheetType(item.sheetType) === normalized);
    return templates.value.filter(item => {
      if (item.enabled === false) return false;
      const current = normalizeSheetType(item.sheetType);
      if (!normalized) return true;
      if (builtin && item.name !== builtin.name && isLegacyBuiltinTemplate(item, builtin)) {
        return false;
      }
      if (builtin && !item.readonly && item.name !== builtin.name && current === normalized && item.content.trim() === builtin.content) {
        return false;
      }
      if (!current || current === normalized) return true;
      return !isBuiltInSheetType(current);
    });
  };

  const getSheetDefaultTemplate = (sheetType?: string) => {
    const normalized = normalizeSheetType(sheetType);
    if (!normalized) return null;
    return templates.value.find(item => !item.readonly && item.isSheetDefault && normalizeSheetType(item.sheetType) === normalized) || null;
  };

  const getBuiltinTemplateBySheetType = (sheetType?: string) => {
    const normalized = normalizeSheetType(sheetType);
    if (!normalized) return null;
    const builtinNames = new Set(BUILTIN_CHARACTER_CARD_TEMPLATES
      .filter(item => normalizeSheetType(item.sheetType) === normalized)
      .map(item => item.name));
    if (builtinNames.size === 0) return null;
    return templates.value.find(item => (
      !item.readonly
      && builtinNames.has(item.name)
      && normalizeSheetType(item.sheetType) === normalized
    )) || null;
  };

  const isSameSheetType = (left?: string, right?: string) => (
    normalizeSheetType(left) === normalizeSheetType(right)
  );

  const getPreferredTemplateBySheetType = (sheetType?: string) => {
    const normalized = normalizeSheetType(sheetType);
    if (!normalized) return null;

    const sheetDefault = getSheetDefaultTemplate(sheetType);
    if (sheetDefault) return sheetDefault;

    const builtinTemplate = getBuiltinTemplateBySheetType(sheetType);
    if (builtinTemplate) return builtinTemplate;

    return templates.value.find(item => !item.readonly && isSameSheetType(item.sheetType, sheetType))
      || templates.value.find(item => item.enabled !== false && isSameSheetType(item.sheetType, sheetType))
      || null;
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
      const builtinKeys = new Set<string>();
      items.forEach(item => {
        if (item?.id) {
          const builtin = BUILTIN_CHARACTER_CARD_TEMPLATES.find(candidate => (
            item.name === candidate.name
            && normalizeSheetType(item.sheetType) === normalizeSheetType(candidate.sheetType)
            && !item.readonly
          ));
          if (builtin) {
            const key = `${builtin.name}::${normalizeSheetType(builtin.sheetType)}`;
            if (builtinKeys.has(key)) return;
            builtinKeys.add(key);
          }
          const key = String(item.ref || item.id || '').trim();
          if (key) nextMap[key] = item;
        }
      });
      templateMap.value = nextMap;
      templatesLoaded.value = true;
      loadedWorldId.value = worldId;
      if (!sheetType) {
        await ensureBuiltinTemplates(items);
      }
      return items;
    } finally {
      loading.value = false;
    }
  };

  const ensureBuiltinTemplates = async (items: CharacterCardTemplate[]) => {
    if (builtinTemplatesEnsured.value) return;
    if (!userStore.info?.id) return;
    if (builtinTemplatesEnsurePromise) return builtinTemplatesEnsurePromise;

    const ensurePromise = (async () => {
      let ensured = true;
      const updateBuiltinTemplate = async (
        template: CharacterCardTemplate,
        builtin: BuiltinCharacterCardTemplate,
      ) => {
        const payload: Partial<TemplatePayload> = {};
        if (template.name !== builtin.name) payload.name = builtin.name;
        if (template.sheetType !== builtin.sheetType) payload.sheetType = builtin.sheetType;
        if (template.content.trim() !== builtin.content) payload.content = builtin.content;
        if (Object.keys(payload).length === 0) return template;
        try {
          const resp = await api.put(`/api/v1/character-card-templates/${template.id}`, payload);
          const updated = resp.data?.item as CharacterCardTemplate | undefined;
          if (!updated?.id) return null;
          templateMap.value = { ...templateMap.value, [updated.id]: updated };
          return updated;
        } catch (e) {
          console.warn('Failed to upgrade builtin character template', e);
          return null;
        }
      };

      for (const builtin of BUILTIN_CHARACTER_CARD_TEMPLATES) {
        const matchingTemplates = items.filter(item => (
          !item.readonly
          && item.name === builtin.name
          && normalizeSheetType(item.sheetType) === normalizeSheetType(builtin.sheetType)
        ));
        const legacyTemplates = items.filter(item => isLegacyBuiltinTemplate(item, builtin));
        const contentDuplicateTemplates = items.filter(item => (
          !item.readonly
          && item.name !== builtin.name
          && normalizeSheetType(item.sheetType) === normalizeSheetType(builtin.sheetType)
          && item.content.trim() === builtin.content
        ));
        let canonical: CharacterCardTemplate | null = legacyTemplates[0]
          || matchingTemplates[0]
          || contentDuplicateTemplates[0]
          || null;
        if (!canonical) {
          canonical = await createTemplate({
            name: builtin.name,
            sheetType: builtin.sheetType,
            content: builtin.content,
            isBuiltin: true,
          });
          if (!canonical?.id) {
            ensured = false;
            continue;
          }
        }

        const updatedCanonical = await updateBuiltinTemplate(canonical, builtin);
        if (!updatedCanonical) {
          ensured = false;
          continue;
        }
        canonical = updatedCanonical;

        const duplicateIds = new Set<string>();
        [...matchingTemplates, ...legacyTemplates, ...contentDuplicateTemplates].forEach((template) => {
          if (template.id && template.id !== canonical?.id) duplicateIds.add(template.id);
        });
        for (const duplicateId of duplicateIds) {
          try {
            await replaceTemplateReferences(duplicateId, canonical.id);
            const nextMap = { ...templateMap.value };
            const duplicate = nextMap[duplicateId];
            if (duplicate?.isSharedToCurrentWorld && !nextMap[canonical.id]?.isSharedToCurrentWorld) {
              nextMap[canonical.id] = {
                ...nextMap[canonical.id],
                isSharedToCurrentWorld: true,
                sharedWorldId: duplicate.sharedWorldId,
                sharedByUserId: duplicate.sharedByUserId,
                sharedByNickname: duplicate.sharedByNickname,
              };
            }
            delete nextMap[duplicateId];
            templateMap.value = nextMap;
          } catch (e) {
            console.warn('Failed to replace duplicate builtin template references', e);
            ensured = false;
          }
        }
      }

      if (ensured) builtinTemplatesEnsured.value = true;
    })().finally(() => {
      builtinTemplatesEnsurePromise = null;
    });

    builtinTemplatesEnsurePromise = ensurePromise;
    return ensurePromise;
  };

  const ensureTemplatesLoaded = async (options?: TemplateQueryOptions) => {
    const worldId = String(options?.worldId ?? loadedWorldId.value ?? '').trim();
    if (templatesLoaded.value && loadedWorldId.value === worldId) {
      if (!options?.sheetType) {
        await ensureBuiltinTemplates(templates.value);
      }
      return;
    }
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

  const replaceTemplateReferences = async (templateId: string, replacementTemplateId: string) => {
    const fromId = String(templateId || '').trim();
    const toId = String(replacementTemplateId || '').trim();
    if (!fromId || !toId || fromId === toId) return;
    Object.keys(bindingsByChannel.value).forEach(channelId => markBindingsMutated(channelId));
    await api.post(`/api/v1/character-card-templates/${fromId}/replace-references`, {
      replacementTemplateId: toId,
    });
    Object.entries(bindingsByChannel.value).forEach(([channelId, channelMap]) => {
      let changed = false;
      const nextMap = { ...channelMap };
      Object.entries(nextMap).forEach(([externalCardId, binding]) => {
        if (binding.templateId !== fromId || binding.mode !== 'managed') return;
        nextMap[externalCardId] = { ...binding, templateId: toId };
        changed = true;
      });
      if (changed) {
        bindingsByChannel.value = {
          ...bindingsByChannel.value,
          [channelId]: nextMap,
        };
      }
    });
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
    const existingPromise = bindingsLoadPromises[channelId];
    if (existingPromise) return existingPromise;

    const requestVersion = getBindingsMutationVersion(channelId);
    const loadPromise = (async () => {
      const resp = await api.get('/api/v1/character-card-template-bindings', { params: { channelId } });
      const items = Array.isArray(resp.data?.items) ? resp.data.items as CharacterCardTemplateBinding[] : [];
      if (getBindingsMutationVersion(channelId) !== requestVersion) {
        return items;
      }

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
    })().finally(() => {
      delete bindingsLoadPromises[channelId];
    });

    bindingsLoadPromises[channelId] = loadPromise;
    return loadPromise;
  };

  const ensureBindingsLoaded = async (channelId: string) => {
    if (!channelId) return;
    if (bindingsLoadPromises[channelId]) {
      await bindingsLoadPromises[channelId];
      return;
    }
    if (bindingsLoadedChannels.value[channelId]) return;
    await loadBindings(channelId);
  };

  const upsertBinding = async (payload: BindingPayload) => {
    markBindingsMutated(payload.channelId);
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

    const preferredTemplate = getPreferredTemplateBySheetType(payload.sheetType);
    if (preferredTemplate?.id) {
      return bindCardToTemplate({
        channelId: payload.channelId,
        externalCardId: payload.externalCardId,
        cardName: payload.cardName,
        sheetType: payload.sheetType,
        templateId: getTemplateRef(preferredTemplate),
      });
    }

    const globalDefault = getGlobalDefaultTemplate();
    if (globalDefault?.id) {
      return bindCardToTemplate({
        channelId: payload.channelId,
        externalCardId: payload.externalCardId,
        cardName: payload.cardName,
        sheetType: payload.sheetType,
        templateId: getTemplateRef(globalDefault),
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
      templates.value.filter(item => !item.readonly).forEach(item => {
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
    getTemplateRef,
    getBinding,
    getTemplatesBySheetType,
    getSheetDefaultTemplate,
    getBuiltinTemplateBySheetType,
    isSameSheetType,
    getPreferredTemplateBySheetType,
    getGlobalDefaultTemplate,
    resolveDefaultTemplate,
    resolveCardTemplate,
    loadTemplates,
    ensureTemplatesLoaded,
    createTemplate,
    updateTemplate,
    replaceTemplateReferences,
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
