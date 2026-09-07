<script lang="ts">
  let {
    roots,
    scanning,
    progress,
    onAdd,
    onScan,
    onCancel,
    onReloadLayout,
  }: {
    roots: string[]
    scanning: boolean
    progress: { done: number; total: number } | null
    onAdd: () => void
    onScan: () => void
    onCancel: () => void
    onReloadLayout: () => void
  } = $props()

  const pct = progress && progress.total > 0 ? Math.round((progress.done / progress.total) * 100) : 0
</script>

<div class="topbar">
  <div class="brand">📚 ComicGo</div>
  <div class="roots" title={roots.join('\n')}>
    {#if roots.length === 0}
      <span class="dim">No library selected</span>
    {:else}
      {#each roots as r (r)}<span class="root">{r}</span>{/each}
    {/if}
  </div>
  <div class="actions">
    {#if scanning}
      <div class="scanbar">
        <div class="fill" style="width:{pct}%"></div>
      </div>
      <span class="pct">{progress?.done ?? 0}/{progress?.total ?? 0}</span>
      <button onclick={onCancel}>Stop</button>
    {:else}
      <button onclick={onAdd}>+ Folder</button>
      <button onclick={onScan} disabled={roots.length === 0}>Scan</button>
    {/if}
    <button title="Reload layout.json from disk" onclick={onReloadLayout}>↻ Layout</button>
  </div>
</div>

<style>
  .topbar {
    display: flex;
    align-items: center;
    gap: 14px;
    height: 52px;
    padding: 0 18px;
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
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 4px;
    padding: 2px 8px;
    font-size: 12px;
    color: var(--text-dim);
    max-width: 220px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dim { color: var(--text-dim); }
  .actions { display: flex; align-items: center; gap: 8px; }
  .scanbar {
    width: 160px;
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
