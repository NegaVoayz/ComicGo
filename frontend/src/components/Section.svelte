<script lang="ts">
  // A single grid section from layout.json: header + filter/sort rule pickers
  // + a virtualized grid.
  import { countComics, listComics, updateSectionRule } from '../lib/api'
  import type { ComicCard, LayoutSection, RuleInfo } from '../lib/api'
  import VirtualGrid from './VirtualGrid.svelte'

  let {
    section,
    filterRules,
    sortRules,
    refreshKey,
    gridHeight = 600,
    onOpen,
    onChanged,
  }: {
    section: LayoutSection
    filterRules: RuleInfo[]
    sortRules: RuleInfo[]
    refreshKey: number
    gridHeight?: number
    onOpen: (c: ComicCard) => void
    onChanged: () => void
  } = $props()

  let total = $state(0)

  $effect(() => {
    countComics(section.filterRuleId || '')
      .then((n) => (total = n))
      .catch(() => (total = 0))
  })

  async function load(offset: number, limit: number): Promise<ComicCard[]> {
    const cards = await listComics({
      offset,
      limit,
      filterRuleId: section.filterRuleId || '',
      sortRuleId: section.sortRuleId || '',
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
    <h2>{section.title}</h2>
    <span class="count">{total}</span>
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
  <VirtualGrid
    total={total}
    {load}
    {refreshKey}
    height={gridHeight}
    onOpen={onOpen}
    onChanged={onChanged}
  />
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
    gap: 8px;
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
  .tools {
    margin-left: auto;
    display: flex;
    gap: 6px;
  }
</style>
