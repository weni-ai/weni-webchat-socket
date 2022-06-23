package history

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collection = "message"

// Repo represents a message repository.
type Repo interface {
	Get(contactURN, channelUUID string, before *time.Time, limit, page int) ([]MessagePayload, error)
	Save(msg MessagePayload) error
}

type repo struct {
	db         *mongo.Database
	collection *mongo.Collection
}

// NewReppo create and returns a new instance of message repository.
func NewRepo(db *mongo.Database) Repo {
	return &repo{
		db:         db,
		collection: db.Collection(collection),
	}
}

// Get returns message records by contact URN.
func (r repo) Get(contactURN, channelUUID string, before *time.Time, limit, page int) ([]MessagePayload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var timestamp int64
	if before != nil {
		timestamp = before.UnixNano()
	} else {
		timestamp = time.Now().UnixNano()
	}
	qry := bson.M{
		"contact_urn":  contactURN,
		"channel_uuid": channelUUID,
		"timestamp":    bson.M{"$lt": timestamp},
	}
	pagination := NewPagination(limit, page)
	cursor, err := r.collection.Find(ctx, qry, pagination.GetOptions())
	if err != nil {
		return nil, fmt.Errorf("find failed: %s", err.Error())
	}
	defer cursor.Close(ctx)
	msgs := []MessagePayload{}
	for cursor.Next(ctx) {
		var msg MessagePayload
		if err = cursor.Decode(&msg); err != nil {
			return nil, fmt.Errorf("failed to parse message from cursor: %s", err.Error())
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// Save stores a given message record to database.
func (r repo) Save(msg MessagePayload) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.collection.InsertOne(ctx, msg)
	if err != nil {
		return fmt.Errorf("unexpected error on save: %s", err.Error())
	}
	return nil
}

// Pagination represents pagination parameters to find options
type pagination struct {
	limit int64
	page  int64
}

func NewPagination(limit, page int) *pagination {
	return &pagination{
		limit: int64(limit),
		page:  int64(page),
	}
}

func (p *pagination) GetOptions() *options.FindOptions {
	l := p.limit
	skip := p.page*p.limit - p.limit
	return &options.FindOptions{Limit: &l, Skip: &skip, Sort: bson.D{{Key: "timestamp", Value: -1}}}
}
