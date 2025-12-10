package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaQueue is a Kafka-backed queue using a single topic.
// Pop consumes using a consumer group; List performs a best-effort peek of recent messages.
type KafkaQueue struct {
	brokers string
	topic   string
	groupID string
}

// NewKafkaQueue constructs a Kafka queue backend.
func NewKafkaQueue(brokers, topic string) *KafkaQueue {
	if topic == "" {
		topic = "refinery.queue"
	}
	return &KafkaQueue{brokers: brokers, topic: topic, groupID: "refinery-pop"}
}

func (k *KafkaQueue) ensure() error {
	if k.brokers == "" {
		return errors.New("kafka brokers not configured")
	}
	return nil
}

func (k *KafkaQueue) writer() *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(k.brokers),
		Topic:        k.topic,
		RequiredAcks: kafka.RequireAll,
		BatchTimeout: 50 * time.Millisecond,
	}
}

func (k *KafkaQueue) Enqueue(ctx context.Context, req Request) error {
	if err := k.ensure(); err != nil {
		return err
	}
	if req.EnqueuedAt == 0 {
		req.EnqueuedAt = time.Now().Unix()
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	w := k.writer()
	defer w.Close()
	return w.WriteMessages(ctx, kafka.Message{Value: data})
}

func (k *KafkaQueue) List(ctx context.Context) ([]Request, error) {
	if err := k.ensure(); err != nil {
		return nil, err
	}
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{k.brokers},
		Topic:       k.topic,
		GroupID:     "refinery-peek",
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	defer r.Close()
	items := []Request{}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for len(items) < 50 {
		m, err := r.ReadMessage(deadline)
		if err != nil {
			break
		}
		var req Request
		if err := json.Unmarshal(m.Value, &req); err == nil {
			items = append(items, req)
		}
	}
	return items, nil
}

func (k *KafkaQueue) Clear(ctx context.Context) error {
	return errors.New("clear not supported for kafka backend")
}

func (k *KafkaQueue) Stats(ctx context.Context) (Stats, error) {
	if err := k.ensure(); err != nil {
		return Stats{}, err
	}
	conn, err := kafka.DialLeader(ctx, "tcp", k.brokers, k.topic, 0)
	if err != nil {
		return Stats{}, err
	}
	defer conn.Close()
	first, err := conn.ReadFirstOffset()
	if err != nil {
		return Stats{}, err
	}
	last, err := conn.ReadLastOffset()
	if err != nil {
		return Stats{}, err
	}
	length := int(last - first)
	return Stats{Length: length, OldestAge: 0}, nil
}

func (k *KafkaQueue) Pop(ctx context.Context, max int) ([]Request, error) {
	if err := k.ensure(); err != nil {
		return nil, err
	}
	if max <= 0 {
		max = 1
	}
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{k.brokers},
		Topic:    k.topic,
		GroupID:  k.groupID,
		MinBytes: 1,
		MaxBytes: 10e6,
	})
	defer r.Close()
	items := []Request{}
	for len(items) < max {
		m, err := r.ReadMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				break
			}
			return items, err
		}
		var req Request
		if err := json.Unmarshal(m.Value, &req); err == nil {
			items = append(items, req)
		}
	}
	return items, nil
}
