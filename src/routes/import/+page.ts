// import/+page.ts — Chargement client-side des jeux existants
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
  try {
    const res = await fetch('/api/games');
    if (!res.ok) return { games: [] };
    const games = await res.json();
    return { games: games ?? [] };
  } catch {
    return { games: [] };
  }
};
