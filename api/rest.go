package api

import (
	"context"
	"net"

	"github.com/gofiber/fiber/v2"
)

type API struct {
	router *fiber.App
}

func New() *API {
	app := fiber.New(
		fiber.Config{
			DisableStartupMessage: true,
		})

	api := &API{router: app}
	api.routes()
	return api
}

func (a *API) routes() {
	// a.router.Get("/v1/monitors", a.listMonitors)
	// a.router.Get("/v1/monitors/:name/status", a.status)
	// a.router.Get("/v1/watch", a.watch) // SSE (fasthttp stream)
}

func (a *API) Serve(ln net.Listener) error {
	return a.router.Listener(ln)
}

func (a *API) Shutdown(ctx context.Context) error {
	return a.router.ShutdownWithContext(ctx)
}
