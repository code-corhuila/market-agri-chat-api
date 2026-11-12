// Package model is the domain of chat-service: conversations, their participants and the agreed
// purchase method, with their invariants. It imports no framework, driver or HTTP package.
package model

import (
	"strings"
	"time"
)

// PurchaseMethod is what the two parties agreed (D-C15); only the choice is stored (FR-021).
type PurchaseMethod string

const (
	Platform    PurchaseMethod = "PLATFORM"
	OffPlatform PurchaseMethod = "OFF_PLATFORM"
)

// ParsePurchaseMethod accepts only the two values of the contract's enum.
func ParsePurchaseMethod(s string) (PurchaseMethod, error) {
	switch m := PurchaseMethod(s); m {
	case Platform, OffPlatform:
		return m, nil
	}
	return "", ErrInvalid
}

// ProductSnapshot is what chat copies from catalog (I-02) when a conversation opens (DEC-003).
type ProductSnapshot struct {
	ID           string
	Name         string
	ProducerID   string
	ProducerName string
}

// Conversation is one thread between a buyer and a producer about a product (AGGR-INV-001).
// The names are snapshots taken when it opens (D-C31): no call to catalog or auth per message.
type Conversation struct {
	ID                   string
	ProductID            string
	ProductName          string
	BuyerID              string
	BuyerName            string
	ProducerID           string
	ProducerName         string
	AgreedPurchaseMethod *PurchaseMethod // nil until a participant chooses
	LastMessageAt        *time.Time      // nil while there are no messages
	CreatedAt            time.Time
}

// Open starts a conversation of buyer about product. A producer cannot talk to himself, and every
// snapshot is required.
func Open(id string, product ProductSnapshot, buyerID, buyerName string, now time.Time) (Conversation, error) {
	for _, required := range []string{id, product.ID, product.Name, product.ProducerID, product.ProducerName, buyerID, buyerName} {
		if strings.TrimSpace(required) == "" {
			return Conversation{}, ErrInvalid
		}
	}
	if buyerID == product.ProducerID {
		return Conversation{}, ErrInvalid
	}
	return Conversation{
		ID: id, ProductID: product.ID, ProductName: product.Name,
		BuyerID: buyerID, BuyerName: strings.TrimSpace(buyerName),
		ProducerID: product.ProducerID, ProducerName: product.ProducerName,
		CreatedAt: now.UTC(),
	}, nil
}

// EnsureParticipant: only the buyer and the producer read and write it (AGGR-INV-002).
func (c Conversation) EnsureParticipant(userID string) error {
	if userID != c.BuyerID && userID != c.ProducerID {
		return ErrNotAParticipant
	}
	return nil
}
