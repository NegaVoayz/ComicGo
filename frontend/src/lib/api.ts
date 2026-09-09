// Thin typed wrappers around the generated Wails bindings + runtime events.
import * as Go from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

export interface PageQuery {
  offset: number
  limit: number
  filterRuleId: string
  sortRuleId: string
  search: string
}

export interface ComicCard {
  id: number
  filename: string
  title: string
  author: string
  tags: string[]
  pageCount: number
  fav: boolean
  lastPage: number
  hasNonUtf8: boolean
  coverUrl: string
}

// The parts of the backend's ComicDetail the info screen reads. meta.kv holds
// every "key=value" comment line (Author, Genre, Source, Title, ...).
export interface ComicDetailMeta {
  id: number
  filename: string
  pageCount: number
  lastReadPage: number
  kv: Record<string, string>
}
export interface ComicDetail {
  meta: ComicDetailMeta
  commentRaw: string
  allMetaKeys: string[]
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

export interface About {
  version: string
  configDir: string
  mediaTransport: string
  comicCount: number
}

export interface ScanProgress {
  done: number
  total: number
  path: string
}

// Payload of the "import:progress" event (emitted while an import converts
// pages; total is 0 when the page count wasn't known up front, e.g. rar).
export interface ImportProgress {
  done: number
  total: number
  name: string // destination base name, e.g. "Vol1.zip"
}

export interface ImportOutcome {
  path: string
  pageCount: number
  source: string
  kind: 'file' | 'folder'
}

// The editable subset of a comic's comment metadata, posted from the info
// screen's edit form. An empty field clears that key.
export interface MetaEdit {
  title: string
  author: string
  genre: string // comma-separated tags
  source: string
}

// -- backend wrappers --
export const listComics = (q: PageQuery) => Go.GetComics(q)
export const countComics = (filter: string) => Go.CountComics(filter)
export const countMatching = (search: string, filter: string) => Go.CountMatching(search, filter)
export const listRules = () => Go.ListRules()
export const getSettings = () => Go.GetSettings()
export const getLayout = () => Go.GetLayout()
export const reloadLayout = () => Go.ReloadLayout()
export const saveLayout = (d: LayoutDoc) => Go.SaveLayout(d)
export const updateSectionRule = (sectionId: string, kind: string, ruleId: string) =>
  Go.UpdateSectionRule(sectionId, kind, ruleId)
export const getComicDetail = (id: number) => Go.GetComicDetail(id) as unknown as Promise<ComicDetail>
export const updateComicMetadata = (id: number, edit: MetaEdit) =>
  Go.UpdateComicMetadata(id, edit) as unknown as Promise<ComicDetail>
export const getCover = (id: number) => Go.GetCover(id)
export const getPageRef = (id: number, page: number) => Go.GetPageRef(id, page)
export const openComic = (id: number) => Go.OpenComic(id)
export const getProgress = (id: number) => Go.GetProgress(id)
export const setProgress = (id: number, page: number, pageCount: number) => Go.SetProgress(id, page, pageCount)
export const toggleFavorite = (id: number) => Go.ToggleFavorite(id)
export const addLibrary = (dir: string) => Go.AddLibrary(dir)
export const removeLibrary = (dir: string) => Go.RemoveLibrary(dir)
export const pickLibrary = () => Go.PickLibrary()
// Import entry points. A cancelled dialog resolves to an outcome with an empty
// path (no error); a drop batch resolves to the successfully imported ones.
export const importArchiveFile = () => Go.ImportArchive() as unknown as Promise<ImportOutcome>
export const importImageFolder = () => Go.ImportFolder() as unknown as Promise<ImportOutcome>
export const importDropped = (paths: string[]) => Go.ImportDropped(paths) as unknown as Promise<ImportOutcome[]>
export const getAbout = () => Go.GetAbout()
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
