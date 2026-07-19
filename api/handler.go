package api

import (
	"context"

	"github.com/gofiber/fiber/v2"
)

type Handler[Req, Resp any] interface {
	Handle(ctx context.Context, req *Req) (*Res, error)
}

func handle[Req, Res any](h Handler[Req, Res]) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req Req
		if err := c.ParamsParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(errBody(err))
		}
		if err := c.QueryParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(errBody(err))
		}
		res, err := h.Handle(context.Background(), &req)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(errBody(err))
		}
		return c.JSON(res)
	}
}

func errBody(err error) fiber.Map {
	return fiber.Map{"error": err.Error()}
}
