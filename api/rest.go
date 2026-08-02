package api

import (
	"context"
	"net"

	"github.com/gofiber/fiber/v2"
)

type API struct {
	router *fiber.App
	store  Reader
}

func New(store Reader) *API {
	app := fiber.New(fiber.Config{})
	a := &API{router: app, store: store}
	a.routes()
	return a
}

func (a *API) routes() {
	listH := &ListHandler{store: a.store}
	statusH := &StatusHandler{store: a.store}
	dashH := &DashboardHandler{store: a.store}

	a.router.Get("/v1/monitors", handle[ListRequest, ListResponse](listH))
	a.router.Get("/v1/monitors/:name/status", handle[StatusRequest, StatusResponse](statusH))
	a.router.Get("/v1/dashboard", handle[DashboardRequest, DashboardResponse](dashH))
}

func (a *API) Serve(ln net.Listener) error {
	return a.router.Listener(ln)
}

func (a *API) Shutdown(ctx context.Context) error {
	return a.router.ShutdownWithContext(ctx)
}
