// admin/games/+page.ts — Chargement client-side de la liste des jeux
import type { Game } from '../../../types/game.type';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }): Promise<{
  games: Game[]
}> => {
  try {
    const res = await fetch('/api/admin/games');
    if (!res.ok) return { games: [] };
    const games = await res.json();
    return { games: games ?? [] };
  } catch {
    return { games: [] };
  }
};
