<script lang="ts">
  // Demo plugin card: library stats. Data comes from the backend CardData id.
  let { data }: { data?: Record<string, any> } = $props()

  const tiles: { key: string; label: string; icon: string }[] = [
    { key: 'total', label: 'Total', icon: '📚' },
    { key: 'favorites', label: 'Favorites', icon: '❤️' },
    { key: 'unread', label: 'Unread', icon: '🆕' },
    { key: 'inProgress', label: 'In progress', icon: '📖' },
  ]
</script>

<div class="stats">
  {#each tiles as t (t.key)}
    <div class="tile">
      <div class="ico">{t.icon}</div>
      <div class="num">{data?.[t.key] ?? '–'}</div>
      <div class="lab">{t.label}</div>
    </div>
  {/each}
</div>

<style>
  .stats {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
  }
  .tile {
    flex: 1 1 140px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--radius);
    padding: 14px 10px;
  }
  .ico { font-size: 20px; }
  .num { font-size: 22px; font-weight: 700; }
  .lab { color: var(--text-dim); font-size: 12px; }
</style>
