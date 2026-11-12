// Package out holds the ports the use cases need from the outside: storage and catalog.
package out

import (
	"context"
	"errors"

	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

// ErrDuplicate: the unique (productId, buyerId) already exists — another open won the race.
var ErrDuplicate = errors.New("duplicate conversation")

// ConversationStore keeps the conversations. A missing conversation is (nil, nil).
type ConversationStore interface {
	Insert(ctx context.Context, c model.Conversation) error
	FindByID(ctx context.Context, id string) (*model.Conversation, error)
	FindByProductAndBuyer(ctx context.Context, productID, buyerID string) (*model.Conversation, error)
	// ListByParticipant: E-31 order; total counts every conversation of the user.
	ListByParticipant(ctx context.Context, userID string, page, limit int) ([]model.Conversation, int, error)
}

// ProductCatalog reads a product from catalog-service (I-02): model.ErrProductNotFound when it is
// missing or deleted, model.ErrCatalogUnavailable when catalog does not answer.
type ProductCatalog interface {
	Product(ctx context.Context, id string) (model.ProductSnapshot, error)
}
