<script lang="ts">
  import { onMount } from 'svelte'
  import {
    countComics,
    getAbout,
    getLayout,
    getSettings,
    listRules,
    on,
    pickLibrary,
    reloadLayout,
    removeLibrary,
    startScan,
    cancelScan,
  } from './lib/api'
  import type { About, ComicCard, LayoutDoc, RuleInfo, Settings } from './lib/api'
  import TopBar from './components/TopBar.svelte'
  import Welcome from './components/Welcome.svelte'
  import Section from './components/Section.svelte'
  import CardSlot from './components/CardSlot.svelte'
  import Reader from './components/Reader.svelte'
  import Toasts from './components/Toasts.svelte'
  import { error, ok } from './lib/toast'
  import * as Go from '../wailsjs/go/main/App'

  const logFront = (m: string) => {
    Go.ReportFrontend(m).catch(() => {})
  }

  interface ComicRef {
    id: number
    title: string
    pageCount: number
    resume: number
  }

  let settings: Settings | null = $state(null)
  let layout: LayoutDoc | null = $state(null)
  let filterRules: RuleInfo[] = $state([])
  let sortRules: RuleInfo[] = $state([])
  let scanning = $state(false)
  let scanProgress = $state<{ done: number; total: number } | null>(null)
  let refreshKey = $state(0)
  let indexed = $state(0)
  let reader: ComicRef | null = $state(null)
  let showAbout = $state(false)
  let about: About | null = $state(null)
  let winH = $state(0) // tracks window height so the grid can fill it (no dead space below)

  function bump() {
    refreshKey += 1
  }

  // keep the top-bar "N indexed" readout in sync with backend data changes
  $effect(() => {
    const k = refreshKey
    void k
    if (settings && (settings.libraryRoots ?? []).length > 0) {
      countComics('')
        .then((n) => (indexed = n))
        .catch(() => {})
    }
  })

  async function loadAll() {
    try {
      const [s, l, rules] = await Promise.all([getSettings(), getLayout(), listRules()])
      settings = s
      layout = l
      filterRules = rules.filter((r) => r.kind === 'filter')
      sortRules = rules.filter((r) => r.kind === 'sort')
    } catch {
      /* backend not ready */
    }
    // A restart with saved roots should repopulate the shelf even if nothing
    // was scanned in this run yet (incremental: unchanged archives are skipped).
    if ((settings?.libraryRoots ?? []).length > 0) {
      startScan().catch(() => {})
    }
  }

  onMount(() => {
    loadAll()
    const un1 = on('scan:started', () => (scanning = true))
    const un2 = on('scan:progress', (p: any) => {
      scanProgress = { done: p.done ?? 0, total: p.total ?? 0 }
    })
    const un3 = on('scan:done', () => {
      scanning = false
      scanProgress = null
      bump()
      getLayout().then((l) => (layout = l)).catch(() => {})
    })
    const un4 = on('layout:changed', () => {
      getLayout().then((l) => (layout = l)).catch(() => {})
    })
    const un5 = on('scan:error', (e: any) => {
      error(e?.path ? `Scan: ${e.path}: ${e.error}` : `Scan error: ${e?.error ?? ''}`)
    })
    const onErr = (e: any) => {
      const st = e?.error?.stack ? String(e.error.stack).split('\n').slice(0, 8).join(' < ') : ''
      logFront('window.error: ' + (e?.message ?? '') + ' STACK[' + st + ']')
    }
    const onRej = (e: any) =>
      logFront('unhandledrejection: ' + (e?.reason?.message ?? String(e?.reason ?? '')))
    window.addEventListener('error', onErr)
    window.addEventListener('unhandledrejection', onRej)
    const onResize = () => (winH = window.innerHeight)
    onResize()
    window.addEventListener('resize', onResize)
    return () => {
      un1()
      un2()
      un3()
      un4()
      un5()
      window.removeEventListener('error', onErr)
      window.removeEventListener('unhandledrejection', onRej)
      window.removeEventListener('resize', onResize)
    }
  })

  async function addFolder() {
    try {
      const s = await pickLibrary()
      if (s) settings = s
      bump()
    } catch (e: any) {
      error(e?.message ?? 'Could not add folder')
    }
  }
  function doScan() {
    startScan().catch((e: any) => error(e?.message ?? 'Could not start scan'))
  }
  function doCancel() {
    cancelScan()
  }
  async function doReloadLayout() {
    try {
      await reloadLayout()
      bump() // force section counts to re-query too
    } catch (e: any) {
      error(e?.message ?? 'Could not reload layout.json')
    }
  }

  async function openAbout() {
    try {
      about = await getAbout()
      showAbout = true
    } catch (e: any) {
      error(e?.message ?? 'Could not load About info')
    }
  }
  function closeAbout() {
    showAbout = false
  }
  async function removeRoot(dir: string) {
    const yes = window.confirm(`Remove "${dir}" from your library?\n\nThe files will not be deleted.`)
    if (!yes) return
    try {
      await removeLibrary(dir) // backend binding returns void, NOT settings
      settings = await getSettings() // re-read the authoritative roots
      bump()
      ok('Folder removed — its comics were unindexed')
    } catch (e: any) {
      error(e?.message ?? 'Could not remove folder')
    }
  }
  function openReader(card: ComicCard) {
    const count = Math.max(card.pageCount, 0)
    const resume = card.lastPage < count ? card.lastPage : Math.max(0, count - 1)
    reader = { id: card.id, title: card.title, pageCount: count, resume }
  }
  function closeReader() {
    reader = null
    bump() // refresh shelf (progress chips, "recently read" ordering)
  }
</script>

{#if reader}
  <Reader comic={reader} onExit={closeReader} />
{:else}
  <TopBar
    roots={settings?.libraryRoots ?? []}
    {scanning}
    progress={scanProgress}
    comics={indexed}
    onAdd={addFolder}
    onScan={doScan}
    onCancel={doCancel}
    onRemoveRoot={removeRoot}
    onAbout={openAbout}
    onReloadLayout={doReloadLayout}
  />

  {#if !settings || (settings.libraryRoots ?? []).length === 0}
    <Welcome onAdd={addFolder} />
  {:else if layout}
    <main class="shelf">
      {#each layout.sections as section (section.id)}
        {#if section.kind === 'grid'}
          <Section
            section={section}
            filterRules={filterRules}
            sortRules={sortRules}
            refreshKey={refreshKey}
            scanning={scanning}
            gridHeight={winH > 0 ? Math.max(300, winH - 90) : 600}
            onOpen={openReader}
            onChanged={bump}
          />
        {:else}
          <section class="section">
            <div class="head"><h2>{section.title || section.cardId}</h2></div>
            <CardSlot cardId={section.cardId || ''} refreshKey={refreshKey} />
          </section>
        {/if}
      {/each}
    </main>
  {/if}
{/if}

{#if showAbout && about}
  <div class="backdrop" onclick={closeAbout}>
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <h3>📚 ComicGo</h3>
      <p class="ver">Version {about.version}</p>
      <dl>
        <div class="row"><dt>Comics indexed</dt><dd>{about.comicCount}</dd></div>
        <div class="row"><dt>Media transport</dt><dd>{about.mediaTransport}</dd></div>
        <div class="row"><dt>Data folder</dt><dd class="mono">{about.configDir}</dd></div>
      </dl>
      <div class="actions">
        <button onclick={closeAbout}>Close</button>
      </div>
    </div>
  </div>
{/if}

<Toasts />

<style>
  .shelf {
    padding-bottom: 4px;
  }
  .section {
    margin: 14px 18px;
  }
  .head h2 {
    margin: 4px 0;
    font-size: 15px;
    font-weight: 600;
  }
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 50;
  }
  .modal {
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 10px;
    padding: 20px 24px;
    width: 420px;
    max-width: 90vw;
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.5);
  }
  .modal h3 { margin: 0 0 2px; }
  .ver { color: var(--text-dim); margin: 0 0 12px; font-size: 12px; }
  .modal dl { margin: 0 0 16px; display: flex; flex-direction: column; gap: 6px; }
  .row { display: flex; gap: 10px; font-size: 13px; }
  .row dt { color: var(--text-dim); width: 130px; flex: none; }
  .row dd { margin: 0; }
  .mono { font-family: Consolas, monospace; font-size: 12px; word-break: break-all; }
</style>
