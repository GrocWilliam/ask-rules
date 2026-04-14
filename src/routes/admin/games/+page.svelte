<script lang="ts">
  import { invalidateAll } from '$app/navigation';
  import type { PageData } from './$types';
  import SEO from '$lib/SEO.svelte';
  import JobTracker from '$lib/JobTracker.svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { resolve } from '$app/paths';

  export let data: PageData;

  let actionMsg: { ok: boolean; msg: string } | null = null;
  let deletingGame: string | null = null;
  let confirmDelete: string | null = null;
  let reprocessingGame: string | null = null;

  // ── Reprocess One ──────────────────────────────────────────────────────────
  type StepEntry = { msg: string; status: 'running' | 'done' | 'error' };

  let reprocessOneActive = false;
  let reprocessOneDone = false;
  let reprocessOneGameName = '';
  let reprocessOneLog: string[] = [];
  let reprocessOneSteps: StepEntry[] = [];
  let reprocessOneEmbedding: { current: number; total: number } | null = null;
  let reprocessOneError: string | null = null;

  // ── Reprocess All ──────────────────────────────────────────────────────────
  type GameProgress = {
    name: string;
    status: 'pending' | 'running' | 'done' | 'error';
    msg?: string;
  };

  let reprocessAllActive = false;
  let reprocessAllDone = false;
  let reprocessAllTotal = 0;
  let reprocessAllCurrent = 0;
  let reprocessAllSuccesses = 0;
  let reprocessAllErrors = 0;
  let reprocessAllLog: string[] = [];
  let reprocessAllGameList: GameProgress[] = [];
  let reprocessAllCurrentGame = '';
  let reprocessAllEmbedding: { current: number; total: number } | null = null;

  // ── Job Tracker ────────────────────────────────────────────────────────────
  let jobTrackerActive = false;
  let currentJobId = '';

  // ── Sélection de jeux pour le recalcul ────────────────────────────────────
  let selectedGameIDs = new SvelteSet<string>();

  function toggleGameSelection(id: string) {
    if (selectedGameIDs.has(id)) {
      selectedGameIDs.delete(id);
    } else {
      selectedGameIDs.add(id);
    }
    selectedGameIDs = new SvelteSet(selectedGameIDs); // déclencher la réactivité Svelte
  }

  function selectAllGames() {
    selectedGameIDs = new SvelteSet(data.games.map((g) => g.id));
  }

  function selectNoGames() {
    selectedGameIDs = new SvelteSet();
  }

  $: selectedCount = selectedGameIDs.size;

  async function reprocessAll() {
    reprocessAllActive = true;
    reprocessAllDone = false;
    reprocessAllCurrent = 0;
    reprocessAllSuccesses = 0;
    reprocessAllErrors = 0;
    reprocessAllLog = [];
    reprocessAllGameList = [];
    reprocessAllCurrentGame = '';
    reprocessAllEmbedding = null;

    // Jeux ciblés : sélection explicite ou tous
    const targetIDs = selectedCount > 0 ? [...selectedGameIDs] : [];
    const targetGames =
      targetIDs.length > 0 ? data.games.filter((g) => targetIDs.includes(g.id)) : data.games;

    try {
      const res = await fetch('/api/admin/reprocess-all', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ game_ids: targetIDs }),
      });
      if (!res.body) throw new Error('Pas de stream SSE');

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() ?? '';

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;
          let evt: Record<string, unknown>;
          try {
            evt = JSON.parse(line.slice(6));
          } catch {
            continue;
          }

          const t = evt.type as string;

          if (t === 'job_started') {
            currentJobId = evt.job_id as string;
            jobTrackerActive = true;
          } else if (t === 'start') {
            reprocessAllTotal = evt.total as number;
            reprocessAllGameList = targetGames.map((g) => ({
              name: g.name,
              status: 'pending' as const,
            }));
          } else if (t === 'game_start') {
            reprocessAllCurrent = evt.index as number;
            reprocessAllCurrentGame = evt.game as string;
            reprocessAllEmbedding = null;
            reprocessAllGameList = reprocessAllGameList.map((g) =>
              g.name === evt.game ? { ...g, status: 'running' } : g
            );
            reprocessAllLog = [...reprocessAllLog, `▶ [${evt.index}/${evt.total}] ${evt.game}`];
          } else if (t === 'step') {
            reprocessAllLog = [...reprocessAllLog, `  · ${evt.message}`];
          } else if (t === 'embedding_start') {
            reprocessAllEmbedding = { current: 0, total: evt.total as number };
          } else if (t === 'embedding_progress') {
            reprocessAllEmbedding = { current: evt.current as number, total: evt.total as number };
          } else if (t === 'game_done') {
            reprocessAllSuccesses++;
            reprocessAllEmbedding = null;
            reprocessAllGameList = reprocessAllGameList.map((g) =>
              g.name === evt.game ? { ...g, status: 'done' } : g
            );
            reprocessAllLog = [...reprocessAllLog, `  ✅ Terminé`];
          } else if (t === 'game_error') {
            reprocessAllErrors++;
            reprocessAllEmbedding = null;
            reprocessAllGameList = reprocessAllGameList.map((g) =>
              g.name === evt.game ? { ...g, status: 'error', msg: evt.error as string } : g
            );
            reprocessAllLog = [...reprocessAllLog, `  ❌ Erreur: ${evt.error}`];
          } else if (t === 'complete') {
            reprocessAllDone = true;
            reprocessAllLog = [
              ...reprocessAllLog,
              `✅ Terminé — ${evt.success}/${evt.total} jeux retraités en ${((evt.duration as number) / 1000).toFixed(1)}s`,
            ];
          } else if (t === 'error') {
            reprocessAllLog = [...reprocessAllLog, `❌ ${evt.error}`];
            reprocessAllDone = true;
          }
        }
      }
    } catch (e: any) {
      reprocessAllLog = [...reprocessAllLog, `❌ Erreur réseau: ${e.message}`];
    } finally {
      reprocessAllActive = false;
      reprocessAllDone = true;
      reprocessAllCurrentGame = '';
    }
  }

  function handleDeleteClick(gameId: string) {
    confirmDelete = confirmDelete === gameId ? null : gameId;
  }

  function formatDate(dateString: string | null) {
    if (!dateString) return 'Date inconnue';
    return new Date(dateString).toLocaleDateString('fr-FR', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  }

  async function deleteGame(gameId: string) {
    deletingGame = gameId;
    try {
      const res = await fetch(`/api/admin/games/${gameId}`, { method: 'DELETE' });
      if (res.ok) {
        actionMsg = { ok: true, msg: 'Jeu supprimé avec succès' };
        confirmDelete = null;
        await invalidateAll();
      } else {
        const d = await res.json().catch(() => ({}));
        actionMsg = { ok: false, msg: d.error ?? 'Erreur lors de la suppression' };
      }
    } catch {
      actionMsg = { ok: false, msg: 'Erreur réseau' };
    } finally {
      deletingGame = null;
    }
  }

  async function reprocessGame(gameId: string, gameName: string) {
    reprocessingGame = gameId;
    reprocessOneActive = true;
    reprocessOneDone = false;
    reprocessOneGameName = gameName;
    reprocessOneLog = [];
    reprocessOneSteps = [];
    reprocessOneEmbedding = null;
    reprocessOneError = null;
    actionMsg = null;

    try {
      const res = await fetch(`/api/admin/games/${gameId}/reprocess`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });

      if (!res.body) throw new Error('Pas de stream SSE');

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      // Marquer le dernier step comme done quand un nouveau step arrive
      const pushStep = (msg: string) => {
        reprocessOneSteps = reprocessOneSteps.map((s, i) =>
          i === reprocessOneSteps.length - 1 && s.status === 'running'
            ? { ...s, status: 'done' as const }
            : s
        );
        reprocessOneSteps = [...reprocessOneSteps, { msg, status: 'running' }];
        reprocessOneLog = [...reprocessOneLog, `· ${msg}`];
      };

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() ?? '';

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;
          let evt: Record<string, unknown>;
          try {
            evt = JSON.parse(line.slice(6));
          } catch {
            continue;
          }

          const t = evt.type as string;

          if (t === 'job_started') {
            currentJobId = evt.job_id as string;
            jobTrackerActive = true;
          } else if (t === 'step') {
            pushStep(evt.message as string);
          } else if (t === 'embedding_start') {
            reprocessOneEmbedding = { current: 0, total: evt.total as number };
            reprocessOneLog = [...reprocessOneLog, `↳ Embeddings : 0/${evt.total}`];
          } else if (t === 'embedding_progress') {
            reprocessOneEmbedding = { current: evt.current as number, total: evt.total as number };
          } else if (t === 'complete') {
            reprocessOneSteps = reprocessOneSteps.map((s, i) =>
              i === reprocessOneSteps.length - 1 && s.status === 'running'
                ? { ...s, status: 'done' as const }
                : s
            );
            reprocessOneEmbedding = null;
            reprocessOneDone = true;
            reprocessOneLog = [...reprocessOneLog, `✅ Terminé`];
            await invalidateAll();
          } else if (t === 'error') {
            const msg = evt.error as string;
            reprocessOneError = msg;
            reprocessOneSteps = reprocessOneSteps.map((s, i) =>
              i === reprocessOneSteps.length - 1 && s.status === 'running'
                ? { ...s, status: 'error' as const }
                : s
            );
            reprocessOneDone = true;
            reprocessOneLog = [...reprocessOneLog, `❌ ${msg}`];
          }
        }
      }
    } catch (e: any) {
      reprocessOneError = e.message;
      reprocessOneDone = true;
      reprocessOneLog = [...reprocessOneLog, `❌ Erreur réseau: ${e.message}`];
    } finally {
      reprocessingGame = null;
      reprocessOneActive = false;
    }
  }
</script>

<SEO title="Gestion des Jeux - Admin" description="Interface d'administration des jeux" />

<svelte:head>
  <meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="page-header">
  <div>
    <h1>🎮 Gestion des Jeux</h1>
    <p class="summary">{data.games?.length ?? 0} jeu(x) dans la base</p>
  </div>
  <div class="header-actions">
    <div class="selection-controls">
      <button class="btn-link" on:click={selectAllGames} disabled={reprocessAllActive}>Tout</button>
      <span class="sep">·</span>
      <button class="btn-link" on:click={selectNoGames} disabled={reprocessAllActive}>Aucun</button>
      {#if selectedCount > 0}
        <span class="selection-badge">{selectedCount} sélectionné(s)</span>
      {/if}
    </div>
    <button class="btn btn-warning" on:click={reprocessAll} disabled={reprocessAllActive}>
      {#if reprocessAllActive}
        <span class="spinner-small" aria-hidden="true"></span>Recalcul en cours...
      {:else if selectedCount > 0}
        🔄 Recalculer ({selectedCount})
      {:else}
        🔄 Tout recalculer
      {/if}
    </button>
    <a href={resolve('/import')} class="btn btn-primary">+ Importer un jeu</a>
  </div>
</div>

{#if actionMsg?.ok}
  <div class="alert alert-success">✅ {actionMsg.msg}</div>
{/if}
{#if actionMsg && !actionMsg.ok}
  <div class="alert alert-error">❌ {actionMsg.msg}</div>
{/if}

{#if reprocessOneActive || reprocessOneDone}
  <div class="reprocess-panel reprocess-one-panel">
    <div class="reprocess-header">
      <h3>🔄 Recalcul — <em>{reprocessOneGameName}</em></h3>
      {#if reprocessOneDone}
        <button
          class="btn-close"
          on:click={() => {
            reprocessOneDone = false;
            reprocessOneLog = [];
            reprocessOneSteps = [];
          }}>✕</button
        >
      {/if}
    </div>

    {#if reprocessOneSteps.length > 0}
      <div class="one-steps">
        {#each reprocessOneSteps as step (step.msg)}
          <div class="one-step one-step-{step.status}">
            {#if step.status === 'running'}
              <span class="spinner-small"></span>
            {:else if step.status === 'done'}
              ✅
            {:else}
              ❌
            {/if}
            {step.msg}
          </div>
        {/each}
      </div>
    {/if}

    {#if reprocessOneEmbedding && !reprocessOneDone}
      <div class="one-embed">
        Embeddings {reprocessOneEmbedding.current} / {reprocessOneEmbedding.total}
        <div class="reprocess-progress-bar-wrap" style="margin-top:0.4rem">
          <div
            class="reprocess-progress-bar embed-color"
            style="width: {Math.round(
              (reprocessOneEmbedding.current / reprocessOneEmbedding.total) * 100
            )}%"
          ></div>
        </div>
      </div>
    {/if}

    {#if reprocessOneError}
      <div class="one-error">❌ {reprocessOneError}</div>
    {/if}

    {#if reprocessOneDone && !reprocessOneError}
      <div class="one-success">✅ Recalcul terminé avec succès</div>
    {/if}

    <details class="reprocess-log">
      <summary>Journal ({reprocessOneLog.length} entrées)</summary>
      <pre>{reprocessOneLog.join('\n')}</pre>
    </details>
  </div>
{/if}

{#if reprocessAllActive || reprocessAllDone}
  <div class="reprocess-panel">
    <div class="reprocess-header">
      <h3>🔄 Recalcul global</h3>
      {#if reprocessAllDone}
        <button
          class="btn-close"
          on:click={() => {
            reprocessAllDone = false;
            reprocessAllLog = [];
            reprocessAllGameList = [];
          }}>✕</button
        >
      {/if}
    </div>

    {#if reprocessAllTotal > 0}
      <div class="reprocess-progress-bar-wrap">
        <div
          class="reprocess-progress-bar"
          style="width: {Math.round((reprocessAllCurrent / reprocessAllTotal) * 100)}%"
        ></div>
      </div>
      <div class="reprocess-stats">
        <span>{reprocessAllCurrent} / {reprocessAllTotal} jeux</span>
        {#if reprocessAllErrors > 0}<span class="stat-error">{reprocessAllErrors} erreur(s)</span
          >{/if}
        {#if reprocessAllDone && reprocessAllSuccesses > 0}<span class="stat-ok"
            >{reprocessAllSuccesses} succès</span
          >{/if}
      </div>
    {/if}

    {#if reprocessAllCurrentGame && !reprocessAllDone}
      <div class="reprocess-current">
        Traitement : <strong>{reprocessAllCurrentGame}</strong>
        {#if reprocessAllEmbedding}
          — embeddings {reprocessAllEmbedding.current}/{reprocessAllEmbedding.total}
          <div class="embed-bar-wrap">
            <div
              class="embed-bar"
              style="width: {Math.round(
                (reprocessAllEmbedding.current / reprocessAllEmbedding.total) * 100
              )}%"
            ></div>
          </div>
        {/if}
      </div>
    {/if}

    {#if reprocessAllGameList.length > 0}
      <div class="game-status-list">
        {#each reprocessAllGameList as gp (gp.name)}
          <div class="game-status-row game-status-{gp.status}">
            {#if gp.status === 'pending'}⏳
            {:else if gp.status === 'running'}<span class="spinner-small"></span>
            {:else if gp.status === 'done'}✅
            {:else}❌
            {/if}
            {gp.name}
            {#if gp.msg}<span class="gp-error"> — {gp.msg}</span>{/if}
          </div>
        {/each}
      </div>
    {/if}

    <details class="reprocess-log">
      <summary>Journal détaillé ({reprocessAllLog.length} entrées)</summary>
      <pre>{reprocessAllLog.join('\n')}</pre>
    </details>
  </div>
{/if}

<div class="games-grid">
  {#each data.games as game (game.id)}
    <div class="game-card" class:selected={selectedGameIDs.has(game.id)}>
      <div class="game-header">
        <label class="game-select-label" title="Sélectionner pour le recalcul">
          <input
            type="checkbox"
            checked={selectedGameIDs.has(game.id)}
            on:change={() => toggleGameSelection(game.id)}
          />
        </label>
        <h2>{game.name}</h2>
        <span class="badge">{game.sections_count} sections</span>
      </div>

      <div class="game-info">
        <div class="info-row">
          <span class="label">ID:</span>
          <code class="value">{game.id}</code>
        </div>
        <div class="info-row">
          <span class="label">Date d'ajout:</span>
          <span class="value">{formatDate(game.added_at)}</span>
        </div>
        {#if game.updated_at && game.updated_at !== game.added_at}
          <div class="info-row">
            <span class="label">Mis à jour:</span>
            <span class="value">{formatDate(game.updated_at)}</span>
          </div>
        {/if}
      </div>

      <div class="game-actions">
        <button
          class="btn btn-secondary-outline"
          on:click={() => reprocessGame(game.id, game.name)}
          disabled={reprocessingGame === game.id}
          title="Recalculer les embeddings avec le modèle actuel"
        >
          {#if reprocessingGame === game.id}
            <span class="spinner-small" aria-hidden="true"></span>Recalcul...
          {:else}
            🔄 Recalculer
          {/if}
        </button>

        {#if confirmDelete === game.id}
          <button
            class="btn btn-danger"
            disabled={deletingGame === game.id}
            on:click={() => deleteGame(game.id)}
          >
            {deletingGame === game.id ? 'Suppression...' : 'Confirmer'}
          </button>
          <button class="btn btn-secondary" on:click={() => (confirmDelete = null)}>Annuler</button>
        {:else}
          <button class="btn btn-danger-outline" on:click={() => handleDeleteClick(game.id)}>
            🗑️ Supprimer
          </button>
        {/if}
      </div>
    </div>
  {/each}

  {#if data.games.length === 0}
    <div class="empty-state">
      <p>Aucun jeu dans la base de données.</p>
      <a href={resolve('/import')} class="btn btn-primary">Importer des règles</a>
    </div>
  {/if}
</div>

<!-- Job Tracker pour reprendre le suivi après déconnexion -->
<JobTracker bind:jobId={currentJobId} bind:active={jobTrackerActive} />

<style>
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: start;
    margin-bottom: 2rem;
    gap: 1rem;
  }

  .header-actions {
    display: flex;
    gap: 0.75rem;
    align-items: center;
  }

  h1 {
    font-size: 2rem;
    margin: 0 0 0.5rem 0;
    color: #1a1a1a;
  }

  .summary {
    color: #666;
    font-size: 1rem;
  }

  .alert {
    padding: 1rem;
    margin-bottom: 1.5rem;
    border-radius: 8px;
    font-weight: 500;
  }

  .alert-success {
    background-color: #d4edda;
    color: #155724;
    border: 1px solid #c3e6cb;
  }

  .alert-error {
    background-color: #f8d7da;
    color: #721c24;
    border: 1px solid #f5c6cb;
  }

  .games-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
    gap: 1.5rem;
  }

  .game-card {
    background: white;
    border: 1px solid #e0e0e0;
    border-radius: 12px;
    padding: 1.5rem;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
    transition: box-shadow 0.2s;
  }

  .game-card:hover {
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  }

  .game-card.selected {
    border-color: #4f46e5;
    box-shadow: 0 0 0 2px rgba(79, 70, 229, 0.25);
  }

  .game-select-label {
    display: flex;
    align-items: center;
    cursor: pointer;
    flex-shrink: 0;
  }

  .game-select-label input[type='checkbox'] {
    width: 1.1rem;
    height: 1.1rem;
    cursor: pointer;
    accent-color: #4f46e5;
  }

  .selection-controls {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.875rem;
    color: #555;
  }

  .btn-link {
    background: none;
    border: none;
    color: #4f46e5;
    cursor: pointer;
    padding: 0;
    font-size: 0.875rem;
    text-decoration: underline;
  }

  .btn-link:hover {
    color: #3730a3;
  }

  .btn-link:disabled {
    color: #aaa;
    cursor: not-allowed;
    text-decoration: none;
  }

  .sep {
    color: #ccc;
  }

  .selection-badge {
    background: #4f46e5;
    color: white;
    padding: 0.1rem 0.5rem;
    border-radius: 12px;
    font-size: 0.75rem;
    font-weight: 600;
    margin-left: 0.25rem;
  }

  .game-header {
    display: flex;
    justify-content: space-between;
    align-items: start;
    margin-bottom: 1rem;
    gap: 1rem;
  }

  .game-header h2 {
    font-size: 1.25rem;
    color: #1a1a1a;
    margin: 0;
    flex: 1;
  }

  .badge {
    background: #4f46e5;
    color: white;
    padding: 0.25rem 0.75rem;
    border-radius: 12px;
    font-size: 0.875rem;
    font-weight: 600;
    white-space: nowrap;
  }

  .game-info {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-bottom: 1.5rem;
  }

  .info-row {
    display: flex;
    gap: 0.5rem;
    font-size: 0.9rem;
  }

  .info-row.mecaniques {
    flex-direction: column;
  }

  .label {
    font-weight: 600;
    color: #555;
    min-width: 90px;
  }

  .value {
    color: #333;
    flex: 1;
  }

  .file-name {
    word-break: break-all;
  }

  .mecaniques-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .mecanique-tag {
    background: #f0f0f0;
    color: #444;
    padding: 0.25rem 0.625rem;
    border-radius: 6px;
    font-size: 0.8rem;
  }

  .mecanique-tag.more {
    background: #e0e0e0;
    font-weight: 600;
  }

  .game-actions {
    padding-top: 1rem;
    border-top: 1px solid #f0f0f0;
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .btn {
    padding: 0.625rem 1.25rem;
    border-radius: 8px;
    border: none;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
    text-decoration: none;
    display: inline-block;
  }

  .btn-primary {
    background: #4f46e5;
    color: white;
  }

  .btn-primary:hover {
    background: #4338ca;
  }

  .btn-warning {
    background: #d97706;
    color: white;
  }

  .btn-warning:hover:not(:disabled) {
    background: #b45309;
  }

  .btn-warning:disabled {
    background: #fcd34d;
    color: #78350f;
    cursor: not-allowed;
  }

  .btn-danger {
    background: #dc2626;
    color: white;
  }

  .btn-danger:hover:not(:disabled) {
    background: #b91c1c;
  }

  .btn-danger:disabled {
    background: #fca5a5;
    cursor: not-allowed;
  }

  .btn-danger-outline {
    background: transparent;
    color: #dc2626;
    border: 2px solid #dc2626;
  }

  .btn-danger-outline:hover {
    background: #dc2626;
    color: white;
  }

  .btn-secondary {
    background: #6b7280;
    color: white;
  }

  .btn-secondary:hover:not(:disabled) {
    background: #4b5563;
  }

  .btn-secondary:disabled {
    background: #d1d5db;
    cursor: not-allowed;
    opacity: 0.7;
  }

  .btn-secondary-outline {
    background: transparent;
    color: #6b7280;
    border: 2px solid #d1d5db;
  }

  .btn-secondary-outline:hover:not(:disabled) {
    background: #f3f4f6;
    border-color: #6b7280;
  }

  .btn-secondary-outline:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .spinner-small {
    display: inline-block;
    width: 1em;
    height: 1em;
    border: 2px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
    vertical-align: middle;
    margin-right: 0.5rem;
  }

  .spinner-small {
    width: 0.875em;
    height: 0.875em;
    border-width: 2px;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .empty-state {
    grid-column: 1 / -1;
    text-align: center;
    padding: 4rem 2rem;
    background: white;
    border-radius: 12px;
    border: 2px dashed #d1d5db;
  }

  .empty-state p {
    font-size: 1.125rem;
    color: #6b7280;
    margin-bottom: 1.5rem;
  }

  /* ── Panneau Reprocess All ─────────────────────────────────────────── */
  .reprocess-panel {
    background: #1e1e2e;
    color: #cdd6f4;
    border-radius: 12px;
    padding: 1.5rem;
    margin-bottom: 2rem;
    font-family: monospace;
    font-size: 0.9rem;
  }

  .reprocess-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1rem;
  }

  .reprocess-header h3 {
    margin: 0;
    color: #cba6f7;
    font-size: 1rem;
    font-family: inherit;
  }

  .btn-close {
    background: transparent;
    border: none;
    color: #6c7086;
    font-size: 1.25rem;
    cursor: pointer;
    padding: 0 0.25rem;
    line-height: 1;
  }

  .btn-close:hover {
    color: #cdd6f4;
  }

  .reprocess-progress-bar-wrap {
    background: #313244;
    border-radius: 6px;
    height: 8px;
    margin-bottom: 0.5rem;
    overflow: hidden;
  }

  .reprocess-progress-bar {
    height: 100%;
    background: #a6e3a1;
    border-radius: 6px;
    transition: width 0.4s ease;
  }

  .reprocess-stats {
    display: flex;
    gap: 1rem;
    font-size: 0.8rem;
    color: #9399b2;
    margin-bottom: 1rem;
  }

  .stat-error {
    color: #f38ba8;
  }
  .stat-ok {
    color: #a6e3a1;
  }

  .reprocess-current {
    color: #89dceb;
    margin-bottom: 1rem;
    font-size: 0.875rem;
  }

  .embed-bar-wrap {
    display: inline-block;
    width: 120px;
    height: 4px;
    background: #313244;
    border-radius: 4px;
    vertical-align: middle;
    margin-left: 0.5rem;
    overflow: hidden;
  }

  .embed-bar {
    height: 100%;
    background: #89b4fa;
    border-radius: 4px;
    transition: width 0.2s;
  }

  .game-status-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    margin-bottom: 1rem;
    max-height: 200px;
    overflow-y: auto;
  }

  .game-status-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
    padding: 0.2rem 0.4rem;
    border-radius: 4px;
  }

  .game-status-running {
    background: #313244;
    color: #cba6f7;
  }
  .game-status-done {
    color: #a6e3a1;
  }
  .game-status-error {
    color: #f38ba8;
  }
  .game-status-pending {
    color: #6c7086;
  }

  .gp-error {
    font-size: 0.8rem;
    color: #f38ba8;
  }

  .reprocess-log {
    margin-top: 0.75rem;
  }

  .reprocess-log summary {
    cursor: pointer;
    color: #9399b2;
    font-size: 0.8rem;
    user-select: none;
  }

  /* ── Panneau Reprocess One ─────────────────────────────────────────── */
  .reprocess-one-panel {
    border-left: 4px solid #89b4fa;
  }

  .one-steps {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    margin-bottom: 0.75rem;
  }

  .one-step {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
    padding: 0.2rem 0.4rem;
    border-radius: 4px;
  }

  .one-step-running {
    color: #cba6f7;
    background: #313244;
  }

  .one-step-done {
    color: #a6e3a1;
  }

  .one-step-error {
    color: #f38ba8;
  }

  .one-embed {
    color: #89dceb;
    font-size: 0.85rem;
    margin-bottom: 0.75rem;
  }

  .embed-color {
    background: #89b4fa;
  }

  .one-error {
    color: #f38ba8;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }

  .one-success {
    color: #a6e3a1;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }

  .reprocess-log pre {
    margin-top: 0.5rem;
    background: #181825;
    border-radius: 6px;
    padding: 0.75rem;
    font-size: 0.78rem;
    max-height: 250px;
    overflow-y: auto;
    white-space: pre-wrap;
    color: #a6adc8;
  }
</style>
