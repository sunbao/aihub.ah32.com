/* AIHub /app PWA service worker
 *
 * Goals:
 * - Cache only static assets under /app/ (NO /v1 API caching)
 * - Avoid stale UI by using network-first for index.html
 */

/* global self, caches, fetch */

const CACHE_PREFIX = "aihub-app-static";
// NOTE: Bump this when shipping embedded asset content changes without a filename hash change,
// so existing clients don't get stuck on stale cached bundles.
const CACHE_VERSION = "v2";
const CACHE_NAME = `${CACHE_PREFIX}-${CACHE_VERSION}`;

function getBasePath() {
  // registration.scope: e.g. https://example.com/app/
  try {
    const scopeUrl = new URL(self.registration.scope);
    const p = scopeUrl.pathname;
    return p.endsWith("/") ? p : `${p}/`;
  } catch {
    return "/app/";
  }
}

const BASE_PATH = getBasePath();
const INDEX_URL = `${BASE_PATH}index.html`;

self.addEventListener("install", (event) => {
  self.skipWaiting();
  event.waitUntil(Promise.resolve());
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(
        keys.map((k) => {
          if (k.startsWith(CACHE_PREFIX) && k !== CACHE_NAME) return caches.delete(k);
          return Promise.resolve(false);
        }),
      );
      await self.clients.claim();
    })(),
  );
});

async function cacheFirst(request) {
  const cache = await caches.open(CACHE_NAME);
  const cached = await cache.match(request);
  if (cached) return cached;

  const res = await fetch(request);
  if (res && res.ok) {
    cache.put(request, res.clone()).catch((err) => {
      // eslint-disable-next-line no-console
      console.warn("[AIHub SW] cache put failed", err);
    });
  }
  return res;
}

async function networkFirstIndex() {
  const cache = await caches.open(CACHE_NAME);
  try {
    const res = await fetch(INDEX_URL, { cache: "no-store" });
    if (res && res.ok) {
      cache.put(INDEX_URL, res.clone()).catch((err) => {
        // eslint-disable-next-line no-console
        console.warn("[AIHub SW] cache put failed", err);
      });
    }
    return res;
  } catch (err) {
    const cached = await cache.match(INDEX_URL);
    if (cached) return cached;
    throw err;
  }
}

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (!req || req.method !== "GET") return;

  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;

  // Only handle requests under /app/
  if (!url.pathname.startsWith(BASE_PATH)) return;

  // Never cache API traffic even if proxied under /app/
  if (url.pathname.startsWith(`${BASE_PATH}v1/`)) return;

  if (req.mode === "navigate") {
    event.respondWith(networkFirstIndex());
    return;
  }

  // Hashed assets: cache-first
  if (url.pathname.startsWith(`${BASE_PATH}assets/`)) {
    event.respondWith(cacheFirst(req));
    return;
  }

  // manifest/icons/etc: cache-first is fine (index is network-first)
  event.respondWith(cacheFirst(req));
});

self.addEventListener("message", (event) => {
  const data = event && event.data;
  if (data && data.type === "SKIP_WAITING") {
    self.skipWaiting();
  }
});
