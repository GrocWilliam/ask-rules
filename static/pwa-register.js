/**
 * Enregistrement du Service Worker pour la PWA.
 * Chargé comme script classique depuis app.html : pas d'`export` ici.
 * L'invite d'installation est gérée par src/lib/PWAInstall.svelte.
 */
(function () {
  if (!('serviceWorker' in navigator)) {
    console.warn('[PWA] Les Service Workers ne sont pas supportés par ce navigateur');
    return;
  }

  // Ne recharger la page que lorsque l'utilisateur a accepté la mise à jour
  // (sinon la première installation, via clients.claim(), déclencherait un rechargement)
  let reloadOnControllerChange = false;
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    if (reloadOnControllerChange) {
      reloadOnControllerChange = false;
      window.location.reload();
    }
  });

  function promptUpdate(worker) {
    if (confirm('Une nouvelle version de Reglomatic est disponible. Voulez-vous recharger ?')) {
      reloadOnControllerChange = true;
      worker.postMessage({ type: 'SKIP_WAITING' });
    }
  }

  window.addEventListener('load', async () => {
    try {
      const registration = await navigator.serviceWorker.register('/service-worker.js', {
        scope: '/',
      });

      // Une mise à jour déjà téléchargée attend peut-être depuis la dernière visite
      if (registration.waiting && navigator.serviceWorker.controller) {
        promptUpdate(registration.waiting);
      }

      registration.addEventListener('updatefound', () => {
        const newWorker = registration.installing;
        if (!newWorker) return;
        newWorker.addEventListener('statechange', () => {
          if (newWorker.state === 'installed' && navigator.serviceWorker.controller) {
            promptUpdate(newWorker);
          }
        });
      });
    } catch (error) {
      console.error("[PWA] Erreur lors de l'enregistrement du Service Worker:", error);
    }
  });
})();
