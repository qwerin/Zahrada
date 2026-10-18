package api

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

type Health struct {
	Status  string `json:"status" example:"ok"`
	Version string `json:"version" example:"1.2.0+abc1234" doc:"Build version"`
}

func registerHealth(g huma.API) {
	huma.Get(g, "/api/health", func(ctx context.Context, _ *struct{}) (*Out[Health], error) {
		return &Out[Health]{Body: Health{Status: "ok", Version: AppVersion()}}, nil
	})
}
