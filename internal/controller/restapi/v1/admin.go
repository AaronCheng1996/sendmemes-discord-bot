package v1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/controller/restapi/v1/request"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/controller/restapi/v1/response"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/gofiber/fiber/v2"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// clampPagination normalises offset/limit so callers cannot abuse the API.
func clampPagination(offset, limit int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	return offset, limit
}

func actorFromCtx(ctx *fiber.Ctx) string {
	actor := strings.TrimSpace(ctx.Get("X-Admin-Actor"))
	if actor == "" {
		return "api_key"
	}
	return actor
}

func parseIntQuery(ctx *fiber.Ctx, key string, defaultVal int) int {
	v := strings.TrimSpace(ctx.Query(key))
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func parseIDParam(ctx *fiber.Ctx) (int, error) {
	return strconv.Atoi(strings.TrimSpace(ctx.Params("id")))
}

func (r *V1) listAlbums(ctx *fiber.Ctx) error {
	offset, limit := clampPagination(parseIntQuery(ctx, "offset", 0), parseIntQuery(ctx, "limit", defaultListLimit))
	albums, total, err := r.a.ListAlbums(ctx.UserContext(), offset, limit)
	if err != nil {
		r.l.Error(err, "restapi - v1 - listAlbums")
		return errorResponse(ctx, http.StatusInternalServerError, "failed to list albums")
	}
	if albums == nil {
		albums = []entity.Album{}
	}
	return ctx.Status(http.StatusOK).JSON(response.Page[entity.Album]{
		Items:  albums,
		Total:  total,
		Offset: offset,
		Limit:  limit,
	})
}

func (r *V1) createAlbum(ctx *fiber.Ctx) error {
	var body request.AlbumCreate
	if err := ctx.BodyParser(&body); err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")
	}
	if err := r.v.Struct(body); err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")
	}
	album, err := r.a.CreateAlbum(ctx.UserContext(), body.Name)
	if err != nil {
		r.l.Error(err, "restapi - v1 - createAlbum")
		return errorResponse(ctx, http.StatusBadRequest, err.Error())
	}
	_ = r.a.RecordAudit(ctx.UserContext(), actorFromCtx(ctx), "album.create", "album", strconv.Itoa(album.ID), map[string]any{"name": album.Name})
	return ctx.Status(http.StatusCreated).JSON(album)
}

func (r *V1) getAlbum(ctx *fiber.Ctx) error {
	id, err := parseIDParam(ctx)
	if err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid id")
	}
	album, err := r.a.GetAlbum(ctx.UserContext(), id)
	if err != nil {
		r.l.Error(err, "restapi - v1 - getAlbum")
		return errorResponse(ctx, http.StatusNotFound, "album not found")
	}
	return ctx.Status(http.StatusOK).JSON(album)
}

func (r *V1) updateAlbum(ctx *fiber.Ctx) error {
	id, err := parseIDParam(ctx)
	if err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid id")
	}
	var body request.AlbumUpdate
	if err = ctx.BodyParser(&body); err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")
	}
	if err = r.v.Struct(body); err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")
	}
	album, err := r.a.UpdateAlbum(ctx.UserContext(), id, body.Name)
	if err != nil {
		r.l.Error(err, "restapi - v1 - updateAlbum")
		return errorResponse(ctx, http.StatusBadRequest, err.Error())
	}
	_ = r.a.RecordAudit(ctx.UserContext(), actorFromCtx(ctx), "album.update", "album", strconv.Itoa(id), map[string]any{"name": body.Name})
	return ctx.Status(http.StatusOK).JSON(album)
}

func (r *V1) deleteAlbum(ctx *fiber.Ctx) error {
	id, err := parseIDParam(ctx)
	if err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid id")
	}
	if err = r.a.DeleteAlbum(ctx.UserContext(), id); err != nil {
		r.l.Error(err, "restapi - v1 - deleteAlbum")
		return errorResponse(ctx, http.StatusBadRequest, "failed to delete album")
	}
	_ = r.a.RecordAudit(ctx.UserContext(), actorFromCtx(ctx), "album.delete", "album", strconv.Itoa(id), nil)
	return ctx.SendStatus(http.StatusNoContent)
}

func (r *V1) listImages(ctx *fiber.Ctx) error {
	offset, limit := clampPagination(parseIntQuery(ctx, "offset", 0), parseIntQuery(ctx, "limit", defaultListLimit))
	images, total, err := r.a.ListImages(
		ctx.UserContext(),
		parseIntQuery(ctx, "album_id", 0),
		offset,
		limit,
	)
	if err != nil {
		r.l.Error(err, "restapi - v1 - listImages")
		return errorResponse(ctx, http.StatusInternalServerError, "failed to list images")
	}
	if images == nil {
		images = []entity.Image{}
	}
	return ctx.Status(http.StatusOK).JSON(response.Page[entity.Image]{
		Items:  images,
		Total:  total,
		Offset: offset,
		Limit:  limit,
	})
}

func (r *V1) createImage(ctx *fiber.Ctx) error {
	var body request.ImageCreate
	if err := ctx.BodyParser(&body); err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")
	}
	if err := r.v.Struct(body); err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")
	}
	img, err := r.a.CreateImage(ctx.UserContext(), entity.Image{
		URL:     body.URL,
		Source:  body.Source,
		GuildID: body.GuildID,
		AlbumID: body.AlbumID,
		FileID:  body.FileID,
	})
	if err != nil {
		r.l.Error(err, "restapi - v1 - createImage")
		return errorResponse(ctx, http.StatusBadRequest, err.Error())
	}
	_ = r.a.RecordAudit(ctx.UserContext(), actorFromCtx(ctx), "image.create", "image", strconv.Itoa(img.ID), map[string]any{"url": img.URL})
	return ctx.Status(http.StatusCreated).JSON(img)
}

func (r *V1) getImage(ctx *fiber.Ctx) error {
	id, err := parseIDParam(ctx)
	if err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid id")
	}
	img, err := r.a.GetImage(ctx.UserContext(), id)
	if err != nil {
		r.l.Error(err, "restapi - v1 - getImage")
		return errorResponse(ctx, http.StatusNotFound, "image not found")
	}
	return ctx.Status(http.StatusOK).JSON(img)
}

func (r *V1) updateImage(ctx *fiber.Ctx) error {
	id, err := parseIDParam(ctx)
	if err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid id")
	}
	var body request.ImageUpdate
	if err = ctx.BodyParser(&body); err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")
	}
	if err = r.v.Struct(body); err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")
	}
	img, err := r.a.UpdateImage(ctx.UserContext(), entity.Image{
		ID:      id,
		URL:     body.URL,
		Source:  body.Source,
		GuildID: body.GuildID,
		AlbumID: body.AlbumID,
		FileID:  body.FileID,
	})
	if err != nil {
		r.l.Error(err, "restapi - v1 - updateImage")
		return errorResponse(ctx, http.StatusBadRequest, err.Error())
	}
	_ = r.a.RecordAudit(ctx.UserContext(), actorFromCtx(ctx), "image.update", "image", strconv.Itoa(id), map[string]any{"url": body.URL})
	return ctx.Status(http.StatusOK).JSON(img)
}

func (r *V1) deleteImage(ctx *fiber.Ctx) error {
	id, err := parseIDParam(ctx)
	if err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid id")
	}
	if err = r.a.DeleteImage(ctx.UserContext(), id); err != nil {
		r.l.Error(err, "restapi - v1 - deleteImage")
		return errorResponse(ctx, http.StatusBadRequest, "failed to delete image")
	}
	_ = r.a.RecordAudit(ctx.UserContext(), actorFromCtx(ctx), "image.delete", "image", strconv.Itoa(id), nil)
	return ctx.SendStatus(http.StatusNoContent)
}

func (r *V1) getSchedule(ctx *fiber.Ctx) error {
	guildID := strings.TrimSpace(ctx.Query("guild_id"))
	out, err := r.a.GetEffectiveSchedule(ctx.UserContext(), guildID)
	if err != nil {
		r.l.Error(err, "restapi - v1 - getSchedule")
		return errorResponse(ctx, http.StatusInternalServerError, "failed to resolve schedule")
	}
	return ctx.Status(http.StatusOK).JSON(out)
}

func (r *V1) putSchedule(ctx *fiber.Ctx) error {
	var body request.SchedulePut
	if err := ctx.BodyParser(&body); err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")
	}
	out, err := r.a.UpsertSchedule(ctx.UserContext(), entity.DiscordScheduleSettings{
		GuildID:         strings.TrimSpace(body.GuildID),
		SendChannelID:   strings.TrimSpace(body.SendChannelID),
		SendInterval:    strings.TrimSpace(body.SendInterval),
		SendHistorySize: body.SendHistorySize,
	})
	if err != nil {
		r.l.Error(err, "restapi - v1 - putSchedule")
		return errorResponse(ctx, http.StatusBadRequest, err.Error())
	}
	_ = r.a.RecordAudit(ctx.UserContext(), actorFromCtx(ctx), "schedule.update", "schedule", strings.TrimSpace(body.GuildID), map[string]any{
		"send_channel_id":   body.SendChannelID,
		"send_interval":     body.SendInterval,
		"send_history_size": body.SendHistorySize,
	})
	return ctx.Status(http.StatusOK).JSON(out)
}

func (r *V1) getSystemStatus(ctx *fiber.Ctx) error {
	guildID := strings.TrimSpace(ctx.Query("guild_id"))
	out, err := r.a.GetSystemStatus(ctx.UserContext(), guildID)
	if err != nil {
		r.l.Error(err, "restapi - v1 - getSystemStatus")
		return errorResponse(ctx, http.StatusInternalServerError, "failed to get system status")
	}
	return ctx.Status(http.StatusOK).JSON(out)
}

func (r *V1) triggerScheduleNow(ctx *fiber.Ctx) error {
	var body request.ScheduleTriggerNow
	if err := ctx.BodyParser(&body); err != nil {
		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")
	}
	res, err := r.a.TriggerScheduleNow(ctx.UserContext(), strings.TrimSpace(body.GuildID), actorFromCtx(ctx))
	if err != nil {
		r.l.Error(err, "restapi - v1 - triggerScheduleNow")
		return errorResponse(ctx, http.StatusBadRequest, err.Error())
	}
	return ctx.Status(http.StatusOK).JSON(res)
}
