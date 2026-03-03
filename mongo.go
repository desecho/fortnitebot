package main

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	databaseName       = "fortnitebot"
	dailyStatsCollName = "daily_stats"
)

type mongoSnapshotStore struct {
	collection *mongo.Collection
}

func newMongoSnapshotStore(uri string) (*mongoSnapshotStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect to mongodb: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}

	collection := client.Database(databaseName).Collection(dailyStatsCollName)

	indexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "accountId", Value: 1},
			{Key: "date", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	}
	if _, err := collection.Indexes().CreateOne(ctx, indexModel); err != nil {
		return nil, fmt.Errorf("create index: %w", err)
	}

	return &mongoSnapshotStore{collection: collection}, nil
}

func (s *mongoSnapshotStore) UpsertSnapshot(snapshot dailySnapshot) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.D{
		{Key: "accountId", Value: snapshot.AccountID},
		{Key: "date", Value: snapshot.Date},
	}

	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "name", Value: snapshot.Name},
			{Key: "stats", Value: snapshot.Stats},
			{Key: "createdAt", Value: snapshot.CreatedAt},
		}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "accountId", Value: snapshot.AccountID},
			{Key: "date", Value: snapshot.Date},
		}},
	}

	opts := options.UpdateOne().SetUpsert(true)
	_, err := s.collection.UpdateOne(ctx, filter, update, opts)
	return err
}

func (s *mongoSnapshotStore) RecentSnapshots(accountID string, limit int) ([]dailySnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.D{{Key: "accountId", Value: accountID}}
	opts := options.Find().
		SetSort(bson.D{{Key: "date", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := s.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find snapshots: %w", err)
	}
	defer cursor.Close(ctx)

	var snapshots []dailySnapshot
	if err := cursor.All(ctx, &snapshots); err != nil {
		return nil, fmt.Errorf("decode snapshots: %w", err)
	}

	return snapshots, nil
}
