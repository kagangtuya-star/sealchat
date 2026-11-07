import { detectEmbeddedRuntime } from './embeddedRuntime';

export interface WindowFocusState {
  hasFocus: boolean;
  isVisible: boolean;
}

type NavigatorWithUserAgentData = Navigator & {
  userAgentData?: { mobile?: boolean };
};

export const isMobileBrowserRuntime = (): boolean => {
  if (typeof navigator === 'undefined') return false;
  const uaDataMobile = (navigator as NavigatorWithUserAgentData).userAgentData?.mobile;
  if (typeof uaDataMobile === 'boolean') return uaDataMobile;
  return /Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent || '');
};

export const shouldSuppressExternalNotification = (): boolean => {
  if (!isMobileBrowserRuntime()) return true;
  if (typeof document === 'undefined') return true;
  return document.visibilityState !== 'hidden';
};

const readDocumentFocusState = (doc: Document): WindowFocusState => ({
  hasFocus: typeof doc.hasFocus === 'function' ? doc.hasFocus() : true,
  isVisible: doc.visibilityState !== 'hidden',
});

export const resolveWindowFocusState = (): WindowFocusState => {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return { hasFocus: true, isVisible: true };
  }

  const localState = readDocumentFocusState(document);
  const runtime = detectEmbeddedRuntime();
  if (runtime.isEmbedRoute) {
    try {
      const topDocument = window.top?.document;
      if (topDocument) {
        return {
          hasFocus: typeof topDocument.hasFocus === 'function' ? topDocument.hasFocus() : true,
          isVisible: window.top?.document.visibilityState !== 'hidden',
        };
      }
    } catch {
      return {
        hasFocus: typeof document.hasFocus === 'function' ? document.hasFocus() : true,
        isVisible: document.visibilityState !== 'hidden',
      };
    }
  }

  return localState;
};
