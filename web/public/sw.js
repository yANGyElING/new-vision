/* new-vision PWA service worker：仅缓存构建产物静态资源，不缓存 API 请求 */
const CACHE_NAME = 'new-vision-static-v1'
const ASSET_CACHE = ['/', '/index.html', '/manifest.json', '/icons/icon.svg']

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(ASSET_CACHE)).then(() => self.skipWaiting()),
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))).then(() => self.clients.claim()),
  )
})

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url)
  // 仅缓存 GET 静态资源（/assets/* 构建产物 + 页面入口），API 请求一律走网络
  if (event.request.method !== 'GET') return
  if (url.pathname.startsWith('/api/')) return
  if (url.pathname.startsWith('/assets/') || url.pathname === '/' || url.pathname === '/index.html' || url.pathname === '/manifest.json') {
    event.respondWith(
      caches.match(event.request).then((cached) => {
        const fetched = fetch(event.request)
          .then((response) => {
            if (response.ok) {
              const copy = response.clone()
              caches.open(CACHE_NAME).then((cache) => cache.put(event.request, copy))
            }
            return response
          })
          .catch(() => cached)
        return cached || fetched
      }),
    )
  }
})
