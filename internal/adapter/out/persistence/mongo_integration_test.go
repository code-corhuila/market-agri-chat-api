//go:build integration

package persistence

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/code-corhuila/market-agri-chat-api/internal/application/port/out"
)

// The MongoDB store against the real database chat, migrated by market-agri-chat-db (validators
// and the unique index included). MONGO_TEST_URL is the administrator's URL of a dev instance
// (README): only the administrator can remove the probes afterwards.
//
//	go test -tags integration ./internal/adapter/out/persistence/
func TestMongoStore(t *testing.T) {
	url := os.Getenv("MONGO_TEST_URL")
	if url == "" {
		t.Skip("MONGO_TEST_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(url))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("chat")
	// The contract's products are b + 7 digits; no seed matches.
	probes := bson.M{"productId": bson.M{"$regex": `^b[0-9]{7}-`}}
	clean := func() {
		if _, err := db.Collection("conversation").DeleteMany(ctx, probes); err != nil {
			t.Fatal(err)
		}
	}
	storeContract(t, func(t *testing.T) out.ConversationStore {
		clean()
		t.Cleanup(clean)
		return NewMongo(db)
	})
}
