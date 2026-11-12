package persistence

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/code-corhuila/market-agri-chat-api/internal/application/port/out"
	"github.com/code-corhuila/market-agri-chat-api/internal/domain/model"
)

// Mongo is the ConversationStore on the database chat, as chat_app (ADR-010). The validators and
// the unique index come from market-agri-chat-db; this adapter never creates them.
type Mongo struct {
	conversations *mongo.Collection
}

func NewMongo(db *mongo.Database) *Mongo { return &Mongo{conversations: db.Collection("conversation")} }

// conversationDoc is the shape of the validator of 01_ddl/00_collections (chat-db).
type conversationDoc struct {
	ID                   string     `bson:"_id"`
	ProductID            string     `bson:"productId"`
	ProductName          string     `bson:"productName"`
	BuyerID              string     `bson:"buyerId"`
	BuyerName            string     `bson:"buyerName"`
	ProducerID           string     `bson:"producerId"`
	ProducerName         string     `bson:"producerName"`
	AgreedPurchaseMethod *string    `bson:"agreedPurchaseMethod"`
	LastMessageAt        *time.Time `bson:"lastMessageAt"`
	CreatedAt            time.Time  `bson:"createdAt"`
}

func toDoc(c model.Conversation) conversationDoc {
	d := conversationDoc{
		ID: c.ID, ProductID: c.ProductID, ProductName: c.ProductName, BuyerID: c.BuyerID, BuyerName: c.BuyerName,
		ProducerID: c.ProducerID, ProducerName: c.ProducerName, LastMessageAt: c.LastMessageAt, CreatedAt: c.CreatedAt,
	}
	if c.AgreedPurchaseMethod != nil {
		m := string(*c.AgreedPurchaseMethod)
		d.AgreedPurchaseMethod = &m
	}
	return d
}

func (d conversationDoc) toModel() model.Conversation {
	c := model.Conversation{
		ID: d.ID, ProductID: d.ProductID, ProductName: d.ProductName, BuyerID: d.BuyerID, BuyerName: d.BuyerName,
		ProducerID: d.ProducerID, ProducerName: d.ProducerName, LastMessageAt: utc(d.LastMessageAt), CreatedAt: d.CreatedAt.UTC(),
	}
	if d.AgreedPurchaseMethod != nil {
		m := model.PurchaseMethod(*d.AgreedPurchaseMethod)
		c.AgreedPurchaseMethod = &m
	}
	return c
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// Insert: the unique idx_conversation_productId_buyerId decides a race (AGGR-INV-001).
func (m *Mongo) Insert(ctx context.Context, c model.Conversation) error {
	_, err := m.conversations.InsertOne(ctx, toDoc(c))
	if mongo.IsDuplicateKeyError(err) {
		return out.ErrDuplicate
	}
	return err
}

func (m *Mongo) FindByID(ctx context.Context, id string) (*model.Conversation, error) {
	return m.findOne(ctx, bson.D{{Key: "_id", Value: id}})
}

func (m *Mongo) FindByProductAndBuyer(ctx context.Context, productID, buyerID string) (*model.Conversation, error) {
	return m.findOne(ctx, bson.D{{Key: "productId", Value: productID}, {Key: "buyerId", Value: buyerID}})
}

func (m *Mongo) findOne(ctx context.Context, filter bson.D) (*model.Conversation, error) {
	var d conversationDoc
	err := m.conversations.FindOne(ctx, filter).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c := d.toModel()
	return &c, nil
}

// ListByParticipant: descending lastMessageAt puts the conversations without messages (null) last,
// then createdAt descending, then _id — the order of E-31.
func (m *Mongo) ListByParticipant(ctx context.Context, userID string, page, limit int) ([]model.Conversation, int, error) {
	filter := bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "buyerId", Value: userID}}, bson.D{{Key: "producerId", Value: userID}}}}}
	total, err := m.conversations.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	find := options.Find().
		SetSort(bson.D{{Key: "lastMessageAt", Value: -1}, {Key: "createdAt", Value: -1}, {Key: "_id", Value: 1}}).
		SetSkip(int64((page - 1) * limit)).SetLimit(int64(limit))
	cursor, err := m.conversations.Find(ctx, filter, find)
	if err != nil {
		return nil, 0, err
	}
	var docs []conversationDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, err
	}
	items := make([]model.Conversation, 0, len(docs))
	for _, d := range docs {
		items = append(items, d.toModel())
	}
	return items, int(total), nil
}

// SetPurchaseMethod: a $set of the one field, so a concurrent choice is simply the last write.
func (m *Mongo) SetPurchaseMethod(ctx context.Context, id string, method model.PurchaseMethod) error {
	_, err := m.conversations.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "agreedPurchaseMethod", Value: string(method)}}}})
	return err
}
