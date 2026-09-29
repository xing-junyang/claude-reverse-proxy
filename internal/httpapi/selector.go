package httpapi

import (
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/local/claude-relay/internal/store"
)

type accountSelector struct {
	mu       sync.Mutex
	inFlight map[int64]int
	random   *rand.Rand
}

func newAccountSelector() *accountSelector {
	return &accountSelector{
		inFlight: make(map[int64]int),
		random:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *accountSelector) acquire(candidates []store.AccountSecret, strategy string, excluded map[int64]bool) (store.AccountSecret, func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	available := make([]store.AccountSecret, 0, len(candidates))
	for _, candidate := range candidates {
		if !excluded[candidate.ID] {
			available = append(available, candidate)
		}
	}
	if len(available) == 0 {
		return store.AccountSecret{}, func() {}, false
	}

	var selected store.AccountSecret
	if strategy == "weighted" {
		selected = s.weighted(available)
	} else {
		sort.SliceStable(available, func(i, j int) bool {
			left, right := available[i], available[j]
			if strategy == "priority" && left.Priority != right.Priority {
				return left.Priority > right.Priority
			}
			leftFlight, rightFlight := s.inFlight[left.ID], s.inFlight[right.ID]
			if leftFlight != rightFlight {
				return leftFlight < rightFlight
			}
			if strategy != "priority" && left.Priority != right.Priority {
				return left.Priority > right.Priority
			}
			return accountLastUsed(left).Before(accountLastUsed(right))
		})
		selected = available[0]
	}
	s.inFlight[selected.ID]++
	once := sync.Once{}
	release := func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.inFlight[selected.ID] <= 1 {
				delete(s.inFlight, selected.ID)
			} else {
				s.inFlight[selected.ID]--
			}
		})
	}
	return selected, release, true
}

func (s *accountSelector) weighted(candidates []store.AccountSecret) store.AccountSecret {
	total := 0.0
	weights := make([]float64, len(candidates))
	for i, candidate := range candidates {
		weight := float64(max(candidate.Weight, 1))
		if candidate.Priority > 0 {
			weight *= 1 + float64(candidate.Priority)*0.25
		}
		weight /= float64(1 + s.inFlight[candidate.ID])
		weights[i] = weight
		total += weight
	}
	pick := s.random.Float64() * total
	for i, weight := range weights {
		pick -= weight
		if pick <= 0 {
			return candidates[i]
		}
	}
	return candidates[len(candidates)-1]
}

func accountLastUsed(account store.AccountSecret) time.Time {
	if account.LastUsedAt == nil {
		return time.Unix(0, 0)
	}
	return *account.LastUsedAt
}
