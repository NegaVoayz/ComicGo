<script lang="ts">
  // Single-page pager. Two-layer prefetching is emulated by warming the
  // browser image cache for the neighbor pages while only one <img> is live.
  import { getPageRef, setProgress } from '../lib/api'

  interface ComicRef {
    id: number
    title: string
    pageCount: number
    resume: number
  }

  let { comic, onExit }: { comic: ComicRef; onExit: () => void } = $props()

  const clamp = (n: number) => Math.max(0, Math.min((comic.pageCount || 1) - 1, n))
  let page = $state(clamp(comic.resume))
  let src = $state('')
  let err = $state(false)
  let busy = $state(false)

  function load(n: number) {
    getPageRef(comic.id, n)
      .then((r) => {
        if (r && r.url) {
          src = r.url
          err = false
        } else {
          err = true
        }
      })
      .catch(() => (err = true))
      .finally(() => (busy = false))
  }
  function warm(n: number) {
    if (n < 0 || n >= comic.pageCount) return
    getPageRef(comic.id, n).then((r) => {
      if (r?.url) {
        const im = new Image()
        im.src = r.url
      }
    })
  }
  function go(n: number) {
    n = clamp(n)
    if (n === page) return
    page = n
    busy = true
    load(n)
    warm(n + 1)
    warm(n - 1)
  }
  function next() {
    go(page + 1)
  }
  function prev() {
    go(page - 1)
  }

  // initial + page-change load / neighbor warm
  $effect(() => {
    busy = true
    load(page)
    warm(page + 1)
    warm(page - 1)
  })

  // debounced progress save, re-armed on every page change
  $effect(() => {
    const p = page
    const timer = setTimeout(() => {
      setProgress(comic.id, p, comic.pageCount).catch(() => {})
    }, 500)
    return () => clearTimeout(timer)
  })

  // keyboard navigation (registered once; handlers read live state)
  $effect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === 'ArrowRight' || e.key === 'PageDown' || e.key === ' ') {
        e.preventDefault()
        next()
      } else if (e.key === 'ArrowLeft' || e.key === 'PageUp') {
        e.preventDefault()
        prev()
      } else if (e.key === 'Escape') {
        onExit()
      } else if (e.key === 'Home') {
        go(0)
      } else if (e.key === 'End') {
        go(comic.pageCount - 1)
      }
    }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  })

  function clickZone(e: MouseEvent) {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const x = e.clientX - rect.left
    if (x < rect.width * 0.28) prev()
    else if (x > rect.width * 0.72) next()
    else go(page) // center: no-op on the same page
  }
  function jump(e: Event) {
    const v = parseInt((e.target as HTMLInputElement).value, 10)
    if (!Number.isNaN(v)) go(v - 1)
  }
  const pct = comic.pageCount > 0 ? Math.round(((page + 1) / comic.pageCount) * 100) : 0
</script>

<div class="reader">
  <div class="stage" onclick={clickZone} role="presentation">
    {#if err}
      <div class="msg">Could not load this page.</div>
    {:else if !src}
      <div class="msg">Loading…</div>
    {:else}
      <img src={src} alt="page {page + 1}" class:ready={!busy} draggable="false" />
    {/if}
  </div>

  <header>
    <button onclick={onExit} title="Back (Esc)">← Library</button>
    <div class="t">
      <span class="title">{comic.title}</span>
      <span class="dim">
        page <input type="number" min="1" max={Math.max(1, comic.pageCount)} value={page + 1} onchange={jump} />
        / {comic.pageCount} · {pct}%
      </span>
    </div>
    <div class="nav">
      <button onclick={prev} disabled={page <= 0}>‹</button>
      <button onclick={next} disabled={page >= comic.pageCount - 1}>›</button>
    </div>
  </header>
</div>

<style>
  .reader {
    position: fixed;
    inset: 0;
    background: #0c0d10;
    display: flex;
    flex-direction: column;
    z-index: 20;
  }
  .stage {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    user-select: none;
    min-height: 0;
  }
  .stage img {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
    opacity: 0;
    transition: opacity 0.12s;
  }
  .stage img.ready { opacity: 1; }
  .msg {
    color: var(--text-dim);
    font-size: 15px;
  }
  header {
    height: 52px;
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 0 16px;
    background: var(--bg-soft);
    border-top: 1px solid var(--line);
  }
  .t {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .title {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dim { color: var(--text-dim); font-size: 12px; display: flex; align-items: center; gap: 6px; }
  .dim input { width: 56px; text-align: center; }
  .nav { display: flex; gap: 6px; }
</style>
