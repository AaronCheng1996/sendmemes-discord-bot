// Package v1 implements routing paths. Each services in own file.
package restapi

import (
	"net/http"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	_ "github.com/AaronCheng1996/sendmemes-discord-bot/docs" // Swagger docs.
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/controller/restapi/middleware"
	v1 "github.com/AaronCheng1996/sendmemes-discord-bot/internal/controller/restapi/v1"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/logger"
	"github.com/AaronCheng1996/sendmemes-discord-bot/sample"
	"github.com/ansrivas/fiberprometheus/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/swagger"
)

// NewRouter -.
// Swagger spec:
// @title       Go Clean Template API
// @description Using a translation service as an example
// @version     1.0
// @host        localhost:8080
// @BasePath    /v1
func NewRouter(app *fiber.App, cfg *config.Config, t usecase.Translation, a usecase.Admin, l logger.Interface) {
	// Options
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, X-Admin-Key, Authorization",
		AllowMethods: "GET, POST, PUT, PATCH, DELETE, OPTIONS",
	}))
	app.Use(middleware.Logger(l))
	app.Use(middleware.Recovery(l))

	// Prometheus metrics
	if cfg.Metrics.Enabled {
		prometheus := fiberprometheus.New("my-service-name")
		prometheus.RegisterAt(app, "/metrics")
		app.Use(prometheus.Middleware)
	}

	// Swagger
	if cfg.Swagger.Enabled {
		app.Get("/swagger/*", swagger.HandlerDefault)
	}

	// K8s probe
	app.Get("/healthz", func(ctx *fiber.Ctx) error { return ctx.SendStatus(http.StatusOK) })

	// Embedded sample image for default /image
	app.Get("/assets/sample/image.png", func(ctx *fiber.Ctx) error {
		ctx.Set("Content-Type", "image/png")
		return ctx.Send(sample.ImagePNG)
	})

	// Routers
	apiV1Group := app.Group("/v1")
	{
		v1.NewTranslationRoutes(apiV1Group, t, a, l)
		adminGroup := apiV1Group.Group("/admin", middleware.AdminAPIKey(cfg.Admin.APIKey))
		v1.NewAdminRoutes(adminGroup, a, l)
	}
}
