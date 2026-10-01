// Désactiver SSR pour toute l'application — rendu côté client uniquement
import type { LayoutLoad } from './$types';

export const ssr = false;
export const prerender = false;

export const load: LayoutLoad = () => {
  // Réveille le backend (mise en veille quand inactif) sans bloquer l'affichage :
  // la page se rend immédiatement, les appels API attendent le backend d'eux-mêmes.
  const startedAt = Date.now();
  fetch('/health')
    .then((response) =>
      console.info(`[backend] /health : HTTP ${response.status} en ${Date.now() - startedAt} ms`)
    )
    .catch(() =>
      console.warn(`[backend] /health : injoignable après ${Date.now() - startedAt} ms`)
    );
  return {};
};
