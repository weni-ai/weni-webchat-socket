package message

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const collection = "message"

// Repo represents a message repository.
type Repo interface {
	Get(contactURN string, channelUUID string) ([]Message, error)
	Save(msg Message) error
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
func (r repo) Get(contactURN string, channelUUID string) ([]Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	qry := bson.M{
		"contact_urn":  contactURN,
		"channel_uuid": channelUUID,
	}
	cursor, err := r.collection.Find(ctx, qry)
	if err != nil {
		return nil, fmt.Errorf("find failed: %s", err.Error())
	}
	defer cursor.Close(ctx)
	msgs := []Message{}
	for cursor.Next(ctx) {
		var msg Message
		if err = cursor.Decode(&msg); err != nil {
			return nil, fmt.Errorf("failed to parse message from cursor: %s", err.Error())
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// Save stores a given message record to database.
func (r repo) Save(msg Message) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.collection.InsertOne(ctx, msg)
	if err != nil {
		return fmt.Errorf("Unexpected error on save: %s", err.Error())
	}
	return nil
}
