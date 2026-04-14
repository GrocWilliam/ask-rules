<script lang="ts">
  import SEO from '$lib/SEO.svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';

  let error: string | null = null;
  let isLoggingIn = false;

  async function handleLogin(e: SubmitEvent) {
    e.preventDefault();
    isLoggingIn = true;
    error = null;
    const form = e.target as HTMLFormElement;
    const pwd = (form.querySelector('#password') as HTMLInputElement).value;
    try {
      const res = await fetch('/api/admin/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: pwd }),
      });
      if (res.ok) {
        goto(resolve('/admin/games'));
      } else {
        const d = await res.json().catch(() => ({}));
        error = d.error ?? 'Mot de passe incorrect';
      }
    } catch {
      error = 'Erreur réseau';
    } finally {
      isLoggingIn = false;
    }
  }
</script>

<SEO
  title="Connexion Admin - Reglomatic"
  description="Page de connexion à l'interface d'administration Reglomatic"
/>

<!-- Robots meta pour bloquer l'indexation -->
<svelte:head>
  <meta name="robots" content="noindex, nofollow" />
</svelte:head>

<main class="login-page">
  <div class="login-container">
    <div class="login-card">
      <h1>🔒 Accès Administration</h1>
      <p class="login-subtitle">Veuillez vous authentifier pour accéder à l'administration</p>

      {#if error}
        <div class="alert alert-error">
          ❌ {error}
        </div>
      {/if}

      <form on:submit|preventDefault={handleLogin}>
        <div class="form-group">
          <label for="password">Mot de passe</label>
          <input
            type="password"
            id="password"
            name="password"
            placeholder="Entrez le mot de passe admin"
            required
            autocomplete="current-password"
          />
        </div>

        <button type="submit" class="btn btn-primary" disabled={isLoggingIn}>
          {isLoggingIn ? 'Connexion...' : 'Se connecter'}
        </button>
      </form>

      <p class="login-hint">
        💡 Configurez le mot de passe avec la variable d'environnement
        <code>ADMIN_PASSWORD</code>
      </p>
    </div>
  </div>
</main>

<style>
  .login-page {
    min-height: 100vh;
    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem;
  }

  .login-container {
    width: 100%;
    max-width: 450px;
  }

  .login-card {
    background: white;
    border-radius: 16px;
    padding: 3rem;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  }

  .login-card h1 {
    font-size: 2rem;
    margin-bottom: 0.5rem;
    color: #1a1a1a;
    text-align: center;
  }

  .login-subtitle {
    text-align: center;
    color: #666;
    margin-bottom: 2rem;
    font-size: 0.95rem;
  }

  .alert {
    padding: 1rem;
    margin-bottom: 1.5rem;
    border-radius: 8px;
    font-weight: 500;
  }

  .alert-error {
    background-color: #fee;
    color: #c33;
    border: 1px solid #fcc;
  }

  .form-group {
    margin-bottom: 1.5rem;
  }

  .form-group label {
    display: block;
    font-weight: 600;
    margin-bottom: 0.5rem;
    color: #333;
  }

  .form-group input {
    width: 100%;
    padding: 0.875rem;
    border: 2px solid #e0e0e0;
    border-radius: 8px;
    font-size: 1rem;
    transition: border-color 0.2s;
    box-sizing: border-box;
  }

  .form-group input:focus {
    outline: none;
    border-color: #4f46e5;
  }

  .btn {
    width: 100%;
    padding: 0.875rem;
    border-radius: 8px;
    border: none;
    font-size: 1rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
  }

  .btn-primary {
    background: #4f46e5;
    color: white;
  }

  .btn-primary:hover:not(:disabled) {
    background: #4338ca;
  }

  .btn-primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .login-hint {
    margin-top: 1.5rem;
    padding: 1rem;
    background: #f9fafb;
    border-radius: 8px;
    font-size: 0.875rem;
    color: #666;
    text-align: center;
  }

  .login-hint code {
    background: #e5e7eb;
    padding: 0.125rem 0.5rem;
    border-radius: 4px;
    font-size: 0.85rem;
    color: #1f2937;
  }
</style>
