package activity

import (
	"context"
	"errors"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// Kafka producer props
	lingerMs         = 200
	compressionCodec = "lz4"
	// Retry props
	retryN    = 3
	retryWait = lingerMs * time.Millisecond
)

// KafkaSink sinks events to a Kafka cluster.
type KafkaSink struct {
	producer *kafka.Producer
	topic    string
	logger   *zap.Logger
	logChan  chan kafka.LogEvent
}

func NewKafkaSink(brokers, topic string, logger *zap.Logger) (*KafkaSink, error) {
	logChan := make(chan kafka.LogEvent, 100)
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":      brokers,
		"go.logs.channel.enable": true,
		"go.logs.channel":        logChan,
		// Configure waiting time before sending out a batch of messages
		"linger.ms": lingerMs,
		// Specify the compression type to be used for messages
		"compression.codec": compressionCodec,
	})
	if err != nil {
		return nil, err
	}

	go forwardKafkaLogEventToLogger(logChan, logger)

	return &KafkaSink{
		producer: producer,
		topic:    topic,
		logger:   logger,
		logChan:  logChan,
	}, nil
}

// Sink doesn't wait till all events are delivered to Kafka
func (s *KafkaSink) Sink(_ context.Context, events []Event) error {
	for _, event := range events {
		message, err := event.Marshal()
		if err != nil {
			return err
		}

		sendMessageFn := func() error {
			return s.producer.Produce(&kafka.Message{
				TopicPartition: kafka.TopicPartition{Topic: &s.topic, Partition: kafka.PartitionAny},
				Value:          message,
			}, nil)
		}

		retryOnErrFn := func(err error) bool {
			kafkaErr := kafka.Error{}
			// Producer queue is full, wait for messages to be delivered then try again.
			return errors.As(err, &kafkaErr) && kafkaErr.Code() == kafka.ErrQueueFull
		}

		err = retry(retryN, retryWait, sendMessageFn, retryOnErrFn)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *KafkaSink) Close() error {
	s.producer.Flush(100)
	s.producer.Close()
	close(s.logChan)
	return nil
}

func forwardKafkaLogEventToLogger(logChan chan kafka.LogEvent, logger *zap.Logger) {
	for logEvent := range logChan {
		zapLevel := kafkaLogLevelToZapLevel(logEvent.Level)
		if logger.Core().Enabled(zapLevel) {
			fields := []zapcore.Field{
				zap.String("kafka.producer.client.name", logEvent.Name),
				zap.String("kafka.producer.tag", logEvent.Tag),
			}
			logger.Log(zapLevel, logEvent.Message, fields...)
		}
	}
}

// Log syslog level, lower is more critical
// https://en.wikipedia.org/wiki/Syslog#Severity_level
func kafkaLogLevelToZapLevel(level int) zapcore.Level {
	switch level {
	case 0, 1, 2:
		return zap.FatalLevel
	case 3:
		return zap.ErrorLevel
	case 4:
		return zap.WarnLevel
	case 5, 6:
		return zap.InfoLevel
	case 7:
		return zap.DebugLevel
	default:
		return zap.DebugLevel // Default to debug for unrecognized levels
	}
}

func retry(maxRetries int, delay time.Duration, fn func() error, retryOnErrFn func(err error) bool) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil // success
		} else if retryOnErrFn(err) {
			time.Sleep(delay) // retry
		} else {
			break // failure
		}
	}
	return err
}
