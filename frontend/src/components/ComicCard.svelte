<script lang="ts">
  import { getCover, toggleFavorite } from '../lib/api'
  import type { ComicCard as Card } from '../lib/api'

  let {
    card,
    onOpen,
    onChanged,
  }: {
    card: Card
    onOpen: (c: Card) => void
    onChanged?: () => void
  } = $props()

  let src = $state(card.coverUrl || '')
  let loaded = $state(false)
  let failed = $state(false)
  let fav = $state(card.fav)

  $effect(() => {
    // Grid mounts only visible cards; fetch a cover URL on demand if none.
    if (!src && !failed) {
      getCover(card.id)
        .then((r) => {
          if (r && r.url) src = r.url
          else failed = true
        })
        .catch(() => (failed = true))
    }
  })

  async function onToggleFav(event: MouseEvent) {
    event.stopPropagation()
    try {
      fav = await toggleFavorite(card.id)
      onChanged?.()
    } catch {
      /* ignore */
    }
  }

  function open() {
    onOpen(card)
  }

  let initials = card.title
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0])
    .join('')
</script>

<div class="card" role="button" tabindex="0" onclick={open} onkeydown={(e) => (e.key === 'Enter' ? open() : null)}>
  <div class="thumb">
    {#if src && !failed}
      <img src={src} alt={card.title} class:loaded onload={() => (loaded = true)} onerror={() => (failed = true)} />
      {#if !loaded}<div class="skel"></div>{/if}
    {:else}
      <div class="ph">{initials || '?'}</div>
    {/if}
    {#if card.lastPage > 0 && card.pageCount > 0}
      <span class="prog">{Math.min(100, Math.round(((card.lastPage + 1) / card.pageCount) * 100))}%</span>
    {/if}
  </div>
  <div class="meta">
    <button class="star" class:on={fav} title="Favorite" onclick={onToggleFav}>{fav ? '♥' : '♡'}</button>
    <div class="name" title={card.title}>{card.title}</div>
    <div class="pages">{card.pageCount} pages</div>
  </div>
</div>

<style>
  .card {
    height: 100%;
    cursor: pointer;
    user-select: none;
  }
  .thumb {
    position: relative;
    width: 100%;
    aspect-ratio: 3 / 4;
    overflow: hidden;
    border-radius: var(--radius);
    background: var(--panel);
  }
  .thumb img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .thumb img.loaded { opacity: 1; }
  .skel {
    position: absolute;
    inset: 0;
    background: linear-gradient(110deg, var(--panel) 30%, var(--panel-2) 50%, var(--panel) 70%);
    background-size: 200% 100%;
    animation: shimmer 1.1s infinite linear;
  }
  @keyframes shimmer { to { background-position: -200% 0; } }
  .ph {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--panel);
    color: var(--text-dim);
    font-size: 34px;
    font-weight: 600;
  }
  .prog {
    position: absolute;
    right: 4px;
    bottom: 4px;
    background: rgba(0, 0, 0, 0.72);
    color: #ffe9c9;
    font-size: 11px;
    padding: 1px 6px;
    border-radius: 4px;
  }
  .meta {
    position: relative;
    padding: 4px 2px 0;
  }
  .star {
    position: absolute;
    right: 0;
    top: 4px;
    border: none;
    background: none;
    color: var(--text-dim);
    font-size: 16px;
    padding: 0 2px;
  }
  .star.on { color: var(--fav); }
  .name {
    color: var(--text);
    font-size: 12.5px;
    line-height: 1.3;
    padding-right: 22px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .pages {
    color: var(--text-dim);
    font-size: 11px;
  }
</style>
