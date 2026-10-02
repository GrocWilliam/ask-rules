// chatHistory.ts — Historique des conversations, conservé dans le navigateur
//
// Pas de compte utilisateur : les conversations sont stockées en localStorage.
// Les derniers échanges de la conversation en cours sont renvoyés à /api/ask
// pour que le LLM comprenne les questions de suivi.
import { writable } from 'svelte/store';

const STORAGE_KEY = 'ask-rules:conversations';
const MAX_CONVERSATIONS = 30;
const MAX_EXCHANGES = 30;
/** Nombre d'échanges renvoyés au serveur (il en garde 3 au plus) */
const CONTEXT_TURNS = 3;
/** Seul un extrait des sections est affiché : inutile de stocker le reste */
const SECTION_EXCERPT_LENGTH = 220;

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
  at: number;
  answer?: string;
  model?: string;
  sections?: SectionResult[];
  cached?: boolean;
  file_path?: string[] | null;
  /** Message d'erreur si la question a échoué (non renvoyée au LLM) */
  error?: string;
};

export type Conversation = {
  id: string;
  game: string;
  createdAt: number;
  updatedAt: number;
  exchanges: Exchange[];
};

export type ChatTurn = { question: string; answer: string };

function load(): Conversation[] {
  if (typeof localStorage === 'undefined') return [];
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]');
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function persist(conversations: Conversation[]) {
  if (typeof localStorage === 'undefined') return;
  // Quota dépassé : on abandonne les conversations les plus anciennes
  for (let kept = conversations; kept.length > 0; kept = kept.slice(0, -1)) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(kept));
      return;
    } catch {
      // réessayer avec une conversation de moins
    }
  }
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // stockage indisponible (navigation privée…) : historique en mémoire seulement
  }
}

/** Conversations triées de la plus récente à la plus ancienne */
export const conversations = writable<Conversation[]>(load());
conversations.subscribe(persist);

function newId(): string {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function compact(exchange: Exchange): Exchange {
  return {
    ...exchange,
    sections: exchange.sections?.map((s) => ({
      ...s,
      content: s.content.slice(0, SECTION_EXCERPT_LENGTH),
    })),
  };
}

/**
 * Ajoute un échange à la conversation `id` (créée si `id` est null) et
 * retourne l'identifiant de la conversation.
 */
export function addExchange(id: string | null, game: string, exchange: Exchange): string {
  const conversationId = id ?? newId();
  conversations.update((list) => {
    const existing = list.find((c) => c.id === conversationId);
    const conversation: Conversation = existing
      ? {
          ...existing,
          updatedAt: exchange.at,
          exchanges: [...existing.exchanges, compact(exchange)].slice(-MAX_EXCHANGES),
        }
      : {
          id: conversationId,
          game,
          createdAt: exchange.at,
          updatedAt: exchange.at,
          exchanges: [compact(exchange)],
        };
    return [conversation, ...list.filter((c) => c.id !== conversationId)].slice(
      0,
      MAX_CONVERSATIONS
    );
  });
  return conversationId;
}

export function deleteConversation(id: string) {
  conversations.update((list) => list.filter((c) => c.id !== id));
}

export function clearConversations() {
  conversations.set([]);
}

/** Derniers échanges réussis, à envoyer comme contexte de la question suivante */
export function contextTurns(conversation: Conversation | undefined): ChatTurn[] {
  if (!conversation) return [];
  return conversation.exchanges
    .filter((e) => !e.error && e.answer && e.model)
    .slice(-CONTEXT_TURNS)
    .map((e) => ({ question: e.question, answer: e.answer as string }));
}
