<script lang="ts">
  // Renders one plugin-card layout section: loads the registered Svelte
  // component for the card id and the backend CardData payload.
  import type { Component } from 'svelte'
  import { cardData } from '../lib/api'
  import { loadCard } from '../lib/pluginRegistry'

  let { cardId, refreshKey }: { cardId: string; refreshKey: number } = $props()

  let comp: Component | null = $state(null)
  let data: Record<string, any> | null = $state(null)
  let missing = $state(false)

  $effect(() => {
    void refreshKey // re-fetch when the data backing the card changes
    const id = cardId
    missing = false
    data = null
    loadCard(id).then((c) => {
      comp = c
      if (!c) missing = true
    })
    cardData(id)
      .then((d) => (data = d))
      .catch(() => {})
  })
</script>

{#if missing}
  <div class="note">Unknown card “{cardId}” — check plugins/registry.go.</div>
{:else if comp}
  <div class="cardwrap">
    <svelte:component this={comp} {data} />
  </div>
{:else}
  <div class="note">Loading card…</div>
{/if}

<style>
  .cardwrap { padding: 4px 2px; }
  .note { color: var(--text-dim); padding: 12px; text-align: center; }
</style>
