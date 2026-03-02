<script lang="ts">
  import type { PageData } from './$types';
  import SEO from '$lib/SEO.svelte';
  import Markdown from '$lib/Markdown.svelte';

  export let data: PageData;

  // Résultat de la dernière requête envoyée au Go backend
  type SectionResult = {
    title: string;
    section_type: string;
    summary: string;
    text: string;
    score: number;
    page_start: number | null;
  };
  type FormResult =
    | {
        ok: true;
        jeu: string;
        jeu_id: string;
        answer: string;
        used_llm: boolean;
        model: string;
        sections: SectionResult[];
        cached: boolean;
      }
    | { ok: false; error: string }
    | null;

  let form: FormResult = null;
  let isLoading = false;
  let selectedGame = '';
  let selectedGameLabel = '';
  let lastQuestion = '';
  let formEl: HTMLFormElement;
  let questionText = '';
  let gameSearch = '';
  let showGameDropdown = false;
  let gameInputEl: HTMLInputElement;

  const MAX_QUESTION_LENGTH = 500;

  $: filteredGames = data.games.filter((g: any) =>
    (g.name ?? '').toLowerCase().includes(gameSearch.toLowerCase())
  );

  function selectGame(name: string) {
    selectedGame = name;
    selectedGameLabel = name;
    gameSearch = name;
    showGameDropdown = false;
  }

  function clearGame() {
    selectedGame = '';
    selectedGameLabel = '';
    gameSearch = '';
    showGameDropdown = false;
  }

  function handleGameInputFocus() {
    gameSearch = '';
    showGameDropdown = true;
  }

  function handleGameInputBlur() {
    // Délai pour laisser le clic sur un item se déclencher
    setTimeout(() => {
      showGameDropdown = false;
      // Restaure le label si un jeu est sélectionné
      gameSearch = selectedGame;
    }, 150);
  }

  const suggestedQuestions = [
    'Comment jouer ?',
    'Comment gagner ?',
    'Comment se déroule un tour ?',
    'Comment se déroule un combat ?',
    'Quelle est la mise en place ?',
    'Quelles sont les actions disponibles ?',
  ];

  function handleTextareaKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      formEl.requestSubmit();
    }
  }

  function fillQuestion(question: string) {
    questionText = question;
    const textarea = formEl.querySelector('textarea[name="question"]') as HTMLTextAreaElement;
    if (textarea) {
      textarea.focus();
    }
  }

  $: charCount = questionText.length;
  $: isNearLimit = charCount > MAX_QUESTION_LENGTH * 0.8;
  $: isAtLimit = charCount >= MAX_QUESTION_LENGTH;

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    const textarea = formEl.querySelector('textarea[name="question"]') as HTMLTextAreaElement;
    const question = textarea?.value?.trim() ?? '';
    if (!question) return;

    isLoading = true;
    lastQuestion = question;
    form = null;

    try {
      const res = await fetch('/api/ask', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ question, jeu: selectedGame }),
      });
      const data = await res.json();
      if (!res.ok) {
        form = { ok: false, error: data.error ?? 'Erreur serveur' };
      } else {
        form = data as FormResult;
      }
    } catch (err) {
      form = { ok: false, error: 'Erreur réseau — vérifiez votre connexion.' };
    } finally {
      isLoading = false;
    }
  }
</script>

<SEO
  title="Reglomatic — Assistant IA pour règles de jeux de société"
  description="Posez des questions sur vos jeux de société préférés et obtenez des réponses instantanées grâce à notre IA. Recherche intelligente dans les règles de vos jeux."
  keywords="jeux de société, règles de jeu, IA, assistant intelligent, board games, questions réponses, recherche règles, aide jeu de société"
/>

<div class="page">
  <!-- En-tête -->
  <header class="header">
    <h1 class="logo">Reglomatic</h1>
    <p class="tagline">Posez une question sur vos règles de jeu de société</p>
  </header>

  <!-- Formulaire -->
  <form bind:this={formEl} class="ask-form" on:submit|preventDefault={handleSubmit}>
    <div class="suggested-tags">
      {#each suggestedQuestions as question}
        <button
          type="button"
          class="tag-btn"
          on:click={() => fillQuestion(question)}
          disabled={isLoading}
        >
          {question}
        </button>
      {/each}
    </div>

    <div class="question-wrapper">
      <textarea
        name="question"
        class="question-input"
        placeholder="Ex : Comment se déroule un combat ? Combien de joueurs ?"
        required
        rows="3"
        maxlength={MAX_QUESTION_LENGTH}
        disabled={isLoading}
        bind:value={questionText}
        on:keydown={handleTextareaKeydown}
      ></textarea>
      <div class="char-counter" class:warning={isNearLimit} class:danger={isAtLimit}>
        {charCount} / {MAX_QUESTION_LENGTH}
      </div>
    </div>

    <div class="form-footer">
      {#if data.games.length > 1}
        <div class="game-search-wrapper">
          <input
            bind:this={gameInputEl}
            type="text"
            class="game-search-input"
            placeholder="Tous les jeux (auto)"
            autocomplete="off"
            disabled={isLoading}
            bind:value={gameSearch}
            on:focus={handleGameInputFocus}
            on:blur={handleGameInputBlur}
          />
          {#if selectedGame}
            <button type="button" class="game-clear-btn" on:click={clearGame} tabindex="-1">
              ×
            </button>
          {:else}
            <span class="game-search-icon">🔍</span>
          {/if}
          <!-- Champ caché pour la soumission -->
          <input type="hidden" name="jeu" value={selectedGame} />
          {#if showGameDropdown}
            <ul class="game-dropdown">
              {#if filteredGames.length === 0}
                <li class="game-dropdown-empty">Aucun jeu trouvé</li>
              {:else}
                {#each filteredGames as g}
                  <li>
                    <button
                      type="button"
                      class="game-dropdown-item{selectedGame === g.name ? ' selected' : ''}"
                      on:mousedown|preventDefault={() => selectGame(g.name)}
                    >
                      🎲 {g.name}
                    </button>
                  </li>
                {/each}
              {/if}
            </ul>
          {/if}
        </div>
      {:else if data.games.length === 1}
        <span class="game-label">🎲 {data.games[0].name}</span>
      {:else}
        <span class="game-label empty">Aucun jeu indexé</span>
      {/if}

      <button
        type="submit"
        class="submit-btn{isLoading ? ' loading' : ''}"
        disabled={isLoading || data.games.length === 0}
      >
        {#if isLoading}
          <span class="spinner" aria-hidden="true"></span>Recherche…
        {:else}
          Poser la question →
        {/if}
      </button>
    </div>
  </form>

  <!-- Résultats -->
  {#if form}
    <section class="result-section" aria-live="polite">
      {#if !form.ok}
        <div class="error-card" role="alert">
          <span class="error-icon">⚠</span>
          <div>
            <div class="error-message">{form.error}</div>
          </div>
        </div>
      {:else}
        <!-- Question posée -->
        {#if lastQuestion}
          <p class="question-reminder">« {lastQuestion} »</p>
        {/if}

        <!-- Jeu sélectionné -->
        <div class="game-badge">
          <span class="game-icon">🎲</span>
          <span>{form.jeu}</span>
        </div>

        <!-- Lien(s) de téléchargement du fichier source -->
        {#if (form as any).file_path}
          {@const filePaths = ((form as any).file_path as string)
            .split(' + ')
            .map((p: string) => p.trim())}
          <div class="file-download">
            {#each filePaths as filePath, index}
              <a href="/files/{filePath}" class="file-download-link" target="_blank" rel="noopener">
                <span class="file-icon">📄</span>
                <span class="file-text">
                  <span class="file-label">
                    {filePaths.length > 1 ? `Fichier source ${index + 1}` : 'Fichier source'}
                  </span>
                  <span class="file-name">
                    {filePath.split('/').pop()?.replace(/^\d+_/, '') || 'Télécharger'}
                  </span>
                </span>
                <span class="file-arrow">↓</span>
              </a>
            {/each}
          </div>
        {/if}

        <!-- Réponse LLM -->
        <div class="answer-card">
          {#if form.used_llm}
            <div class="answer-header">
              Réponse
              <span class="model-tag">{form.model}</span>
            </div>
            <div class="answer-text">
              <Markdown content={form.answer} />
            </div>
          {:else}
            <p class="no-llm-notice">
              Aucun LLM configuré. Ajoutez <code>MISTRAL_API_KEY</code>,
              <code>OPENAI_API_KEY</code> ou <code>OLLAMA_MODEL</code> dans
              <code>.env</code>.
            </p>
          {/if}
        </div>

        <!-- Sections source -->
        <details class="sources-details">
          <summary class="sources-summary">
            {form.sections.length} section{form.sections.length > 1 ? 's' : ''} source
          </summary>
          <div class="sources-list">
            {#each form.sections as s}
              <div class="source-card">
                <div class="source-header">
                  <span class="source-title">{s.title}</span>
                  <div class="source-meta">
                    {#if s.page_start}
                      <span class="source-page">p.{s.page_start}</span>
                    {/if}
                    <span class="source-score">{(s.score * 100).toFixed(0)}%</span>
                  </div>
                </div>
                <p class="source-text">
                  {s.summary || s.text.slice(0, 220) + '…'}
                </p>
              </div>
            {/each}
          </div>
        </details>
      {/if}
    </section>
  {/if}

  <!-- Footer -->
  <footer class="footer">
    {#if data.games.length > 0}
      <span
        >{data.games.length} jeu{data.games.length > 1 ? 'x' : ''} indexé{data.games.length > 1
          ? 's'
          : ''}</span
      >
    {/if}
    <span class="version">v{__APP_VERSION__}</span>
  </footer>
</div>

<style>
  .suggested-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }

  .tag-btn {
    background: rgba(255, 255, 255, 0.08);
    border: 1px solid rgba(255, 255, 255, 0.15);
    border-radius: 1rem;
    padding: 0.4rem 0.9rem;
    font-size: 0.875rem;
    color: rgba(255, 255, 255, 0.75);
    cursor: pointer;
    transition: all 0.2s ease;
    font-family: inherit;
    white-space: nowrap;
  }

  .tag-btn:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.12);
    border-color: rgba(255, 255, 255, 0.3);
    color: rgba(255, 255, 255, 0.95);
    transform: translateY(-1px);
  }

  .tag-btn:active:not(:disabled) {
    transform: translateY(0);
  }

  .tag-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .question-wrapper {
    position: relative;
    width: 100%;
  }

  .char-counter {
    position: absolute;
    bottom: 0.5rem;
    right: 0.75rem;
    font-size: 0.75rem;
    color: rgba(255, 255, 255, 0.4);
    background: rgba(0, 0, 0, 0.3);
    padding: 0.25rem 0.5rem;
    border-radius: 0.375rem;
    backdrop-filter: blur(4px);
    pointer-events: none;
    transition: color 0.3s ease;
  }

  .char-counter.warning {
    color: rgba(255, 200, 100, 0.8);
  }

  .char-counter.danger {
    color: rgba(255, 100, 100, 0.9);
    font-weight: 600;
  }

  /* ── Searchable game selector ─────────────────────────── */

  .game-search-wrapper {
    position: relative;
    flex: 1;
    min-width: 0;
  }

  .game-search-input {
    width: 100%;
    padding: 0.6rem 2.4rem 0.6rem 0.9rem;
    background: rgba(255, 255, 255, 0.08);
    border: 1px solid rgba(255, 255, 255, 0.18);
    border-radius: 0.5rem;
    color: #fff;
    font-size: 0.95rem;
    font-family: inherit;
    outline: none;
    transition: border-color 0.2s;
    box-sizing: border-box;
  }

  .game-search-input::placeholder {
    color: rgba(255, 255, 255, 0.4);
  }

  .game-search-input:focus {
    border-color: rgba(255, 255, 255, 0.5);
    background: rgba(255, 255, 255, 0.11);
  }

  .game-search-input:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .game-search-icon {
    position: absolute;
    right: 0.7rem;
    top: 50%;
    transform: translateY(-50%);
    font-size: 0.9rem;
    pointer-events: none;
    opacity: 0.5;
  }

  .game-clear-btn {
    position: absolute;
    right: 0.5rem;
    top: 50%;
    transform: translateY(-50%);
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.55);
    font-size: 1.2rem;
    line-height: 1;
    cursor: pointer;
    padding: 0.1rem 0.3rem;
    border-radius: 0.25rem;
    transition: color 0.15s;
  }

  .game-clear-btn:hover {
    color: rgba(255, 255, 255, 0.9);
  }

  .game-dropdown {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    background: #1e1e2e;
    border: 1px solid rgba(255, 255, 255, 0.18);
    border-radius: 0.5rem;
    max-height: 260px;
    overflow-y: auto;
    z-index: 100;
    list-style: none;
    margin: 0;
    padding: 0.25rem 0;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
  }

  .game-dropdown li {
    margin: 0;
    padding: 0;
  }

  .game-dropdown-item {
    display: block;
    width: 100%;
    text-align: left;
    background: none;
    border: none;
    padding: 0.55rem 0.9rem;
    color: rgba(255, 255, 255, 0.8);
    font-size: 0.92rem;
    font-family: inherit;
    cursor: pointer;
    transition:
      background 0.15s,
      color 0.15s;
  }

  .game-dropdown-item:hover,
  .game-dropdown-item.selected {
    background: rgba(255, 255, 255, 0.1);
    color: #fff;
  }

  .game-dropdown-item.selected {
    font-weight: 600;
  }

  .game-dropdown-empty {
    padding: 0.6rem 0.9rem;
    color: rgba(255, 255, 255, 0.4);
    font-size: 0.88rem;
    font-style: italic;
  }
</style>
