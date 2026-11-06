package archive

import "testing"

func TestRankQueuesDuplicates(t *testing.T) {
	queues := rankQueues([]string{"b.jpg", "a.jpg", "a.jpg"})
	if got := queues["a.jpg"].pop(); got != 1 {
		t.Errorf("first a.jpg rank = %d, want 1", got)
	}
	if got := queues["a.jpg"].pop(); got != 2 {
		t.Errorf("second a.jpg rank = %d, want 2", got)
	}
	if got := queues["b.jpg"].pop(); got != 3 {
		t.Errorf("b.jpg rank = %d, want 3", got)
	}
}
