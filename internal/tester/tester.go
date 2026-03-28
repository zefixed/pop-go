package tester

import (
	"log/slog"
	"pop-go/internal/models"
)

type Tester struct {
	cfg *models.Config
	log *slog.Logger
}

func NewTester(cfg *models.Config, log *slog.Logger) *Tester {
	return &Tester{
		cfg: cfg,
		log: log,
	}
}
