export type CharacterSheetProbeMatchMode = 'equals' | 'contains';

export interface CharacterSheetIntegrationProbe {
  command: string;
  matchMode: CharacterSheetProbeMatchMode;
  expected: string;
  timeoutMs?: number;
}

export interface CharacterSheetPluginInfo {
  name: string;
  description?: string;
  acquireText?: string;
  acquireUrl?: string;
}

export interface CharacterSheetIntegration {
  provider: 'sealdice';
  probe: CharacterSheetIntegrationProbe;
  plugin: CharacterSheetPluginInfo;
}

export interface CharacterSheetTemplateManifest {
  version: 1;
  sheetType: string;
  label?: string;
  integration?: CharacterSheetIntegration;
}

export interface CharacterSheetTypeMeta {
  key: string;
  label: string;
  aliases?: string[];
}

export const CHARACTER_SHEET_TYPES: readonly CharacterSheetTypeMeta[] = [
  { key: 'coc7', label: 'COC7', aliases: ['coc'] },
  { key: 'dnd5e', label: 'DND5', aliases: ['dnd5', 'dnd'] },
  { key: 'shinobigami', label: '忍神', aliases: ['忍神'] },
  { key: 'daggerheart', label: '匕首之心', aliases: ['dh', '匕首之心'] },
  { key: 'ta', label: '三角机构', aliases: ['triangle-agency', '三角机构'] },
];

export const normalizeCharacterSheetType = (value?: string) => {
  const normalized = (value || '').trim().toLowerCase();
  return CHARACTER_SHEET_TYPES.find(meta => (
    meta.key === normalized || meta.aliases?.some(alias => alias.toLowerCase() === normalized)
  ))?.key || normalized;
};

export const getCharacterSheetTypeMeta = (value?: string) => {
  const key = normalizeCharacterSheetType(value);
  return CHARACTER_SHEET_TYPES.find(meta => meta.key === key);
};

const isRecord = (value: unknown): value is Record<string, any> => (
  !!value && typeof value === 'object' && !Array.isArray(value)
);

const readText = (value: unknown, maxLength: number, required = false) => {
  if (value === undefined || value === null) return required ? null : undefined;
  if (typeof value !== 'string') return null;
  const text = value.trim();
  if (!text || text.length > maxLength) return null;
  return text;
};

const readHttpUrl = (value: unknown) => {
  const url = readText(value, 2048, false);
  if (url === undefined) return undefined;
  if (url === null || !/^https?:\/\/[^\s]+$/i.test(url)) return null;
  return url;
};

const parseIntegration = (value: unknown): CharacterSheetIntegration | null | undefined => {
  if (value === undefined) return undefined;
  if (!isRecord(value) || value.provider !== 'sealdice' || !isRecord(value.probe) || !isRecord(value.plugin)) {
    return null;
  }
  const command = readText(value.probe.command, 200, true);
  const expected = readText(value.probe.expected, 200, true);
  const pluginName = readText(value.plugin.name, 100, true);
  if (!command || !expected || !pluginName || /[\r\n]/.test(command)) return null;
  const matchMode = value.probe.matchMode;
  if (matchMode !== 'equals' && matchMode !== 'contains') return null;
  const timeoutRaw = value.probe.timeoutMs;
  let timeoutMs: number | undefined;
  if (timeoutRaw !== undefined) {
    if (!Number.isInteger(timeoutRaw) || timeoutRaw < 1000 || timeoutRaw > 15000) return null;
    timeoutMs = timeoutRaw;
  }
  const description = readText(value.plugin.description, 300, false);
  const acquireText = readText(value.plugin.acquireText, 300, false);
  const acquireUrl = readHttpUrl(value.plugin.acquireUrl);
  if (description === null || acquireText === null || acquireUrl === null) return null;
  return {
    provider: 'sealdice',
    probe: {
      command,
      matchMode,
      expected,
      ...(timeoutMs !== undefined ? { timeoutMs } : {}),
    },
    plugin: {
      name: pluginName,
      ...(description ? { description } : {}),
      ...(acquireText ? { acquireText } : {}),
      ...(acquireUrl ? { acquireUrl } : {}),
    },
  };
};

export const parseCharacterSheetTemplateManifest = (templateHtml?: string): CharacterSheetTemplateManifest | null => {
  const html = String(templateHtml || '');
  if (!html) return null;
  const scriptPattern = /<script\b([^>]*)>([\s\S]*?)<\/script\s*>/gi;
  let match: RegExpExecArray | null;
  while ((match = scriptPattern.exec(html))) {
    const attrs = match[1] || '';
    if (!/\bdata-sealchat-template-manifest(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+))?/i.test(attrs)) continue;
    if (!/\btype\s*=\s*(?:"application\/json"|'application\/json'|application\/json)(?:\s|$)/i.test(attrs)) continue;
    try {
      const raw = JSON.parse((match[2] || '').trim());
      if (!isRecord(raw) || raw.version !== 1) return null;
      const sheetType = readText(raw.sheetType, 64, true);
      const label = readText(raw.label, 100, false);
      const integration = parseIntegration(raw.integration);
      if (!sheetType || label === null || integration === null) return null;
      return {
        version: 1,
        sheetType,
        ...(label ? { label } : {}),
        ...(integration ? { integration } : {}),
      };
    } catch {
      return null;
    }
  }
  return null;
};

export const getCharacterSheetTemplateIntegration = (templateHtml?: string, sheetType?: string) => {
  const manifest = parseCharacterSheetTemplateManifest(templateHtml);
  if (!manifest?.integration) return undefined;
  if (sheetType && normalizeCharacterSheetType(manifest.sheetType) !== normalizeCharacterSheetType(sheetType)) {
    return undefined;
  }
  return manifest.integration;
};

export const matchCharacterSheetProbeResponse = (probe: CharacterSheetIntegrationProbe, content: string) => {
  switch (probe.matchMode) {
    case 'equals': return content === probe.expected;
    case 'contains': return content.includes(probe.expected);
    default: return false;
  }
};
