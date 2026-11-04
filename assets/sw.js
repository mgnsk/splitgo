// splitgo service worker: network only, with an offline fallback page for
// navigations. Data is never cached so balances are never stale.
const CACHE = "splitgo-offline-v1";
const OFFLINE_URL = "/assets/offline.html";

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE).then((cache) => cache.addAll([OFFLINE_URL, "/assets/app.css"])),
  );
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((key) => key !== CACHE).map((key) => caches.delete(key))),
    ),
  );
  self.clients.claim();
});

self.addEventListener("fetch", (event) => {
  if (event.request.mode !== "navigate") {
    return;
  }
  event.respondWith(
    fetch(event.request).catch(async () => {
      const cache = await caches.open(CACHE);
      return cache.match(OFFLINE_URL);
    }),
  );
});
