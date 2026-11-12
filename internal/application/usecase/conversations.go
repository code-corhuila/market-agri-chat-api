// Package usecase implements the input ports with the domain and the output ports.
package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/code-corhuila/market-agri-chat-api/internal/application/port/in"
	"github.com/code-corhuila/market-agri-chat-api/internal/application/port/out"
	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

// Conversations implements in.Conversations.
type Conversations struct {
	Store   out.ConversationStore
	Catalog out.ProductCatalog
	NewID   func() string
	Now     func() time.Time
}

var _ in.Conversations = (*Conversations)(nil)

// Open (E-30) is idempotent on (product, buyer): an existing conversation is returned without
// calling catalog; a race with another open is decided by the unique index, and the loser answers
// with the winner's conversation.
func (u *Conversations) Open(ctx context.Context, buyer in.Buyer, productID string) (model.Conversation, bool, error) {
	if existing, err := u.Store.FindByProductAndBuyer(ctx, productID, buyer.ID); err != nil || existing != nil {
		return deref(existing), false, err
	}
	product, err := u.Catalog.Product(ctx, productID)
	if err != nil {
		return model.Conversation{}, false, err
	}
	c, err := model.Open(u.NewID(), product, buyer.ID, buyer.Name, u.Now())
	if err != nil {
		return model.Conversation{}, false, err
	}
	if err := u.Store.Insert(ctx, c); errors.Is(err, out.ErrDuplicate) {
		winner, err := u.Store.FindByProductAndBuyer(ctx, productID, buyer.ID)
		if err == nil && winner == nil {
			err = errors.New("duplicate conversation not found after the race")
		}
		return deref(winner), false, err
	} else if err != nil {
		return model.Conversation{}, false, err
	}
	return c, true, nil
}

// Get (E-32): 404 before 403, so a stranger learns nothing more than a missing id.
func (u *Conversations) Get(ctx context.Context, userID, id string) (model.Conversation, error) {
	c, err := u.GetForService(ctx, id)
	if err != nil {
		return model.Conversation{}, err
	}
	return c, c.EnsureParticipant(userID)
}

// GetForService (I-05): the checkout saga reads any conversation.
func (u *Conversations) GetForService(ctx context.Context, id string) (model.Conversation, error) {
	c, err := u.Store.FindByID(ctx, id)
	if err != nil {
		return model.Conversation{}, err
	}
	if c == nil {
		return model.Conversation{}, model.ErrConversationNotFound
	}
	return *c, nil
}

// List (E-31): the conversations where the caller is the buyer or the producer.
func (u *Conversations) List(ctx context.Context, userID string, page, limit int) (in.Page, error) {
	items, total, err := u.Store.ListByParticipant(ctx, userID, page, limit)
	return in.Page{Items: items, Total: total}, err
}

func deref(c *model.Conversation) model.Conversation {
	if c == nil {
		return model.Conversation{}
	}
	return *c
}
