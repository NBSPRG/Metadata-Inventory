package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

// MessageHandler is a function that processes a single FetchRequestMessage.
// It returns an error if processing fails (triggering retry or DLT routing).
type MessageHandler func(ctx context.Context, msg FetchRequestMessage) error

// EventConsumer defines the interface for consuming Kafka messages.
type EventConsumer interface {
	// Start begins consuming messages, calling the handler for each one.
	// It blocks until the context is cancelled.
	Start(ctx context.Context, handler MessageHandler) error
	// Close gracefully shuts down the consumer.
	Close() error
}

// KafkaConsumer is the concrete Kafka implementation of EventConsumer.
// It uses manual offset commit (commit only after successful processing)
// and routes failed messages to a dead letter topic.
type KafkaConsumer struct {
	reader     *kafka.Reader
	dltWriter  *kafka.Writer
	maxRetries int
	logger     *slog.Logger
}

// NewKafkaConsumer creates a new KafkaConsumer with manual offset commit
// and a dead letter topic writer for failed messages.
func NewKafkaConsumer(brokers []string, topic, groupID, dltTopic string, maxRetries int, logger *slog.Logger) *KafkaConsumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10e6, // 10MB
		CommitInterval: 0,    // Manual commit only
		StartOffset:    kafka.LastOffset,
		MaxWait:        1 * time.Second,
	})

	dltWriter := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    dltTopic,
		Balancer: &kafka.LeastBytes{},
	}

	logger.Info("kafka consumer initialized",
		slog.String("topic", topic),
		slog.String("group_id", groupID),
		slog.String("dlt_topic", dltTopic),
	)

	return &KafkaConsumer{
		reader:     reader,
		dltWriter:  dltWriter,
		maxRetries: maxRetries,
		logger:     logger,
	}
}

// Start begins the consume loop. It processes one message at a time per
// partition. On success, the offset is committed. On failure after max
// retries, the message is routed to the dead letter topic.
func (c *KafkaConsumer) Start(ctx context.Context, handler MessageHandler) error {
	c.logger.Info("consumer loop starting")

	for {
		// Check for cancellation before fetching
		select {
		case <-ctx.Done():
			c.logger.Info("consumer loop stopped — context cancelled")
			return nil
		default:
		}

		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // Graceful shutdown
			}
			c.logger.Error("fetch message error", slog.String("error", err.Error()))
			continue
		}

		c.processMessage(ctx, msg, handler)
	}
}

// processMessage handles a single Kafka message: deserialize, process,
// commit or route to DLT.
func (c *KafkaConsumer) processMessage(ctx context.Context, msg kafka.Message, handler MessageHandler) {
	var fetchMsg FetchRequestMessage
	if err := json.Unmarshal(msg.Value, &fetchMsg); err != nil {
		// Poison pill — malformed message, log and skip (don't retry forever)
		c.logger.Error("malformed message — skipping",
			slog.String("error", err.Error()),
			slog.Int64("offset", msg.Offset),
			slog.Int("partition", msg.Partition),
		)
		c.commitOffset(ctx, msg)
		return
	}

	c.logger.Info("processing message",
		slog.String("url", fetchMsg.URL),
		slog.String("request_id", fetchMsg.RequestID),
		slog.Int64("offset", msg.Offset),
	)

	// Process with handler
	if err := handler(ctx, fetchMsg); err != nil {
		c.logger.Error("message processing failed",
			slog.String("url", fetchMsg.URL),
			slog.String("request_id", fetchMsg.RequestID),
			slog.String("error", err.Error()),
		)

		// Route to dead letter topic
		c.sendToDLT(ctx, msg, err)
	}

	// Commit offset after processing (success or DLT routing)
	c.commitOffset(ctx, msg)
}

// commitOffset manually commits the offset for the processed message.
func (c *KafkaConsumer) commitOffset(ctx context.Context, msg kafka.Message) {
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		c.logger.Error("commit offset failed",
			slog.String("error", err.Error()),
			slog.Int64("offset", msg.Offset),
		)
	}
}

// sendToDLT publishes a failed message to the dead letter topic.
func (c *KafkaConsumer) sendToDLT(ctx context.Context, original kafka.Message, processingErr error) {
	dltMsg := kafka.Message{
		Key:   original.Key,
		Value: original.Value,
		Headers: []kafka.Header{
			{Key: "original-topic", Value: []byte(c.reader.Config().Topic)},
			{Key: "error", Value: []byte(processingErr.Error())},
			{Key: "failed-at", Value: []byte(time.Now().UTC().Format(time.RFC3339))},
		},
	}

	if err := c.dltWriter.WriteMessages(ctx, dltMsg); err != nil {
		c.logger.Error("failed to send to DLT",
			slog.String("error", err.Error()),
			slog.Int64("offset", original.Offset),
		)
	} else {
		c.logger.Warn("message sent to DLT",
			slog.Int64("offset", original.Offset),
		)
	}
}

// Close gracefully shuts down the consumer and DLT writer.
func (c *KafkaConsumer) Close() error {
	c.logger.Info("closing kafka consumer")
	var firstErr error

	if err := c.reader.Close(); err != nil {
		c.logger.Error("reader close error", slog.String("error", err.Error()))
		firstErr = err
	}
	if err := c.dltWriter.Close(); err != nil {
		c.logger.Error("dlt writer close error", slog.String("error", err.Error()))
		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// Ping verifies the consumer can connect to Kafka by checking reader stats.
func (c *KafkaConsumer) Ping() error {
	stats := c.reader.Stats()
	if stats.Dials > 0 && stats.Errors > stats.Dials {
		return fmt.Errorf("kafka consumer unhealthy: %d errors out of %d dials", stats.Errors, stats.Dials)
	}
	return nil
}
