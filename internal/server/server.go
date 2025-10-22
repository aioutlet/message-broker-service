package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	"github.com/aioutlet/message-broker-service/internal/broker"
	"github.com/aioutlet/message-broker-service/internal/config"
	"github.com/aioutlet/message-broker-service/internal/logger"
	"github.com/aioutlet/message-broker-service/internal/server/handlers"
	"github.com/aioutlet/message-broker-service/internal/server/middleware"
)

// Server represents the HTTP server
type Server struct {
	app    *fiber.App
	config *config.Config
	log    *logger.Logger
	broker *broker.Manager
}

// New creates a new server instance
func New(cfg *config.Config, log *logger.Logger) (*Server, error) {
	// Create broker manager
	brokerMgr, err := broker.NewManager(&cfg.Broker, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create broker manager: %w", err)
	}

	// Connect to broker
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := brokerMgr.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to broker: %w", err)
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:               cfg.ServiceName,
		DisableStartupMessage: false,
		ReadTimeout:           cfg.API.RequestTimeout,
		WriteTimeout:          cfg.API.RequestTimeout,
		BodyLimit:             parseBodyLimit(cfg.API.MaxRequestSize),
		ErrorHandler:          customErrorHandler(log),
	})

	// Setup middleware
	setupMiddleware(app, cfg, log)

	// Create handlers
	h := handlers.NewHandler(brokerMgr, cfg, log)

	// Setup routes
	setupRoutes(app, h, cfg)

	return &Server{
		app:    app,
		config: cfg,
		log:    log,
		broker: brokerMgr,
	}, nil
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.config.Port)
	s.log.Info("Starting HTTP server", "address", addr)
	return s.app.Listen(addr)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("Shutting down HTTP server...")

	// Shutdown HTTP server
	if err := s.app.ShutdownWithContext(ctx); err != nil {
		s.log.Error("Error shutting down HTTP server", "error", err)
	}

	// Disconnect from broker
	if err := s.broker.Disconnect(ctx); err != nil {
		s.log.Error("Error disconnecting from broker", "error", err)
	}

	return nil
}

// setupMiddleware configures all middleware
func setupMiddleware(app *fiber.App, cfg *config.Config, log *logger.Logger) {
	// Request ID middleware
	app.Use(requestid.New())

	// Logger middleware
	app.Use(middleware.Logger(log))

	// Recover middleware
	app.Use(recover.New(recover.Config{
		EnableStackTrace: cfg.Environment == "development",
	}))

	// CORS middleware
	app.Use(cors.New(cors.Config{
		AllowOrigins: joinStrings(cfg.API.CORSOrigins),
		AllowMethods: "GET,POST,HEAD,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-Request-ID",
	}))

	// Rate limiting middleware
	if cfg.RateLimit.Enabled {
		app.Use(limiter.New(limiter.Config{
			Max:        cfg.RateLimit.RequestsPerSecond,
			Expiration: time.Second,
			Storage:    nil, // Use in-memory storage
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"success": false,
					"error":   "rate limit exceeded",
					"code":    "RATE_LIMIT_EXCEEDED",
				})
			},
		}))
	}
}

// setupRoutes configures all routes
func setupRoutes(app *fiber.App, h *handlers.Handler, cfg *config.Config) {
	// Root health endpoint for standardized dependency checking
	app.Get("/health", h.Health)

	// API v1 routes
	api := app.Group("/api/v1")

	// Public routes
	api.Get("/health", h.Health) // Keep for API consistency
	api.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": cfg.ServiceName,
			"version": "1.0.0",
			"status":  "running",
		})
	})

	// Protected routes (require API key)
	protected := api.Group("", middleware.Auth(cfg.API.Key))
	protected.Post("/publish", h.Publish)
	protected.Post("/events/publish", h.Publish) // Alternative endpoint
	protected.Get("/stats", h.Stats)
}

// customErrorHandler handles Fiber errors
func customErrorHandler(log *logger.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		message := "Internal server error"

		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			message = e.Message
		}

		log.Error("Request error",
			"path", c.Path(),
			"method", c.Method(),
			"error", err.Error(),
			"status", code,
		)

		return c.Status(code).JSON(fiber.Map{
			"success": false,
			"error":   message,
		})
	}
}

// parseBodyLimit parses body limit string to bytes
func parseBodyLimit(limit string) int {
	// Simple parser for MB
	var size int
	fmt.Sscanf(limit, "%dMB", &size)
	return size * 1024 * 1024
}

// joinStrings joins string slice with comma
func joinStrings(strs []string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += ","
		}
		result += s
	}
	return result
}
