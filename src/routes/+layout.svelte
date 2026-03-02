<script>
  import '../app.css';
  import { page } from '$app/stores';
  import PWAInstall from '$lib/PWAInstall.svelte';
  import CookieConsent from '$lib/CookieConsent.svelte';
</script>

<nav class="nav">
  <a href="/" class="nav-brand">Reglomatic</a>
  <div class="nav-links">
    <a href="/" class="nav-link" class:active={$page.url.pathname === '/'}>Poser une question</a>
    <a href="/import" class="nav-link" class:active={$page.url.pathname === '/import'}
      >Importer des règles</a
    >
  </div>
</nav>

<slot />

<PWAInstall />
<CookieConsent />
