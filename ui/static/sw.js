const CACHE_NAME = 'uplarr-cache-v8';
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
    console.log('SW: Install event');
    self.skipWaiting();
    event.waitUntil(
        caches.open(CACHE_NAME).then((cache) => {
            console.log('SW: Pre-caching assets');
            return cache.addAll(ASSETS);
        })
    );
});

self.addEventListener('fetch', (event) => {
    const url = new URL(event.request.url);

    // Never intercept API requests — let SSE, auth, and data endpoints go
    // straight to the network without any service worker interference.
    if (url.pathname.startsWith('/api/')) {
        // console.log('SW: Bypassing API request:', url.pathname);
        return;
    }

    // Network-First for navigation requests (HTML)
    if (event.request.mode === 'navigate') {
        event.respondWith(
            fetch(event.request).then(response => {
                // console.log('SW: Navigation - Network success:', url.pathname);
                return response;
            }).catch(async (err) => {
                console.warn('SW: Navigation - Network failed, falling back to cache/offline:', url.pathname, err);
                const cache = await caches.open(CACHE_NAME);
                const cachedResponse = await cache.match(event.request);
                if (cachedResponse) return cachedResponse;
                return cache.match(OFFLINE_URL);
            })
        );
        return;
    }

    // Network-First for JS files to avoid stale code after deployments.
    // Falls back to cache for offline support.
    if (url.pathname.endsWith('.js')) {
        event.respondWith(
            fetch(event.request).then((response) => {
                // console.log('SW: JS - Network success, updating cache:', url.pathname);
                const clone = response.clone();
                caches.open(CACHE_NAME).then((cache) => cache.put(event.request, clone));
                return response;
            }).catch((err) => {
                console.warn('SW: JS - Network failed, falling back to cache:', url.pathname, err);
                return caches.match(event.request);
            })
        );
        return;
    }

    // Cache-First for other static assets (CSS, fonts, images)
    event.respondWith(
        caches.match(event.request).then((response) => {
            if (response) {
                // console.log('SW: Asset - Cache hit:', url.pathname);
                return response;
            }
            // console.log('SW: Asset - Cache miss, fetching:', url.pathname);
            return fetch(event.request);
        })
    );
});

self.addEventListener('activate', (event) => {
    console.log('SW: Activate event - Taking control of clients');
    event.waitUntil(
        caches.keys().then((keys) => {
            return Promise.all(
                keys.filter(key => key !== CACHE_NAME).map(key => caches.delete(key))
            );
        }).then(() => {
            console.log('SW: Old caches cleared');
            return self.clients.claim();
        })
    );
});
