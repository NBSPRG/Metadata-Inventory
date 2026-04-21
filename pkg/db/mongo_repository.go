package db

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collectionName = "metadata"

// MongoRepository is the MongoDB implementation of MetadataRepository.
type MongoRepository struct {
	collection *mongo.Collection
	logger     *slog.Logger
}

// NewMongoRepository creates a new MongoRepository and ensures required
// indexes exist on the metadata collection.
func NewMongoRepository(ctx context.Context, db *mongo.Database, logger *slog.Logger) (*MongoRepository, error) {
	coll := db.Collection(collectionName)
	repo := &MongoRepository{
		collection: coll,
		logger:     logger,
	}

	if err := repo.ensureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("ensure indexes: %w", err)
	}

	return repo, nil
}

// ensureIndexes creates the required indexes for the metadata collection.
func (r *MongoRepository) ensureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "url", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "url_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("create indexes: %w", err)
	}

	r.logger.Info("mongodb indexes ensured", slog.String("collection", collectionName))
	return nil
}

// FindByURL retrieves a metadata record by its URL using the url_hash index
// for O(1) lookup performance.
func (r *MongoRepository) FindByURL(ctx context.Context, url string) (*MetadataRecord, error) {
	hash := hashURL(url)
	filter := bson.M{"url_hash": hash}

	var record MetadataRecord
	err := r.collection.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Not found is not an error
		}
		return nil, fmt.Errorf("find by url: %w", err)
	}

	return &record, nil
}

// Upsert inserts a new metadata record or updates an existing one.
// Uses the url_hash as the unique key for the upsert.
func (r *MongoRepository) Upsert(ctx context.Context, record *MetadataRecord) error {
	now := time.Now().UTC()
	record.URLHash = hashURL(record.URL)
	record.UpdatedAt = now
	record.SchemaVersion = CurrentSchemaVersion

	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}

	filter := bson.M{"url_hash": record.URLHash}
	update := bson.M{
		"$set": record,
		"$setOnInsert": bson.M{
			"created_at": record.CreatedAt,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("upsert: %w", err)
	}

	return nil
}

// UpdateStatus updates the status and optionally the fetch result for a record.
func (r *MongoRepository) UpdateStatus(ctx context.Context, url string, status Status, result *FetchResult) error {
	hash := hashURL(url)
	filter := bson.M{"url_hash": hash}

	updateFields := bson.M{
		"status":     status,
		"updated_at": time.Now().UTC(),
	}

	if result != nil {
		updateFields["headers"] = result.Headers
		updateFields["cookies"] = result.Cookies
		updateFields["page_source"] = result.PageSource
		updateFields["page_source_size_bytes"] = result.PageSourceSizeBytes
		updateFields["fetch_duration_ms"] = result.FetchDurationMs
		updateFields["fetched_at"] = result.FetchedAt
	}

	if status == StatusFailed {
		// Increment retry count on failure
		update := bson.M{
			"$set": updateFields,
			"$inc": bson.M{"retry_count": 1},
		}
		_, err := r.collection.UpdateOne(ctx, filter, update)
		if err != nil {
			return fmt.Errorf("update status (failed): %w", err)
		}
		return nil
	}

	update := bson.M{"$set": updateFields}
	_, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	return nil
}

// Ping verifies the MongoDB connection is alive.
func (r *MongoRepository) Ping(ctx context.Context) error {
	return r.collection.Database().Client().Ping(ctx, nil)
}

// hashURL returns the SHA-256 hex digest of a URL for use as an index key.
func hashURL(url string) string {
	h := sha256.Sum256([]byte(url))
	return fmt.Sprintf("%x", h)
}
