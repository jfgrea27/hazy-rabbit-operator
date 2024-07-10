package hhttp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// Wrapper around any HTTP Error
type HttpError struct {
	StatusCode int
}

func (e *HttpError) Error() string {
	return fmt.Sprintf("Invalid status %v.", e.StatusCode)
}

// A wrapper around net/http
type HttpClient struct {
	Username string
	Password string
	Host     string
	client   *http.Client
}

func (h *HttpClient) DoHttpRequest(url string, method string, body []byte) (*http.Response, error) {

	ep := fmt.Sprintf("http://%v/%v", h.Host, url)

	var reader io.Reader

	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, ep, reader)

	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(h.Username, h.Password)

	res, err := h.client.Do(req)

	if err != nil {
		return nil, err
	}

	if !(res.StatusCode >= 200 && res.StatusCode < 300) {
		return res, &HttpError{res.StatusCode}
	}

	return res, nil
}

func BuildHost(s string, p string) string {
	if len(p) == 0 {
		return s
	}
	return fmt.Sprintf("%s:%s", s, p)
}

func BuildHttpClient(u string, p string, h string) *HttpClient {
	client := HttpClient{
		Username: u,
		Password: p,
		Host:     h,
		client:   &http.Client{},
	}
	return &client
}
