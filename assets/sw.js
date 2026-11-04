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

// Show a push notification. iOS revokes the subscription of apps that
// receive pushes without showing a notification, so one is always shown.
self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { body: event.data.text() };
  }
  event.waitUntil(
    self.registration.showNotification(data.title || "splitgo", {
      body: data.body || "",
      tag: data.tag || undefined,
      icon: "/assets/icons/icon-192.png",
      // Android draws the status bar badge from the alpha channel only.
      badge: "/assets/icons/badge-96.png",
      data: { url: data.url || "/" },
    }),
  );
});

// Open the notification's page, reusing an open window of the app.
self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = new URL(event.notification.data?.url || "/", self.location.origin);
  if (url.origin !== self.location.origin) {
    return;
  }
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      for (const client of windows) {
        if (client.url.startsWith(self.location.origin)) {
          await client.focus();
          // navigate() fails for windows this worker does not control.
          return client.navigate(url.href).catch(() => self.clients.openWindow(url.href));
        }
      }
      return self.clients.openWindow(url.href);
    })(),
  );
});
