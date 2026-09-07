// Thin typed wrappers around the generated Wails bindings + runtime events.
import * as Go from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

export interface PageQuery {
  offset: number
  limit: number
  filterRuleId: string
  sortRuleId: string
}

export interface ComicCard {
  id: number
  filename: string
  title: string
  pageCount: number
  fav: boolean
  lastPage: number
  hasNonUtf8: boolean
  coverUrl: string
}

export interface RuleInfo {
  id: string
  label: string
  kind: 'filter' | 'sort'
}

export interface LayoutSection {
  id: string
  title: string
  kind: 'grid' | 'card'
  cardId?: string
  filterRuleId?: string
  sortRuleId?: string
  pageSize?: number
}

export interface LayoutDoc {
  version: number
  sections: LayoutSection[]
}

export interface Settings {
  libraryRoots: string[]
  mediaTransport: string
  schemaVersion: number
}

export interface ScanProgress {
  done: number
  total: number
  path: string
}

// -- backend wrappers --
export const listComics = (q: PageQuery) => Go.GetComics(q)
export const countComics = (filter: string) => Go.CountComics(filter)
export const listRules = () => Go.ListRules()
export const getSettings = () => Go.GetSettings()
export const getLayout = () => Go.GetLayout()
export const reloadLayout = () => Go.ReloadLayout()
export const saveLayout = (d: LayoutDoc) => Go.SaveLayout(d)
export const updateSectionRule = (sectionId: string, kind: string, ruleId: string) =>
  Go.UpdateSectionRule(sectionId, kind, ruleId)
export const getCover = (id: number) => Go.GetCover(id)
export const getPageRef = (id: number, page: number) => Go.GetPageRef(id, page)
export const openComic = (id: number) => Go.OpenComic(id)
export const getProgress = (id: number) => Go.GetProgress(id)
export const setProgress = (id: number, page: number, pageCount: number) => Go.SetProgress(id, page, pageCount)
export const toggleFavorite = (id: number) => Go.ToggleFavorite(id)
export const addLibrary = (dir: string) => Go.AddLibrary(dir)
export const pickLibrary = () => Go.PickLibrary()
export const startScan = () => Go.StartScan()
export const cancelScan = () => Go.CancelScan()
export const isScanning = () => Go.IsScanning()
export const cardData = (id: string) => Go.CardData(id)

// -- events --
export function on(event: string, cb: (...data: any[]) => void): () => void {
  return EventsOn(event, cb)
}
export function off(event: string) {
  EventsOff(event)
}
