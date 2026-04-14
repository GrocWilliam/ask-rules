// Désactiver SSR pour toute l'application — rendu côté client uniquement
import type { LayoutLoad } from './$types';

export const ssr = false;
export const prerender = false;

export const load: LayoutLoad = async ({ fetch }) => {
  let healthy = false;

  try {
    const response = await fetch('/health');
    healthy = response.ok;
  } catch {
    healthy = false;
  }

  return { healthy };
};
