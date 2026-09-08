<script lang="ts">
  import { getCover, toggleFavorite } from '../lib/api'
  import type { ComicCard as Card } from '../lib/api'

  let {
    card,
    onOpen,
    onChanged,
    onAddTag,
  }: {
    card: Card
    onOpen: (c: Card) => void
    onChanged?: () => void
    onAddTag?: (tag: string) => void
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
      // The grid no longer remounts on a favorite toggle (no cover flash), so
      // persist the new state onto the shared card object: a later re-render
      // (scroll) reads card.fav and must show the same heart.
      card.fav = fav
      onChanged?.()
    } catch {
      /* ignore */
    }
  }

  function open() {
    onOpen(card)
  }

  // Clicking a tag chip filters the shelf by that tag instead of opening the
  // comic. Without an onAddTag handler the chip click falls through to open().
  function pickTag(e: MouseEvent, tag: string) {
    if (!onAddTag) return
    e.stopPropagation()
    onAddTag(tag)
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
      <img
        src={src}
        alt={card.title}
        loading="lazy"
        decoding="async"
        class:loaded
        onload={() => (loaded = true)}
        onerror={() => (failed = true)}
      />
      {#if !loaded}<div class="skel"></div>{/if}
    {:else}
      <div class="ph">{initials || '?'}</div>
    {/if}
    {#if card.hasNonUtf8}
      <span class="warn" title="Some file names or the comment are not valid UTF-8">⚠</span>
    {/if}
    {#if card.lastPage > 0 && card.pageCount > 0}
      <span class="prog">{Math.min(100, Math.round(((card.lastPage + 1) / card.pageCount) * 100))}%</span>
    {/if}
  </div>
  <div class="meta">
    <button class="star" class:on={fav} title="Favorite" onclick={onToggleFav}>{fav ? '♥' : '♡'}</button>
    <div class="name" title={card.title}>{card.title}</div>
    {#if card.author}
      <div class="author" title={card.author}>{card.author}</div>
    {/if}
    <div class="row">
      {#if card.tags && card.tags.length > 0}
        <span class="tags" title={card.tags.join(', ')}>
          {#each card.tags.slice(0, 3) as tag (tag)}
            <button
              class="tag"
              type="button"
              title={onAddTag ? `Show comics tagged ${tag}` : tag}
              onclick={(e) => pickTag(e, tag)}
            >
              {tag}
            </button>
          {/each}
        </span>
      {/if}
      <span class="pages">{card.pageCount} pages</span>
    </div>
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
  .warn {
    position: absolute;
    left: 4px;
    top: 4px;
    width: 20px;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.7);
    border-radius: 50%;
    font-size: 12px;
    cursor: help;
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
  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: 2px;
    min-height: 16px;
  }
  .tags {
    flex: 1;
    min-width: 0;
    display: flex;
    gap: 4px;
    overflow: hidden;
  }
  .tag {
    flex: 0 1 auto; /* shrink rather than get sliced when the card is narrow */
    min-width: 0;
    max-width: 76px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 10px;
    line-height: 1;
    padding: 2px 5px;
    border: none;
    border-radius: 4px;
    background: var(--panel-2);
    color: var(--text-dim);
  }
  .tag:hover {
    color: var(--text);
    background: var(--panel);
  }
  .pages {
    flex-shrink: 0;
    margin-left: auto;
    color: var(--text-dim);
    font-size: 11px;
    text-align: right;
  }
  .author {
    color: var(--text-dim);
    font-size: 11px;
    line-height: 1.35;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    margin-top: 1px;
    padding-right: 4px;
  }
</style>
