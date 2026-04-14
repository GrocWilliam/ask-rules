// +page.ts — Chargement client-side de la liste des jeux depuis le Go backend
import type { Game } from '../types/game.type';
import type { PageLoad } from './$types';

const DEFAULT_VERSION = '0.0.0';

type VersionInfo = {
  version?: string;
};

export const load: PageLoad = async ({ fetch }): Promise<{ games: Game[], version: string }> => {
  try {
    const [gamesResponse, versionResponse] = await Promise.all([
      fetch('/api/games'),
      fetch('/version.json'),
    ]);

    const games = gamesResponse.ok ? ((await gamesResponse.json()) as Game[]) : [];
    const versionInfo = versionResponse.ok
      ? ((await versionResponse.json()) as VersionInfo)
      : { version: DEFAULT_VERSION };

    return {
      games: games ?? [],
      version: versionInfo.version ?? DEFAULT_VERSION,
    };
  } catch {
    return { games: [], version: DEFAULT_VERSION };
  }
};
