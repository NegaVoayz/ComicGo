// Minimal toast notifications backed by a svelte store.
import { writable } from 'svelte/store'

export type ToastKind = 'info' | 'ok' | 'error'

export interface Toast {
  id: number
  text: string
  kind: ToastKind
}

export const toasts = writable<Toast[]>([])

let nextId = 1

export function push(text: string, kind: ToastKind = 'info') {
  const id = nextId++
  toasts.update((t) => [...t, { id, text, kind }])
  const ms = kind === 'error' ? 6000 : 3200
  setTimeout(() => dismiss(id), ms)
}

export const info = (text: string) => push(text, 'info')
export const ok = (text: string) => push(text, 'ok')
export const error = (text: string) => push(text, 'error')

export function dismiss(id: number) {
  toasts.update((t) => t.filter((x) => x.id !== id))
}
