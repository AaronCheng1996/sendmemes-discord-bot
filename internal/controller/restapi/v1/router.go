package v1

import (
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/logger"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

// NewIngestRoutes registers the write-only run-reporting routes used by
// external clients. They live outside the admin group deliberately: a crawler
// should be able to append run records without holding a key that can rewrite
// delivery rules.
func NewIngestRoutes(ingestGroup fiber.Router, runs usecase.TaskRuns, l logger.Interface) {
	r := &Runs{runs: runs, l: l}
	ingestGroup.Post("/runs", r.createRun)
	ingestGroup.Patch("/runs/:id", r.updateRun)
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
	adminGroup.Get("/albums/:id/media", r.listAlbumMedia)

	adminGroup.Get("/images", r.listImages)
	adminGroup.Post("/images", r.createImage)
	adminGroup.Get("/images/:id", r.getImage)
	adminGroup.Patch("/images/:id", r.updateImage)
	adminGroup.Delete("/images/:id", r.deleteImage)

	adminGroup.Get("/delivery-rules", r.listRules)
	adminGroup.Post("/delivery-rules", r.createRule)
	adminGroup.Get("/delivery-rules/:id", r.getRule)
	adminGroup.Patch("/delivery-rules/:id", r.updateRule)
	adminGroup.Delete("/delivery-rules/:id", r.deleteRule)
	adminGroup.Post("/delivery-rules/:id/test", r.testRule)

	adminGroup.Post("/schedule/trigger-now", r.triggerScheduleNow)

	adminGroup.Get("/sync-settings", r.getSyncSettings)
	adminGroup.Put("/sync-settings", r.putSyncSettings)
	adminGroup.Get("/ingest-key", r.getIngestKeyStatus)
	adminGroup.Put("/ingest-key", r.putIngestKey)
	adminGroup.Put("/message-defaults", r.putMessageDefaults)
	adminGroup.Post("/sync/trigger-now", r.triggerSyncNow)
	adminGroup.Get("/sync-events", r.listSyncEvents)
	adminGroup.Get("/sync-events/:id/media", r.listSyncEventMedia)
	adminGroup.Get("/task-runs", r.listTaskRuns)
	adminGroup.Get("/task-runs/sources", r.listTaskRunSources)
	adminGroup.Get("/system/status", r.getSystemStatus)
	adminGroup.Get("/jobs", r.listJobs)
}
