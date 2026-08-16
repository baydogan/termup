package api

import (
	"context"
	"net"

	"github.com/gofiber/fiber/v2"
)

type API struct {
	router *fiber.App
	store  Reader
	state  StateReader
}

func New(store Reader, state StateReader) *API {
	app := fiber.New(fiber.Config{})
	a := &API{router: app, store: store, state: state}
	a.routes()
	return a
}

func (a *API) routes() {
	statusH := &StatusHandler{store: a.store, state: a.state}
	dashH := &DashboardHandler{store: a.store, state: a.state}

	a.router.Get("/v1/monitors/:name/status", handle[StatusRequest, StatusResponse](statusH))
	a.router.Get("/v1/dashboard", handle[DashboardRequest, DashboardResponse](dashH))
}

func (a *API) Serve(ln net.Listener) error {
	return a.router.Listener(ln)
}

func (a *API) Shutdown(ctx context.Context) error {
	return a.router.ShutdownWithContext(ctx)
}
