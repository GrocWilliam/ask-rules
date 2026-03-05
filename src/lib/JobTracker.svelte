<script lang="ts">
  /**
   * JobTracker.svelte - Composant pour reprendre le suivi d'un job admin après déconnexion
   *
   * Usage:
   * <JobTracker bind:jobId bind:active />
   *
   * Features:
   * - Sauvegarde automatique du job_id dans localStorage
   * - Reprise automatique au chargement de la page si un job est en cours
   * - Affichage de la progression en temps réel
   * - Reconnexion automatique en cas de déconnexion
   */

  import { onMount, onDestroy } from 'svelte';

  // Props
  export let jobId: string = '';
  export let active: boolean = false;

  // État local
  let eventSource: EventSource | null = null;
  let events: Array<{ type: string; data: any; timestamp: Date }> = [];
  let status: 'idle' | 'connecting' | 'running' | 'completed' | 'failed' = 'idle';
  let progress: { current: number; total: number; message?: string } | null = null;
  let error: string = '';

  // Clé localStorage
  const STORAGE_KEY = 'ask_rules_current_job';

  /**
   * Se connecter à un job (nouveau ou existant)
   */
  function connectToJob(id: string, isResume: boolean = false) {
    if (eventSource) {
      eventSource.close();
    }

    jobId = id;
    active = true;
    status = 'connecting';

    // Sauvegarder dans localStorage
    localStorage.setItem(STORAGE_KEY, id);

    // URL différente selon nouveau job ou reprise
    const url = isResume ? `/api/admin/jobs/${id}/stream` : `/api/admin/jobs/${id}/stream`;

    eventSource = new EventSource(url);

    eventSource.onopen = () => {
      status = 'running';
      console.log(`[JobTracker] Connected to job ${id}`);
    };

    eventSource.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data);
        handleEvent(data);
      } catch (err) {
        console.error('[JobTracker] Failed to parse event:', err);
      }
    };

    eventSource.onerror = (err) => {
      console.error('[JobTracker] EventSource error:', err);
      if (eventSource?.readyState === EventSource.CLOSED) {
        // Connexion fermée, vérifier si le job est terminé
        checkJobStatus(id);
      }
    };
  }

  /**
   * Gérer un événement reçu
   */
  function handleEvent(data: any) {
    const event = {
      type: data.type,
      data: data,
      timestamp: new Date(),
    };

    events = [...events, event];

    // Garder seulement les 100 derniers événements
    if (events.length > 100) {
      events = events.slice(-100);
    }

    switch (data.type) {
      case 'job_started':
        jobId = data.job_id;
        localStorage.setItem(STORAGE_KEY, data.job_id);
        break;

      case 'progress':
      case 'embedding_progress':
        if (data.current !== undefined && data.total !== undefined) {
          progress = {
            current: data.current,
            total: data.total,
            message: data.message,
          };
        } else if (data.done !== undefined && data.total !== undefined) {
          progress = {
            current: data.done,
            total: data.total,
            message: data.message,
          };
        }
        break;

      case 'job_status':
        status = data.status === 'completed' ? 'completed' : 'failed';
        if (data.error) {
          error = data.error;
        }
        cleanup();
        break;

      case 'complete':
        status = 'completed';
        cleanup();
        break;

      case 'error':
        status = 'failed';
        error = data.error || 'Une erreur est survenue';
        cleanup();
        break;
    }
  }

  /**
   * Vérifier le statut d'un job via l'API REST
   */
  async function checkJobStatus(id: string) {
    try {
      const response = await fetch(`/api/admin/jobs/${id}`);
      if (!response.ok) {
        throw new Error('Job not found');
      }

      const job = await response.json();
      status = job.status;

      if (job.progress) {
        progress = job.progress;
      }

      if (job.status === 'completed' || job.status === 'failed') {
        if (job.error) {
          error = job.error;
        }
        cleanup();
      }
    } catch (err) {
      console.error('[JobTracker] Failed to check job status:', err);
      status = 'failed';
      error = 'Impossible de vérifier le statut du job';
      cleanup();
    }
  }

  /**
   * Nettoyer les ressources
   */
  function cleanup() {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    localStorage.removeItem(STORAGE_KEY);
    active = false;
  }

  /**
   * Annuler manuellement un job
   */
  export function cancel() {
    cleanup();
    status = 'idle';
    events = [];
    progress = null;
    error = '';
  }

  /**
   * Reprendre un job existant
   */
  export function resume(id: string) {
    connectToJob(id, true);
  }

  // Lifecycle
  onMount(() => {
    // Vérifier s'il y a un job en cours sauvegardé
    const savedJobId = localStorage.getItem(STORAGE_KEY);
    if (savedJobId) {
      console.log(`[JobTracker] Resuming job ${savedJobId}`);
      resume(savedJobId);
    }
  });

  onDestroy(() => {
    if (eventSource) {
      eventSource.close();
    }
  });

  // Calculer le pourcentage de progression
  $: progressPercent = progress ? Math.round((progress.current / progress.total) * 100) : 0;
</script>

{#if active}
  <div
    class="job-tracker"
    class:completed={status === 'completed'}
    class:failed={status === 'failed'}
  >
    <div class="header">
      <h3>
        {#if status === 'connecting'}
          🔄 Connexion au job...
        {:else if status === 'running'}
          ⚙️ Traitement en cours
        {:else if status === 'completed'}
          ✅ Terminé
        {:else if status === 'failed'}
          ❌ Échec
        {/if}
      </h3>

      {#if status === 'running'}
        <button on:click={cancel} class="cancel-btn"> Arrêter le suivi </button>
      {/if}
    </div>

    <div class="job-info">
      <p class="job-id">Job ID: <code>{jobId}</code></p>

      {#if status === 'running' && progress}
        <div class="progress-bar">
          <div class="progress-fill" style="width: {progressPercent}%"></div>
        </div>
        <p class="progress-text">
          {progress.current} / {progress.total} ({progressPercent}%)
          {#if progress.message}
            <span class="progress-message">{progress.message}</span>
          {/if}
        </p>
      {/if}

      {#if error}
        <p class="error">{error}</p>
      {/if}
    </div>

    <details class="events-log">
      <summary>📋 Historique des événements ({events.length})</summary>
      <div class="events-list">
        {#each events.slice().reverse() as event}
          <div class="event">
            <span class="event-time">{event.timestamp.toLocaleTimeString()}</span>
            <span class="event-type">{event.type}</span>
            <span class="event-data">{JSON.stringify(event.data, null, 2)}</span>
          </div>
        {/each}
      </div>
    </details>

    {#if status === 'completed' || status === 'failed'}
      <button on:click={cancel} class="close-btn"> Fermer </button>
    {/if}
  </div>
{/if}

<style>
  .job-tracker {
    position: fixed;
    bottom: 20px;
    right: 20px;
    width: 400px;
    max-width: 90vw;
    background: white;
    border: 2px solid #3b82f6;
    border-radius: 8px;
    padding: 16px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
    z-index: 1000;
  }

  .job-tracker.completed {
    border-color: #10b981;
  }

  .job-tracker.failed {
    border-color: #ef4444;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }

  h3 {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
  }

  .cancel-btn,
  .close-btn {
    padding: 4px 12px;
    font-size: 12px;
    border: 1px solid #d1d5db;
    background: white;
    border-radius: 4px;
    cursor: pointer;
  }

  .cancel-btn:hover,
  .close-btn:hover {
    background: #f3f4f6;
  }

  .job-info {
    margin-bottom: 12px;
  }

  .job-id {
    font-size: 12px;
    color: #6b7280;
    margin: 4px 0;
  }

  .job-id code {
    background: #f3f4f6;
    padding: 2px 6px;
    border-radius: 3px;
    font-size: 11px;
  }

  .progress-bar {
    width: 100%;
    height: 8px;
    background: #e5e7eb;
    border-radius: 4px;
    overflow: hidden;
    margin: 8px 0;
  }

  .progress-fill {
    height: 100%;
    background: linear-gradient(90deg, #3b82f6, #2563eb);
    transition: width 0.3s ease;
  }

  .progress-text {
    font-size: 13px;
    color: #374151;
    margin: 4px 0;
  }

  .progress-message {
    display: block;
    font-size: 12px;
    color: #6b7280;
    margin-top: 2px;
  }

  .error {
    color: #ef4444;
    font-size: 13px;
    margin: 8px 0;
    padding: 8px;
    background: #fef2f2;
    border-radius: 4px;
  }

  .events-log {
    margin-top: 12px;
    border-top: 1px solid #e5e7eb;
    padding-top: 8px;
  }

  .events-log summary {
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
    color: #374151;
    user-select: none;
  }

  .events-log summary:hover {
    color: #3b82f6;
  }

  .events-list {
    max-height: 200px;
    overflow-y: auto;
    margin-top: 8px;
    font-size: 11px;
    font-family: monospace;
  }

  .event {
    display: flex;
    flex-direction: column;
    padding: 4px;
    border-bottom: 1px solid #f3f4f6;
  }

  .event-time {
    color: #6b7280;
  }

  .event-type {
    color: #3b82f6;
    font-weight: 600;
  }

  .event-data {
    color: #374151;
    white-space: pre-wrap;
    word-break: break-all;
  }

  .close-btn {
    width: 100%;
    margin-top: 12px;
    padding: 8px;
  }
</style>
