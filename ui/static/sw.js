const CACHE_NAME = 'uplarr-cache-v6';
const OFFLINE_URL = '/static/offline.html';
const ASSETS = [
    '/static/style.css',
    '/static/app.js',
    '/static/crypto.js',
    '/static/fonts.css',
    '/static/manifest.json',
    '/static/favicon.png',
    '/static/icon-maskable-1024.png',
    OFFLINE_URL
];

self.addEventListener('install', (event) => {
    event.waitUntil(
        caches.open(CACHE_NAME).then((cache) => {
            return cache.addAll(ASSETS);
        })
    );
});

self.addEventListener('fetch', (event) => {
    const url = new URL(event.request.url);

    // Never intercept API requests — let SSE, auth, and data endpoints go
    // straight to the network without any service worker interference.
    if (url.pathname.startsWith('/api/')) {
        return;
    }

    // Network-First for navigation requests (HTML)
    if (event.request.mode === 'navigate') {
        event.respondWith(
            fetch(event.request).catch(async () => {
                const cache = await caches.open(CACHE_NAME);
                const cachedResponse = await cache.match(event.request);
                if (cachedResponse) return cachedResponse;
                return cache.match(OFFLINE_URL);
            })
        );
        return;
    }

    // Cache-First for static assets
    event.respondWith(
        caches.match(event.request).then((response) => {
            return response || fetch(event.request);
        })
    );
});

self.addEventListener('activate', (event) => {
    event.waitUntil(
        caches.keys().then((keys) => {
            return Promise.all(
                keys.filter(key => key !== CACHE_NAME).map(key => caches.delete(key))
            );
        })
    );
});
