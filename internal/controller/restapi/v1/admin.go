package v1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/controller/restapi/v1/request"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/gofiber/fiber/v2"
)

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
	albums, err := r.a.ListAlbums(ctx.UserContext(), parseIntQuery(ctx, "offset", 0), parseIntQuery(ctx, "limit", 50))
	if err != nil {
		r.l.Error(err, "restapi - v1 - listAlbums")
		return errorResponse(ctx, http.StatusInternalServerError, "failed to list albums")
	}
	return ctx.Status(http.StatusOK).JSON(albums)
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
	return ctx.SendStatus(http.StatusNoContent)
}

func (r *V1) listImages(ctx *fiber.Ctx) error {
	images, err := r.a.ListImages(
		ctx.UserContext(),
		parseIntQuery(ctx, "album_id", 0),
		parseIntQuery(ctx, "offset", 0),
		parseIntQuery(ctx, "limit", 50),
	)
	if err != nil {
		r.l.Error(err, "restapi - v1 - listImages")
		return errorResponse(ctx, http.StatusInternalServerError, "failed to list images")
	}
	return ctx.Status(http.StatusOK).JSON(images)
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
	return ctx.Status(http.StatusOK).JSON(out)
}
