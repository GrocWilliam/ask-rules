// gamesLoader.ts — Chargement de la liste des jeux avec suivi de progression
//
// Le backend est mis en veille quand il est inactif : au réveil, /api/games peut
// échouer (502) ou rester en attente longtemps. On limite chaque tentative dans
// le temps, on réessaie, et on publie l'état dans un store pour l'afficher.
import { writable } from 'svelte/store';
import type { Game } from '../types/game.type';

const MAX_ATTEMPTS = 12;
const ATTEMPT_TIMEOUT_MS = 10_000;
const RETRY_DELAY_MS = 3_000;

export type GamesLoadProgress = {
  attempt: number;
  maxAttempts: number;
  startedAt: number;
  /** Description de la dernière erreur ("HTTP 502", "pas de réponse en 10 s"…) */
  lastError: string | null;
};

export const gamesProgress = writable<GamesLoadProgress | null>(null);

function describeError(error: unknown): string {
  if (error instanceof DOMException && error.name === 'TimeoutError') {
    return `pas de réponse en ${ATTEMPT_TIMEOUT_MS / 1000} s`;
  }
  if (error instanceof SyntaxError) {
    return 'réponse invalide (pas du JSON)';
  }
  return 'serveur injoignable';
}

export async function loadGames(): Promise<Game[]> {
  const progress: GamesLoadProgress = {
    attempt: 0,
    maxAttempts: MAX_ATTEMPTS,
    startedAt: Date.now(),
    lastError: null,
  };

  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
    progress.attempt = attempt;
    gamesProgress.set({ ...progress });
    try {
      const response = await fetch('/api/games', {
        signal: AbortSignal.timeout(ATTEMPT_TIMEOUT_MS),
      });
      if (response.ok) {
        const games = ((await response.json()) as Game[]) ?? [];
        console.info(
          `[backend] /api/games OK (tentative ${attempt}, ${Date.now() - progress.startedAt} ms)`
        );
        gamesProgress.set(null);
        return games;
      }
      progress.lastError = `HTTP ${response.status}`;
    } catch (error) {
      progress.lastError = describeError(error);
    }
    console.warn(
      `[backend] /api/games tentative ${attempt}/${MAX_ATTEMPTS} : ${progress.lastError}`
    );
    gamesProgress.set({ ...progress });
    if (attempt < MAX_ATTEMPTS) {
      await new Promise((resolve) => setTimeout(resolve, RETRY_DELAY_MS));
    }
  }
  throw new Error(progress.lastError ?? 'backend indisponible');
}
