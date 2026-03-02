// +page.ts — Chargement client-side de la liste des jeux depuis le Go backend
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
  try {
    const res = await fetch('/api/games');
    if (!res.ok) return { games: [], version: '1.0.0' };
    const games = await res.json();
    return { games: games ?? [], version: '1.0.0' };
  } catch {
    return { games: [], version: '1.0.0' };
  }
};
