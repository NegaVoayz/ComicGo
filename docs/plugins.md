# Writing plugins

Plugins are **compile-time Go packages**. There is no scripting host and no
plugin ABI: you write a package that implements one of the small interfaces,
import it once in `internal/plugins/registry.go`, and rebuild.

A rule sees a read-only `model.Meta` (fields + the parsed `key=value` comment
map `KV`). It never touches the database.

## 1. A sort rule

Sort rules must define a **strict weak ordering** (`Less`), same contract as
`sort.Slice`, so pagination is deterministic.

```go
// internal/plugins/builtin/byvol/byvol.go
package byvol

import (
	"strconv"
	"strings"

	"comicgo/internal/model"
)

// ByVolume sorts by a numeric "Vol" comment key, newest volume first.
type ByVolume struct{}

func (ByVolume) ID() string    { return "byvolume" }
func (ByVolume) Label() string { return "Volume (desc)" }

func (ByVolume) Less(a, b model.Meta) bool {
	av, _ := strconv.Atoi(strings.TrimSpace(a.KV["Vol"]))
	bv, _ := strconv.Atoi(strings.TrimSpace(b.KV["Vol"]))
	if av != bv {
		return av > bv
	}
	return a.ID < b.ID // total order tie-break
}
```

## 2. A filter rule

```go
// internal/plugins/builtin/bygenre/bygenre.go
package bygenre

import (
	"strings"

	"comicgo/internal/model"
)

// Genre keeps comics whose comment "Genre" contains the given tag.
// The tag is set at construction, so instantiate multiple variants.
type Genre struct{ Tag string }

func (g Genre) ID() string    { return "genre:" + g.Tag }
func (g Genre) Label() string { return "Genre: " + g.Tag }
func (g Genre) Keep(m model.Meta) bool {
	return strings.Contains(strings.ToLower(m.KV["Genre"]), strings.ToLower(g.Tag))
}
```

## 3. A card

A card declares a `Type` string; the frontend maps that type to a Svelte
component. Dynamic payloads (counts, etc.) come from the app's `CardData`
binding keyed by the card id — the `Card.Render()` spec is static.

```go
package mycard

import "comicgo/internal/model"

type Card struct{}

func (Card) ID() string { return "mycard" }
func (Card) Render() model.CardSpec {
	return model.CardSpec{Type: "mycard"}
}
```

Frontend side:

```ts
// frontend/src/lib/pluginRegistry.ts
loaders["mycard"] = () => import('../components/MyCard.svelte')
```

```svelte
<!-- frontend/src/components/MyCard.svelte -->
<script lang="ts">
  let { data }: { data?: Record<string, any> } = $props()
</script>
<p>my card: {JSON.stringify(data)}</p>
```

## Wiring

`internal/plugins/registry.go` is the only file you edit to add a plugin —
import the package, append it to the list:

```go
import (
	// ...existing...
	"comicgo/internal/plugins/builtin/bygenre"
	"comicgo/internal/plugins/builtin/byvol"
	"comicgo/internal/plugins/builtin/mycard"
)

var FilterRules = []FilterRule{
	// ...existing...
	bygenre.Genre{Tag: "Romance"},
}
var SortRules = []SortRule{
	// ...existing...
	byvol.ByVolume{},
}
var Cards = []Card{
	// ...existing...
	mycard.Card{},
}
```

Rebuild with `wails dev` or `wails build`. The new rule appears in every grid
section's dropdown; a new card id can be placed into `layout.json` as a
`kind: "card"` section.

## Conventions

- **`ID()` must be stable forever.** It is stored in `layout.json`. Change the
  label, never the id.
- The built-in ids are `bytitle`, `bytitle-desc`, `bymodtime`, `bylastread`
  (sorts) and `favorites`, `unread`, `inprogress` (filters) and `stats` (card).
- `model.Meta.Val(key)` returns the comment value or `""`; `Meta.Title()`
  prefers the `Title` key and falls back to the filename.
