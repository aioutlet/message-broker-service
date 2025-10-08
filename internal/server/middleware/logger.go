package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/aioutlet/message-broker-service/internal/logger"
)

// Logger returns a middleware that logs HTTP requests
func Logger(log *logger.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Process request
		err := c.Next()

		// Log request
		duration := time.Since(start)
		status := c.Response().StatusCode()

		log.Info("HTTP request",
			"method", c.Method(),
			"path", c.Path(),
			"status", status,
			"duration", duration.String(),
			"ip", c.IP(),
			"requestId", c.Get("X-Request-ID"),
		)

		return err
	}
}
