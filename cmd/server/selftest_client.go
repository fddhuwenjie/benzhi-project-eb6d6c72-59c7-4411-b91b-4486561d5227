package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type apiEnvelope[T any] struct {
	Data T `json:"data"`
}

type smokeClient struct {
	baseURL string
	client  *http.Client
}

func newSmokeClient(addr string) *smokeClient {
	return &smokeClient{baseURL: "http://" + addr, client: &http.Client{Timeout: 5 * time.Second}}
}

func doJSON[T any](c *smokeClient, ctx context.Context, method, path string, body any, actor, requestID string, revision int64) (T, error) {
	var zero T
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return zero, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return zero, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if actor != "" {
		req.Header.Set("X-Actor-ID", actor)
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	if revision > 0 {
		req.Header.Set("If-Match", strconv.FormatInt(revision, 10))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return zero, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("%s %s 返回 %d: %s", method, path, resp.StatusCode, string(responseBody))
	}
	var envelope apiEnvelope[T]
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return zero, fmt.Errorf("解析响应: %w", err)
	}
	return envelope.Data, nil
}
