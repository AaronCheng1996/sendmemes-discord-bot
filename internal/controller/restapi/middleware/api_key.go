package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// AdminAPIKey validates the configured admin key from header.

func AdminAPIKey(apiKey string) func(c *fiber.Ctx) error {

	return APIKey(apiKey, "X-Admin-Key", "admin api key")

}

// IngestAPIKey validates the configured ingest key. It is deliberately a

// separate credential from the admin key: an ingest client runs elsewhere (the

// crawler, on another host), and a leaked key should let someone append run

// records, not rewrite delivery rules or delete albums.

func IngestAPIKey(apiKey string) func(c *fiber.Ctx) error {

	return APIKey(apiKey, "X-Ingest-Key", "ingest api key")

}

// APIKey compares the credential in header (or a Bearer Authorization header)

// against apiKey in constant time. An unconfigured key rejects everything

// rather than opening the route.

func APIKey(apiKey, header, label string) func(c *fiber.Ctx) error {

	return func(ctx *fiber.Ctx) error {

		if apiKey == "" {

			return fiber.NewError(fiber.StatusForbidden, label+" is not configured")

		}

		got := strings.TrimSpace(ctx.Get(header))

		if got == "" {

			auth := strings.TrimSpace(ctx.Get("Authorization"))

			const prefix = "Bearer "

			if strings.HasPrefix(auth, prefix) {

				got = strings.TrimSpace(strings.TrimPrefix(auth, prefix))

			}

		}

		if subtle.ConstantTimeCompare([]byte(got), []byte(apiKey)) != 1 {

			return fiber.NewError(fiber.StatusUnauthorized, "invalid "+label)

		}

		return ctx.Next()

	}

}
