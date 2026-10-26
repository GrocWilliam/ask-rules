// chatHistory.ts — Conversation en cours, gardée en mémoire (perdue au rechargement)
//
// Les derniers échanges de la conversation en cours sont renvoyés à /api/ask
// pour que le LLM comprenne les questions de suivi.

const MAX_EXCHANGES = 30;
/** Nombre d'échanges renvoyés au serveur (il en garde 3 au plus) */
const CONTEXT_TURNS = 3;

// Ancien historique sauvegardé dans le navigateur : on l'efface
try {
  localStorage.removeItem('ask-rules:conversations');
} catch {
  // rendu serveur ou stockage indisponible : rien à effacer
}

export type SectionResult = {
  title: string;
  content: string;
  score: number;
  page_num: number | null;
  page_end?: number | null;
  file?: string;
};

export type Exchange = {
  question: string;
  answer?: string;
  model?: string;
  sections?: SectionResult[];
  cached?: boolean;
  file_path?: string[] | null;
  /** Message d'erreur si la question a échoué (non renvoyée au LLM) */
  error?: string;
};

export type Conversation = {
  game: string;
  exchanges: Exchange[];
};

export type ChatTurn = { question: string; answer: string };

/** Ajoute un échange à la conversation (créée si elle est null) */
export function addExchange(
  conversation: Conversation | null,
  game: string,
  exchange: Exchange
): Conversation {
  return conversation
    ? { ...conversation, exchanges: [...conversation.exchanges, exchange].slice(-MAX_EXCHANGES) }
    : { game, exchanges: [exchange] };
}

/** Derniers échanges réussis, à envoyer comme contexte de la question suivante */
export function contextTurns(conversation: Conversation | null): ChatTurn[] {
  if (!conversation) return [];
  return conversation.exchanges
    .filter((e) => !e.error && e.answer && e.model)
    .slice(-CONTEXT_TURNS)
    .map((e) => ({ question: e.question, answer: e.answer as string }));
}
