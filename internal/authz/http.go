package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTP posts authorization requests to an external policy service.
type HTTP struct {
	URL        string
	TimeoutSec int
	client     *http.Client
}

func (h *HTTP) clientOrDefault() *http.Client {
	if h.client != nil {
		return h.client
	}
	sec := h.TimeoutSec
	if sec <= 0 {
		sec = 5
	}
	h.client = &http.Client{Timeout: time.Duration(sec) * time.Second}
	return h.client
}

func (h *HTTP) Authorize(ctx context.Context, subject Subject, resource Resource) error {
	body, err := json.Marshal(Request{Subject: subject, Resource: resource})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := h.clientOrDefault().Do(req)
	if err != nil {
		return fmt.Errorf("authz http: %w", err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized {
		var out Response
		_ = json.Unmarshal(raw, &out)
		if out.Reason != "" {
			return fmt.Errorf("%w: %s", ErrDenied, out.Reason)
		}
		return ErrDenied
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("authz http: %s (%s)", res.Status, string(raw))
	}

	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("authz http: invalid response: %w", err)
	}
	if !out.Allowed {
		if out.Reason != "" {
			return fmt.Errorf("%w: %s", ErrDenied, out.Reason)
		}
		return ErrDenied
	}
	return nil
}
