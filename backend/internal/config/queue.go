package config

import "time"

const (
	maxQueueSize  = 100_000
	maxQueueBytes = 4 << 30
	maxBatchSize  = 10_000
	maxWorkers    = 64
)

type QueueConfig struct {
	QueueSize      int           `config:"queue_size"`
	QueueMaxBytes  int64         `config:"queue_max_bytes"`
	EnqueueTimeout time.Duration `config:"enqueue_timeout"`
	BatchSize      int           `config:"batch_size"`
	BatchTimeout   time.Duration `config:"batch_timeout"`
	FlushInterval  time.Duration `config:"flush_interval"`
	Workers        int           `config:"workers"`
}

func (configuration Config) validateQueue(validator *validator) {
	queue := configuration.Queue

	if queue.BatchSize <= 0 || queue.BatchSize > maxBatchSize {
		validator.add("queue.batch_size", "must be between 1 and 10000")
	}
	if queue.QueueSize < queue.BatchSize {
		validator.add("queue.queue_size", "must be >= batch_size")
	}
	if queue.QueueSize > maxQueueSize {
		validator.add("queue.queue_size", "must not exceed 100000")
	}
	if queue.QueueMaxBytes < configuration.Ingestion.MaxRequestBytes {
		validator.add("queue.queue_max_bytes", "must be >= ingestion.max_request_bytes")
	}
	if queue.QueueMaxBytes > maxQueueBytes {
		validator.add("queue.queue_max_bytes", "must not exceed 4294967296")
	}
	if queue.Workers <= 0 || queue.Workers > maxWorkers {
		validator.add("queue.workers", "must be between 1 and 64")
	}
	if queue.EnqueueTimeout <= 0 {
		validator.add("queue.enqueue_timeout", "must be positive")
	}
	if queue.BatchTimeout <= 0 {
		validator.add("queue.batch_timeout", "must be positive")
	}
	if queue.FlushInterval <= 0 {
		validator.add("queue.flush_interval", "must be positive")
	}
}
