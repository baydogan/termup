package api

import (
	"errors"

	"github.com/baydogan/termup/storage"
	"github.com/gofiber/fiber/v2"
)

func httpStatus(err error) int {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return fiber.StatusNotFound
	default:
		return fiber.StatusInternalServerError
	}
}
