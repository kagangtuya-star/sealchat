// Web code effects run untrusted, user-authored HTML. The iframe sandbox is the
// isolation boundary: scripts only, opaque origin, so the document cannot reach the
// SealChat DOM, cookies or storage, open popups, submit forms or navigate the top
// window. The CSP is defense in depth for the current srcdoc document only; it narrows
// connect/frame capabilities, but network isolation is not a security guarantee because
// allowed HTTPS subresources and self-navigation can still reach external origins.
export const THEATER_EFFECT_WEB_SANDBOX = 'allow-scripts'

export const THEATER_EFFECT_WEB_CSP = [
  "default-src 'none'",
  "script-src 'unsafe-inline' 'unsafe-eval' https: blob: data:",
  "style-src 'unsafe-inline' https:",
  'img-src https: data: blob:',
  'font-src https: data:',
  'media-src https: data: blob:',
  'connect-src data: blob:',
  'worker-src blob:',
  "frame-src 'none'",
  "form-action 'none'",
  "base-uri 'none'",
].join('; ')

// Prepended before the author's styles so the author can still override layout,
// while the default canvas stays transparent and does not scroll.
const BASE_STYLE = 'html,body{margin:0;padding:0;width:100%;height:100%;overflow:hidden;background:transparent;}'

interface HtmlDocumentParser {
  parseFromString(source: string, type: 'text/html'): Document
}

const defaultParser = (): HtmlDocumentParser | null => (
  typeof DOMParser === 'undefined' ? null : new DOMParser()
)

// Parsing (which never runs scripts) and re-serializing guarantees the policy meta is
// the first element of the real <head>, regardless of comments or markup tricks in the
// author's source. Without a DOM parser nothing is returned, so nothing runs unguarded.
export const buildTheaterEffectWebSrcdoc = (html: string, parser: HtmlDocumentParser | null = defaultParser()) => {
  if (!parser || !html.trim()) return ''
  const document = parser.parseFromString(html, 'text/html')
  const csp = document.createElement('meta')
  csp.setAttribute('http-equiv', 'Content-Security-Policy')
  csp.setAttribute('content', THEATER_EFFECT_WEB_CSP)
  const referrer = document.createElement('meta')
  referrer.setAttribute('name', 'referrer')
  referrer.setAttribute('content', 'no-referrer')
  const style = document.createElement('style')
  style.textContent = BASE_STYLE
  document.head.prepend(csp, referrer, style)
  return `<!doctype html>${document.documentElement.outerHTML}`
}
