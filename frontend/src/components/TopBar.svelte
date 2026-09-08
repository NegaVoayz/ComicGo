<script lang="ts">
  let {
    roots,
    scanning,
    progress,
    comics = 0,
    importing = false,
    onAdd,
    onScan,
    onCancel,
    onRemoveRoot,
    onImportFile,
    onImportFolder,
    onAbout,
    onReloadLayout,
  }: {
    roots: string[]
    scanning: boolean
    progress: { done: number; total: number } | null
    comics?: number
    importing?: boolean
    onAdd: () => void
    onScan: () => void
    onCancel: () => void
    onRemoveRoot: (dir: string) => void
    onImportFile: () => void
    onImportFolder: () => void
    onAbout: () => void
    onReloadLayout: () => void
  } = $props()

  const pct = $derived(progress && progress.total > 0 ? Math.round((progress.done / progress.total) * 100) : 0)

  let showImport = $state(false)

  function baseName(p: string): string {
    const trimmed = p.replace(/[\\/]+$/, '')
    const parts = trimmed.split(/[\\/]/)
    return parts[parts.length - 1] || p
  }
</script>

<div class="topbar">
  <div class="brand">📚 ComicGo</div>
  <div class="roots" title={roots.join('\n')}>
    {#if roots.length === 0}
      <span class="dim">No library selected</span>
    {:else}
      {#each roots as r (r)}
        <span class="root" title={r}>
          <span class="rname">{baseName(r)}</span>
          <button class="rm" title="Remove this folder from the library" onclick={() => onRemoveRoot(r)}>✕</button>
        </span>
      {/each}
    {/if}
  </div>
  {#if comics > 0}
    <span class="chip">{comics} indexed</span>
  {/if}
  <div class="actions">
    {#if scanning}
      <div class="scanbar">
        <div class="fill" style="width:{pct}%"></div>
      </div>
      <span class="pct">{progress?.done ?? 0}/{progress?.total ?? 0}</span>
      <button onclick={onCancel}>Stop</button>
    {:else}
      <div class="imp">
        <button
          class="import"
          disabled={roots.length === 0 || importing}
          title={roots.length === 0 ? 'Add a library folder first' : 'Add a new comic from a file or an image folder'}
          onclick={() => (showImport = !showImport)}
        >
          {importing ? '…' : '＋ Import'}
        </button>
        {#if showImport}
          <button
            class="menu-backdrop"
            type="button"
            aria-label="Close import menu"
            tabindex="-1"
            onclick={() => (showImport = false)}
          ></button>
          <div class="menu">
            <button
              onclick={() => {
                showImport = false
                onImportFile()
              }}
            >
              🗜️ From archive file…
            </button>
            <button
              onclick={() => {
                showImport = false
                onImportFolder()
              }}
            >
              🖼️ From image folder…
            </button>
          </div>
        {/if}
      </div>
      <button onclick={onAdd}>+ Folder</button>
      <button onclick={onScan} disabled={roots.length === 0}>Scan</button>
    {/if}
    <button title="Reload layout.json from disk" onclick={onReloadLayout}>↻ Layout</button>
    <button title="About ComicGo" onclick={onAbout}>ⓘ</button>
  </div>
</div>

<style>
  .topbar {
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    height: 52px;
    padding: 0 14px;
    background: var(--bg-soft);
    border-bottom: 1px solid var(--line);
  }
  .brand { font-weight: 700; font-size: 16px; white-space: nowrap; }
  .roots {
    flex: 1;
    min-width: 0;
    display: flex;
    gap: 6px;
    overflow: hidden;
  }
  .root {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 6px;
    padding: 2px 4px 2px 8px;
    font-size: 12px;
    color: var(--text-dim);
    max-width: 230px;
  }
  .rname {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .rm {
    border: none;
    background: none;
    color: var(--text-dim);
    padding: 0 2px;
    line-height: 1;
    font-size: 11px;
  }
  .rm:hover { color: var(--fav); }
  .dim { color: var(--text-dim); }
  .chip {
    background: var(--accent-soft);
    color: var(--accent);
    border: 1px solid var(--accent);
    border-radius: 10px;
    padding: 1px 9px;
    font-size: 12px;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
  .actions { display: flex; align-items: center; gap: 6px; }
  .actions button { padding: 5px 9px; }
  .imp {
    position: relative;
    display: inline-flex;
  }
  .import {
    background: var(--accent-soft);
    border-color: var(--accent);
    color: var(--accent);
    font-weight: 600;
  }
  .import:hover:not(:disabled) { border-color: var(--accent); color: #ffe9c9; }
  .menu-backdrop {
    position: fixed;
    inset: 0;
    z-index: 30;
    background: transparent;
    border: none;
    padding: 0;
    cursor: default;
  }
  .menu-backdrop:hover { border: none; }
  .menu {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 40;
    display: flex;
    flex-direction: column;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 8px;
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.45);
    overflow: hidden;
    min-width: 190px;
  }
  .menu button {
    text-align: left;
    background: none;
    border: none;
    border-radius: 0;
    padding: 9px 14px;
    font-size: 13px;
  }
  .menu button:hover { background: var(--panel-2); border: none; }
  .scanbar {
    width: 150px;
    height: 8px;
    border-radius: 4px;
    background: var(--panel);
    overflow: hidden;
  }
  .fill {
    height: 100%;
    background: var(--accent);
    transition: width 0.15s;
  }
  .pct { color: var(--text-dim); font-size: 12px; font-variant-numeric: tabular-nums; }
</style>
