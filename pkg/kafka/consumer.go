package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

// MessageHandler is a function that processes a single FetchRequestMessage.
// It returns an error if processing fails.
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
// It uses manual offset commit and routes terminal failures to a dead letter topic.
type KafkaConsumer struct {
	reader     *kafka.Reader
	dltWriter  *kafka.Writer
	brokers    []string
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
		MaxBytes:       10e6,
		CommitInterval: 0,
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
		slog.Int("max_retries", maxRetries),
	)

	return &KafkaConsumer{
		reader:     reader,
		dltWriter:  dltWriter,
		brokers:    brokers,
		maxRetries: maxRetries,
		logger:     logger,
	}
}

// Start begins the consume loop.
func (c *KafkaConsumer) Start(ctx context.Context, handler MessageHandler) error {
	c.logger.Info("consumer loop starting")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("consumer loop stopped: context cancelled")
			return nil
		default:
		}

		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Error("fetch message error", slog.String("error", err.Error()))
			continue
		}

		if c.processMessage(ctx, msg, handler) {
			c.commitOffset(ctx, msg)
		}
	}
}

// processMessage handles a single Kafka message.
// It returns true when the offset should be committed.
func (c *KafkaConsumer) processMessage(ctx context.Context, msg kafka.Message, handler MessageHandler) bool {
	var fetchMsg FetchRequestMessage
	if err := json.Unmarshal(msg.Value, &fetchMsg); err != nil {
		c.logger.Error("malformed message: skipping",
			slog.String("error", err.Error()),
			slog.Int64("offset", msg.Offset),
			slog.Int("partition", msg.Partition),
		)
		return true
	}

	c.logger.Info("processing message",
		slog.String("url", fetchMsg.URL),
		slog.String("request_id", fetchMsg.RequestID),
		slog.Int64("offset", msg.Offset),
	)

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if err := handler(ctx, fetchMsg); err != nil {
			if attempt < c.maxRetries {
				backoff := retryBackoff(attempt + 1)
				c.logger.Warn("message processing failed: retrying",
					slog.String("url", fetchMsg.URL),
					slog.String("request_id", fetchMsg.RequestID),
					slog.String("error", err.Error()),
					slog.Int("attempt", attempt+1),
					slog.Int("max_retries", c.maxRetries),
					slog.Duration("backoff", backoff),
				)

				select {
				case <-ctx.Done():
					c.logger.Warn("context cancelled before retry completed",
						slog.String("url", fetchMsg.URL),
						slog.String("request_id", fetchMsg.RequestID),
					)
					return false
				case <-time.After(backoff):
				}

				continue
			}

			c.logger.Error("message processing failed: routing to DLT",
				slog.String("url", fetchMsg.URL),
				slog.String("request_id", fetchMsg.RequestID),
				slog.String("error", err.Error()),
				slog.Int("retries_used", attempt),
			)

			if err := c.sendToDLT(ctx, msg, err, attempt); err != nil {
				return false
			}
			return true
		}

		c.logger.Info("message processed successfully",
			slog.String("url", fetchMsg.URL),
			slog.String("request_id", fetchMsg.RequestID),
			slog.Int("retries_used", attempt),
		)
		return true
	}

	return true
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
func (c *KafkaConsumer) sendToDLT(ctx context.Context, original kafka.Message, processingErr error, retriesUsed int) error {
	dltMsg := kafka.Message{
		Key:   original.Key,
		Value: original.Value,
		Headers: []kafka.Header{
			{Key: "original-topic", Value: []byte(c.reader.Config().Topic)},
			{Key: "error", Value: []byte(processingErr.Error())},
			{Key: "failed-at", Value: []byte(time.Now().UTC().Format(time.RFC3339))},
			{Key: "retry-count", Value: []byte(strconv.Itoa(retriesUsed))},
			{Key: "max-retries", Value: []byte(strconv.Itoa(c.maxRetries))},
		},
	}

	if err := c.dltWriter.WriteMessages(ctx, dltMsg); err != nil {
		c.logger.Error("failed to send to DLT",
			slog.String("error", err.Error()),
			slog.Int64("offset", original.Offset),
		)
		return fmt.Errorf("write to DLT: %w", err)
	}

	c.logger.Warn("message sent to DLT",
		slog.Int64("offset", original.Offset),
		slog.Int("retry_count", retriesUsed),
	)
	return nil
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

// Ping verifies the consumer can connect to Kafka.
func (c *KafkaConsumer) Ping(ctx context.Context) error {
	if len(c.brokers) == 0 || c.brokers[0] == "" {
		return fmt.Errorf("no kafka brokers configured")
	}

	conn, err := kafka.DialContext(ctx, "tcp", c.brokers[0])
	if err != nil {
		return fmt.Errorf("dial kafka broker %s: %w", c.brokers[0], err)
	}
	defer conn.Close()

	return nil
}

func retryBackoff(attempt int) time.Duration {
	backoff := time.Duration(attempt) * 200 * time.Millisecond
	if backoff > 2*time.Second {
		return 2 * time.Second
	}
	return backoff
}
