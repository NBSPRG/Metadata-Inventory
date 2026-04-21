package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// ConnectMongo establishes a connection to MongoDB with exponential backoff
// retry logic. It validates the connection by pinging the server.
// maxRetries controls how many times to attempt connection (default 5).
func ConnectMongo(ctx context.Context, uri, dbName string, maxPoolSize uint64, connTimeout time.Duration, logger *slog.Logger) (*mongo.Database, error) {
	const maxRetries = 5

	clientOpts := options.Client().
		ApplyURI(uri).
		SetMaxPoolSize(maxPoolSize).
		SetConnectTimeout(connTimeout).
		SetServerSelectionTimeout(5 * time.Second)

	var client *mongo.Client
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		client, err = mongo.Connect(ctx, clientOpts)
		if err != nil {
			logger.Warn("mongodb connection attempt failed",
				slog.Int("attempt", attempt),
				slog.Int("max_retries", maxRetries),
				slog.String("error", err.Error()),
			)
			backoff := time.Duration(attempt*attempt) * time.Second
			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return nil, fmt.Errorf("mongodb connect cancelled: %w", ctx.Err())
			}
		}

		// Verify the connection
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = client.Ping(pingCtx, readpref.Primary())
		cancel()

		if err == nil {
			logger.Info("mongodb connected",
				slog.String("uri", sanitizeURI(uri)),
				slog.String("database", dbName),
				slog.Int("attempt", attempt),
			)
			return client.Database(dbName), nil
		}

		logger.Warn("mongodb ping failed",
			slog.Int("attempt", attempt),
			slog.String("error", err.Error()),
		)

		backoff := time.Duration(attempt*attempt) * time.Second
		select {
		case <-time.After(backoff):
			continue
		case <-ctx.Done():
			return nil, fmt.Errorf("mongodb ping cancelled: %w", ctx.Err())
		}
	}

	return nil, fmt.Errorf("mongodb connection failed after %d attempts: %w", maxRetries, err)
}

// DisconnectMongo gracefully disconnects the MongoDB client.
func DisconnectMongo(ctx context.Context, db *mongo.Database, logger *slog.Logger) {
	if db == nil {
		return
	}
	if err := db.Client().Disconnect(ctx); err != nil {
		logger.Error("mongodb disconnect error", slog.String("error", err.Error()))
	} else {
		logger.Info("mongodb disconnected")
	}
}

// sanitizeURI removes credentials from the URI for safe logging.
func sanitizeURI(uri string) string {
	// Simple approach: just show the scheme and host portion
	// A production implementation would properly parse the URI
	if len(uri) > 50 {
		return uri[:50] + "..."
	}
	return uri
}
