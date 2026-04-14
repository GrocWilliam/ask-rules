<script lang="ts">
  import type { PageData } from './$types';
  import SEO from '$lib/SEO.svelte';
  import type { GameFile } from '../../../types/gamefile.type';
  import { resolve } from '$app/paths';

  export let data: PageData;

  let files: GameFile[] = data.files as GameFile[];
  let deletingFile: string | null = null;
  let confirmDelete: string | null = null;
  let toast: { ok: boolean; msg: string } | null = null;

  function handleDeleteClick(filePath: string) {
    confirmDelete = confirmDelete === filePath ? null : filePath;
  }

  async function deleteFile(file: GameFile) {
    deletingFile = file.path;
    try {
      const [slug, filename] = file.relative_path.split('/');
      const res = await fetch(`/api/admin/files/${slug}/${filename}`, { method: 'DELETE' });
      if (!res.ok) {
        const text = await res.text();
        toast = { ok: false, msg: text || 'Erreur lors de la suppression' };
      } else {
        files = files.filter((f: GameFile) => f.path !== file.path);
        toast = { ok: true, msg: `Fichier « ${file.name} » supprimé.` };
      }
    } catch {
      toast = { ok: false, msg: 'Erreur réseau' };
    } finally {
      deletingFile = null;
      confirmDelete = null;
    }
  }

  function formatFileSize(bytes: number) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB';
    if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
    return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
  }

  function formatDate(dateString: string) {
    return new Date(dateString).toLocaleDateString('fr-FR', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  }

  function getFileExtension(filename: string) {
    return filename.split('.').pop()?.toLowerCase() || '';
  }

  function getFileIcon(filename: string) {
    const icons: Record<string, string> = {
      pdf: '📄',
      txt: '📝',
      doc: '📘',
      docx: '📘',
      json: '📋',
      md: '📝',
    };
    return icons[getFileExtension(filename)] || '📎';
  }

  // Grouper les fichiers par jeu
  $: filesByGame = files.reduce(
    (acc: Record<string, GameFile[]>, file: GameFile) => {
      if (!acc[file.game]) acc[file.game] = [];
      acc[file.game].push(file);
      return acc;
    },
    {} as Record<string, GameFile[]>
  );
</script>

<SEO title="Gestion des Fichiers - Admin" description="Interface d'administration des fichiers" />

<svelte:head>
  <meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="page-header">
  <div>
    <h1>📁 Gestion des Fichiers</h1>
    <p class="summary">
      {files.length} fichier(s)
    </p>
  </div>
</div>

{#if toast?.ok}
  <div class="alert alert-success">✅ {toast.msg}</div>
{/if}
{#if toast && !toast.ok}
  <div class="alert alert-error">❌ {toast.msg}</div>
{/if}

{#if files.length === 0}
  <div class="empty-state">
    <p>Aucun fichier uploadé pour le moment.</p>
    <a href={resolve('/import')} class="btn btn-primary">Importer des règles</a>
  </div>
{:else}
  <div class="games-section">
    {#each Object.entries(filesByGame) as [game, files] (game)}
      <div class="game-group">
        <h2 class="game-title">
          🎮 {game}
          <span class="file-count">{files.length} fichier(s)</span>
        </h2>

        <div class="files-list">
          {#each files as file (file.name)}
            <div class="file-card">
              <div class="file-icon">{getFileIcon(file.name)}</div>

              <div class="file-info">
                <div class="file-name">{file.name}</div>
                <div class="file-meta">
                  <span>{formatFileSize(file.size)}</span>
                  <span>•</span>
                  <span>{formatDate(file.modified)}</span>
                </div>
                <div class="file-path">{file.relative_path}</div>
              </div>

              <div class="file-actions">
                <a href="/files/{file.relative_path}" class="btn btn-view" target="_blank">
                  👁️ Voir
                </a>

                {#if confirmDelete === file.path}
                  <button
                    class="btn btn-danger-sm"
                    disabled={deletingFile === file.path}
                    on:click={() => deleteFile(file)}
                  >
                    {deletingFile === file.path ? '...' : 'Confirmer'}
                  </button>
                  <button class="btn btn-secondary-sm" on:click={() => (confirmDelete = null)}
                    >Annuler</button
                  >
                {:else}
                  <button class="btn btn-delete" on:click={() => handleDeleteClick(file.path)}>
                    🗑️
                  </button>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .page-header {
    margin-bottom: 2rem;
  }

  h1 {
    font-size: 2rem;
    margin: 0 0 0.5rem 0;
    color: #1a1a1a;
  }

  .summary {
    color: #666;
    font-size: 1rem;
  }

  .alert {
    padding: 1rem;
    margin-bottom: 1.5rem;
    border-radius: 8px;
    font-weight: 500;
  }

  .alert-success {
    background-color: #d4edda;
    color: #155724;
    border: 1px solid #c3e6cb;
  }

  .alert-error {
    background-color: #f8d7da;
    color: #721c24;
    border: 1px solid #f5c6cb;
  }

  .empty-state {
    text-align: center;
    padding: 4rem 2rem;
    background: white;
    border-radius: 12px;
    border: 2px dashed #d1d5db;
  }

  .empty-state p {
    font-size: 1.125rem;
    color: #6b7280;
    margin-bottom: 1.5rem;
  }

  .games-section {
    display: flex;
    flex-direction: column;
    gap: 2rem;
  }

  .game-group {
    background: white;
    border-radius: 12px;
    padding: 1.5rem;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
  }

  .game-title {
    font-size: 1.25rem;
    margin: 0 0 1rem 0;
    color: #1a1a1a;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding-bottom: 1rem;
    border-bottom: 2px solid #f0f0f0;
  }

  .file-count {
    font-size: 0.875rem;
    color: #666;
    font-weight: normal;
  }

  .files-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .file-card {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 1rem;
    background: #f9fafb;
    border: 1px solid #e5e7eb;
    border-radius: 8px;
    transition: all 0.2s;
  }

  .file-card:hover {
    background: #f3f4f6;
    border-color: #d1d5db;
  }

  .file-icon {
    font-size: 2rem;
    flex-shrink: 0;
  }

  .file-info {
    flex: 1;
    min-width: 0;
  }

  .file-name {
    font-weight: 600;
    color: #1a1a1a;
    margin-bottom: 0.25rem;
    word-break: break-all;
  }

  .file-meta {
    font-size: 0.875rem;
    color: #666;
    display: flex;
    gap: 0.5rem;
  }

  .file-path {
    font-size: 0.75rem;
    color: #999;
    font-family: monospace;
    margin-top: 0.25rem;
  }

  .file-actions {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    flex-shrink: 0;
  }

  .btn {
    padding: 0.5rem 1rem;
    border-radius: 6px;
    border: none;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
    text-decoration: none;
    display: inline-block;
  }

  .btn-primary {
    background: #4f46e5;
    color: white;
  }

  .btn-primary:hover {
    background: #4338ca;
  }

  .btn-view {
    background: #3b82f6;
    color: white;
  }

  .btn-view:hover {
    background: #2563eb;
  }

  .btn-delete {
    background: transparent;
    color: #dc2626;
    border: 1px solid #dc2626;
    padding: 0.5rem 0.75rem;
  }

  .btn-delete:hover {
    background: #dc2626;
    color: white;
  }

  .btn-danger-sm {
    background: #dc2626;
    color: white;
    padding: 0.5rem 0.75rem;
  }

  .btn-danger-sm:hover:not(:disabled) {
    background: #b91c1c;
  }

  .btn-danger-sm:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-secondary-sm {
    background: #6b7280;
    color: white;
    padding: 0.5rem 0.75rem;
  }

  .btn-secondary-sm:hover {
    background: #4b5563;
  }

  @media (max-width: 768px) {
    .file-card {
      flex-direction: column;
      align-items: start;
    }

    .file-actions {
      width: 100%;
      justify-content: flex-end;
    }
  }
</style>
