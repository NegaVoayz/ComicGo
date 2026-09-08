<script lang="ts">
  // A single grid section from layout.json: header + filter/sort rule pickers
  // + a virtualized grid.
  import { countMatching, listComics, updateSectionRule } from '../lib/api'
  import type { ComicCard, LayoutSection, RuleInfo } from '../lib/api'
  import VirtualGrid from './VirtualGrid.svelte'
  import * as Go from '../../wailsjs/go/main/App'

  const report = (m: string) => {
    Go.ReportFrontend(m).catch(() => {})
  }

  let {
    section,
    filterRules,
    sortRules,
    refreshKey,
    scanning = false,
    searchText = '',
    gridHeight = 600,
    tags = [],
    onOpen,
    onSearchChange,
    onAddTag,
    onRemoveTag,
    onClearTags,
    onChanged,
  }: {
    section: LayoutSection
    filterRules: RuleInfo[]
    sortRules: RuleInfo[]
    refreshKey: number
    scanning?: boolean
    searchText?: string
    gridHeight?: number
    tags?: string[]
    onOpen: (c: ComicCard) => void
    onSearchChange: (q: string) => void
    onAddTag: (tag: string) => void
    onRemoveTag: (tag: string) => void
    onClearTags: () => void
    onChanged: () => void
  } = $props()

  let total = $state(0)

  // The search box shows only the raw text; the queries the backend sees append
  // each active tag as a quoted "tag:..." term. Count and list go through this
  // same query, and every Section shares it (still scoped to its own filter).
  const query = $derived(
    [searchText.trim(), ...tags.map((t) => `tag:"${t.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`)]
      .filter(Boolean)
      .join(' ')
  )

  // Refetch the count whenever the search/filter/scan/refresh change.
  $effect(() => {
    void refreshKey
    void scanning
    void query
    countMatching(query, section.filterRuleId || '')
      .then((n) => {
        total = n
        report(`Section(${section.id}) count resolved -> ${n} scanning=${scanning} rk=${refreshKey}`)
      })
      .catch(() => {
        total = 0
        report(`Section(${section.id}) countMatching rejected`)
      })
  })

  async function load(offset: number, limit: number): Promise<ComicCard[]> {
    const cards = await listComics({
      offset,
      limit,
      filterRuleId: section.filterRuleId || '',
      sortRuleId: section.sortRuleId || '',
      search: query,
    })
    return cards as unknown as ComicCard[]
  }

  function chooseFilter(event: Event) {
    const id = (event.target as HTMLSelectElement).value
    updateSectionRule(section.id, 'filter', id).then(onChanged).catch(() => {})
  }
  function chooseSort(event: Event) {
    const id = (event.target as HTMLSelectElement).value
    updateSectionRule(section.id, 'sort', id).then(onChanged).catch(() => {})
  }
</script>

<section class="section">
  <div class="head">
    <div class="left">
      <h2>{section.title}</h2>
      <span class="count">{total}</span>
    </div>
    <input
      class="search"
      type="search"
      placeholder="Search this section…"
      value={searchText}
      oninput={(e) => onSearchChange((e.target as HTMLInputElement).value)}
    />
    <div class="tools">
      <select value={section.filterRuleId || ''} onchange={chooseFilter} title="Filter">
        <option value="">All</option>
        {#each filterRules as r (r.id)}
          <option value={r.id}>{r.label}</option>
        {/each}
      </select>
      <select value={section.sortRuleId || ''} onchange={chooseSort} title="Sort">
        {#each sortRules as r (r.id)}
          <option value={r.id}>{r.label}</option>
        {/each}
      </select>
    </div>
  </div>
  {#if tags.length > 0}
    <div class="tagbar">
      <span class="tb-label">Tags</span>
      {#each tags as t (t)}
        <button class="chip" type="button" title="Remove this tag filter" onclick={() => onRemoveTag(t)}>
          {t}<span class="x">✕</span>
        </button>
      {/each}
      <button class="clear" type="button" onclick={onClearTags}>Clear</button>
    </div>
  {/if}
  {#if scanning && total === 0}
    <div class="hint">Scanning your library…</div>
  {:else}
    {#key `${total}|${query}|${section.filterRuleId || ''}|${section.sortRuleId || ''}`}
      <!-- remount only when this section's dataset really changes; favorite
           toggles and progress updates must NOT reload every cover -->
      <VirtualGrid
        total={total}
        {load}
        {refreshKey}
        height={gridHeight}
        onOpen={onOpen}
        {onAddTag}
        onChanged={onChanged}
      />
    {/key}
  {/if}
</section>

<style>
  .section {
    margin: 14px 18px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 18px; /* equal spacing between the left, middle and right blocks */
  }
  .left {
    display: flex;
    align-items: baseline;
    gap: 10px;
    white-space: nowrap;
    flex: none;
  }
  h2 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }
  .count {
    color: var(--text-dim);
    font-size: 12px;
  }
  .search {
    flex: 1 1 auto; /* stretch as wide as possible between left and right */
    min-width: 140px;
  }
  .tools {
    display: flex;
    gap: 6px;
    white-space: nowrap;
    flex: none;
  }
  .hint {
    padding: 40px 20px;
    color: var(--text-dim);
    text-align: center;
    background: var(--bg);
    border-radius: var(--radius);
  }
  .tagbar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
  }
  .tb-label {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-dim);
    margin-right: 2px;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 8px 3px 10px;
    border: 1px solid var(--accent);
    border-radius: 12px;
    background: var(--accent-soft);
    color: var(--accent);
    font-size: 12px;
  }
  .chip:hover {
    border-color: var(--fav);
    color: var(--fav);
  }
  .chip .x {
    font-size: 10px;
    line-height: 1;
  }
  .clear {
    padding: 3px 8px;
    font-size: 12px;
    color: var(--text-dim);
    background: none;
    border: none;
  }
  .clear:hover {
    color: var(--text);
  }
</style>
