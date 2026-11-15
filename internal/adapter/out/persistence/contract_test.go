package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/code-corhuila/market-agri-chat-api/internal/application/port/out"
	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

// storeContract is what every ConversationStore must do; the in-memory store and the MongoDB store
// (integration test) run the same cases, so the fake used by the HTTP tests cannot drift. Ids and
// the producer are not those of the seeds, so the cases also run on the database of dev.
func storeContract(t *testing.T, newStore func(t *testing.T) out.ConversationStore) {
	ctx := context.Background()
	base := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	product := func(id string) model.ProductSnapshot {
		return model.ProductSnapshot{ID: id, Name: "Café", ProducerID: "e5555555-5555-4555-8555-555555555555", ProducerName: "Ana Ruiz"}
	}
	open := func(id, productID, buyer string, at time.Time) model.Conversation {
		c, err := model.Open(id, product(productID), buyer, "Carlos Pérez", at)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	const buyer = "6a1d9b2c-3e4f-4a5b-8c6d-7e8f9a0b1c2d"

	t.Run("insert, find by id and by (product, buyer)", func(t *testing.T) {
		s := newStore(t)
		c := open("a1111111-1111-4111-8111-111111111111", "b1111111-1111-4111-8111-111111111111", buyer, base)
		if err := s.Insert(ctx, c); err != nil {
			t.Fatal(err)
		}
		got, err := s.FindByID(ctx, c.ID)
		if err != nil || got == nil || got.ProducerName != "Ana Ruiz" || !got.CreatedAt.Equal(base) || got.AgreedPurchaseMethod != nil {
			t.Fatalf("by id: %+v %v", got, err)
		}
		got, err = s.FindByProductAndBuyer(ctx, c.ProductID, buyer)
		if err != nil || got == nil || got.ID != c.ID {
			t.Fatalf("by pair: %+v %v", got, err)
		}
		if got, err := s.FindByID(ctx, "c1111111-1111-4111-8111-111111111111"); err != nil || got != nil {
			t.Fatalf("missing must be (nil, nil): %+v %v", got, err)
		}
	})

	t.Run("a second conversation for the same pair is ErrDuplicate", func(t *testing.T) {
		s := newStore(t)
		p := "b2222222-2222-4222-8222-222222222222"
		if err := s.Insert(ctx, open("a2222222-2222-4222-8222-222222222222", p, buyer, base)); err != nil {
			t.Fatal(err)
		}
		err := s.Insert(ctx, open("a3333333-3333-4333-8333-333333333333", p, buyer, base))
		if !errors.Is(err, out.ErrDuplicate) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("list of a participant: with messages by lastMessageAt desc, then by createdAt desc, then id", func(t *testing.T) {
		s := newStore(t)
		u := "d4444444-4444-4444-8444-444444444444"
		older := open("a0000001-0000-4000-8000-000000000001", "b0000001-0000-4000-8000-000000000001", u, base)
		newer := open("a0000002-0000-4000-8000-000000000002", "b0000002-0000-4000-8000-000000000002", u, base.Add(time.Hour))
		talked := open("a0000003-0000-4000-8000-000000000003", "b0000003-0000-4000-8000-000000000003", u, base.Add(-time.Hour))
		at := base.Add(-30 * time.Minute)
		talked.LastMessageAt = &at
		other := open("a0000004-0000-4000-8000-000000000004", "b0000004-0000-4000-8000-000000000004", buyer, base)
		for _, c := range []model.Conversation{older, newer, talked, other} {
			if err := s.Insert(ctx, c); err != nil {
				t.Fatal(err)
			}
		}
		items, total, err := s.ListByParticipant(ctx, u, 1, 2)
		if err != nil || total != 3 || len(items) != 2 || items[0].ID != talked.ID || items[1].ID != newer.ID {
			t.Fatalf("page 1: total=%d %v %v", total, ids(items), err)
		}
		items, _, _ = s.ListByParticipant(ctx, u, 2, 2)
		if len(items) != 1 || items[0].ID != older.ID {
			t.Fatalf("page 2: %v", ids(items))
		}
		items, total, _ = s.ListByParticipant(ctx, "e5555555-5555-4555-8555-555555555555", 1, 20)
		if total != 4 || len(items) != 4 {
			t.Fatalf("the producer takes part in all of them: %d", total)
		}
		items, total, _ = s.ListByParticipant(ctx, u, 9, 20)
		if total != 3 || len(items) != 0 {
			t.Fatalf("a page past the last is empty with the real total: %d %d", total, len(items))
		}
	})
}

func ids(cs []model.Conversation) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func TestMemoryStore(t *testing.T) {
	storeContract(t, func(*testing.T) out.ConversationStore { return NewMemory() })
}
