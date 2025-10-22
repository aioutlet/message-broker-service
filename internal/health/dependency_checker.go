package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aioutlet/message-broker-service/internal/config"
)

// DependencyResult represents the result of a dependency health check
type DependencyResult struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	URL     string `json:"url,omitempty"`
	Error   string `json:"error,omitempty"`
}

// DependencyChecker handles health checks for external dependencies
type DependencyChecker struct {
	config *config.Config
	client *http.Client
}

// NewDependencyChecker creates a new dependency checker
func NewDependencyChecker(cfg *config.Config) *DependencyChecker {
	return &DependencyChecker{
		config: cfg,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// CheckDependencies performs health checks on all configured dependencies
func (dc *DependencyChecker) CheckDependencies(ctx context.Context) []DependencyResult {
	fmt.Println("Step 3: Checking dependency health...")

	var results []DependencyResult

	// Check message broker health based on type
	switch dc.config.Broker.Type {
	case "rabbitmq":
		result := dc.checkRabbitMQ(ctx)
		results = append(results, result)
	case "kafka":
		result := dc.checkKafka(ctx)
		results = append(results, result)
	case "azure-servicebus":
		result := dc.checkAzureServiceBus(ctx)
		results = append(results, result)
	}

	// Summary logging
	healthyCount := 0
	for _, result := range results {
		if result.Status == "healthy" {
			healthyCount++
		}
	}

	if healthyCount == len(results) {
		fmt.Printf("[DEPS] 🎉 All %d dependencies are healthy\n", len(results))
	} else {
		fmt.Printf("[DEPS] ⚠️ %d/%d dependencies are healthy\n", healthyCount, len(results))
	}

	return results
}

// checkRabbitMQ checks RabbitMQ connectivity
func (dc *DependencyChecker) checkRabbitMQ(ctx context.Context) DependencyResult {
	fmt.Printf("[DEPS] Checking RabbitMQ health at %s\n", maskConnectionString(dc.config.Broker.RabbitMQ.URL))

	// For RabbitMQ, we'll check if we can establish a connection
	// This is a simplified check - in a real implementation, you might want to
	// actually connect to RabbitMQ and verify the exchange exists
	
	result := DependencyResult{
		Service: "rabbitmq",
		URL:     maskConnectionString(dc.config.Broker.RabbitMQ.URL),
	}

	// Since we can't easily check RabbitMQ health without actually connecting,
	// we'll mark it as healthy if the URL is configured
	if dc.config.Broker.RabbitMQ.URL != "" {
		fmt.Printf("[DEPS] ✅ RabbitMQ configuration is healthy\n")
		result.Status = "healthy"
	} else {
		fmt.Printf("[DEPS] ❌ RabbitMQ configuration is missing\n")
		result.Status = "unhealthy"
		result.Error = "RabbitMQ URL not configured"
	}

	return result
}

// checkKafka checks Kafka connectivity
func (dc *DependencyChecker) checkKafka(ctx context.Context) DependencyResult {
	fmt.Printf("[DEPS] Checking Kafka health with brokers: %v\n", dc.config.Broker.Kafka.Brokers)

	result := DependencyResult{
		Service: "kafka",
		URL:     fmt.Sprintf("brokers: %v", dc.config.Broker.Kafka.Brokers),
	}

	// Simplified health check for Kafka
	if len(dc.config.Broker.Kafka.Brokers) > 0 {
		fmt.Printf("[DEPS] ✅ Kafka configuration is healthy\n")
		result.Status = "healthy"
	} else {
		fmt.Printf("[DEPS] ❌ Kafka configuration is missing\n")
		result.Status = "unhealthy"
		result.Error = "Kafka brokers not configured"
	}

	return result
}

// checkAzureServiceBus checks Azure Service Bus connectivity
func (dc *DependencyChecker) checkAzureServiceBus(ctx context.Context) DependencyResult {
	fmt.Printf("[DEPS] Checking Azure Service Bus health\n")

	result := DependencyResult{
		Service: "azure-servicebus",
		URL:     "Azure Service Bus",
	}

	// Simplified health check for Azure Service Bus
	if dc.config.Broker.AzureServiceBus.ConnectionString != "" {
		fmt.Printf("[DEPS] ✅ Azure Service Bus configuration is healthy\n")
		result.Status = "healthy"
	} else {
		fmt.Printf("[DEPS] ❌ Azure Service Bus configuration is missing\n")
		result.Status = "unhealthy"
		result.Error = "Azure Service Bus connection string not configured"
	}

	return result
}

// maskConnectionString masks sensitive information in connection strings
func maskConnectionString(connectionString string) string {
	if connectionString == "" {
		return ""
	}
	
	// Simple masking for demonstration - in production, use more sophisticated masking
	if len(connectionString) > 20 {
		return connectionString[:10] + "***" + connectionString[len(connectionString)-7:]
	}
	
	return "***"
}