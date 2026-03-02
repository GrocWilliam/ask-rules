<script lang="ts">
  import { onMount } from 'svelte';

  let visible = false;

  onMount(() => {
    try {
      const stored = localStorage.getItem('cookie_consent');
      if (!stored) visible = true;
    } catch {
      visible = true;
    }
  });

  function grant() {
    try {
      localStorage.setItem('cookie_consent', 'granted');
      // @ts-ignore
      window.gtag?.('consent', 'update', {
        analytics_storage: 'granted',
        ad_storage: 'granted',
        ad_user_data: 'granted',
        ad_personalization: 'granted',
      });
      // Déclencher la vue de page maintenant que le consentement est accordé
      // @ts-ignore
      window.gtag?.('event', 'page_view');
    } catch {}
    visible = false;
  }

  function deny() {
    try {
      localStorage.setItem('cookie_consent', 'denied');
    } catch {}
    visible = false;
  }
</script>

{#if visible}
  <div
    class="consent-backdrop"
    role="dialog"
    aria-modal="true"
    aria-label="Consentement aux cookies"
  >
    <div class="consent-banner">
      <p class="consent-text">
        Nous utilisons des cookies analytiques (Google Analytics) pour mesurer l'audience de ce site
        et améliorer votre expérience. Aucune donnée n'est vendue à des tiers.
      </p>
      <div class="consent-actions">
        <button class="btn-deny" on:click={deny}>Refuser</button>
        <button class="btn-grant" on:click={grant}>Accepter</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .consent-backdrop {
    position: fixed;
    inset: 0;
    z-index: 9999;
    display: flex;
    align-items: flex-end;
    justify-content: center;
    padding: 1rem;
    pointer-events: none;
  }

  .consent-banner {
    pointer-events: all;
    background: var(--surface-2, #1c1c2a);
    border: 1px solid var(--border, #2a2a3c);
    border-radius: var(--radius, 12px);
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
    padding: 1.25rem 1.5rem;
    max-width: 640px;
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 1rem;
    animation: slide-up 0.25s ease;
  }

  @keyframes slide-up {
    from {
      opacity: 0;
      transform: translateY(16px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .consent-text {
    font-size: 0.875rem;
    color: var(--text-muted, #7c7c96);
    line-height: 1.5;
  }

  .consent-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
  }

  .btn-deny,
  .btn-grant {
    padding: 0.5rem 1.25rem;
    border-radius: var(--radius-sm, 8px);
    font-size: 0.875rem;
    font-weight: 500;
    cursor: pointer;
    border: none;
    transition: opacity 0.15s;
  }

  .btn-deny:hover,
  .btn-grant:hover {
    opacity: 0.85;
  }

  .btn-deny {
    background: var(--surface, #14141e);
    color: var(--text-muted, #7c7c96);
    border: 1px solid var(--border, #2a2a3c);
  }

  .btn-grant {
    background: var(--accent, #7c3aed);
    color: #fff;
  }
</style>
