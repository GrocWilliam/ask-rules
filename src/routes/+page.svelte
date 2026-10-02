<script lang="ts">
  import type { PageData } from './$types';
  import SEO from '$lib/SEO.svelte';
  import Markdown from '$lib/Markdown.svelte';
  import type { Game } from '../types/game.type';
  import { onDestroy, tick } from 'svelte';
  import { gamesProgress, loadGames } from '$lib/gamesLoader';
  import { askStream } from '$lib/askStream';
  import { fileHref, linkCitations } from '$lib/citations';
  import { detectGame } from '$lib/gameMatch';
  import { keepScreenOn } from '$lib/wakeLock';
  import VoiceInput from '$lib/VoiceInput.svelte';
  import {
    conversations,
    addExchange,
    deleteConversation,
    clearConversations,
    contextTurns,
    type Exchange,
    type SectionResult,
  } from '$lib/chatHistory';

  export let data: PageData;

  function pageHref(s: SectionResult): string {
    return fileHref(s.file ?? '', s.page_num ?? 1);
  }

  function pageLabel(s: SectionResult): string {
    return s.page_end && s.page_end > (s.page_num ?? 0)
      ? `p.${s.page_num}-${s.page_end}`
      : `p.${s.page_num}`;
  }

  let isLoading = false;
  let pendingQuestion = '';
  let pendingGame = '';
  // Réponse en cours de génération (streaming)
  let pendingAnswer = '';
  let gameError = '';
  let selectedGame = '';
  let selectedGameLabel = '';
  let formEl: HTMLFormElement;
  let scrollEl: HTMLElement;
  let questionText = '';
  let gameSearch = '';
  let showGameDropdown = false;
  let gameInputEl: HTMLInputElement;

  const MAX_QUESTION_LENGTH = 500;

  // Liste des jeux chargée en arrière-plan (le backend peut être en train de se réveiller)
  let games: Game[] = [];
  let gamesStatus: 'loading' | 'ready' | 'error' = 'loading';
  $: resolveGames(data.games);

  let gamesError = '';

  async function resolveGames(promise: Promise<Game[]>) {
    gamesStatus = 'loading';
    try {
      games = await promise;
      gamesStatus = 'ready';
      restoreGame();
    } catch (error) {
      games = [];
      gamesError = error instanceof Error ? error.message : '';
      gamesStatus = 'error';
    }
  }

  function retryGames() {
    resolveGames(loadGames());
  }

  // Horloge pour afficher le temps d'attente pendant le réveil du backend
  let now = Date.now();
  const clock = setInterval(() => (now = Date.now()), 1000);
  onDestroy(() => clearInterval(clock));
  $: waitedSeconds = $gamesProgress
    ? Math.max(0, Math.round((now - $gamesProgress.startedAt) / 1000))
    : 0;

  $: filteredGames = games.filter((g: any) =>
    (g.name ?? '').toLowerCase().includes(gameSearch.toLowerCase())
  );

  // Conversation affichée (null = nouvelle conversation)
  let currentId: string | null = null;
  $: current = $conversations.find((c) => c.id === currentId);
  $: currentFiles =
    [...(current?.exchanges ?? [])].reverse().find((e) => e.file_path?.length)?.file_path ?? null;

  // Jeu en cours mémorisé : on le choisit une fois pour toute la partie
  const GAME_KEY = 'ask-rules:game';

  function rememberGame(name: string) {
    try {
      if (name) localStorage.setItem(GAME_KEY, name);
      else localStorage.removeItem(GAME_KEY);
    } catch {
      // stockage indisponible : sans conséquence
    }
  }

  function restoreGame() {
    if (selectedGame) return;
    let saved: string | null = null;
    try {
      saved = localStorage.getItem(GAME_KEY);
    } catch {
      return;
    }
    if (saved && games.some((g) => g.name === saved)) selectGame(saved);
  }

  function selectGame(name: string) {
    selectedGame = name;
    selectedGameLabel = name;
    gameSearch = name;
    showGameDropdown = false;
    gameError = '';
    rememberGame(name);
    // Le contexte d'une conversation est propre à un jeu
    if (current && current.game !== name) currentId = null;
  }

  function clearGame() {
    selectedGame = '';
    selectedGameLabel = '';
    gameSearch = '';
    showGameDropdown = false;
    currentId = null;
    rememberGame('');
  }

  function newConversation() {
    currentId = null;
    questionText = '';
  }

  function openConversation(id: string) {
    const conversation = $conversations.find((c) => c.id === id);
    if (!conversation) return;
    selectGame(conversation.game);
    currentId = id;
    historyOpen = false;
    scrollToEnd();
  }

  function removeConversation(id: string) {
    deleteConversation(id);
    if (id === currentId) currentId = null;
  }

  function clearHistory() {
    if (confirm('Effacer tout l’historique des conversations ?')) {
      clearConversations();
      currentId = null;
    }
  }

  let historyOpen = false;

  const dateFormat = new Intl.DateTimeFormat('fr-FR', {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });

  async function scrollToEnd() {
    await tick();
    scrollEl?.scrollTo({ top: scrollEl.scrollHeight, behavior: 'smooth' });
  }

  // Pendant le streaming, suivre la réponse sauf si l'utilisateur est remonté lire
  async function followAnswer() {
    if (!scrollEl) return;
    const nearBottom = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight < 120;
    await tick();
    if (nearBottom) scrollEl.scrollTop = scrollEl.scrollHeight;
  }

  // Écran allumé tant qu'une partie est en cours
  $: keepScreenOn(!!current || isLoading);
  onDestroy(() => keepScreenOn(false));

  // Dictée : le texte dicté complète ce qui était déjà saisi
  let voiceBase = '';
  function handleVoiceStart() {
    voiceBase = questionText.trim() ? questionText.trim() + ' ' : '';
  }
  function handleVoiceTranscript(text: string, final: boolean) {
    questionText = (voiceBase + text).slice(0, MAX_QUESTION_LENGTH);
    if (final && text) formEl.requestSubmit();
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
    // questionText plutôt que la valeur du DOM : la dictée peut l'avoir modifié
    // juste avant l'envoi, avant la mise à jour du champ
    const question = questionText.trim();
    if (!question) return;

    // Jeu : sélectionné, unique, ou cité dans la question
    const detected = selectedGame
      ? null
      : detectGame(
          question,
          games.map((g) => g.name)
        );
    if (detected) selectGame(detected);
    const game = selectedGame || (games.length === 1 ? games[0].name : '');
    if (!game) {
      gameError = 'Choisissez d’abord le jeu auquel vous jouez.';
      gameInputEl?.focus();
      return;
    }
    // Conversation figée au moment de l'envoi : la réponse y est ajoutée même
    // si l'utilisateur change de jeu entre-temps
    const conversationId = current?.game === game ? currentId : null;
    const history = conversationId ? contextTurns(current) : [];

    currentId = conversationId;
    isLoading = true;
    pendingQuestion = question;
    pendingGame = game;
    pendingAnswer = '';
    questionText = '';
    scrollToEnd();

    let exchange: Exchange = { question, at: Date.now() };
    try {
      const data = await askStream({ question, game, history }, (text) => {
        pendingAnswer += text;
        followAnswer();
      });
      exchange = {
        ...exchange,
        answer: data.answer,
        model: data.model,
        sections: data.sections ?? [],
        cached: data.cached,
        file_path: data.file_path,
      };
    } catch (err) {
      exchange.error = err instanceof Error ? err.message : 'Erreur serveur';
    } finally {
      currentId = addExchange(conversationId, game, exchange);
      isLoading = false;
      pendingQuestion = '';
      pendingAnswer = '';
      followAnswer();
    }
  }
</script>

<SEO
  title="Reglomatic — Assistant IA pour règles de jeux de société"
  description="Posez des questions sur vos jeux de société préférés et obtenez des réponses instantanées grâce à notre IA. Recherche intelligente dans les règles de vos jeux."
  keywords="jeux de société, règles de jeu, IA, assistant intelligent, board games, questions réponses, recherche règles, aide jeu de société"
/>

<div class="page chat">
  <!-- Zone défilante : en-tête, historique, conversation -->
  <div class="chat-scroll" bind:this={scrollEl}>
    <!-- En-tête -->
    <header class="header">
      <h1 class="logo">Reglomatic</h1>
      <p class="tagline">Posez une question sur vos règles de jeu de société</p>
    </header>

    <!-- Historique des conversations -->
    {#if $conversations.length > 0}
      <details class="history-details" bind:open={historyOpen}>
        <summary class="history-summary">
          Historique · {$conversations.length} conversation{$conversations.length > 1 ? 's' : ''}
        </summary>
        <ul class="history-list">
          {#each $conversations as c (c.id)}
            <li class="history-item" class:active={c.id === currentId}>
              <button
                type="button"
                class="history-open"
                on:click={() => openConversation(c.id)}
                disabled={isLoading}
              >
                <span class="history-question">{c.exchanges[0]?.question}</span>
                <span class="history-meta">
                  🎲 {c.game} · {c.exchanges.length} question{c.exchanges.length > 1 ? 's' : ''} ·
                  {dateFormat.format(c.updatedAt)}
                </span>
              </button>
              <button
                type="button"
                class="history-delete"
                title="Supprimer cette conversation"
                aria-label="Supprimer cette conversation"
                on:click={() => removeConversation(c.id)}
                disabled={isLoading}>×</button
              >
            </li>
          {/each}
        </ul>
        <button type="button" class="history-clear" on:click={clearHistory} disabled={isLoading}>
          Effacer l’historique
        </button>
      </details>
    {/if}

    <!-- Conversation en cours -->
    {#if current || pendingQuestion}
      <section class="result-section" aria-live="polite">
        <div class="conversation-header">
          <div class="game-badge">
            <span class="game-icon">🎲</span>
            <span>{current?.game ?? pendingGame}</span>
          </div>
          {#if current}
            <button
              type="button"
              class="new-conversation-btn"
              on:click={newConversation}
              disabled={isLoading}
            >
              + Nouvelle conversation
            </button>
          {/if}
        </div>

        <!-- Lien(s) de téléchargement du fichier source -->
        {#if currentFiles}
          <div class="file-download">
            {#each currentFiles as filePath, index (filePath)}
              <a href="/files/{filePath}" class="file-download-link" target="_blank" rel="noopener">
                <span class="file-icon">📄</span>
                <span class="file-text">
                  <span class="file-label">
                    {currentFiles.length > 1 ? `Fichier source ${index + 1}` : 'Fichier source'}
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

        {#each current?.exchanges ?? [] as exchange, index (index)}
          <article class="exchange">
            <p class="question-reminder">« {exchange.question} »</p>
            {#if exchange.error}
              <div class="error-card" role="alert">
                <span class="error-icon">⚠</span>
                <div>
                  <div class="error-message">{exchange.error}</div>
                </div>
              </div>
            {:else}
              <!-- Réponse LLM -->
              <div class="answer-card">
                {#if exchange.model}
                  <div class="answer-header">
                    Réponse
                    <span class="model-tag">{exchange.model}</span>
                  </div>
                  <div class="answer-text">
                    <Markdown content={linkCitations(exchange.answer ?? '', exchange.sections)} />
                  </div>
                {:else}
                  <p class="no-llm-notice">
                    Aucun LLM configuré. Ajoutez <code>LLM_BASE_URL</code> dans
                    <code>.env</code>.
                  </p>
                {/if}
              </div>

              <!-- Sections source -->
              <details class="sources-details">
                <summary class="sources-summary">
                  {exchange.sections?.length ?? 0} section{(exchange.sections?.length ?? 0) > 1
                    ? 's'
                    : ''} source
                </summary>
                <div class="sources-list">
                  {#each exchange.sections ?? [] as s, i (i)}
                    <div class="source-card">
                      <div class="source-header">
                        <span class="source-title">{s.title}</span>
                        <div class="source-meta">
                          {#if s.page_num && s.file?.toLowerCase().endsWith('.pdf')}
                            <a
                              href={pageHref(s)}
                              class="source-page source-page-link"
                              target="_blank"
                              rel="noopener"
                              title="Ouvrir le livret de règles à cette page">{pageLabel(s)} ↗</a
                            >
                          {:else if s.page_num}
                            <span class="source-page">{pageLabel(s)}</span>
                          {/if}
                          <span class="source-score">{(s.score * 100).toFixed(0)}%</span>
                        </div>
                      </div>
                      <p class="source-text">
                        {s.content.slice(0, 220) + '…'}
                      </p>
                    </div>
                  {/each}
                </div>
              </details>
            {/if}
          </article>
        {/each}

        {#if pendingQuestion}
          <article class="exchange">
            <p class="question-reminder">« {pendingQuestion} »</p>
            {#if pendingAnswer}
              <div class="answer-card">
                <div class="answer-text">
                  <Markdown content={pendingAnswer} />
                </div>
              </div>
            {:else}
              <div class="answer-card pending">
                <span class="spinner" aria-hidden="true"></span>Recherche dans les règles…
              </div>
            {/if}
          </article>
        {/if}
      </section>
    {/if}
  </div>

  <!-- Formulaire (fixé en bas) -->
  <form bind:this={formEl} class="ask-form" on:submit|preventDefault={handleSubmit}>
    {#if !current}
      <div class="suggested-tags">
        {#each suggestedQuestions as question, index (index)}
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
    {/if}

    <div class="question-wrapper">
      <textarea
        name="question"
        class="question-input"
        placeholder={current
          ? 'Question de suivi : Et à deux joueurs ? Et en fin de partie ?'
          : 'Ex : Comment se déroule un combat ? Combien de joueurs ?'}
        required
        rows={current ? 2 : 3}
        maxlength={MAX_QUESTION_LENGTH}
        disabled={isLoading}
        bind:value={questionText}
        on:keydown={handleTextareaKeydown}
      ></textarea>
      <VoiceInput
        disabled={isLoading}
        onstart={handleVoiceStart}
        ontranscript={handleVoiceTranscript}
      />
      <div class="char-counter" class:warning={isNearLimit} class:danger={isAtLimit}>
        {charCount} / {MAX_QUESTION_LENGTH}
      </div>
    </div>

    <div class="form-footer">
      {#if gamesStatus === 'loading'}
        <span class="game-label empty games-status" role="status">
          {#if !$gamesProgress || ($gamesProgress.attempt === 1 && !$gamesProgress.lastError)}
            ⏳ Chargement des jeux…{waitedSeconds >= 3 ? ` (${waitedSeconds} s)` : ''}
          {:else}
            ⏳ Réveil du serveur… {waitedSeconds} s
            <small
              >tentative {$gamesProgress.attempt}/{$gamesProgress.maxAttempts}{$gamesProgress.lastError
                ? ` · dernière erreur : ${$gamesProgress.lastError}`
                : ''}</small
            >
          {/if}
        </span>
      {:else if gamesStatus === 'error'}
        <span class="game-label empty games-status" role="alert">
          Serveur indisponible{gamesError ? ` (${gamesError})` : ''}
          <button type="button" class="games-retry-btn" on:click={retryGames}>Réessayer</button>
        </span>
      {:else if games.length > 1}
        <div class="game-search-wrapper">
          <input
            bind:this={gameInputEl}
            type="text"
            class="game-search-input"
            placeholder="Choisir le jeu…"
            aria-invalid={gameError ? 'true' : undefined}
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
          {#if showGameDropdown}
            <ul class="game-dropdown">
              {#if filteredGames.length === 0}
                <li class="game-dropdown-empty">Aucun jeu trouvé</li>
              {:else}
                {#each filteredGames as g (g.id)}
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
      {:else if games.length === 1}
        <span class="game-label">🎲 {games[0].name}</span>
      {:else}
        <span class="game-label empty">Aucun jeu indexé</span>
      {/if}

      <button
        type="submit"
        class="submit-btn{isLoading ? ' loading' : ''}"
        disabled={isLoading || games.length === 0}
      >
        {#if isLoading}
          <span class="spinner" aria-hidden="true"></span>Recherche…
        {:else}
          Poser la question →
        {/if}
      </button>
    </div>
    {#if gameError}
      <p class="game-error" role="alert">{gameError}</p>
    {/if}
  </form>

  <!-- Footer -->
  <footer class="footer">
    {#if games.length > 0}
      <span
        >{games.length} jeu{games.length > 1 ? 'x' : ''} indexé{games.length > 1 ? 's' : ''}</span
      >
    {/if}
    <span class="version">v{__APP_VERSION__}</span>
  </footer>
</div>

<style>
  /* ── Mise en page chat : saisie fixée en bas, conversation défilante ── */

  :global(body:has(.page.chat)) {
    height: 100dvh;
    overflow: hidden;
  }

  .page.chat {
    flex: 1;
    min-height: 0;
    padding: 0 1.5rem 0.75rem;
    gap: 0.75rem;
  }

  .chat-scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overscroll-behavior: contain;
    display: flex;
    flex-direction: column;
    gap: 2rem;
    padding: 1.5rem 0.5rem 1rem 0;
    margin-right: -0.5rem;
    scrollbar-width: thin;
    scrollbar-color: var(--border) transparent;
  }

  .chat-scroll::-webkit-scrollbar {
    width: 8px;
  }

  .chat-scroll::-webkit-scrollbar-thumb {
    background: var(--border);
    border-radius: 4px;
  }

  .chat-scroll::-webkit-scrollbar-thumb:hover {
    background: var(--text-faint);
  }

  .page.chat .ask-form {
    flex: none;
    padding: 1rem;
  }

  /* Saisie en bas d'écran : la liste des jeux s'ouvre vers le haut */
  .page.chat .game-dropdown {
    top: auto;
    bottom: calc(100% + 4px);
  }

  .page.chat .footer {
    flex: none;
    padding-top: 0;
  }

  @media (max-width: 520px) {
    .page.chat {
      padding: 0 1rem 0.5rem;
    }

    .page.chat .header {
      padding: 0.5rem 0 0;
    }
  }

  .question-wrapper .question-input {
    padding-right: 3.25rem; /* place du bouton micro */
  }

  .game-error {
    margin: 0;
    color: var(--red);
    font-size: 0.85rem;
  }

  .game-search-input[aria-invalid='true'] {
    border-color: var(--red);
  }

  /* Citations de pages cliquables dans les réponses */
  .answer-text :global(a.citation) {
    color: #c4b5fd;
    text-decoration: underline dotted;
    text-underline-offset: 2px;
    white-space: nowrap;
  }

  /* ── Conversation ─────────────────────────────────────── */

  .conversation-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .new-conversation-btn {
    background: none;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0.4rem 0.8rem;
    color: var(--text-muted);
    font-size: 0.85rem;
    font-family: inherit;
    cursor: pointer;
    transition:
      color 0.15s,
      border-color 0.15s;
  }

  .new-conversation-btn:hover:not(:disabled) {
    color: var(--text);
    border-color: var(--accent);
  }

  .exchange {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    padding-top: 1rem;
    border-top: 1px solid var(--border);
    animation: fadeIn 0.3s ease;
  }

  .answer-card.pending {
    display: flex;
    align-items: center;
    color: var(--text-muted);
  }

  /* ── Historique ───────────────────────────────────────── */

  .history-details {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.75rem 1rem;
  }

  .history-summary {
    cursor: pointer;
    color: var(--text-muted);
    font-size: 0.9rem;
  }

  .history-list {
    list-style: none;
    margin: 0.75rem 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    max-height: 320px;
    overflow-y: auto;
  }

  .history-item {
    display: flex;
    align-items: center;
    border-radius: var(--radius-sm);
  }

  .history-item:hover,
  .history-item.active {
    background: var(--surface-2);
  }

  .history-open {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    text-align: left;
    background: none;
    border: none;
    padding: 0.5rem 0.6rem;
    color: var(--text);
    font-family: inherit;
    cursor: pointer;
  }

  .history-question {
    font-size: 0.92rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .history-meta {
    font-size: 0.78rem;
    color: var(--text-muted);
  }

  .history-delete {
    background: none;
    border: none;
    color: var(--text-faint);
    font-size: 1.2rem;
    line-height: 1;
    padding: 0.3rem 0.6rem;
    cursor: pointer;
  }

  .history-delete:hover:not(:disabled) {
    color: var(--red);
  }

  .history-clear {
    background: none;
    border: none;
    color: var(--text-muted);
    font-size: 0.8rem;
    font-family: inherit;
    text-decoration: underline;
    cursor: pointer;
  }

  .history-clear:hover:not(:disabled) {
    color: var(--red);
  }

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
