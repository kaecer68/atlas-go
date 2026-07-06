// Shared head config — injected by both client_web and admin_web at init
// Replaces duplicated <meta> tags across multiple index.html files

const SHARED_META = [
  { charset: 'UTF-8' },
  { 'http-equiv': 'X-UA-Compatible', content: 'IE=edge' },
];

const SHARED_LINK = [
  { rel: 'icon', type: 'image/x-icon', href: '/favicon.ico' },
];

const SHARED_OG = [
  { property: 'og:type', content: 'website' },
  { property: 'og:site_name', content: 'Atlas-Go' },
  { property: 'og:locale', content: 'zh_TW' },
];

const SHARED_TWITTER = [
  { name: 'twitter:site', content: '@atlas_go' },
];

function injectSharedHead() {
  if (typeof document === 'undefined') return;
  const head = document.head || document.getElementsByTagName('head')[0];
  if (!head) return;
  const existing = new Set(
    Array.from(head.querySelectorAll('meta,link'))
      .map(el => `${el.tagName}:${el.getAttribute('name') || el.getAttribute('property') || el.getAttribute('charset')}`)
  );
  for (const attrs of [...SHARED_META, ...SHARED_OG, ...SHARED_TWITTER]) {
    const key = `META:${attrs.name || attrs.property || attrs.charset || attrs['http-equiv']}`;
    if (existing.has(key)) continue;
    const el = document.createElement('meta');
    for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
    head.appendChild(el);
  }
  for (const attrs of SHARED_LINK) {
    const key = `LINK:${attrs.rel}`;
    if (existing.has(key)) continue;
    const el = document.createElement('link');
    for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
    head.appendChild(el);
  }
}

export { injectSharedHead, SHARED_META, SHARED_OG, SHARED_TWITTER };
