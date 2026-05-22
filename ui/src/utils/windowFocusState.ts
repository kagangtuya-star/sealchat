import { detectEmbeddedRuntime } from './embeddedRuntime';

export interface WindowFocusState {
  hasFocus: boolean;
  isVisible: boolean;
}

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
          hasFocus: typeof topDocument.hasFocus === 'function' ? window.top?.document.hasFocus() : true,
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
