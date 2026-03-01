import { memoize } from "lodash-es";

export function formDataToJson(formData: FormData): Record<string, any> {
  const jsonObject: Record<string, any> = {};

  formData.forEach((value, key) => {
    const keys = key.split('.');
    let currentObject = jsonObject;

    for (let i = 0; i < keys.length - 1; i++) {
      const currentKey = keys[i];
      currentObject[currentKey] = currentObject[currentKey] || {};
      currentObject = currentObject[currentKey];
    }

    currentObject[keys[keys.length - 1]] = value.toString();
  });

  return jsonObject;
}

export function blobToArrayBuffer(blob: Blob) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      resolve(reader.result);
    };
    reader.onerror = reject;
    reader.readAsArrayBuffer(blob);
  }) as Promise<string | ArrayBuffer | null>;
}

export function arrayBufferToBase64(buffer: ArrayBuffer): string {
  let binary = '';
  const bytes = new Uint8Array(buffer);
  for (let i = 0; i < bytes.byteLength; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

export function base64ToUint8Array(base64: string): Uint8Array {
  const cleaned = base64.replace(/\s/g, '');
  const binary = atob(cleaned);
  const len = binary.length;
  const bytes = new Uint8Array(len);
  for (let i = 0; i < len; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

export function dataURItoBlob(dataURI: string) {
  // convert base64 to raw binary data held in a string
  // doesn't handle URLEncoded DataURIs - see SO answer #6850276 for code that does this
  var byteString = atob(dataURI.split(',')[1])

  // separate out the mime component
  var mimeString = dataURI.split(',')[0].split(':')[1].split(';')[0]

  // write the bytes of the string to an ArrayBuffer
  var ab = new ArrayBuffer(byteString.length)
  var ia = new Uint8Array(ab)
  for (var i = 0; i < byteString.length; i++) {
    ia[i] = byteString.charCodeAt(i)
  }

  return new Blob([ab], { type: mimeString })
}

export function memoizeWithTimeout<T>(func: (...args: any[]) => T, timeout: number): (...args: any[]) => T {
  const resolveMemoizeKey = (...args: any[]): string => args
    .map((arg) => {
      if (arg === null) return 'null';
      const t = typeof arg;
      if (t === 'string' || t === 'number' || t === 'boolean' || t === 'undefined') {
        return `${t}:${String(arg)}`;
      }
      try {
        return `json:${JSON.stringify(arg)}`;
      } catch {
        return `str:${String(arg)}`;
      }
    })
    .join('|');
  const memoizedFunc = memoize(func, resolveMemoizeKey);

  function timedMemoizedFunc(...args: any[]): T {
    const cacheKey = resolveMemoizeKey(...args);
    const result = memoizedFunc(...args);
    setTimeout(() => {
      memoizedFunc.cache.delete(cacheKey);
    }, timeout);
    return result;
  }

  return timedMemoizedFunc;
}

export function contentEscape(text: string) {
  return text.replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

const CONTENT_UNESCAPE_ROUNDS = 4;

export function contentUnescape(str: string): string {
  let current = str;
  for (let i = 0; i < CONTENT_UNESCAPE_ROUNDS; i += 1) {
    const next = current
      .replace(/&quot;/g, '"')
      .replace(/&#039;/g, "'")
      .replace(/&apos;/g, "'")
      .replace(/&gt;/g, '>')
      .replace(/&lt;/g, '<')
      .replace(/&amp;/g, '&');
    if (next === current) {
      return next;
    }
    current = next;
  }
  return current;
}

export function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}
