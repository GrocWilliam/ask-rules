// askStream.ts — Pose une question à /api/ask/stream et lit la réponse au fil de l'eau
//
// Le serveur envoie des Server-Sent Events : « delta » (fragment de réponse),
// « done » (réponse complète) ou « error ». Une erreur survenue avant la
// réponse (jeu inconnu, limite atteinte…) arrive en JSON avec son code HTTP.
import type { ChatTurn, SectionResult } from './chatHistory';

export type AskResult = {
  game: string;
  answer: string;
  model: string;
  sections: SectionResult[];
  cached: boolean;
  file_path: string[] | null;
};

export async function askStream(
  body: { question: string; game: string; history: ChatTurn[] },
  onDelta: (text: string) => void
): Promise<AskResult> {
  let res: Response;
  try {
    res = await fetch('/api/ask/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
      body: JSON.stringify(body),
    });
  } catch {
    throw new Error('Erreur réseau — vérifiez votre connexion.');
  }

  if (!res.ok || !res.body) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error ?? `Erreur serveur (${res.status})`);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  for (;;) {
    const { done, value } = await reader.read().catch(() => {
      throw new Error('Connexion interrompue pendant la réponse.');
    });
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    // Un événement se termine par une ligne vide
    let end: number;
    while ((end = buffer.indexOf('\n\n')) >= 0) {
      const raw = buffer.slice(0, end);
      buffer = buffer.slice(end + 2);
      let event = 'message';
      let data = '';
      for (const line of raw.split('\n')) {
        if (line.startsWith('event:')) event = line.slice(6).trim();
        else if (line.startsWith('data:')) data += line.slice(5).trim();
      }
      if (!data) continue;
      const payload = JSON.parse(data);
      if (event === 'delta') onDelta(payload.text ?? '');
      else if (event === 'done') return payload as AskResult;
      else if (event === 'error') throw new Error(payload.error ?? 'Erreur serveur');
    }
  }
  throw new Error('Connexion interrompue pendant la réponse.');
}
