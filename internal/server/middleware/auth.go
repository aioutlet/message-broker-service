package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/aioutlet/message-broker-service/internal/models"
)

// Auth is a middleware that validates the API key
func Auth(apiKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip authentication if API key is not configured
		if apiKey == "" {
			return c.Next()
		}

		// Get authorization header
		auth := c.Get("Authorization")
		if auth == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
				Success: false,
				Error:   "missing authorization header",
				Code:    "UNAUTHORIZED",
			})
		}

		// Extract token
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth {
			// Try API-Key header as alternative
			token = c.Get("X-API-Key")
		}

		// Validate token
		if token != apiKey {
			return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
				Success: false,
				Error:   "invalid API key",
				Code:    "INVALID_API_KEY",
			})
		}

		return c.Next()
	}
}
