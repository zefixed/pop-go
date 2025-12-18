package builder

import (
	"log/slog"
	"pop-go/internal/models"
)

type Builder struct {
	cfg *models.Config
	log *slog.Logger
}

func NewBuilder(cfg *models.Config, log *slog.Logger) *Builder {
	return &Builder{
		cfg: cfg,
		log: log,
	}
}
