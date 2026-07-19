package api

import (
	"context"
	"net"

	"github.com/gofiber/fiber/v2"
)

type API struct {
	router *fiber.App
}

func New(store Reader) *API {
	app := fiber.New(fiber.Config{})
	a := &API{router: app, store: store}
	a.routes()
	return a
}

func (a *API) routes() {
	//listH := &ListHandler{store: a.store}
	// statusH := &StatusHandler{store: a.store}

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
