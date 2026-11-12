// Package in holds the ports the adapters call: the use cases of chat-service.
package in

import (
	"context"

	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

// Buyer is the caller of E-30, with the name of the token (D-C31).
type Buyer struct {
	ID   string
	Name string
}

// Page of conversations of E-31.
type Page struct {
	Items []model.Conversation
	Total int
}

// Conversations: open (E-30), read (E-32), read for the checkout (I-05), list (E-31) and choose
// the purchase method (E-34).
type Conversations interface {
	Open(ctx context.Context, buyer Buyer, productID string) (c model.Conversation, created bool, err error)
	Get(ctx context.Context, userID, id string) (model.Conversation, error)
	GetForService(ctx context.Context, id string) (model.Conversation, error)
	List(ctx context.Context, userID string, page, limit int) (Page, error)
	ChoosePurchaseMethod(ctx context.Context, userID, id string, m model.PurchaseMethod) (model.Conversation, error)
}
