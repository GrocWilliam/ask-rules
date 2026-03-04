<script lang="ts">
  import type { PageData } from './$types';
  import SEO from '$lib/SEO.svelte';

  export let data: PageData;

  // ── État du formulaire ──────────────────────────────────────────────────────
  let isLoading = false;
  let selectedGame = '';
  let gameNameValue = '';
  let importMode: 'file' | 'url' = 'file';

  function onGameSelect(e: Event) {
    const select = e.target as HTMLSelectElement;
    selectedGame = select.value;
    if (selectedGame) {
      const game = data.games.find((g: any) => g.id === selectedGame);
      if (game) gameNameValue = game.name;
    }
  }

  // ── État de la progression ──────────────────────────────────────────────────
  type Step = { message: string };
  type EmbeddingState = { current: number; total: number };
  type SuccessResult = {
    ok: true;
    jeu: string;
    sections: number;
    action: string;
    mecaniques: string[];
  };
  type ErrorResult = { ok: false; error: string };

  let steps: Step[] = [];
  let embedding: EmbeddingState | null = null;
  let result: SuccessResult | ErrorResult | null = null;

  // État intermédiaire pour accumuler les infos du pipeline
  let totalChunks = 0;

  // ── Soumission avec affichage en temps réel ─────────────────────────────────
  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    isLoading = true;
    steps = [];
    embedding = null;
    result = null;
    totalChunks = 0;

    const form = e.currentTarget as HTMLFormElement;
    const formData = new FormData(form);

    try {
      const response = await fetch('/api/import', {
        method: 'POST',
        body: formData,
      });

      if (!response.body) throw new Error(`Erreur serveur (${response.status})`);

      const reader = response.body.getReader();
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

          // Événements émis par le handler Go
          if (evt.type === 'ping') {
            // Heartbeat pour garder la connexion SSE ouverte - ignorer silencieusement
            continue;
          } else if (evt.type === 'start') {
            steps = [...steps, { message: `Import de « ${evt.game} » — ${evt.files} fichier(s)` }];
          } else if (evt.type === 'replacing') {
            steps = [
              ...steps,
              { message: `🗑 Sections existantes supprimées pour « ${evt.game} »` },
            ];
          } else if (evt.type === 'uploading') {
            steps = [...steps, { message: `Envoi du fichier : ${evt.file}` }];
          } else if (evt.type === 'extracting') {
            steps = [...steps, { message: `Extraction du texte : ${evt.file}` }];
          } else if (evt.type === 'chunking') {
            steps = [...steps, { message: `Découpage en sections…` }];
          } else if (evt.type === 'embedding') {
            // début des embeddings
            embedding = { current: 0, total: evt.chunks as number };
          } else if (evt.type === 'progress') {
            embedding = { current: evt.done as number, total: evt.total as number };
          } else if (evt.type === 'gameplay') {
            if ((evt as any).status === 'done') {
              steps = [
                ...steps,
                {
                  message: `Gameplay extrait — ${evt.mechanics ?? 0} mécaniques, ${evt.phases ?? 0} phases`,
                },
              ];
            }
          } else if (evt.type === 'done') {
            // done du pipeline (fin d'un fichier)
            totalChunks = evt.total_chunks as number;
          } else if (evt.type === 'complete') {
            // complete final de ImportSSE
            embedding = null;
            result = {
              ok: true,
              jeu: evt.game as string,
              sections: totalChunks,
              action: 'indexé',
              mecaniques: [],
            };
          } else if (evt.type === 'file_error' || evt.type === 'section_error') {
            steps = [...steps, { message: `⚠ ${evt.error}` }];
          } else if (evt.type === 'error') {
            console.log(evt);
            result = { ok: false, error: (evt.error ?? evt.message) as string };
          }
        }
      }
    } catch (err) {
      result = {
        ok: false,
        error: err instanceof Error ? err.message : String(err),
      };
    }

    isLoading = false;
  }
</script>

<SEO
  title="Importer des règles - Reglomatic"
  description="Importez et indexez vos règles de jeux de société au format PDF ou TXT. Notre IA analyse automatiquement les règles pour répondre à vos questions."
  keywords="importer règles, PDF jeu de société, indexation règles, upload règles jeu, analyse règles PDF"
/>

<div class="page">
  <!-- En-tête -->
  <header class="header">
    <h1 class="logo">Importer des règles</h1>
    <p class="tagline">Ajoutez un fichier .txt ou .pdf à la base de connaissance</p>
  </header>

  <!-- Formulaire -->
  <form method="POST" enctype="multipart/form-data" class="import-form" on:submit={handleSubmit}>
    <!-- Nom du jeu -->
    <div class="form-group">
      <label class="form-label" for="gameName">Nom du jeu <span class="required">*</span></label>
      <input
        id="gameName"
        name="gameName"
        type="text"
        class="text-input"
        placeholder="Ex : Catan, 7 Wonders, Terraforming Mars…"
        bind:value={gameNameValue}
        disabled={isLoading}
      />
    </div>

    <!-- Jeu existant (optionnel) -->
    {#if data.games.length > 0}
      <div class="form-group">
        <label class="form-label" for="existingGame">
          Mettre à jour un jeu existant
          <span class="form-hint">optionnel — pré-remplit le nom</span>
        </label>
        <select id="existingGame" class="game-select" disabled={isLoading} on:change={onGameSelect}>
          <option value="">— Nouveau jeu —</option>
          {#each data.games as g}
            <option value={g.id}>{(g as any).name}</option>
          {/each}
        </select>
      </div>

      <!-- Mode : remplacer ou fusionner -->
      {#if selectedGame}
        <div class="form-group">
          <span class="form-label">Mode d'import</span>
          <div class="radio-group">
            <label class="radio-label">
              <input type="radio" name="mode" value="replace" checked disabled={isLoading} />
              <span class="radio-text">
                <strong>Remplacer</strong>
                <span class="radio-hint">Efface les sections existantes et les remplace</span>
              </span>
            </label>
            <label class="radio-label">
              <input type="radio" name="mode" value="merge" disabled={isLoading} />
              <span class="radio-text">
                <strong>Fusionner</strong>
                <span class="radio-hint"
                  >Ajoute les nouvelles sections sans toucher aux existantes</span
                >
              </span>
            </label>
          </div>
        </div>
      {:else}
        <input type="hidden" name="mode" value="replace" />
      {/if}
    {:else}
      <input type="hidden" name="mode" value="replace" />
    {/if}

    <!-- Source : onglets Fichier / URL -->
    <div class="form-group">
      <span class="form-label">Source <span class="required">*</span></span>

      <div class="import-tabs" role="group" aria-label="Mode d'import">
        <button
          type="button"
          class="import-tab{importMode === 'file' ? ' active' : ''}"
          disabled={isLoading}
          on:click={() => (importMode = 'file')}>Fichier</button
        >
        <button
          type="button"
          class="import-tab{importMode === 'url' ? ' active' : ''}"
          disabled={isLoading}
          on:click={() => (importMode = 'url')}>URL</button
        >
      </div>

      <input type="hidden" name="importMode" value={importMode} />

      {#if importMode === 'file'}
        <div class="file-input-wrapper">
          <input
            id="fichier"
            name="fichier"
            type="file"
            accept=".txt,.pdf"
            disabled={isLoading}
            class="file-input"
          />
          <span class="file-hint">.txt ou .pdf</span>
        </div>
      {:else}
        <input
          id="url"
          name="url"
          type="url"
          class="text-input"
          placeholder="https://exemple.com/regles.pdf  ou  https://exemple.com/faq"
          disabled={isLoading}
        />
        <span class="form-hint">PDF, page HTML (FAQ) ou fichier texte brut</span>
      {/if}
    </div>

    <!-- Bouton -->
    <div class="form-actions">
      <button type="submit" class="submit-btn{isLoading ? ' loading' : ''}" disabled={isLoading}>
        {#if isLoading}
          <span class="spinner" aria-hidden="true"></span>Indexation en cours…
        {:else}
          Importer →
        {/if}
      </button>
    </div>
  </form>

  <!-- Progression + résultat -->
  {#if isLoading || result || steps.length > 0}
    <section class="result-section" aria-live="polite">
      <!-- Étapes terminées -->
      {#if steps.length > 0}
        <div class="progress-steps">
          {#each steps as step}
            <div class="progress-step">
              <span class="progress-step-icon" aria-hidden="true">✔</span>
              <span>{step.message}</span>
            </div>
          {/each}
        </div>
      {/if}

      <!-- Barre de progression des embeddings -->
      {#if embedding}
        <div class="progress-bar-wrapper">
          <div class="progress-bar-label">
            <span>Génération des embeddings</span>
            <span class="progress-bar-count">{embedding.current} / {embedding.total}</span>
          </div>
          <div
            class="progress-bar-track"
            role="progressbar"
            aria-valuenow={embedding.current}
            aria-valuemax={embedding.total}
          >
            <div
              class="progress-bar-fill"
              style="width: {embedding.total > 0
                ? (embedding.current / embedding.total) * 100
                : 0}%"
            ></div>
          </div>
        </div>
      {/if}

      <!-- Résultat final -->
      {#if result}
        {#if !result.ok}
          <div class="error-card" role="alert">
            <span class="error-icon">⚠</span>
            {result.error}
          </div>
        {:else}
          <div class="success-card">
            <div class="success-header">
              <span class="success-icon">✔</span>
              <span>Indexation terminée</span>
            </div>
            <div class="success-body">
              <div class="success-row">
                <span class="success-key">Jeu</span>
                <span class="success-val">{result.jeu}</span>
              </div>
              <div class="success-row">
                <span class="success-key">Sections</span>
                <span class="success-val"
                  >{result.sections} section{result.sections > 1 ? 's' : ''} indexée{result.sections >
                  1
                    ? 's'
                    : ''}</span
                >
              </div>
              <div class="success-row">
                <span class="success-key">Action</span>
                <span class="success-val success-action">{result.action}</span>
              </div>
              {#if result.mecaniques && result.mecaniques.length > 0}
                <div class="success-row">
                  <span class="success-key">Mécaniques</span>
                  <span class="success-val">{result.mecaniques.join(', ')}</span>
                </div>
              {/if}
            </div>
          </div>
        {/if}
      {/if}
    </section>
  {/if}

  <!-- Footer -->
  <footer class="footer">
    {#if data.games.length > 0}
      <span
        >{data.games.length} jeu{data.games.length > 1 ? 'x' : ''} indexé{data.games.length > 1
          ? 's'
          : ''} dans la base</span
      >
    {:else}
      <span>Base vide — importez votre premier jeu</span>
    {/if}
  </footer>
</div>
