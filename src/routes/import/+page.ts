// import/+page.ts — Import réservé à l'admin, puis chargement des jeux existants
import { redirect } from '@sveltejs/kit';
import type { Game } from '../../types/game.type';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }): Promise<{ games: Game[] }> => {
  // L'API d'import exige une session admin : se connecter d'abord
  const auth = await fetch('/api/admin/check').catch(() => null);
  if (!auth?.ok) {
    throw redirect(302, '/admin/login?next=/import');
  }

  try {
    const res = await fetch('/api/games');
    if (!res.ok) return { games: [] };
    const games = await res.json();
    return { games: games ?? [] };
  } catch {
    return { games: [] };
  }
};
