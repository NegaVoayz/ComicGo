<script lang="ts">
  // Full-screen comic details shown when a shelf card is clicked: hero cover,
  // author, every tag (unlike the capped shelf caption), page count, the
  // download source (a magnet) with a copy button, and a Start-reading action.
  // Replaces the old "click straight into the reader" flow.
  import { onMount } from 'svelte'
  import { getComicDetail, getPageRef, getProgress, updateComicMetadata } from '../lib/api'
  import { ok, error } from '../lib/toast'
  import { ClipboardSetText } from '../../wailsjs/runtime/runtime'

  interface StartRef {
    id: number
    title: string
    pageCount: number
    resume: number
  }

  let {
    comic,
    onExit,
    onStart,
    onAddTag,
  }: {
    comic: { id: number; title: string }
    onExit: () => void
    onStart: (r: StartRef) => void
    onAddTag?: (tag: string) => void
  } = $props()

  let title = $state(comic.title)
  let author = $state('')
  let tags = $state<string[]>([])
  let source = $state('')
  let pageCount = $state(0)
  let lastPage = $state(0)
  let hero = $state('')
  let heroFailed = $state(false)
  let heroReady = $state(false)
  let copied = $state(false)
  let editing = $state(false)
  let busy = $state(false)
  let saveErr = $state('')
  let fTitle = $state('')
  let fAuthor = $state('')
  let fTags = $state('')
  let fSource = $state('')

  // splitGenre mirrors the backend's Genre-tag list handling (trim + dedupe).
  function splitGenre(genre: string): string[] {
    const out: string[] = []
    const seen = new Set<string>()
    for (const part of genre.split(',')) {
      const t = part.trim()
      if (t !== '' && !seen.has(t)) {
        seen.add(t)
        out.push(t)
      }
    }
    return out
  }

  onMount(() => {
    let alive = true
    getComicDetail(comic.id)
      .then((d) => {
        if (!alive) return
        const kv = d.meta?.kv ?? {}
        title = kv['Title'] || title
        author = kv['Author'] || ''
        tags = splitGenre(kv['Genre'] || '')
        source = kv['Source'] || ''
        if (d.meta?.pageCount) pageCount = d.meta.pageCount
      })
      .catch(() => {
        /* no meta; shelf title still shows */
      })
    getProgress(comic.id)
      .then((p) => {
        if (!alive) return
        if (p.pageCount) pageCount = p.pageCount
        if (p.pageCount > 0) {
          lastPage = Math.max(0, Math.min(p.lastPage, p.pageCount - 1))
        }
      })
      .catch(() => {})
    // The hero is the comic's own first page (usually its cover) at full size;
    // the shelf thumbnail is only a 256px placeholder.
    getPageRef(comic.id, 0)
      .then((r) => {
        if (!alive) return
        if (r?.url) hero = r.url
        else heroFailed = true
      })
      .catch(() => (heroFailed = true))
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        if (editing) closeEdit()
        else onExit()
      }
    }
    window.addEventListener('keydown', key)
    return () => {
      alive = false
      window.removeEventListener('keydown', key)
    }
  })

  const initials = $derived(
    title
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((w) => w[0])
      .join('')
  )

  const pctRead = $derived(pageCount > 0 ? Math.round(((lastPage + 1) / pageCount) * 100) : 0)

  function start() {
    onStart({ id: comic.id, title, pageCount, resume: lastPage })
  }

  // Clicking a tag filters the library by it and returns to the shelf.
  function pickTag(t: string) {
    onAddTag?.(t)
    onExit()
  }

  async function copySource() {
    if (!source) return
    const flash = () => {
      copied = true
      setTimeout(() => (copied = false), 1600)
    }
    try {
      if (await ClipboardSetText(source)) {
        flash()
        return
      }
    } catch {
      /* fall through to the browser clipboard */
    }
    try {
      await navigator.clipboard.writeText(source)
      flash()
    } catch {
      /* clipboard unavailable; nothing more we can do */
    }
  }

  // ---- metadata editing ----

  function openEdit() {
    fTitle = title
    fAuthor = author
    fTags = tags.join(', ')
    fSource = source
    saveErr = ''
    editing = true
  }

  function closeEdit() {
    if (busy) return
    editing = false
  }

  async function save() {
    if (busy) return
    busy = true
    saveErr = ''
    try {
      const d = await updateComicMetadata(comic.id, {
        title: fTitle.trim(),
        author: fAuthor.trim(),
        genre: fTags.trim(),
        source: fSource.trim(),
      })
      // Refresh the info screen from the saved comment. A title edit may have
      // renamed the file, but the id is stable so nothing else changes.
      const kv = d?.meta?.kv ?? {}
      title = kv['Title'] || comic.title
      author = kv['Author'] || ''
      tags = splitGenre(kv['Genre'] || '')
      source = kv['Source'] || ''
      editing = false
      ok('Metadata saved')
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      saveErr = msg
      error(msg)
    } finally {
      busy = false
    }
  }

  const startLabel = $derived(
    pageCount > 0 ? (lastPage > 0 ? `Continue · page ${lastPage + 1} of ${pageCount}` : `Start reading · ${pageCount} pages`) : 'Open'
  )
</script>

<div class="info">
  <div class="top">
    <button onclick={onExit} title="Back to library (Esc)">← Library</button>
    <button class="edit" onclick={openEdit} title="Edit title, author, tags and source">✎ Edit metadata</button>
  </div>

  <div class="body">
    <div class="cover">
      {#if hero && !heroFailed}
        <img
          src={hero}
          alt={title}
          draggable="false"
          decoding="async"
          class:ready={heroReady}
          onload={() => (heroReady = true)}
          onerror={() => (heroFailed = true)}
        />
        {#if !heroReady}<div class="ph">…</div>{/if}
      {:else}
        <div class="ph">{initials || '?'}</div>
      {/if}
    </div>

    <div class="meta">
      <h1 class="title" title={title}>{title}</h1>

      {#if author}
        <div class="author" title={author}>{author}</div>
      {/if}

      {#if pageCount > 0}
        <div class="sub">
          <span>{pageCount} pages</span>
          <span class="dot">·</span>
          {#if lastPage > 0}<span>{pctRead}% read</span>{:else}<span>not started</span>{/if}
        </div>
      {/if}

      {#if tags.length > 0}
        <div class="block">
          <div class="lbl">Tags</div>
          <div class="tags">
            {#each tags as t (t)}
              <button class="tag" type="button" title={`Show comics tagged ${t}`} onclick={() => pickTag(t)}>
                {t}
              </button>
            {/each}
          </div>
        </div>
      {/if}

      {#if source}
        <div class="block">
          <div class="lbl">Source</div>
          <div class="src">
            <code class="mag" title={source}>{source}</code>
            <button class="copy" onclick={copySource} disabled={copied} title="Copy the magnet link to the clipboard">
              {copied ? 'Copied ✓' : 'Copy'}
            </button>
          </div>
          <p class="hint">Magnet link from the original download — paste it into a torrent client to re-fetch the file.</p>
        </div>
      {/if}

      <button class="start" onclick={start} disabled={pageCount <= 0} title={pageCount > 0 ? 'Open the reader' : 'No pages yet'}>
        ▶ {startLabel}
      </button>
    </div>
  </div>

  {#if editing}
    <button
      class="backdrop"
      type="button"
      aria-label="Close metadata editor"
      tabindex="-1"
      onclick={closeEdit}
    ></button>
    <div class="modal" role="dialog" aria-label="Edit metadata">
      <h3>✎ Edit metadata</h3>
      <label>
        Title
        <input type="text" bind:value={fTitle} placeholder="Comic title" />
      </label>
      <label>
        Author
        <input type="text" bind:value={fAuthor} placeholder="Author / circle name" />
      </label>
      <label>
        Tags
        <input type="text" bind:value={fTags} placeholder="Comma-separated, e.g. comedy, romance" />
      </label>
      <label>
        Source
        <input type="text" bind:value={fSource} placeholder="Magnet link or source URL" />
      </label>
      {#if saveErr}<p class="err">{saveErr}</p>{/if}
      <div class="actions">
        <button type="button" onclick={closeEdit} disabled={busy}>Cancel</button>
        <button class="save" type="button" onclick={save} disabled={busy}>{busy ? 'Saving…' : 'Save'}</button>
      </div>
    </div>
  {/if}
</div>

<style>
  .info {
    position: fixed;
    inset: 0;
    background: var(--bg-soft);
    display: flex;
    flex-direction: column;
    z-index: 20;
  }
  .top {
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 16px;
    border-bottom: 1px solid var(--line);
    background: var(--bg-soft);
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: clamp(24px, 5vw, 56px);
    padding: 28px 34px;
  }
  .cover {
    flex: none;
    max-width: min(34vw, 340px);
    border-radius: var(--radius);
    border: 1px solid var(--line);
    background: var(--panel);
    overflow: hidden;
    box-shadow: 0 18px 50px rgba(0, 0, 0, 0.45);
    aspect-ratio: 3 / 4;
  }
  .cover img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .cover img.ready { opacity: 1; }
  .ph {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--panel);
    color: var(--text-dim);
    font-size: 40px;
    font-weight: 600;
  }
  .meta {
    flex: 1 1 460px;
    max-width: 680px;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;
  }
  .title {
    margin: 0;
    font-size: 24px;
    font-weight: 700;
    line-height: 1.25;
    word-break: break-word;
  }
  .author {
    color: var(--accent);
    font-size: 14px;
    word-break: break-word;
  }
  .sub {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--text-dim);
    font-size: 13px;
    margin-top: 2px;
  }
  .dot { opacity: 0.6; }
  .block {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-top: 16px;
    width: 100%;
  }
  .lbl {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-dim);
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .tag {
    font-size: 12px;
    line-height: 1;
    padding: 4px 8px;
    border-radius: 5px;
    background: var(--panel);
    border: 1px solid var(--line);
    color: var(--text);
    cursor: pointer;
  }
  .tag:hover {
    border-color: var(--accent);
    color: var(--accent);
  }
  .src {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .mag {
    flex: 1;
    min-width: 0;
    font-size: 12px;
    color: var(--text-dim);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    direction: ltr;
    padding: 6px 9px;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: 6px;
  }
  .copy {
    flex: none;
  }
  .copy:disabled {
    opacity: 1;
    border-color: var(--ok);
    color: var(--ok);
  }
  .hint {
    margin: 0;
    font-size: 12px;
    color: var(--text-dim);
  }
  .start {
    margin-top: 22px;
    padding: 10px 22px;
    font-size: 14px;
    font-weight: 600;
    background: var(--accent);
    color: #14100a;
    border: none;
    border-radius: 8px;
  }
  .start:hover:not(:disabled) {
    background: #ecb769;
    border: none;
  }
  .top .edit {
    margin-left: auto;
  }
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 30;
    padding: 0;
    background: rgba(0, 0, 0, 0.5);
    border: none;
    cursor: default;
  }
  .modal {
    position: fixed;
    left: 50%;
    top: 50%;
    transform: translate(-50%, -50%);
    z-index: 31;
    width: min(420px, 92vw);
    max-height: 85vh;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 18px;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 10px;
    box-shadow: 0 18px 50px rgba(0, 0, 0, 0.5);
  }
  .modal h3 {
    margin: 0;
    font-size: 15px;
  }
  .modal label {
    display: flex;
    flex-direction: column;
    gap: 5px;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-dim);
  }
  .modal input {
    width: 100%;
    box-sizing: border-box;
  }
  .err {
    margin: 0;
    font-size: 12px;
    color: #e6a08c;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 4px;
  }
  .save {
    font-weight: 600;
    background: var(--accent);
    color: #14100a;
    border: none;
  }
</style>
