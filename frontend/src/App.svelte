<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getLayout,
    getSettings,
    listRules,
    on,
    pickLibrary,
    reloadLayout,
    startScan,
    cancelScan,
  } from './lib/api'
  import type { ComicCard, LayoutDoc, RuleInfo, Settings } from './lib/api'
  import TopBar from './components/TopBar.svelte'
  import Welcome from './components/Welcome.svelte'
  import Section from './components/Section.svelte'
  import CardSlot from './components/CardSlot.svelte'
  import Reader from './components/Reader.svelte'
  import Toasts from './components/Toasts.svelte'
  import { error } from './lib/toast'

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
  let reader: ComicRef | null = $state(null)

  function bump() {
    refreshKey += 1
  }

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
    return () => {
      un1()
      un2()
      un3()
      un4()
      un5()
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
    } catch (e: any) {
      error(e?.message ?? 'Could not reload layout.json')
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
    onAdd={addFolder}
    onScan={doScan}
    onCancel={doCancel}
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
<Toasts />

<style>
  .shelf {
    padding-bottom: 30px;
  }
  .section {
    margin: 14px 18px;
  }
  .head h2 {
    margin: 4px 0;
    font-size: 15px;
    font-weight: 600;
  }
</style>
