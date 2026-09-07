import type { Component } from 'svelte'

// Maps a plugin card id (as declared by a Go plugin in plugins/registry.go)
// to the Svelte component that renders it. Adding a card plugin = one entry
// here (frontend side) + one import in internal/plugins/registry.go (backend).
const loaders: Record<string, () => Promise<{ default: Component }>> = {
  stats: () => import('../components/StatsCard.svelte'),
}

export async function loadCard(id: string): Promise<Component | null> {
  const loader = loaders[id]
  if (!loader) return null
  const mod = await loader()
  return mod.default
}

export function hasCard(id: string): boolean {
  return id in loaders
}
