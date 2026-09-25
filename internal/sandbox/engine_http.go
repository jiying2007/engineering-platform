package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Retain bounded Engine setup diagnostics, never request bodies or credentials.
func (e *Engine) call(ctx context.Context, method, p string, body any, max int) ([]byte, int, error) {
	var input []byte
	var err error
	if body != nil {
		input, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+apiVersion+p, bytes.NewReader(input))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, int64(max)+1))
	if err != nil {
		return nil, res.StatusCode, err
	}
	if len(data) > max {
		return nil, res.StatusCode, ErrPolicy
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var response struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &response)
		if len(response.Message) > 1024 {
			response.Message = response.Message[:1024]
		}
		return nil, res.StatusCode, fmt.Errorf("engine %s %s returned HTTP %d: %q", method, p, res.StatusCode, response.Message)
	}
	return data, res.StatusCode, nil
}
