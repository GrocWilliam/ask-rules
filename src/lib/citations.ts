// citations.ts — Rend cliquables les pages citées dans une réponse (« p. 4 »)
//
// Le LLM cite les pages du livret de règles ; chaque citation devient un lien
// qui ouvre le PDF à cette page. Le fichier est déduit des sections sources :
// celle dont la plage de pages contient la page citée, sinon le seul PDF cité.
import type { SectionResult } from './chatHistory';

const CITATION = /\b(p\.|pages?)\s?(\d{1,4})(?:\s?[-–]\s?(\d{1,4}))?/gi;

/** URL du PDF ouvert à une page (chaque segment du chemin est encodé). */
export function fileHref(file: string, page: number): string {
  const path = file.split('/').map(encodeURIComponent).join('/');
  return `/files/${path}#page=${page}`;
}

function fileForPage(page: number, sections: SectionResult[]): string | null {
  const pdfs = sections.filter((s) => s.file?.toLowerCase().endsWith('.pdf'));
  const containing = pdfs.find(
    (s) => s.page_num != null && s.page_num <= page && page <= (s.page_end || s.page_num)
  );
  if (containing?.file) return containing.file;
  const files = new Set(pdfs.map((s) => s.file));
  return files.size === 1 ? (pdfs[0].file ?? null) : null;
}

/** Remplace les citations de pages du markdown par des liens vers le PDF. */
export function linkCitations(markdown: string, sections: SectionResult[] | undefined): string {
  if (!sections?.length) return markdown;
  return markdown.replace(CITATION, (match, _prefix: string, start: string) => {
    const page = Number(start);
    const file = page > 0 ? fileForPage(page, sections) : null;
    if (!file) return match;
    return `<a href="${fileHref(file, page)}" class="citation" target="_blank" rel="noopener" title="Ouvrir le livret de règles à cette page">${match}</a>`;
  });
}
