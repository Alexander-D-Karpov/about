package plugins

import (
	"context"
	"net"
	"net/http"
	"time"
)

func NewHTTPClient() *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

func NewHTTPClientWithTimeout(timeout time.Duration) *http.Client {
	client := NewHTTPClient()
	client.Timeout = timeout
	return client
}

func DoRequestWithContext(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}

	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Connection", "close")

	return client.Do(req)
}

func SafeHTTPRequest(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	if client == nil {
		client = NewHTTPClient()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return DoRequestWithContext(ctx, client, req)
}
