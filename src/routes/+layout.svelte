<script>
  import '../app.css';
  import PWAInstall from '$lib/PWAInstall.svelte';
  import { resolve } from '$app/paths';
</script>

<nav class="nav">
  <a href={resolve('/')} class="nav-brand">Reglomatic</a>
</nav>

<slot />

<PWAInstall />
