<script>
  import '../app.css';
  import { page } from '$app/stores';
  import PWAInstall from '$lib/PWAInstall.svelte';
  import { resolve } from '$app/paths';
</script>

<nav class="nav">
  <a href={resolve('/')} class="nav-brand">Reglomatic</a>
  <div class="nav-links">
    <a href={resolve('/')} class="nav-link" class:active={$page.url.pathname === '/'}
      >Poser une question</a
    >
    <a href={resolve('/import')} class="nav-link" class:active={$page.url.pathname === '/import'}
      >Importer des règles</a
    >
  </div>
</nav>

<slot />

<PWAInstall />
