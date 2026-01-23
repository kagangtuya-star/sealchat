import twemoji from '@twemoji/api';

const CDN_BASE = 'https://cdn.jsdelivr.net/gh/twitter/twemoji@latest/assets/';
const LOCAL_BASE = `${import.meta.env.BASE_URL}twemoji/`;

let cdnAvailable: boolean | null = null;
let cdnCheckPromise: Promise<boolean> | null = null;
let localAvailable: boolean | null = null;
let localCheckPromise: Promise<boolean> | null = null;

async function checkAssetAvailability(src: string, assign: (value: boolean) => void): Promise<boolean> {
  if (typeof window === 'undefined') {
    assign(false);
    return false;
  }
  return new Promise((resolve) => {
    const img = new Image();
    let settled = false;
    const finalize = (result: boolean) => {
      if (settled) return;
      settled = true;
      assign(result);
      resolve(result);
    };
    img.onload = () => finalize(true);
    img.onerror = () => finalize(false);
    img.src = src;
    window.setTimeout(() => {
      finalize(false);
    }, 3000);
  });
}

async function checkCdnAvailability(): Promise<boolean> {
  if (cdnAvailable !== null) return cdnAvailable;
  if (cdnCheckPromise) return cdnCheckPromise;
  cdnCheckPromise = checkAssetAvailability(`${CDN_BASE}svg/1f44d.svg`, (value) => {
    cdnAvailable = value;
  });
  return cdnCheckPromise;
}

async function checkLocalAvailability(): Promise<boolean> {
  if (localAvailable !== null) return localAvailable;
  if (localCheckPromise) return localCheckPromise;
  localCheckPromise = checkAssetAvailability(`${LOCAL_BASE}svg/1f44d.svg`, (value) => {
    localAvailable = value;
  });
  return localCheckPromise;
}

if (typeof window !== 'undefined') {
  void checkCdnAvailability();
  void checkLocalAvailability();
}

function getBaseUrl(): string {
  if (cdnAvailable === true) return CDN_BASE;
  if (localAvailable === true) return LOCAL_BASE;
  if (cdnAvailable === false && localAvailable === false) return CDN_BASE;
  return CDN_BASE;
}

function getFallbackBase(primary: string): string {
  if (primary === CDN_BASE) {
    if (localAvailable === true) return LOCAL_BASE;
    return CDN_BASE;
  }
  if (cdnAvailable === true) return CDN_BASE;
  if (localAvailable === true) return LOCAL_BASE;
  return primary;
}

export function getEmojiUrl(emoji: string): string {
  const codePoint = twemoji.convert.toCodePoint(emoji);
  return `${getBaseUrl()}svg/${codePoint}.svg`;
}

export function getFallbackUrl(emoji: string): string {
  const codePoint = twemoji.convert.toCodePoint(emoji);
  const base = getFallbackBase(getBaseUrl());
  return `${base}svg/${codePoint}.svg`;
}

export function parseEmoji(text: string): string {
  return twemoji.parse(text, {
    folder: 'svg',
    ext: '.svg',
    base: getBaseUrl(),
  });
}

export const twemojiClass = 'twemoji-img';
