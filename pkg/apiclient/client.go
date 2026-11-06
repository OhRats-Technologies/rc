package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ApiError struct {
	Status  int
	Message string
	Err     error
}

func (e *ApiError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("RC request failed: %v", e.Err)
	}
	return fmt.Sprintf("%d: %s", e.Status, e.Message)
}

func (e *ApiError) Unwrap() error {
	return e.Err
}

type ApiClient struct {
	base       *url.URL
	credential Credential
	http       *http.Client
}

func New(server string, cred Credential) (*ApiClient, error) {
	parsed, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil {
		return nil, &ApiError{Message: "invalid RC server URL", Err: err}
	}
	return &ApiClient{
		base:       parsed,
		credential: cred,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (c *ApiClient) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *ApiClient) Post(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

func (c *ApiClient) Put(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPut, path, body, out)
}

func (c *ApiClient) Delete(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodDelete, path, nil, out)
}

func (c *ApiClient) RequestEmpty(ctx context.Context, method, path string) error {
	return c.do(ctx, method, path, nil, nil)
}

func (c *ApiClient) Devices(ctx context.Context) ([]Device, error) {
	var resp struct {
		Devices []Device `json:"devices"`
	}
	if err := c.Get(ctx, "/api/v1/devices", &resp); err != nil {
		return nil, err
	}
	return resp.Devices, nil
}

func (c *ApiClient) Device(ctx context.Context, id string) (*Device, error) {
	var resp struct {
		Device Device `json:"device"`
	}
	path := fmt.Sprintf("/api/v1/devices/%s", url.PathEscape(id))
	if err := c.Get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Device, nil
}

func (c *ApiClient) do(ctx context.Context, method, path string, body any, out any) error {
	rel, err := url.Parse(path)
	if err != nil {
		return &ApiError{Err: err}
	}
	targetURL := c.base.ResolveReference(rel)

	var bodyBytes []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return &ApiError{Err: err}
		}
		bodyBytes = data
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return &ApiError{Err: err}
	}

	requestURI := targetURL.RequestURI()
	if c.credential.Kind == CredentialBearer {
		req.Header.Set("Authorization", "Bearer "+c.credential.Bearer)
	} else if c.credential.Pop != nil {
		for k, v := range c.credential.Pop.Headers(method, requestURI, bodyBytes) {
			req.Header.Set(k, v)
		}
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.http.Do(req)
	if err != nil {
		return &ApiError{Err: err}
	}
	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return &ApiError{Err: err}
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var errObj struct {
			Error string `json:"error"`
		}
		msg := string(resBody)
		if json.Unmarshal(resBody, &errObj) == nil && errObj.Error != "" {
			msg = errObj.Error
		}
		return &ApiError{Status: res.StatusCode, Message: msg}
	}

	if out != nil && len(resBody) > 0 {
		if err := json.Unmarshal(resBody, out); err != nil {
			return &ApiError{Err: err}
		}
	}
	return nil
}

func PublicPost(ctx context.Context, server, path string, body any, out any) error {
	parsed, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil {
		return &ApiError{Err: err}
	}
	rel, err := url.Parse(path)
	if err != nil {
		return &ApiError{Err: err}
	}
	targetURL := parsed.ResolveReference(rel)

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return &ApiError{Err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return &ApiError{Err: err}
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return &ApiError{Err: err}
	}
	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return &ApiError{Err: err}
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &ApiError{Status: res.StatusCode, Message: string(resBody)}
	}

	if out != nil && len(resBody) > 0 {
		return json.Unmarshal(resBody, out)
	}
	return nil
}
