export interface GeneratedAvatarThemeSeed {
  palette?: 'day' | 'night';
  customThemeEnabled?: boolean;
  activeCustomThemeId?: string | null;
}

export interface GeneratedAvatarImageOptions {
  displayName?: string;
  accentColor?: string;
  size?: number;
  themeSeed?: GeneratedAvatarThemeSeed;
}

const DAY_DEFAULTS = {
  background: 'rgba(226, 232, 240, 0.96)',
  border: 'rgba(148, 163, 184, 0.35)',
  text: '#334155',
  accent: '#3388de',
};

const NIGHT_DEFAULTS = {
  background: 'rgba(51, 65, 85, 0.92)',
  border: 'rgba(148, 163, 184, 0.28)',
  text: 'rgba(241, 245, 249, 0.95)',
  accent: '#60a5fa',
};

const normalizeDisplayText = (value?: string) => {
  const collapsed = String(value || '').replace(/\s+/g, '').trim();
  if (!collapsed) {
    return '匿';
  }
  return Array.from(collapsed).slice(0, 2).join('');
};

const readCssVar = (name: string) => {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return '';
  }
  return window.getComputedStyle(document.documentElement).getPropertyValue(name).trim();
};

const resolvePaletteColors = (themeSeed?: GeneratedAvatarThemeSeed) => {
  const defaults = themeSeed?.palette === 'night' ? NIGHT_DEFAULTS : DAY_DEFAULTS;
  const background = readCssVar('--sc-bg-layer') || readCssVar('--sc-bg-elevated') || defaults.background;
  const border = readCssVar('--sc-border-mute') || defaults.border;
  const text = readCssVar('--sc-text-primary') || readCssVar('--sc-text-secondary') || defaults.text;
  const accent = readCssVar('--sc-primary-color') || defaults.accent;
  return { background, border, text, accent };
};

const createCanvas = (size: number) => {
  if (typeof document === 'undefined') {
    return null;
  }
  const canvas = document.createElement('canvas');
  canvas.width = size;
  canvas.height = size;
  return canvas;
};

const drawRoundedRect = (ctx: CanvasRenderingContext2D, x: number, y: number, width: number, height: number, radius: number) => {
  const nextRadius = Math.min(radius, width / 2, height / 2);
  ctx.beginPath();
  ctx.moveTo(x + nextRadius, y);
  ctx.arcTo(x + width, y, x + width, y + height, nextRadius);
  ctx.arcTo(x + width, y + height, x, y + height, nextRadius);
  ctx.arcTo(x, y + height, x, y, nextRadius);
  ctx.arcTo(x, y, x + width, y, nextRadius);
  ctx.closePath();
};

const renderAvatarCanvas = (options: GeneratedAvatarImageOptions) => {
  const size = Math.max(64, Math.min(512, Math.round(options.size || 128)));
  const canvas = createCanvas(size);
  if (!canvas) {
    return null;
  }
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    return null;
  }
  const colors = resolvePaletteColors(options.themeSeed);
  const accentColor = String(options.accentColor || '').trim() || colors.accent;
  const text = normalizeDisplayText(options.displayName);
  const radius = size * 0.24;

  ctx.clearRect(0, 0, size, size);

  drawRoundedRect(ctx, 0, 0, size, size, radius);
  ctx.fillStyle = colors.background;
  ctx.fill();

  ctx.save();
  ctx.beginPath();
  drawRoundedRect(ctx, 0, 0, size, size, radius);
  ctx.clip();
  ctx.globalAlpha = 0.18;
  ctx.fillStyle = accentColor;
  ctx.beginPath();
  ctx.arc(size * 0.82, size * 0.16, size * 0.36, 0, Math.PI * 2);
  ctx.fill();
  ctx.globalAlpha = 0.12;
  ctx.fillRect(0, size * 0.72, size, size * 0.28);
  ctx.restore();

  drawRoundedRect(ctx, 1, 1, size - 2, size - 2, radius);
  ctx.strokeStyle = colors.border;
  ctx.lineWidth = Math.max(2, size * 0.03);
  ctx.stroke();

  const fontSize = text.length > 1 ? size * 0.34 : size * 0.5;
  ctx.fillStyle = colors.text;
  ctx.font = `700 ${fontSize}px sans-serif`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(text, size / 2, size / 2 + size * 0.015);

  return canvas;
};

export const buildGeneratedAvatarDataUrl = (options: GeneratedAvatarImageOptions) => {
  const canvas = renderAvatarCanvas(options);
  return canvas ? canvas.toDataURL('image/png') : '';
};

export const buildGeneratedAvatarFile = async (options: GeneratedAvatarImageOptions, filename = 'generated-avatar.png') => {
  const canvas = renderAvatarCanvas(options);
  if (!canvas) {
    throw new Error('生成头像失败');
  }
  const blob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((value) => {
      if (value) {
        resolve(value);
        return;
      }
      reject(new Error('生成头像失败'));
    }, 'image/png', 0.92);
  });
  return new File([blob], filename, { type: 'image/png' });
};
