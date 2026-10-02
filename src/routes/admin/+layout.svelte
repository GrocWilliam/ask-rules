<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import type { LayoutData } from './$types';
  import { resolve } from '$app/paths';

  export let data: LayoutData;

  let isLoggingOut = false;

  async function handleLogout() {
    isLoggingOut = true;
    try {
      await fetch('/api/admin/logout', { method: 'POST' });
    } finally {
      isLoggingOut = false;
      goto(resolve('/admin/login'));
    }
  }
</script>

{#if data.isAuthenticated}
  <div class="admin-layout">
    <nav class="admin-nav">
      <div class="admin-nav-header">
        <a href={resolve('/')} class="admin-brand">← Reglomatic</a>
        <h1>Administration</h1>
      </div>

      <div class="admin-nav-links">
        <a
          href={resolve('/admin/games')}
          class="nav-link"
          class:active={$page.url.pathname.startsWith('/admin/games')}
        >
          🎮 Jeux
        </a>
        <a
          href={resolve('/admin/files')}
          class="nav-link"
          class:active={$page.url.pathname.startsWith('/admin/files')}
        >
          📁 Fichiers
        </a>
        <a
          href={resolve('/admin/logs')}
          class="nav-link"
          class:active={$page.url.pathname.startsWith('/admin/logs')}
        >
          📋 Logs
        </a>
        <a href={resolve('/import')} class="nav-link"> 📥 Importer des règles </a>
      </div>

      <button class="btn-logout" disabled={isLoggingOut} on:click={handleLogout}>
        {isLoggingOut ? 'Déconnexion...' : '🚪 Se déconnecter'}
      </button>
    </nav>

    <main class="admin-content">
      <slot />
    </main>
  </div>
{:else}
  <slot />
{/if}

<style>
  .admin-layout {
    display: flex;
    height: calc(100vh - 50px);
    width: 100%;
  }

  .admin-nav {
    width: 250px;
    background: #1f2937;
    color: white;
    padding: 2rem 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 2rem;
    position: fixed;
    height: calc(100vh - 50px);
    overflow-y: auto;
  }

  .admin-nav-header {
    border-bottom: 1px solid #374151;
    padding-bottom: 1rem;
  }

  .admin-brand {
    color: #9ca3af;
    text-decoration: none;
    font-size: 0.875rem;
    display: block;
    margin-bottom: 0.5rem;
    transition: color 0.2s;
  }

  .admin-brand:hover {
    color: #d1d5db;
  }

  .admin-nav-header h1 {
    font-size: 1.5rem;
    margin: 0;
    color: white;
  }

  .admin-nav-links {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    flex: 1;
  }

  .nav-link {
    color: #d1d5db;
    text-decoration: none;
    padding: 0.75rem 1rem;
    border-radius: 8px;
    transition: all 0.2s;
    font-weight: 500;
  }

  .nav-link:hover {
    background: #374151;
    color: white;
  }

  .nav-link.active {
    background: #4f46e5;
    color: white;
  }

  .btn-logout {
    width: 100%;
    padding: 0.75rem;
    background: #374151;
    color: white;
    border: none;
    border-radius: 8px;
    font-weight: 500;
    cursor: pointer;
    transition: background 0.2s;
  }

  .btn-logout:hover:not(:disabled) {
    background: #4b5563;
  }

  .btn-logout:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .admin-content {
    margin-left: 250px;
    flex: 1;
    padding: 2rem;
    background: #f9fafb;
    height: calc(100vh - 50px);
  }

  @media (max-width: 768px) {
    .admin-layout {
      flex-direction: column;
    }

    .admin-nav {
      width: 100%;
      height: auto;
      position: relative;
    }

    .admin-content {
      margin-left: 0;
    }
  }
</style>
