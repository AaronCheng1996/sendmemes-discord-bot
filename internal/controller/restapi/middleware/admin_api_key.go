package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// AdminAPIKey validates the configured admin key from header.
func AdminAPIKey(apiKey string) func(c *fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		if apiKey == "" {
			return fiber.NewError(fiber.StatusForbidden, "admin api key is not configured")
		}
		got := strings.TrimSpace(ctx.Get("X-Admin-Key"))
		if got == "" {
			auth := strings.TrimSpace(ctx.Get("Authorization"))
			const prefix = "Bearer "
			if strings.HasPrefix(auth, prefix) {
				got = strings.TrimSpace(strings.TrimPrefix(auth, prefix))
			}
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(apiKey)) != 1 {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid admin api key")
		}
		return ctx.Next()
	}
}
