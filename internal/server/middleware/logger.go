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
		requestId := c.Get("X-Request-ID")

		if requestId == "" {
			requestId = "none"
		}

		log.Infof("🌐 HTTP Request: method=%s | path=%s | status=%d | duration=%s | ip=%s | requestId=%s | userAgent=%s",
			c.Method(),
			c.Path(),
			status,
			duration.String(),
			c.IP(),
			requestId,
			c.Get("User-Agent"),
		)

		return err
	}
}
