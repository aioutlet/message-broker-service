package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the application
type Config struct {
	ServiceName     string
	Port            string
	Environment     string
	Broker          BrokerConfig
	API             APIConfig
	RateLimit       RateLimitConfig
	Log             LogConfig
	HealthCheck     HealthCheckConfig
	ShutdownTimeout time.Duration
}

// BrokerConfig holds message broker configuration
type BrokerConfig struct {
	Type string // rabbitmq, kafka, azure-servicebus

	// RabbitMQ
	RabbitMQ RabbitMQConfig

	// Kafka
	Kafka KafkaConfig

	// Azure Service Bus
	AzureServiceBus AzureServiceBusConfig
}

// RabbitMQConfig holds RabbitMQ specific configuration
type RabbitMQConfig struct {
	URL          string
	Exchange     string
	ExchangeType string
}

// KafkaConfig holds Kafka specific configuration
type KafkaConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

// AzureServiceBusConfig holds Azure Service Bus specific configuration
type AzureServiceBusConfig struct {
	ConnectionString string
	Topic            string
}

// APIConfig holds API configuration
type APIConfig struct {
	Key             string
	CORSOrigins     []string
	RequestTimeout  time.Duration
	MaxRequestSize  string
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool
	RequestsPerSecond int
	Burst             int
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string
	Format string
}

// HealthCheckConfig holds health check configuration
type HealthCheckConfig struct {
	Interval time.Duration
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		ServiceName: getEnv("SERVICE_NAME", "message-broker-service"),
		Port:        getEnv("PORT", "4000"),
		Environment: getEnv("ENVIRONMENT", "development"),
		Broker: BrokerConfig{
			Type: getEnv("MESSAGE_BROKER_TYPE", "rabbitmq"),
			RabbitMQ: RabbitMQConfig{
				URL:          getEnv("RABBITMQ_URL", "amqp://admin:admin@localhost:5672/"),
				Exchange:     getEnv("RABBITMQ_EXCHANGE", "aioutlet.events"),
				ExchangeType: getEnv("RABBITMQ_EXCHANGE_TYPE", "topic"),
			},
			Kafka: KafkaConfig{
				Brokers: getEnvSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
				Topic:   getEnv("KAFKA_TOPIC", "aioutlet.events"),
				GroupID: getEnv("KAFKA_GROUP_ID", "message-broker-service"),
			},
			AzureServiceBus: AzureServiceBusConfig{
				ConnectionString: getEnv("AZURE_SERVICE_BUS_CONNECTION_STRING", ""),
				Topic:            getEnv("AZURE_SERVICE_BUS_TOPIC", "aioutlet.events"),
			},
		},
		API: APIConfig{
			Key:            getEnv("API_KEY", ""),
			CORSOrigins:    getEnvSlice("CORS_ORIGINS", []string{"*"}),
			RequestTimeout: getEnvDuration("REQUEST_TIMEOUT", 30*time.Second),
			MaxRequestSize: getEnv("MAX_REQUEST_SIZE", "5MB"),
		},
		RateLimit: RateLimitConfig{
			Enabled:           getEnvBool("RATE_LIMIT_ENABLED", true),
			RequestsPerSecond: getEnvInt("RATE_LIMIT_REQUESTS_PER_SECOND", 1000),
			Burst:             getEnvInt("RATE_LIMIT_BURST", 2000),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		HealthCheck: HealthCheckConfig{
			Interval: getEnvDuration("HEALTH_CHECK_INTERVAL", 30*time.Second),
		},
		ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate broker type
	validBrokers := map[string]bool{
		"rabbitmq":          true,
		"kafka":             true,
		"azure-servicebus":  true,
	}

	if !validBrokers[c.Broker.Type] {
		return fmt.Errorf("invalid MESSAGE_BROKER_TYPE: %s (must be: rabbitmq, kafka, or azure-servicebus)", c.Broker.Type)
	}

	// Validate broker-specific configuration
	switch c.Broker.Type {
	case "rabbitmq":
		if c.Broker.RabbitMQ.URL == "" {
			return fmt.Errorf("RABBITMQ_URL is required when MESSAGE_BROKER_TYPE=rabbitmq")
		}
	case "kafka":
		if len(c.Broker.Kafka.Brokers) == 0 {
			return fmt.Errorf("KAFKA_BROKERS is required when MESSAGE_BROKER_TYPE=kafka")
		}
	case "azure-servicebus":
		if c.Broker.AzureServiceBus.ConnectionString == "" {
			return fmt.Errorf("AZURE_SERVICE_BUS_CONNECTION_STRING is required when MESSAGE_BROKER_TYPE=azure-servicebus")
		}
	}

	return nil
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
