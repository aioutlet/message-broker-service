package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aioutlet/message-broker-service/internal/config"
	"github.com/aioutlet/message-broker-service/internal/health"
	"github.com/aioutlet/message-broker-service/internal/logger"
	"github.com/aioutlet/message-broker-service/internal/server"
	"github.com/aioutlet/message-broker-service/internal/tracing"
	"github.com/joho/godotenv"
)

func main() {
	// Industry-standard initialization pattern:
	// 1. Load environment variables
	// 2. Validate configuration (blocking - must pass)
	// 3. Check dependency health (wait for completion)
	// 4. Initialize observability (logger)
	// 5. Start application

	// Step 1: Load environment variables
	fmt.Println("Step 1: Loading environment variables...")
	if err := godotenv.Load(); err != nil {
		fmt.Println("Warning: .env file not found, using system environment variables")
	}

	// Step 2: Load and validate configuration (includes validation)
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("❌ Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Step 3: Check dependency health
	fmt.Println("Step 3: Checking dependency health...")
	healthChecker := health.NewDependencyChecker(cfg)
	healthCtx, healthCancel := context.WithTimeout(context.Background(), 10*time.Second)
	results := healthChecker.CheckDependencies(healthCtx)
	healthCancel()

	// Log dependency health results but don't block startup
	unhealthyCount := 0
	for _, result := range results {
		if result.Status != "healthy" {
			unhealthyCount++
			fmt.Printf("[DEPS] ⚠️ %s is %s: %s\n", result.Service, result.Status, result.Error)
		}
	}

	// Step 4: Initialize observability (logger + tracing)
	fmt.Println("Step 4: Initializing observability...")
	log, err := logger.New(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		fmt.Printf("❌ Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	// Initialize distributed tracing
	_, err = tracing.InitTracing(cfg, log)
	if err != nil {
		log.Error("Failed to initialize tracing", "error", err)
		// Don't exit - tracing is optional, continue without it
	} else if cfg.Observability.EnableTracing {
		log.Info("Distributed tracing initialized successfully")
		// Ensure tracing is properly shut down
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := tracing.Shutdown(shutdownCtx, log); err != nil {
				log.Error("Failed to shutdown tracing", "error", err)
			}
		}()
	}

	log.Info("Starting message-broker-service",
		"version", "1.0.0",
		"environment", cfg.Environment,
		"brokerType", cfg.Broker.Type,
		"dependenciesHealthy", len(results)-unhealthyCount,
		"dependenciesTotal", len(results),
	)

	// Step 5: Start the application
	fmt.Println("Step 5: Starting message broker service...")
	// Create server
	srv, err := server.New(cfg, log)
	if err != nil {
		log.Fatal("Failed to create server", "error", err)
	}

	// Start server in a goroutine
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatal("Server failed to start", "error", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server gracefully...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
	}

	log.Info("Server stopped")
}
