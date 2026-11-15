// Package persistence holds the ConversationStore adapters: MongoDB for the service and an
// in-memory one for the tests, both checked by the same contract test.
package persistence

import (
	"context"
	"sort"
	"sync"

	"github.com/code-corhuila/market-agri-chat-api/internal/application/port/out"
	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

// Memory is a ConversationStore in memory, safe for concurrent use.
type Memory struct {
	mu            sync.Mutex
	conversations map[string]model.Conversation
	raceWinner    *model.Conversation
}

func NewMemory() *Memory { return &Memory{conversations: map[string]model.Conversation{}} }

// InsertAfterNextLookup simulates a concurrent open: c is stored right after the next lookup by
// (product, buyer), so the caller's insert loses the race.
func (m *Memory) InsertAfterNextLookup(c model.Conversation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.raceWinner = &c
}

func (m *Memory) Insert(_ context.Context, c model.Conversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.conversations {
		if existing.ProductID == c.ProductID && existing.BuyerID == c.BuyerID {
			return out.ErrDuplicate
		}
	}
	m.conversations[c.ID] = c
	return nil
}

func (m *Memory) FindByID(_ context.Context, id string) (*model.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conversations[id]; ok {
		return &c, nil
	}
	return nil, nil
}

func (m *Memory) FindByProductAndBuyer(_ context.Context, productID, buyerID string) (*model.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.conversations {
		if c.ProductID == productID && c.BuyerID == buyerID {
			return &c, nil
		}
	}
	if w := m.raceWinner; w != nil {
		m.raceWinner = nil
		m.conversations[w.ID] = *w
	}
	return nil, nil
}

func (m *Memory) ListByParticipant(_ context.Context, userID string, page, limit int) ([]model.Conversation, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var mine []model.Conversation
	for _, c := range m.conversations {
		if c.BuyerID == userID || c.ProducerID == userID {
			mine = append(mine, c)
		}
	}
	sort.Slice(mine, func(i, j int) bool { return listedBefore(mine[i], mine[j]) })
	from := (page - 1) * limit
	if from >= len(mine) {
		return []model.Conversation{}, len(mine), nil
	}
	to := min(from+limit, len(mine))
	return mine[from:to], len(mine), nil
}

func (m *Memory) SetPurchaseMethod(_ context.Context, id string, method model.PurchaseMethod) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conversations[id]; ok {
		c.ChoosePurchaseMethod(method)
		m.conversations[id] = c
	}
	return nil
}

// listedBefore is the order of E-31: conversations with messages first, by lastMessageAt
// descending; then those without, by createdAt descending; then by id.
func listedBefore(a, b model.Conversation) bool {
	switch {
	case a.LastMessageAt != nil && b.LastMessageAt == nil:
		return true
	case a.LastMessageAt == nil && b.LastMessageAt != nil:
		return false
	case a.LastMessageAt != nil && !a.LastMessageAt.Equal(*b.LastMessageAt):
		return a.LastMessageAt.After(*b.LastMessageAt)
	case !a.CreatedAt.Equal(b.CreatedAt):
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID < b.ID
}
