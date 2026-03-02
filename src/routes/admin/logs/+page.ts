// admin/logs/+page.ts — Chargement client-side des logs
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
  try {
    const res = await fetch('/api/admin/logs?limit=100');
    if (!res.ok) return { logs: [] };
    const logs = await res.json();
    return { logs: logs ?? [] };
  } catch {
    return { logs: [] };
  }
};
