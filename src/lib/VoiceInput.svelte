<script lang="ts">
  // VoiceInput.svelte — Dictée de la question (reconnaissance vocale du navigateur)
  //
  // Les mains sont souvent prises par les cartes : un appui sur le micro, on
  // parle, la question est transcrite. Masqué si le navigateur ne sait pas faire
  // (Firefox notamment).

  interface Props {
    disabled?: boolean;
    /** Début de la dictée */
    onstart?: () => void;
    /** Transcription en cours ; `final` = la phrase est terminée */
    ontranscript: (text: string, final: boolean) => void;
  }

  let { disabled = false, onstart, ontranscript }: Props = $props();

  // API non standard, absente des types DOM de TypeScript
  type RecognitionResult = { isFinal: boolean; 0: { transcript: string } };
  type Recognition = {
    lang: string;
    interimResults: boolean;
    continuous: boolean;
    onresult: ((e: { results: ArrayLike<RecognitionResult> }) => void) | null;
    onerror: ((e: { error: string }) => void) | null;
    onend: (() => void) | null;
    start: () => void;
    stop: () => void;
  };
  type RecognitionCtor = new () => Recognition;

  const Ctor: RecognitionCtor | undefined =
    typeof window === 'undefined'
      ? undefined
      : ((window as unknown as Record<string, RecognitionCtor | undefined>).SpeechRecognition ??
        (window as unknown as Record<string, RecognitionCtor | undefined>).webkitSpeechRecognition);

  let recognition: Recognition | null = null;
  let listening = $state(false);
  let errorMessage = $state('');

  function toggle() {
    if (listening) {
      recognition?.stop();
      return;
    }
    if (!Ctor) return;
    errorMessage = '';
    recognition = new Ctor();
    recognition.lang = 'fr-FR';
    recognition.interimResults = true;
    recognition.continuous = false;
    recognition.onresult = (e) => {
      const results = Array.from(e.results);
      const text = results.map((r) => r[0].transcript).join('');
      ontranscript(
        text.trim(),
        results.every((r) => r.isFinal)
      );
    };
    recognition.onerror = (e) => {
      if (e.error === 'not-allowed' || e.error === 'service-not-allowed') {
        errorMessage = 'Micro refusé : autorisez-le dans les réglages du navigateur.';
      } else if (e.error !== 'no-speech' && e.error !== 'aborted') {
        errorMessage = 'Dictée indisponible pour le moment.';
      }
    };
    recognition.onend = () => {
      listening = false;
    };
    listening = true;
    onstart?.();
    recognition.start();
  }
</script>

{#if Ctor}
  <button
    type="button"
    class="mic-btn"
    class:listening
    aria-pressed={listening}
    aria-label={listening ? 'Arrêter la dictée' : 'Dicter la question'}
    title={errorMessage || (listening ? 'Arrêter la dictée' : 'Dicter la question')}
    onclick={toggle}
    {disabled}
  >
    <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
      <path
        fill="currentColor"
        d="M12 14a3 3 0 0 0 3-3V5a3 3 0 0 0-6 0v6a3 3 0 0 0 3 3Zm5-3a5 5 0 0 1-10 0H5a7 7 0 0 0 6 6.92V21h2v-3.08A7 7 0 0 0 19 11h-2Z"
      />
    </svg>
  </button>
  {#if errorMessage}
    <span class="mic-error" role="alert">{errorMessage}</span>
  {/if}
{/if}

<style>
  .mic-btn {
    position: absolute;
    top: 0.5rem;
    right: 0.5rem;
    width: 2.25rem;
    height: 2.25rem;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text-muted);
    cursor: pointer;
    transition:
      color 0.15s,
      border-color 0.15s,
      background 0.15s;
  }

  .mic-btn:hover:not(:disabled) {
    color: var(--text);
    border-color: var(--accent);
  }

  .mic-btn.listening {
    color: #fff;
    background: var(--red);
    border-color: var(--red);
    animation: pulse 1.2s ease-in-out infinite;
  }

  .mic-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .mic-error {
    position: absolute;
    top: 3rem;
    right: 0.5rem;
    max-width: 16rem;
    padding: 0.4rem 0.6rem;
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    border: 1px solid var(--red);
    color: var(--red);
    font-size: 0.8rem;
    z-index: 5;
  }

  @keyframes pulse {
    50% {
      box-shadow: 0 0 0 6px rgba(248, 113, 113, 0.25);
    }
  }
</style>
