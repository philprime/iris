/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/philprime/iris/api/v1alpha1"
)

// HTTPTarget is a resolved HTTP destination: the endpoint, method, payload
// shaping, and the bearer token read from its referenced Secret.
type HTTPTarget struct {
	URL       string
	Method    string
	Format    v1alpha1.PayloadFormat
	Jsonnet   string
	AuthToken string
	Client    *http.Client
}

// deliverHTTP posts the rendered body to the destination, attaching the
// idempotency key and, when present, a bearer Authorization header. A non-2xx
// response is a delivery failure.
func deliverHTTP(ctx context.Context, t *HTTPTarget, idempotencyKey string, body []byte, contentType string) error {
	method := t.Method
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequestWithContext(ctx, method, t.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	if t.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+t.AuthToken)
	}

	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("deliver http: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("destination returned status %d", resp.StatusCode)
	}
	return nil
}
