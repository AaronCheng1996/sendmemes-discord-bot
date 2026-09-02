package v1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/controller/restapi/v1/request"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/logger"
	"github.com/gofiber/fiber/v2"
)

// Runs holds the ingest routes' dependencies. They are separate from V1's

// because they authenticate with a different, weaker credential and must never

// gain access to the admin use case.

type Runs struct {
	runs usecase.TaskRuns

	l logger.Interface
}

// createRun records one execution. A body carrying a terminal status reports a

// finished run in a single call, which is all a crawler pass needs; status

// "running" opens a row for updateRun to close.

func (r *Runs) createRun(ctx *fiber.Ctx) error {

	var body request.TaskRunCreate

	if err := ctx.BodyParser(&body); err != nil {

		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")

	}

	run := entity.TaskRun{

		Source: strings.TrimSpace(body.Source),

		Task: strings.TrimSpace(body.Task),

		Status: strings.TrimSpace(body.Status),

		Summary: body.Summary,

		Detail: body.Detail,

		Error: body.Error,
	}

	if body.StartedAt != nil {

		run.StartedAt = *body.StartedAt

	}

	if body.FinishedAt != nil {

		run.FinishedAt = body.FinishedAt

	}

	saved, err := r.runs.Record(ctx.UserContext(), run)

	if err != nil {

		r.l.Error(err, "restapi - v1 - createRun")

		return errorResponse(ctx, http.StatusBadRequest, err.Error())

	}

	return ctx.Status(http.StatusCreated).JSON(saved)

}

// updateRun closes a run opened with status "running".

func (r *Runs) updateRun(ctx *fiber.Ctx) error {

	id, err := strconv.ParseInt(strings.TrimSpace(ctx.Params("id")), 10, 64)

	if err != nil {

		return errorResponse(ctx, http.StatusBadRequest, "invalid id")

	}

	var body request.TaskRunComplete

	if err = ctx.BodyParser(&body); err != nil {

		return errorResponse(ctx, http.StatusBadRequest, "invalid request body")

	}

	outcome := entity.TaskRun{

		Status: strings.TrimSpace(body.Status),

		Summary: body.Summary,

		Detail: body.Detail,

		Error: body.Error,
	}

	if body.FinishedAt != nil {

		outcome.FinishedAt = body.FinishedAt

	}

	saved, err := r.runs.Complete(ctx.UserContext(), id, outcome)

	if err != nil {

		r.l.Error(err, "restapi - v1 - updateRun")

		return errorResponse(ctx, http.StatusBadRequest, err.Error())

	}

	return ctx.Status(http.StatusOK).JSON(saved)

}
