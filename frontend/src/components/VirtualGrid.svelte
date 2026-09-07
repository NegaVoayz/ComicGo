<script lang="ts">
  // A windowed, virtualized grid. Only the cells near the viewport exist in
  // the DOM; pages are fetched lazily from the backend as the user scrolls.
  import ComicCard from './ComicCard.svelte'
  import type { ComicCard as Card } from '../lib/api'

  let {
    total,
    load,
    height = 600,
    minCard = 150,
    gap = 14,
    pad = 14,
    refreshKey = 0,
    onOpen,
    onChanged,
  }: {
    total: number
    load: (offset: number, limit: number) => Promise<Card[]>
    height?: number
    minCard?: number
    gap?: number
    pad?: number
    refreshKey?: number
    onOpen: (c: Card) => void
    onChanged?: () => void
  } = $props()

  let outer: HTMLElement | undefined = $state()
  let scrollTop = $state(0)
  let width = $state(0)
  let tick = $state(0)

  const loaded = new Map<number, Card>()
  const pending = new Set<number>()

  const ROWS_PER_CHUNK = 6
  const CAPTION_H = 44

  const cols = $derived(width > 0 ? Math.max(2, Math.floor((width - pad + gap) / (minCard + gap))) : 1)
  const cardW = $derived(
    width > 0 ? Math.max(80, Math.floor((width - pad - gap * (cols - 1)) / cols)) : minCard
  )
  const cardH = $derived(Math.floor(cardW * (4 / 3)))
  const rowH = $derived(cardH + CAPTION_H + gap)
  const totalRows = $derived(total > 0 ? Math.ceil(total / cols) : 0)

  // visible row range
  const rowTop = $derived(Math.max(0, Math.floor(scrollTop / rowH) - 1))
  const rowBottom = $derived(totalRows > 0 ? Math.min(totalRows - 1, Math.ceil((scrollTop + height) / rowH)) : -1)

  $effect(() => {
    // reset whenever the backing dataset changes (total or refresh key)
    const _t = total
    const _k = refreshKey
    loaded.clear()
    pending.clear()
    tick++
    scrollTop = 0
    if (outer) outer.scrollTop = 0
  })
  $effect(() => {
    const el = outer
    if (!el) return
    width = el.clientWidth
    const ro = new ResizeObserver(() => {
      width = el.clientWidth
    })
    ro.observe(el)
    return () => ro.disconnect()
  })

  // fetch pages covering the visible window
  $effect(() => {
    if (width === 0 || total === 0 || cols < 1) return
    const pageSize = Math.max(1, ROWS_PER_CHUNK * cols)
    let begin = Math.max(0, rowTop * cols)
    let end = Math.min(total - 1, (rowBottom + 1) * cols - 1)
    if (end < begin) end = begin
    for (let pageStart = Math.floor(begin / pageSize) * pageSize; pageStart <= end; pageStart += pageSize) {
      if (pending.has(pageStart)) continue
      let haveAll = true
      for (let i = pageStart; i < Math.min(total, pageStart + pageSize); i++) {
        if (!loaded.has(i)) {
          haveAll = false
          break
        }
      }
      if (haveAll) continue
      pending.add(pageStart)
      load(pageStart, pageSize)
        .then((cards) => {
          cards.forEach((c, i) => loaded.set(pageStart + i, c))
        })
        .catch(() => {
          /* transient; next scroll will retry */
        })
        .finally(() => {
          pending.delete(pageStart)
          tick++
        })
    }
  })

  function onScroll() {
    if (outer) scrollTop = outer.scrollTop
  }
</script>

<div class="vg" bind:this={outer} style="height:{height}px" onscroll={onScroll}>
  {#if total === 0}
    <div class="empty">Nothing here yet</div>
  {:else}
    <div class="inner" style="height:{totalRows * rowH + pad}px">
      {#if rowBottom >= rowTop}
        {#each Array.from({ length: rowBottom - rowTop + 1 }, (_, k) => rowTop + k) as row (row)}
          {#each Array.from({ length: cols }) as _, col}
            {@const idx = row * cols + col}
            {#if idx < total}
              {@const card = loaded.get(idx)}
              <div
                class="cell"
                style="left:{col * (cardW + gap) + pad}px; top:{row * rowH + pad}px; width:{cardW}px;"
              >
                {#if card}
                  <ComicCard {card} {onOpen} {onChanged} />
                {:else}
                  <div class="cellph" style="height:{cardH}px"></div>
                {/if}
              </div>
            {/if}
          {/each}
        {/each}
      {/if}
    </div>
  {/if}
</div>

<style>
  .vg {
    overflow-y: auto;
    overflow-x: hidden;
    position: relative;
    border-radius: var(--radius);
    background: var(--bg);
  }
  .inner {
    position: relative;
    width: 100%;
  }
  .cell {
    position: absolute;
  }
  .cellph {
    border-radius: var(--radius);
    background: var(--panel);
  }
  .empty {
    padding: 40px 20px;
    color: var(--text-dim);
    text-align: center;
  }
</style>
