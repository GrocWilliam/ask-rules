<script lang="ts">
  import { page } from '$app/stores';
  import { SvelteMap } from 'svelte/reactivity';
  import { resolve } from '$app/paths';

  export let data: {
    logs: Array<{
      id: number;
      event_type: string;
      message: string;
      metadata: Record<string, unknown> | null;
      ip_address: string | null;
      user_agent: string | null;
      created_at: string;
    }>;
  };

  // Regrouper les logs par date (calculé côté client)
  $: currentFilter = $page.url.searchParams.get('type');
  $: filteredLogs = currentFilter
    ? data.logs.filter((l) => l.event_type === currentFilter)
    : data.logs;

  $: logsByDate = (() => {
    const groups = new SvelteMap<string, typeof data.logs>();
    for (const log of filteredLogs) {
      const date = new Date(log.created_at).toLocaleDateString('fr-FR', {
        weekday: 'long',
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      });
      if (!groups.has(date)) groups.set(date, []);
      groups.get(date)!.push(log);
    }
    return [...groups.entries()];
  })();

  const eventTypeLabels: Record<string, string> = {
    game_added: '🎲 Jeu ajouté',
    game_updated: '🔄 Jeu mis à jour',
    game_deleted: '🗑️ Jeu supprimé',
    rate_limit_hit: '⚠️ Rate limit dépassé',
    rate_limit_blocked: '🚫 IP bloquée',
  };

  const eventTypeColors: Record<string, string> = {
    game_added: 'green',
    game_updated: 'blue',
    game_deleted: 'red',
    rate_limit_hit: 'orange',
    rate_limit_blocked: 'red',
  };

  function formatTime(dateStr: string): string {
    return new Date(dateStr).toLocaleTimeString('fr-FR', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  }

  function formatDateTime(dateStr: string): string {
    return new Date(dateStr).toLocaleString('fr-FR', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  }
</script>

<div class="page">
  <header class="header">
    <h1>📋 Logs système</h1>
    <p class="subtitle">
      Historique des événements ({data.logs.length} entrée{data.logs.length > 1 ? 's' : ''})
    </p>
  </header>

  <!-- Filtres -->
  <div class="filters">
    <a href={resolve('/admin/logs')} class="filter-btn" class:active={!currentFilter}>Tous</a>
    <a
      href="/admin/logs?type=game_added"
      class="filter-btn green"
      class:active={currentFilter === 'game_added'}
    >
      🎲 Jeux ajoutés
    </a>
    <a
      href="/admin/logs?type=game_updated"
      class="filter-btn blue"
      class:active={currentFilter === 'game_updated'}
    >
      🔄 Mises à jour
    </a>
    <a
      href="/admin/logs?type=rate_limit_blocked"
      class="filter-btn red"
      class:active={currentFilter === 'rate_limit_blocked'}
    >
      🚫 IPs bloquées
    </a>
  </div>

  <!-- Logs groupés par date -->
  {#if logsByDate.length === 0}
    <div class="empty-state">
      <p>Aucun log enregistré</p>
    </div>
  {:else}
    {#each logsByDate as [date, logs] (date)}
      <section class="date-group">
        <h2 class="date-header">{date}</h2>
        <div class="logs-list">
          {#each logs as log (log.id)}
            <article class="log-card {eventTypeColors[log.event_type]}">
              <div class="log-header">
                <span class="log-type">
                  {eventTypeLabels[log.event_type] || log.event_type}
                </span>
                <time class="log-time">{formatTime(log.created_at)}</time>
              </div>

              <p class="log-message">{log.message}</p>

              {#if log.metadata}
                <details class="log-metadata">
                  <summary>Détails</summary>
                  <pre>{JSON.stringify(log.metadata, null, 2)}</pre>
                </details>
              {/if}

              {#if log.ip_address}
                <div class="log-ip">
                  <span class="label">IP:</span>
                  <code>{log.ip_address}</code>
                </div>
              {/if}
            </article>
          {/each}
        </div>
      </section>
    {/each}
  {/if}

  <!-- Navigation -->
  <footer class="footer">
    <a href={resolve('/admin')} class="back-link">← Retour à l'administration</a>
  </footer>
</div>

<style>
  .page {
    max-width: 900px;
    margin: 0 auto;
    padding: 2rem 1.5rem;
  }

  .header {
    margin-bottom: 2rem;
  }

  .header h1 {
    font-size: 2rem;
    font-weight: 700;
    margin-bottom: 0.5rem;
  }

  .subtitle {
    color: var(--text-muted);
    font-size: 0.95rem;
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    margin-bottom: 2rem;
    padding: 1rem;
    background: var(--surface);
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
  }

  .filter-btn {
    padding: 0.5rem 1rem;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--text);
    text-decoration: none;
    font-size: 0.9rem;
    transition: all 0.2s;
  }

  .filter-btn:hover {
    background: var(--surface);
    transform: translateY(-1px);
  }

  .filter-btn.active {
    background: var(--accent);
    border-color: var(--accent);
    color: white;
  }

  .filter-btn.green.active {
    background: var(--green);
    border-color: var(--green);
  }

  .filter-btn.blue.active {
    background: #3b82f6;
    border-color: #3b82f6;
  }

  .filter-btn.red.active {
    background: var(--red);
    border-color: var(--red);
  }

  .date-group {
    margin-bottom: 2rem;
  }

  .date-header {
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-muted);
    margin-bottom: 1rem;
    padding-bottom: 0.5rem;
    border-bottom: 1px solid var(--border);
  }

  .logs-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .log-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-left: 3px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 1rem;
  }

  .log-card.green {
    border-left-color: var(--green);
  }

  .log-card.blue {
    border-left-color: #3b82f6;
  }

  .log-card.red {
    border-left-color: var(--red);
  }

  .log-card.orange {
    border-left-color: #fb923c;
  }

  .log-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
  }

  .log-type {
    font-weight: 600;
    font-size: 0.9rem;
  }

  .log-time {
    font-size: 0.85rem;
    color: var(--text-muted);
    font-family: monospace;
  }

  .log-message {
    margin-bottom: 0.5rem;
    line-height: 1.5;
  }

  .log-metadata {
    margin-top: 0.75rem;
  }

  .log-metadata summary {
    cursor: pointer;
    font-size: 0.85rem;
    color: var(--text-muted);
    user-select: none;
  }

  .log-metadata summary:hover {
    color: var(--accent);
  }

  .log-metadata pre {
    margin-top: 0.5rem;
    padding: 0.75rem;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    font-size: 0.8rem;
    overflow-x: auto;
    color: var(--text-muted);
  }

  .log-ip {
    margin-top: 0.5rem;
    font-size: 0.85rem;
    color: var(--text-muted);
  }

  .log-ip .label {
    margin-right: 0.5rem;
  }

  .log-ip code {
    background: var(--surface-2);
    padding: 0.2rem 0.5rem;
    border-radius: 0.25rem;
    font-size: 0.85rem;
  }

  .empty-state {
    text-align: center;
    padding: 4rem 2rem;
    color: var(--text-muted);
  }

  .footer {
    margin-top: 3rem;
    padding-top: 2rem;
    border-top: 1px solid var(--border);
  }

  .back-link {
    color: var(--accent);
    text-decoration: none;
    font-size: 0.95rem;
  }

  .back-link:hover {
    text-decoration: underline;
  }
</style>
