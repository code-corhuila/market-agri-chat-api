package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/code-corhuila/market-agri-chat-api/internal/adapter/out/persistence"
	"github.com/code-corhuila/market-agri-chat-api/internal/application/port/in"
	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

const buyerID = "6a1d9b2c-3e4f-4a5b-8c6d-7e8f9a0b1c2d"

var mora = model.ProductSnapshot{
	ID: "5e1f0c2a-7b3d-4e8f-9a6b-000000000021", Name: "Mora de Castilla",
	ProducerID: "0b6f2c7e-4a51-4c1e-9d0e-2f1c3a7b9e10", ProducerName: "Ana Ruiz",
}

// fakeCatalog answers I-02 from a map and counts the calls.
type fakeCatalog struct {
	products map[string]model.ProductSnapshot
	err      error
	calls    int
}

func (f *fakeCatalog) Product(_ context.Context, id string) (model.ProductSnapshot, error) {
	f.calls++
	if f.err != nil {
		return model.ProductSnapshot{}, f.err
	}
	p, ok := f.products[id]
	if !ok {
		return model.ProductSnapshot{}, model.ErrProductNotFound
	}
	return p, nil
}

func newService(catalog *fakeCatalog) (*Conversations, *persistence.Memory) {
	store := persistence.NewMemory()
	ids := 0
	return &Conversations{
		Store:   store,
		Catalog: catalog,
		NewID: func() string {
			ids++
			return []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}[ids-1]
		},
		Now: func() time.Time { return time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC) },
	}, store
}

var carlos = in.Buyer{ID: buyerID, Name: "Carlos Pérez"}

func TestOpenCreatesFromTheProduct(t *testing.T) {
	catalog := &fakeCatalog{products: map[string]model.ProductSnapshot{mora.ID: mora}}
	svc, _ := newService(catalog)
	c, created, err := svc.Open(context.Background(), carlos, mora.ID)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if c.ID != "11111111-1111-4111-8111-111111111111" || c.ProducerName != "Ana Ruiz" || c.BuyerName != "Carlos Pérez" {
		t.Fatalf("got %+v", c)
	}
}

func TestOpenTwiceReturnsTheSameWithoutCallingCatalog(t *testing.T) {
	catalog := &fakeCatalog{products: map[string]model.ProductSnapshot{mora.ID: mora}}
	svc, _ := newService(catalog)
	first, _, _ := svc.Open(context.Background(), carlos, mora.ID)
	again, created, err := svc.Open(context.Background(), carlos, mora.ID)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("created=%v err=%v id=%s", created, err, again.ID)
	}
	if catalog.calls != 1 {
		t.Fatalf("catalog called %d times; the second open must not call it", catalog.calls)
	}
}

func TestOpenLosingTheRaceAnswersTheWinner(t *testing.T) {
	catalog := &fakeCatalog{products: map[string]model.ProductSnapshot{mora.ID: mora}}
	svc, store := newService(catalog)
	winner, _ := model.Open("99999999-9999-4999-8999-999999999999", mora, buyerID, "Carlos Pérez", time.Now())
	store.InsertAfterNextLookup(winner) // another request inserts between our lookup and our insert
	c, created, err := svc.Open(context.Background(), carlos, mora.ID)
	if err != nil || created || c.ID != winner.ID {
		t.Fatalf("created=%v err=%v id=%s", created, err, c.ID)
	}
}

func TestOpenErrors(t *testing.T) {
	svc, _ := newService(&fakeCatalog{products: map[string]model.ProductSnapshot{}})
	if _, _, err := svc.Open(context.Background(), carlos, mora.ID); !errors.Is(err, model.ErrProductNotFound) {
		t.Fatalf("missing product: %v", err)
	}
	svc, _ = newService(&fakeCatalog{err: model.ErrCatalogUnavailable})
	if _, _, err := svc.Open(context.Background(), carlos, mora.ID); !errors.Is(err, model.ErrCatalogUnavailable) {
		t.Fatalf("catalog down: %v", err)
	}
}

func TestGetChecksExistenceBeforeParticipation(t *testing.T) {
	svc, _ := newService(&fakeCatalog{products: map[string]model.ProductSnapshot{mora.ID: mora}})
	c, _, _ := svc.Open(context.Background(), carlos, mora.ID)
	if _, err := svc.Get(context.Background(), mora.ProducerID, c.ID); err != nil {
		t.Fatalf("the producer reads it: %v", err)
	}
	if _, err := svc.Get(context.Background(), "x", c.ID); !errors.Is(err, model.ErrNotAParticipant) {
		t.Fatalf("stranger: %v", err)
	}
	if _, err := svc.Get(context.Background(), "x", "missing"); !errors.Is(err, model.ErrConversationNotFound) {
		t.Fatalf("missing must be 404 even for a stranger: %v", err)
	}
	if _, err := svc.GetForService(context.Background(), c.ID); err != nil {
		t.Fatalf("I-05: %v", err)
	}
	if _, err := svc.GetForService(context.Background(), "missing"); !errors.Is(err, model.ErrConversationNotFound) {
		t.Fatalf("I-05 missing: %v", err)
	}
}

func TestListIsOfTheCaller(t *testing.T) {
	svc, _ := newService(&fakeCatalog{products: map[string]model.ProductSnapshot{mora.ID: mora}})
	_, _, _ = svc.Open(context.Background(), carlos, mora.ID)
	page, err := svc.List(context.Background(), mora.ProducerID, 1, 20)
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("producer sees it: %+v %v", page, err)
	}
	page, _ = svc.List(context.Background(), "someone-else", 1, 20)
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("a stranger sees nothing: %+v", page)
	}
}
