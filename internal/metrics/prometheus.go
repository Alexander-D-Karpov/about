package metrics

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

type MetricsProvider interface {
	Name() string
	GetMetrics() map[string]interface{}
}

type PrometheusClient struct {
	pushGatewayURL string
	remoteWriteURL string
	queryURL       string
	username       string
	password       string
	jobName        string
	httpClient     *http.Client
	providers      []MetricsProvider
	mutex          sync.RWMutex
	lastPush       time.Time
	lastPushError  error
	pushEnabled    bool
	queryEnabled   bool
	metricsCache   map[string]interface{}
	cacheMutex     sync.RWMutex
	useRemoteWrite bool
}

func NewPrometheusClient(pushGatewayURL, queryURL, username, password, jobName string) *PrometheusClient {
	pushEnabled := pushGatewayURL != ""
	queryEnabled := queryURL != ""

	useRemoteWrite := false
	remoteWriteURL := ""

	if strings.Contains(pushGatewayURL, "/api/v1/write") {
		useRemoteWrite = true
		remoteWriteURL = pushGatewayURL
		pushGatewayURL = ""
	}

	if pushEnabled || useRemoteWrite {
		log.Printf("[Prometheus] Push enabled: gateway=%s, remoteWrite=%v", pushGatewayURL, useRemoteWrite)
	}
	if queryEnabled {
		log.Printf("[Prometheus] Query enabled: %s", queryURL)
	}

	return &PrometheusClient{
		pushGatewayURL: strings.TrimRight(pushGatewayURL, "/"),
		remoteWriteURL: remoteWriteURL,
		queryURL:       strings.TrimRight(queryURL, "/"),
		username:       username,
		password:       password,
		jobName:        jobName,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		providers:      make([]MetricsProvider, 0),
		pushEnabled:    pushEnabled || useRemoteWrite,
		queryEnabled:   queryEnabled,
		metricsCache:   make(map[string]interface{}),
		useRemoteWrite: useRemoteWrite,
	}
}

func (c *PrometheusClient) RegisterProvider(provider MetricsProvider) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, p := range c.providers {
		if p.Name() == provider.Name() {
			return
		}
	}

	c.providers = append(c.providers, provider)
	log.Printf("[Prometheus] Registered provider: %s", provider.Name())
}

func (c *PrometheusClient) IsEnabled() bool {
	return c.pushEnabled
}

func (c *PrometheusClient) IsQueryEnabled() bool {
	return c.queryEnabled
}

func (c *PrometheusClient) GetLastError() error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lastPushError
}

func (c *PrometheusClient) CollectMetrics() map[string]interface{} {
	c.mutex.RLock()
	providers := make([]MetricsProvider, len(c.providers))
	copy(providers, c.providers)
	c.mutex.RUnlock()

	allMetrics := make(map[string]interface{})

	for _, provider := range providers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Prometheus] Panic collecting from %s: %v", provider.Name(), r)
				}
			}()

			metrics := provider.GetMetrics()
			if metrics == nil {
				return
			}

			prefix := provider.Name()
			for key, value := range metrics {
				metricName := fmt.Sprintf("about_%s_%s", prefix, key)
				metricName = sanitizeMetricName(metricName)
				allMetrics[metricName] = value
			}
		}()
	}

	c.cacheMutex.Lock()
	c.metricsCache = allMetrics
	c.cacheMutex.Unlock()

	return allMetrics
}

func (c *PrometheusClient) GetCachedMetrics() map[string]interface{} {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()

	result := make(map[string]interface{}, len(c.metricsCache))
	for k, v := range c.metricsCache {
		result[k] = v
	}
	return result
}

func (c *PrometheusClient) CollectAndPush(ctx context.Context) error {
	if !c.pushEnabled {
		c.CollectMetrics()
		return nil
	}

	allMetrics := c.CollectMetrics()

	if len(allMetrics) == 0 {
		return nil
	}

	var err error
	if c.useRemoteWrite {
		err = c.pushRemoteWrite(ctx, allMetrics)
	} else {
		metricsText := formatPrometheusMetrics(allMetrics, c.jobName)
		err = c.pushToGateway(ctx, metricsText)
	}

	c.mutex.Lock()
	c.lastPushError = err
	if err == nil {
		c.lastPush = time.Now()
	}
	c.mutex.Unlock()

	return err
}

func (c *PrometheusClient) pushToGateway(ctx context.Context, metricsText string) error {
	pushURL := fmt.Sprintf("%s/metrics/job/%s", c.pushGatewayURL, c.jobName)

	req, err := http.NewRequestWithContext(ctx, "POST", pushURL, bytes.NewBufferString(metricsText))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("push request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gateway status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[Prometheus] Pushed %d metrics to gateway", strings.Count(metricsText, "\n"))
	return nil
}

func (c *PrometheusClient) pushRemoteWrite(ctx context.Context, metrics map[string]interface{}) error {
	now := time.Now().UnixMilli()

	var timeseries []prompb.TimeSeries

	for name, value := range metrics {
		var floatVal float64
		switch v := value.(type) {
		case int:
			floatVal = float64(v)
		case int64:
			floatVal = float64(v)
		case float64:
			floatVal = v
		case float32:
			floatVal = float64(v)
		case bool:
			if v {
				floatVal = 1
			} else {
				floatVal = 0
			}
		default:
			continue
		}

		ts := prompb.TimeSeries{
			Labels: []prompb.Label{
				{Name: "__name__", Value: name},
				{Name: "job", Value: c.jobName},
				{Name: "instance", Value: "about_page"},
			},
			Samples: []prompb.Sample{
				{Value: floatVal, Timestamp: now},
			},
		}
		timeseries = append(timeseries, ts)
	}

	writeReq := &prompb.WriteRequest{
		Timeseries: timeseries,
	}

	data, err := proto.Marshal(writeReq)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	compressed := snappy.Encode(nil, data)

	req, err := http.NewRequestWithContext(ctx, "POST", c.remoteWriteURL, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("remote write: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remote write status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[Prometheus] Remote write: %d metrics", len(timeseries))
	return nil
}

func (c *PrometheusClient) StartPeriodicPush(ctx context.Context, interval time.Duration) {
	log.Printf("[Prometheus] Starting periodic collection every %v", interval)

	if err := c.CollectAndPush(ctx); err != nil {
		log.Printf("[Prometheus] Initial push failed: %v", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Prometheus] Stopping periodic push")
			return
		case <-ticker.C:
			pushCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if err := c.CollectAndPush(pushCtx); err != nil {
				log.Printf("[Prometheus] Push failed: %v", err)
			}
			cancel()
		}
	}
}

func (c *PrometheusClient) GetLastPushTime() time.Time {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lastPush
}

func (c *PrometheusClient) GetProviderCount() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.providers)
}

func (c *PrometheusClient) GetProviderNames() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	names := make([]string, len(c.providers))
	for i, p := range c.providers {
		names[i] = p.Name()
	}
	return names
}

func sanitizeMetricName(name string) string {
	var result strings.Builder
	for i, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
			result.WriteRune(c)
		} else if c >= '0' && c <= '9' {
			if i == 0 {
				result.WriteRune('_')
			}
			result.WriteRune(c)
		} else {
			result.WriteRune('_')
		}
	}
	return strings.ToLower(result.String())
}

func formatPrometheusMetrics(metrics map[string]interface{}, jobName string) string {
	var lines []string

	keys := make([]string, 0, len(metrics))
	for k := range metrics {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	timestamp := time.Now().UnixMilli()

	for _, key := range keys {
		value := metrics[key]
		var formattedValue string

		switch v := value.(type) {
		case int:
			formattedValue = fmt.Sprintf("%d", v)
		case int64:
			formattedValue = fmt.Sprintf("%d", v)
		case int32:
			formattedValue = fmt.Sprintf("%d", v)
		case float64:
			formattedValue = fmt.Sprintf("%g", v)
		case float32:
			formattedValue = fmt.Sprintf("%g", v)
		case bool:
			if v {
				formattedValue = "1"
			} else {
				formattedValue = "0"
			}
		default:
			continue
		}

		lines = append(lines, fmt.Sprintf("%s{job=\"%s\"} %s %d", key, jobName, formattedValue, timestamp))
	}

	return strings.Join(lines, "\n") + "\n"
}
