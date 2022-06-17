package db

import (
	"context"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func NewDB() *mongo.Database {
	dbURI := os.Getenv("WWC_DB_URI")
	dbName := os.Getenv("WWC_DB_NAME")
	options := options.Client().ApplyURI(dbURI)
	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()
	connection, err := mongo.Connect(ctx, options)
	if err != nil {
		log.Error("fail to connect to MongoDB", err.Error())
		panic(err.Error())
	}

	if err := connection.Ping(ctx, readpref.Primary()); err != nil {
		log.Error("fail to ping MongoDB", err.Error())
		panic(err.Error())
	} else {
		log.Info("MongoDB connection OK")
	}

	return connection.Database(dbName)
}

func Clear(db *mongo.Database) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := db.Drop(ctx)
	if err != nil {
		return err
	}
	return nil
}
