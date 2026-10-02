// gameMatch.ts — Retrouve le jeu nommé dans une question (« … à Wingspan ? »)

function normalize(s: string): string {
  return s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, ' ')
    .trim();
}

/** Nom du jeu sans article initial : « Les Aventuriers du Rail » → « aventuriers du rail » */
function key(name: string): string {
  return normalize(name).replace(/^(le|la|les|l|the) /, '');
}

/**
 * Retourne le nom du jeu cité dans la question, ou null si aucun (ou si
 * plusieurs jeux différents sont cités). Le nom le plus long l'emporte :
 * « Catan Junior » plutôt que « Catan ».
 */
export function detectGame(question: string, gameNames: string[]): string | null {
  const text = ` ${normalize(question)} `;
  const found = gameNames
    .filter((name) => key(name) && text.includes(` ${key(name)} `))
    .sort((a, b) => b.length - a.length);
  if (found.length === 0) return null;
  // Les autres correspondances doivent être contenues dans la plus longue
  const longest = key(found[0]);
  return found.every((name) => longest.includes(key(name))) ? found[0] : null;
}
