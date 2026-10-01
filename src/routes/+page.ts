// +page.ts — Chargement client-side de la liste des jeux depuis le Go backend
import { loadGames } from '$lib/gamesLoader';
import type { PageLoad } from './$types';

const DEFAULT_VERSION = '0.0.0';

type VersionInfo = {
  version?: string;
};

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
