/* gtkm PWA service worker.
   Static assets (css/js) are cached first for offline + fast start.
   Navigation (the token-bearing index page) is always network-first so a
   fresh RPC token from a node restart is never served stale from cache. */
'use strict';

const VERSION = 'gtkm-shield2-v7';
const ASSETS = [
  '/css/style.css',
  '/js/app.js',
  '/js/dashboard.js',
  '/js/mining.js',
  '/js/kings.js',
  '/js/wallet.js',
  '/js/phone.js',
  '/js/mail.js',
  '/js/supply.js',
  '/js/governance.js',
  '/js/console.js',
  '/manifest.webmanifest',
  '/icons/icon-192.png',
  '/icons/icon-512.png',
  '/icons/maskable-512.png',
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(VERSION).then((cache) => cache.addAll(ASSETS)).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k)))).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (url.pathname === '/rpc' || url.pathname.startsWith('/prover/') || url.pathname === '/healthz') return;

  if (req.mode === 'navigate') {
    // Network first: keep the dashboard in sync with the latest token/page.
    event.respondWith(
      fetch(req).catch(() => caches.match(req).then((r) => r || caches.match('/')))
    );
    return;
  }
  // Static assets: cache first, falling back to network, then populate cache.
  event.respondWith(
    caches.match(req).then((cached) => {
      if (cached) return cached;
      return fetch(req).then((res) => {
        if (res && res.status === 200) {
          const clone = res.clone();
          caches.open(VERSION).then((cache) => cache.put(req, clone));
        }
        return res;
      });
    })
  );
});