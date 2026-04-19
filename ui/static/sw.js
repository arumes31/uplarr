const CACHE_NAME = 'uplarr-cache-v2';
const ASSETS = [
    '/',
    '/static/style.css',
    '/static/app.js',
    '/static/crypto.js',
    '/static/fonts.css',
    '/static/manifest.json',
    '/static/uplarr_logo_pwa_1776558824937.png'
];

self.addEventListener('install', (event) => {
    event.waitUntil(
        caches.open(CACHE_NAME).then((cache) => {
            return cache.addAll(ASSETS);
        })
    );
});

self.addEventListener('fetch', (event) => {
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
