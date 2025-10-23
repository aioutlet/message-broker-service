package middleware

import (
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

		// Check X-API-Key header (standard for API keys)
		token := c.Get("X-API-Key")
		
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
				Success: false,
				Error:   "missing X-API-Key header",
				Code:    "UNAUTHORIZED",
			})
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
