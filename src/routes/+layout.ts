// Désactiver SSR pour toute l'application — rendu côté client uniquement
import type { LayoutLoad } from './$types';

export const ssr = false;
export const prerender = false;

export const load: LayoutLoad = () => {
  // Réveille le backend (mise en veille quand inactif) sans bloquer l'affichage :
  // la page se rend immédiatement, les appels API attendent le backend d'eux-mêmes.
  fetch('/health').catch(() => {});
  return {};
};
