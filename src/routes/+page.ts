// +page.ts — Chargement client-side de la liste des jeux depuis le Go backend
import type { Game } from '../types/game.type';
import type { PageLoad } from './$types';

const DEFAULT_VERSION = '0.0.0';
// Le backend peut mettre du temps à sortir de veille : on réessaie pendant ~1 min
const GAMES_MAX_ATTEMPTS = 20;
const GAMES_RETRY_DELAY_MS = 3000;

type VersionInfo = {
  version?: string;
};

async function loadGames(): Promise<Game[]> {
  for (let attempt = 1; attempt <= GAMES_MAX_ATTEMPTS; attempt++) {
    try {
      const response = await fetch('/api/games');
      if (response.ok) {
        return ((await response.json()) as Game[]) ?? [];
      }
    } catch {
      // backend pas encore joignable
    }
    if (attempt < GAMES_MAX_ATTEMPTS) {
      await new Promise((resolve) => setTimeout(resolve, GAMES_RETRY_DELAY_MS));
    }
  }
  throw new Error('backend indisponible');
}

export const load: PageLoad = async ({ fetch }) => {
  // Non attendu : la page s'affiche pendant que le backend se réveille
  const games = loadGames();

  // version.json est servi par le frontend : rapide, même backend en veille
  let version = DEFAULT_VERSION;
  try {
    const response = await fetch('/version.json');
    if (response.ok) {
      version = ((await response.json()) as VersionInfo).version ?? DEFAULT_VERSION;
    }
  } catch {
    // version par défaut
  }

  return { games, version };
};
