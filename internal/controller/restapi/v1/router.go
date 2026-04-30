package v1

import (
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/logger"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

// NewTranslationRoutes -.
func NewTranslationRoutes(apiV1Group fiber.Router, t usecase.Translation, a usecase.Admin, l logger.Interface) {
	r := &V1{t: t, a: a, l: l, v: validator.New(validator.WithRequiredStructEnabled())}

	translationGroup := apiV1Group.Group("/translation")

	{
		translationGroup.Get("/history", r.history)
		translationGroup.Post("/do-translate", r.doTranslate)
	}
}

// NewAdminRoutes registers admin CRUD routes.
func NewAdminRoutes(adminGroup fiber.Router, a usecase.Admin, l logger.Interface) {
	r := &V1{a: a, l: l, v: validator.New(validator.WithRequiredStructEnabled())}
	adminGroup.Get("/albums", r.listAlbums)
	adminGroup.Post("/albums", r.createAlbum)
	adminGroup.Post("/albums/:id/send-test", r.sendAlbumTest)
	adminGroup.Get("/albums/:id", r.getAlbum)
	adminGroup.Patch("/albums/:id", r.updateAlbum)
	adminGroup.Delete("/albums/:id", r.deleteAlbum)

	adminGroup.Get("/images", r.listImages)
	adminGroup.Post("/images", r.createImage)
	adminGroup.Get("/images/:id", r.getImage)
	adminGroup.Patch("/images/:id", r.updateImage)
	adminGroup.Delete("/images/:id", r.deleteImage)

	adminGroup.Get("/schedule", r.getSchedule)
	adminGroup.Put("/schedule", r.putSchedule)
	adminGroup.Post("/schedule/trigger-now", r.triggerScheduleNow)
	adminGroup.Get("/system/status", r.getSystemStatus)
}
