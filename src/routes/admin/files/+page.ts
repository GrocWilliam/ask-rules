import type { GameFile } from '../../../types/gamefile.type';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }): Promise<{ files: GameFile[] }> => {
  try {
    const res = await fetch('/api/admin/files');
    if (!res.ok) return { files: [] };
    const files = await res.json();
    return { files: files ?? [] };
  } catch {
    return { files: [] };
  }
};
