/// <reference lib="webworker" />

const CACHE_NAME = 'reglomatic-v2'; // Version incrémentée pour forcer la mise à jour
const OFFLINE_URL = '/';

// Liste des fichiers à mettre en cache lors de l'installation
const STATIC_ASSETS = ['/', '/manifest.json', '/icon-192.png', '/icon-512.png'];

const self = /** @type {ServiceWorkerGlobalScope} */ (/** @type {unknown} */ (globalThis.self));

// Installation du service worker
self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHE_NAME);

      try {
        await cache.addAll(STATIC_ASSETS);
        console.log('[SW] Assets mis en cache');
      } catch (error) {
        console.error('[SW] Erreur lors de la mise en cache:', error);
      }

      // Force l'activation immédiate
      await self.skipWaiting();
    })()
  );
});

// Activation et nettoyage des anciens caches
self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      // Nettoyage des anciens caches
      const cacheNames = await caches.keys();
      await Promise.all(
        cacheNames.map((cacheName) => {
          if (cacheName !== CACHE_NAME) {
            console.log('[SW] Suppression ancien cache:', cacheName);
            return caches.delete(cacheName);
          }
        })
      );

      // Prend le contrôle immédiatement
      await self.clients.claim();
    })()
  );
});

// Stratégie de cache : Network First, puis Cache, puis Offline
self.addEventListener('fetch', (event) => {
  // Ignorer les requêtes qui ne sont pas HTTP/HTTPS
  if (!event.request.url.startsWith('http')) {
    return;
  }

  // Ignorer les requêtes POST, PUT, DELETE, etc.
  if (event.request.method !== 'GET') {
    return;
  }

  // Ignorer les requêtes API (SSE, streaming, JSON dynamique)
  // Cela évite les problèmes avec Server-Sent Events et les requêtes dynamiques
  const url = new URL(event.request.url);
  if (url.pathname.startsWith('/api/')) {
    return; // Laisser le navigateur gérer directement les requêtes API
  }

  // Ignorer les requêtes avec Accept: text/event-stream (SSE)
  const acceptHeader = event.request.headers.get('Accept');
  if (acceptHeader && acceptHeader.includes('text/event-stream')) {
    return;
  }

  event.respondWith(
    (async () => {
      try {
        // Essayer de récupérer depuis le réseau en premier
        const networkResponse = await fetch(event.request);

        // Si la réponse est OK, la mettre en cache
        if (networkResponse.ok) {
          const cache = await caches.open(CACHE_NAME);
          cache.put(event.request, networkResponse.clone());
        }

        return networkResponse;
      } catch (error) {
        // En cas d'échec réseau, chercher dans le cache
        const cachedResponse = await caches.match(event.request);

        if (cachedResponse) {
          return cachedResponse;
        }

        // Si pas de cache et pas de réseau, retourner la page offline pour les pages HTML
        if (event.request.destination === 'document') {
          const offlineCache = await caches.match(OFFLINE_URL);
          if (offlineCache) {
            return offlineCache;
          }
        }

        // Sinon, laisser échouer
        throw error;
      }
    })()
  );
});

// Gestion des messages depuis l'app
self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }
});
