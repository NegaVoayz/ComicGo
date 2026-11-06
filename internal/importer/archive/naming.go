package archive

import (
	"fmt"
	"sort"
	"strings"

	"comicgo/internal/comic"
)

// EntryName is the destination zip name for a one-based page rank.
func EntryName(rank int, ext string) string {
	return fmt.Sprintf("%06d%s", rank, strings.ToLower(ext))
}

type intQueue struct{ items []int }

func (q *intQueue) push(value int) { q.items = append(q.items, value) }

func (q *intQueue) pop() int {
	value := q.items[0]
	q.items = q.items[1:]
	return value
}

// rankQueues assigns natural-order ranks to duplicate archive entry names.
func rankQueues(names []string) map[string]*intQueue {
	sorted := append([]string(nil), names...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return comic.NaturalLess(sorted[i], sorted[j])
	})
	queues := make(map[string]*intQueue, len(sorted))
	for i, name := range sorted {
		queue := queues[name]
		if queue == nil {
			queue = &intQueue{}
			queues[name] = queue
		}
		queue.push(i + 1)
	}
	return queues
}
