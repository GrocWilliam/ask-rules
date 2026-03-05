# Exemple d'intégration du JobTracker

## Dans la page admin des jeux

```svelte
<script lang="ts">
  import JobTracker from '$lib/JobTracker.svelte';
  import type { PageData } from './$types';

  export let data: PageData;

  // État du job tracker
  let jobId = '';
  let jobActive = false;

  async function reprocessGame(gameId: string) {
    const response = await fetch(`/api/admin/games/${gameId}/reprocess`, {
      method: 'POST',
    });

    if (!response.body) {
      alert('Erreur serveur');
      return;
    }

    jobActive = true; // Active l'affichage du JobTracker

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

        try {
          const evt = JSON.parse(line.slice(6));

          // Le JobTracker va capturer automatiquement le job_id
          // depuis l'événement 'job_started'

          if (evt.type === 'error') {
            console.error('Erreur:', evt.error);
          }
        } catch (err) {
          console.error('Parse error:', err);
        }
      }
    }
  }

  async function reprocessAll() {
    const response = await fetch('/api/admin/reprocess-all', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ game_ids: [] }),
    });

    if (!response.body) return;

    jobActive = true;

    // Même logique que reprocessGame...
  }
</script>

<div class="admin-games">
  <h1>Gestion des jeux</h1>

  <div class="actions">
    <button on:click={reprocessAll}> Recalculer tous les jeux </button>
  </div>

  <div class="games-list">
    {#each data.games as game}
      <div class="game-card">
        <h3>{game.name}</h3>
        <p>{game.sections_count} sections</p>

        <button on:click={() => reprocessGame(game.id)}> Recalculer </button>
      </div>
    {/each}
  </div>
</div>

<!-- Job Tracker - affiche automatiquement en bas à droite quand actif -->
<JobTracker bind:jobId bind:active={jobActive} />

<style>
  .admin-games {
    padding: 20px;
  }

  .actions {
    margin-bottom: 20px;
  }

  .games-list {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: 16px;
  }

  .game-card {
    border: 1px solid #e5e7eb;
    border-radius: 8px;
    padding: 16px;
  }

  button {
    background: #3b82f6;
    color: white;
    border: none;
    padding: 8px 16px;
    border-radius: 4px;
    cursor: pointer;
  }

  button:hover {
    background: #2563eb;
  }
</style>
```

## Fonctionnalités automatiques

Le composant `JobTracker` gère automatiquement :

1. **Capture du job_id** : Depuis l'événement `job_started`
2. **Sauvegarde en localStorage** : Persiste entre les rechargements
3. **Reprise automatique** : Au chargement de la page, reprend le dernier job en cours
4. **Affichage de la progression** : Barre de progression en temps réel
5. **Historique des événements** : Log détaillé (dépliable)
6. **Reconnexion automatique** : En cas de coupure réseau

## Méthodes exposées

```typescript
// Annuler le suivi (n'annule pas le traitement serveur)
jobTrackerRef.cancel();

// Reprendre manuellement un job spécifique
jobTrackerRef.resume('reprocess-1709638800123456789');
```

## Scénarios testés

### Scénario 1 : Utilisation normale

1. Admin clique sur "Recalculer"
2. JobTracker s'affiche en bas à droite
3. Progression s'affiche en temps réel
4. À la fin, message "✅ Terminé"

### Scénario 2 : Fermeture de page

1. Admin démarre un reprocess
2. Ferme l'onglet (volontairement ou crash navigateur)
3. Rouvre la page admin
4. **JobTracker reprend automatiquement** le suivi du dernier job
5. Affiche tout l'historique + continue en temps réel

### Scénario 3 : Déconnexion réseau

1. Admin démarre un reprocess
2. Connexion WiFi coupée
3. Reconnexion WiFi
4. EventSource se reconnecte automatiquement
5. JobTracker continue d'afficher les événements

### Scénario 4 : Timeout proxy

1. Admin démarre un reprocess long (>15 min)
2. Proxy coupe la connexion SSE
3. Traitement continue côté serveur
4. Admin peut manuellement se reconnecter :
   - Via le bouton "Reprendre" dans l'UI
   - Ou en rafraîchissant la page (reprise auto)

## Style personnalisable

Le composant utilise des variables CSS que vous pouvez surcharger :

```css
:global(.job-tracker) {
  /* Position */
  bottom: 20px;
  right: 20px;

  /* Taille */
  width: 400px;

  /* Couleurs */
  --color-running: #3b82f6;
  --color-completed: #10b981;
  --color-failed: #ef4444;
}
```

## API utilisée

Le composant fait appel aux endpoints suivants :

- `GET /api/admin/jobs/{id}` : Vérifier le statut
- `GET /api/admin/jobs/{id}/stream` : Reconnexion SSE

Tous ces endpoints sont authentifiés (cookie admin requis).
