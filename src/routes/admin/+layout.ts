// admin/+layout.ts — Vérification de l'auth via le Go backend
import type { LayoutLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { browser } from '$app/environment';

export const load: LayoutLoad = async ({ fetch, url }) => {
  if (!browser) return { isAuthenticated: false };

  try {
    const res = await fetch('/api/admin/check');
    const isAuthenticated = res.ok;

    if (!isAuthenticated && !url.pathname.startsWith('/admin/login')) {
      throw redirect(302, '/admin/login');
    }
    if (isAuthenticated && url.pathname === '/admin/login') {
      throw redirect(302, '/admin/games');
    }
    return { isAuthenticated };
  } catch (err) {
    // Si c'est un redirect, le laisser passer
    if (err && typeof err === 'object' && 'status' in err) throw err;
    if (!url.pathname.startsWith('/admin/login')) {
      throw redirect(302, '/admin/login');
    }
    return { isAuthenticated: false };
  }
};
