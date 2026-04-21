package kafka

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	kgo "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessMessageRetriesUntilSuccess(t *testing.T) {
	consumer := &KafkaConsumer{
		maxRetries: 2,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	payload, err := json.Marshal(FetchRequestMessage{
		URL:       "https://example.com",
		RequestID: "req-1",
	})
	require.NoError(t, err)

	attempts := 0
	shouldCommit := consumer.processMessage(context.Background(), kgo.Message{Value: payload}, func(_ context.Context, _ FetchRequestMessage) error {
		attempts++
		if attempts < 3 {
			return assert.AnError
		}
		return nil
	})

	assert.True(t, shouldCommit)
	assert.Equal(t, 3, attempts)
}

func TestProcessMessageMalformedPayloadCommitsOffset(t *testing.T) {
	consumer := &KafkaConsumer{
		maxRetries: 2,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	shouldCommit := consumer.processMessage(context.Background(), kgo.Message{Value: []byte("not-json")}, func(_ context.Context, _ FetchRequestMessage) error {
		t.Fatal("handler should not be called for malformed payload")
		return nil
	})

	assert.True(t, shouldCommit)
}
