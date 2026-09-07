<script lang="ts">
  // A windowed, virtualized grid. Only cells near the viewport exist in the
  // DOM. Loading is driven imperatively (mount + scroll), NOT by $effect —
  // a reactive effect loop here froze the whole app (effect_update_depth_exceeded).
  import { onMount } from 'svelte'
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
  let width = $state(900)
  let tick = $state(0)

  const loaded = new Map<number, Card>()
  const pending = new Set<number>()
  let ro: ResizeObserver | null = null
  let raf = 0

  const ROWS_PER_CHUNK = 6
  const CAPTION_H = 44

  function geometry() {
    const c = width > 0 ? Math.max(2, Math.floor((width - pad + gap) / (minCard + gap))) : 1
    const w = width > 0 ? Math.max(80, Math.floor((width - pad - gap * (c - 1)) / c)) : minCard
    const h = Math.floor(w * (4 / 3))
    return { cols: c, cardW: w, rowH: h + CAPTION_H + gap }
  }

  const cols = $derived(geometry().cols)
  const cardW = $derived(geometry().cardW)
  const cardH = $derived(Math.floor(cardW * (4 / 3)))
  const rowH = $derived(geometry().rowH)
  const totalRows = $derived(total > 0 ? Math.ceil(total / cols) : 0)
  const rowTop = $derived(Math.max(0, Math.floor(scrollTop / rowH) - 1))
  const rowBottom = $derived(totalRows > 0 ? Math.min(totalRows - 1, Math.ceil((scrollTop + height) / rowH)) : -1)

  function viewportH() {
    return (outer && outer.clientHeight > 0 ? outer.clientHeight : height) || height
  }

  function loadVisible() {
    raf = 0
    if (!outer || total <= 0) return
    const { cols: c } = geometry()
    const rH = geometry().rowH
    const tr = Math.ceil(total / c)
    const st = outer.scrollTop
    const top = Math.max(0, Math.floor(st / rH) - 1)
    const bot = tr > 0 ? Math.min(tr - 1, Math.ceil((st + viewportH()) / rH)) : -1
    if (bot < top) return
    const pageSize = Math.max(1, ROWS_PER_CHUNK * c)
    const begin = top * c
    const end = Math.min(total - 1, (bot + 1) * c - 1)
    for (let p = Math.floor(begin / pageSize) * pageSize; p <= end; p += pageSize) {
      if (pending.has(p)) continue
      let haveAll = true
      for (let i = p; i < Math.min(total, p + pageSize); i++) {
        if (!loaded.has(i)) {
          haveAll = false
          break
        }
      }
      if (haveAll) continue
      pending.add(p)
      load(p, pageSize)
        .then((cards) => {
          cards.forEach((card, i) => loaded.set(p + i, card))
        })
        .catch(() => {
          /* transient; next scroll retries */
        })
        .finally(() => {
          pending.delete(p)
          tick++
        })
    }
  }

  function scheduleLoad() {
    cancelAnimationFrame(raf)
    raf = requestAnimationFrame(loadVisible)
  }

  onMount(() => {
    const el = outer
    if (el) {
      if (el.clientWidth > 0) width = el.clientWidth
      ro = new ResizeObserver(() => {
        if (el.clientWidth > 0) width = el.clientWidth
        scheduleLoad()
      })
      ro.observe(el)
      scheduleLoad()
    }
    return () => {
      if (ro) ro.disconnect()
      cancelAnimationFrame(raf)
    }
  })

  function onScroll() {
    if (!outer) return
    scrollTop = outer.scrollTop
    scheduleLoad()
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
