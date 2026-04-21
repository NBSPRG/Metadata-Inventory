package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

// EventProducer defines the interface for publishing messages to Kafka.
// Business logic depends on this interface, never on the concrete
// kafka-go writer directly.
type EventProducer interface {
	// Publish sends a FetchRequestMessage to the configured topic.
	Publish(ctx context.Context, msg FetchRequestMessage) error
	// Ping verifies the producer can reach Kafka.
	Ping(ctx context.Context) error
	// Close gracefully shuts down the producer, flushing buffered messages.
	Close() error
}

// KafkaProducer is the concrete Kafka implementation of EventProducer.
type KafkaProducer struct {
	writer  *kafka.Writer
	brokers []string
	topic   string
	logger  *slog.Logger
}

// NewKafkaProducer creates a new KafkaProducer that writes to the specified
// topic on the given brokers. It uses LeastBytes balancer for even distribution.
func NewKafkaProducer(brokers []string, topic string, logger *slog.Logger) *KafkaProducer {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks: kafka.RequireOne,
		Async:        false, // Synchronous writes for reliability
	}

	logger.Info("kafka producer initialized",
		slog.String("topic", topic),
		slog.Any("brokers", brokers),
	)

	return &KafkaProducer{
		writer:  writer,
		brokers: brokers,
		topic:   topic,
		logger:  logger,
	}
}

// Publish serializes and sends a FetchRequestMessage to Kafka.
// The URL is used as the message key for partition affinity (same URL
// always goes to the same partition, preventing duplicate concurrent fetches).
func (p *KafkaProducer) Publish(ctx context.Context, msg FetchRequestMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	kafkaMsg := kafka.Message{
		Key:   []byte(msg.URL),
		Value: data,
	}

	if err := p.writer.WriteMessages(ctx, kafkaMsg); err != nil {
		return fmt.Errorf("publish to kafka: %w", err)
	}

	p.logger.Debug("message published",
		slog.String("topic", p.topic),
		slog.String("url", msg.URL),
		slog.String("request_id", msg.RequestID),
	)

	return nil
}

// Ping verifies the producer can reach a configured Kafka broker.
func (p *KafkaProducer) Ping(ctx context.Context) error {
	if len(p.brokers) == 0 || p.brokers[0] == "" {
		return fmt.Errorf("no kafka brokers configured")
	}

	conn, err := kafka.DialContext(ctx, "tcp", p.brokers[0])
	if err != nil {
		return fmt.Errorf("dial kafka broker %s: %w", p.brokers[0], err)
	}
	defer conn.Close()

	return nil
}

// Close gracefully closes the Kafka writer.
func (p *KafkaProducer) Close() error {
	p.logger.Info("closing kafka producer")
	return p.writer.Close()
}

// MockProducer is a test double for EventProducer that records published
// messages for assertion.
type MockProducer struct {
	Messages   []FetchRequestMessage
	Err        error
	PingErr    error
	Closed     bool
	PingCalled int
}

// Publish records the message (or returns the configured error).
func (m *MockProducer) Publish(_ context.Context, msg FetchRequestMessage) error {
	if m.Err != nil {
		return m.Err
	}
	m.Messages = append(m.Messages, msg)
	return nil
}

// Ping records the call and returns the configured error.
func (m *MockProducer) Ping(_ context.Context) error {
	m.PingCalled++
	return m.PingErr
}

// Close marks the producer as closed.
func (m *MockProducer) Close() error {
	m.Closed = true
	return nil
}
