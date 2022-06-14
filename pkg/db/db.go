package db

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func NewDB() *mongo.Database {
	dbURI := fmt.Sprintf("mongodb://%s:%s@%s:%v/", "admin", "admin", "localhost", 27017)
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

	return connection.Database("weni-web-chat")
}
