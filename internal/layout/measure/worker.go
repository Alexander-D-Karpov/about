package measure

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/Alexander-D-Karpov/about/internal/layout"
)

const (
	staleAfter    = 6 * time.Hour
	debounceDelay = 45 * time.Second
	backstopEvery = 3 * time.Hour
	settleDelay   = 1200 * time.Millisecond

	navBudget   = 45 * time.Second
	probeBudget = 15 * time.Second
	probeSettle = 500 * time.Millisecond

	readyTimeout  = 90 * time.Second
	readyInterval = 500 * time.Millisecond

	retryDelay = 2 * time.Minute
	maxRetries = 5

	chromeLogCap = 8192
)

var errServerNotReady = errors.New("server did not become reachable in time")

type chromeLog struct {
	mu  sync.Mutex
	buf []byte
}

func (c *chromeLog) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	if len(c.buf) > chromeLogCap {
		c.buf = c.buf[len(c.buf)-chromeLogCap:]
	}
	return len(p), nil
}

func (c *chromeLog) String() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(string(c.buf))
}

type Worker struct {
	store     *HeightStore
	baseURL   string
	navURL    string
	remoteURL string

	runMu    sync.Mutex
	dirty    atomic.Bool
	disabled atomic.Bool

	debounceMu sync.Mutex
	debounce   *time.Timer
	rootCtx    context.Context

	retryMu    sync.Mutex
	retryTimer *time.Timer
	retries    int
}

func NewWorker(store *HeightStore, baseURL string) *Worker {
	navURL := os.Getenv("MEASURE_BASE_URL")
	if navURL == "" {
		navURL = baseURL
	}
	return &Worker{
		store:     store,
		baseURL:   strings.TrimRight(baseURL, "/"),
		navURL:    strings.TrimRight(navURL, "/"),
		remoteURL: strings.TrimSpace(os.Getenv("CHROME_REMOTE_URL")),
	}
}

func (w *Worker) hasDirty() bool { return w.dirty.Load() }

func (w *Worker) remote() bool { return w.remoteURL != "" }

func (w *Worker) needsMeasure() bool {
	if w.store.Empty() {
		return true
	}
	if age, ok := w.store.NewestAge(); ok && age > staleAfter {
		return true
	}
	return false
}

func navigateNoWait(urlstr string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		_, _, errorText, _, err := page.Navigate(urlstr).Do(ctx)
		if err != nil {
			return err
		}
		if errorText != "" {
			return fmt.Errorf("page load error %s", errorText)
		}
		return nil
	}
}

func (w *Worker) rootContext() context.Context {
	w.debounceMu.Lock()
	defer w.debounceMu.Unlock()
	if w.rootCtx == nil {
		return context.Background()
	}
	return w.rootCtx
}

func (w *Worker) Notify(plugin string) {
	if w.disabled.Load() {
		return
	}
	w.dirty.Store(true)
	w.debounceMu.Lock()
	if w.debounce != nil {
		w.debounce.Stop()
	}
	w.debounce = time.AfterFunc(debounceDelay, func() {
		ctx, cancel := context.WithTimeout(w.rootContext(), 5*time.Minute)
		defer cancel()
		w.runIfNeeded(ctx)
	})
	w.debounceMu.Unlock()
}

func (w *Worker) waitForReady(ctx context.Context) error {
	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(readyTimeout)

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.baseURL+"/health", nil)
		if err == nil {
			resp, derr := client.Do(req)
			if derr == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				return nil
			}
		}

		if time.Now().After(deadline) {
			return errServerNotReady
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyInterval):
		}
	}
}

func (w *Worker) Start(ctx context.Context) {
	w.debounceMu.Lock()
	w.rootCtx = ctx
	w.debounceMu.Unlock()

	if w.remote() {
		log.Printf("[Measure] using remote chrome at %s, page url %s", w.remoteURL, w.navURL)
	}

	if err := w.waitForReady(ctx); err != nil {
		log.Printf("[Measure] %s not reachable, skipping startup pass: %v", w.baseURL, err)
	} else if w.needsMeasure() {
		rctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		w.dirty.Store(true)
		w.runIfNeeded(rctx)
		cancel()
	}

	ticker := time.NewTicker(backstopEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.retryMu.Lock()
			if w.retryTimer != nil {
				w.retryTimer.Stop()
			}
			w.retryMu.Unlock()
			return
		case <-ticker.C:
			rctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			w.dirty.Store(true)
			w.runIfNeeded(rctx)
			cancel()
		}
	}
}

func (w *Worker) scheduleRetry() {
	w.retryMu.Lock()
	defer w.retryMu.Unlock()

	if w.retries >= maxRetries {
		log.Printf("[Measure] giving up until the next backstop tick")
		return
	}
	w.retries++
	delay := retryDelay * time.Duration(w.retries)

	if w.retryTimer != nil {
		w.retryTimer.Stop()
	}
	w.retryTimer = time.AfterFunc(delay, func() {
		ctx, cancel := context.WithTimeout(w.rootContext(), 5*time.Minute)
		defer cancel()
		w.dirty.Store(true)
		w.runIfNeeded(ctx)
	})

	log.Printf("[Measure] retrying in %s (attempt %d/%d)", delay, w.retries, maxRetries)
}

func (w *Worker) runIfNeeded(ctx context.Context) {
	if w.disabled.Load() || !w.dirty.Load() {
		return
	}
	if !w.runMu.TryLock() {
		return
	}
	defer w.runMu.Unlock()

	w.dirty.Store(false)

	if err := w.runOnce(ctx); err != nil {
		log.Printf("[Measure] pass failed, keeping heuristic heights: %v", err)

		if !w.remote() && isChromeUnavailable(err) {
			w.disabled.Store(true)
			log.Printf("[Measure] local Chromium unavailable; measurement disabled for this run")
			return
		}

		w.dirty.Store(true)
		w.scheduleRetry()
		return
	}

	w.retryMu.Lock()
	w.retries = 0
	w.retryMu.Unlock()
}

func isChromeUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "chrome failed to start") ||
		strings.Contains(msg, "error while loading shared libraries") ||
		strings.Contains(msg, "symbol lookup error")
}

func resolveChromePath() string {
	if bin := os.Getenv("CHROME_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
		log.Printf("[Measure] CHROME_BIN=%s does not exist, falling back to PATH", bin)
	}

	for _, name := range []string{
		"chromium",
		"chromium-browser",
		"headless-shell",
		"google-chrome-stable",
		"google-chrome",
		"chrome",
	} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}

	return ""
}

func (w *Worker) newAllocator(ctx context.Context) (context.Context, context.CancelFunc, *chromeLog, error) {
	if w.remote() {
		allocCtx, cancel := chromedp.NewRemoteAllocator(ctx, w.remoteURL)
		return allocCtx, cancel, nil, nil
	}

	execPath := resolveChromePath()
	if execPath == "" {
		return nil, nil, nil, errors.New("chromium executable file not found in CHROME_BIN or PATH")
	}

	cl := &chromeLog{}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
		chromedp.CombinedOutput(cl),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	return allocCtx, cancel, cl, nil
}

const probeJS = `
(function () {
  function norm(n){ if(n==='techstack')return 'tech'; if(n==='links')return 'social'; return n; }
  var mosaic = document.querySelector('.mosaic') || document.body;
  var style = getComputedStyle(mosaic);
  var gap = parseFloat(style.columnGap || style.gap || '12') || 12;
  var cols = Math.max(1, style.gridTemplateColumns.split(' ').length);
  var mosaicWidth = mosaic.clientWidth;
  var colWidth = (mosaicWidth - gap*(cols-1)) / cols;
  var probe = document.createElement('div');
  probe.style.cssText = 'position:absolute;left:-99999px;top:0;visibility:hidden;pointer-events:none;';
  document.body.appendChild(probe);
  var out = {};
  document.querySelectorAll('.mosaic > .plugin').forEach(function(el){
    var name = '';
    el.classList.forEach(function(c){ if(c.slice(-8)==='-section'){ name = norm(c.slice(0,-8)); } });
    if(!name) return;
    var perSpan = {};
    for(var span=1; span<=Math.min(3,cols); span++){
      var targetWidth = colWidth*span + gap*(span-1);
      var clone = el.cloneNode(true);
      clone.style.cssText = 'width:'+targetWidth+'px;height:auto;min-height:0;max-height:none;position:static;display:block;';
      clone.style.gridColumn=''; clone.style.gridRow='';
      probe.appendChild(clone);
      perSpan[String(span)] = Math.ceil(clone.getBoundingClientRect().height) || 0;
      probe.removeChild(clone);
    }
    out[name] = perSpan;
  });
  document.body.removeChild(probe);
  return out;
})()
`

func (w *Worker) runOnce(ctx context.Context) error {
	allocCtx, cancelAlloc, cl, err := w.newAllocator(ctx)
	if err != nil {
		return err
	}
	defer cancelAlloc()

	// Cancelling this closes only the tab; a remote browser stays running.
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	// chromedp ties a tab's lifetime to the context of its first Run, so bind it
	// here rather than inside a per-navigate timeout child.
	if err := chromedp.Run(browserCtx); err != nil {
		return fmt.Errorf("attach to browser failed: %w | chrome stderr: %s", err, cl.String())
	}

	buckets := layout.ViewportBuckets()
	totalBuckets := len(buckets)
	if totalBuckets == 0 {
		return nil
	}

	// Navigate once at the widest bucket; later buckets only re-emulate the
	// viewport, and the clone probe derives column width from the live mosaic.
	widest := buckets[totalBuckets-1].SampleWidth
	navCtx, navCancel := context.WithTimeout(browserCtx, navBudget)
	nerr := chromedp.Run(navCtx,
		network.SetBlockedURLs([]string{
			"*://*.basemaps.cartocdn.com/*",
			"*://*.tile.openstreetmap.org/*",
			"*://unpkg.com/*",
			"*://raw.githubusercontent.com/*",
		}),
		chromedp.EmulateViewport(int64(widest), 1200),
		navigateNoWait(w.navURL+"/?measure=1"),
		chromedp.WaitVisible(`.mosaic .plugin`, chromedp.ByQuery),
		chromedp.Sleep(settleDelay),
	)
	navCancel()
	if nerr != nil {
		return fmt.Errorf("navigate %s failed: %w | chrome stderr: %s", w.navURL, nerr, cl.String())
	}

	// aggregate: plugin -> bucket -> span -> height
	agg := map[string]map[string]map[int]int{}
	var firstErr error

	for _, bucket := range buckets {
		bctx, cancel := context.WithTimeout(browserCtx, probeBudget)
		var result map[string]map[string]int
		perr := chromedp.Run(bctx,
			chromedp.EmulateViewport(int64(bucket.SampleWidth), 1200),
			chromedp.Sleep(probeSettle),
			chromedp.Evaluate(probeJS, &result),
		)
		cancel()

		if perr != nil {
			if firstErr == nil {
				firstErr = perr
			}
			log.Printf("[Measure] bucket %s failed: %v", bucket.Name, perr)
			continue
		}

		for name, spans := range result {
			// meme (random per request) and profile (bio height depends on the
			// viewer's fonts and the packer may down-span it) are reserved
			// deterministically in the prebake, not from measurement.
			if name == "meme" || name == "profile" {
				continue
			}
			if agg[name] == nil {
				agg[name] = map[string]map[int]int{}
			}
			m := map[int]int{}
			for spanStr, h := range spans {
				if span, e := strconv.Atoi(spanStr); e == nil && h > 0 {
					m[span] = h
				}
			}
			agg[name][bucket.Name] = m
		}
	}

	if len(agg) == 0 {
		if firstErr != nil {
			return fmt.Errorf("all buckets failed: %w | chrome stderr: %s", firstErr, cl.String())
		}
		return nil
	}

	for name, buckets := range agg {
		w.store.SetPlugin(name, buckets)
	}
	w.store.Flush()

	log.Printf("[Measure] measured %d plugins across %d buckets", len(agg), totalBuckets)
	return nil
}
