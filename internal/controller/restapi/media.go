package restapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/gofiber/fiber/v2"
)

// registerLocalMediaRoute serves files under root at GET /media/*. It is only
// registered when MEDIA_SOURCE=local (the pCloud source never needs it) and is
// deliberately NOT behind the admin key: both Discord's CDN and a browser
// rendering the dashboard need to fetch it anonymously.
func registerLocalMediaRoute(app *fiber.App, cfg *config.Config) {
	if !strings.EqualFold(strings.TrimSpace(cfg.Media.Source), entity.MediaSourceLocal) {
		return
	}
	root := filepath.Clean(cfg.Media.LocalRoot)

	app.Get("/media/*", func(ctx *fiber.Ctx) error {
		full, ok := resolveMediaPath(root, ctx.Params("*"))
		if !ok {
			return ctx.SendStatus(http.StatusForbidden)
		}

		data, err := os.ReadFile(full)
		if err != nil {
			return ctx.SendStatus(http.StatusNotFound)
		}

		ctx.Type(filepath.Ext(full))
		return ctx.Send(data)
	})
}

// resolveMediaPath joins rel onto root and returns the resulting absolute
// path, or ok=false when rel is empty, names an unrecognized media extension,
// or — via enough "../" segments (e.g. "../../etc/passwd") — would resolve
// outside root. filepath.Join+Clean alone does not prevent that escape, so
// the prefix check below is what actually blocks path traversal.
func resolveMediaPath(root, rel string) (string, bool) {
	if rel == "" {
		return "", false
	}
	if _, ok := entity.KindOfExtension(rel); !ok {
		return "", false
	}
	full := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}
