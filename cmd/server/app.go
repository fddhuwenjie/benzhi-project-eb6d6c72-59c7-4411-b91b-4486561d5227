package main

import (
	"fmt"
	"net/http"
	"time"

	"archive-review/internal/httpapi"
	"archive-review/internal/redaction"
	"archive-review/internal/store"
	"archive-review/internal/workflow"
)

func buildServer(dataDir string) (*http.Server, error) {
	repo, err := store.OpenDisk(dataDir)
	if err != nil {
		return nil, fmt.Errorf("打开持久化仓储: %w", err)
	}
	service := workflow.New(repo, redaction.NewDefaultDetector())
	api := httpapi.New(service)
	return &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 32 << 10}, nil
}
