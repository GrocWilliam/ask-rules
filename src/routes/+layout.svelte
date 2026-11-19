<script>
  import '../app.css';
  import PWAInstall from '$lib/PWAInstall.svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/stores';
</script>

<nav class="nav">
  <a href={resolve('/')} class="nav-brand">Reglomatic</a>
  <div class="nav-links">
    <a
      href={resolve('/question')}
      class="nav-link"
      class:active={$page.url.pathname === '/question'}>Poser une question</a
    >
  </div>
</nav>

<slot />

<PWAInstall />
