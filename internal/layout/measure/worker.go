package measure

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/Alexander-D-Karpov/about/internal/layout"
)

const (
	staleAfter    = 6 * time.Hour
	debounceDelay = 45 * time.Second
	backstopEvery = 3 * time.Hour
	settleDelay   = 1200 * time.Millisecond

	navBudget   = 45 * time.Second       // one cold, possibly-under-load initial render
	probeBudget = 15 * time.Second       // per-bucket viewport-change + probe
	probeSettle = 500 * time.Millisecond // reflow after an emulated viewport change
)

type Worker struct {
	store   *HeightStore
	baseURL string

	runMu    sync.Mutex // singleflight: only one measurement pass at a time
	dirty    atomic.Bool
	disabled atomic.Bool

	debounceMu sync.Mutex
	debounce   *time.Timer
}

func NewWorker(store *HeightStore, baseURL string) *Worker {
	return &Worker{store: store, baseURL: baseURL}
}

func (w *Worker) hasDirty() bool { return w.dirty.Load() }

func (w *Worker) needsMeasure() bool {
	if w.store.Empty() {
		return true
	}
	if age, ok := w.store.NewestAge(); ok && age > staleAfter {
		return true
	}
	return false
}

// Notify satisfies plugins.LayoutNotifier. Debounced so a burst of
// invalidations triggers a single measurement pass.
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
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		w.runIfNeeded(ctx)
	})
	w.debounceMu.Unlock()
}

func (w *Worker) Start(ctx context.Context) {
	if w.needsMeasure() {
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
			return
		case <-ticker.C:
			rctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			w.dirty.Store(true)
			w.runIfNeeded(rctx)
			cancel()
		}
	}
}

func (w *Worker) runIfNeeded(ctx context.Context) {
	if w.disabled.Load() || !w.dirty.Load() {
		return
	}
	if !w.runMu.TryLock() {
		// NOTE: a pass is already running and holds dirty=false semantics for
		// whatever it started measuring; any invalidation that arrives after
		// that snapshot won't be picked up by this run. We deliberately don't
		// reschedule here to avoid unbounded run chains under a hot invalidation
		// stream. Coverage is provided by the backstopEvery ticker in Start()
		// and by the next Notify() call re-arming the debounce timer.
		return // a pass is already running
	}
	defer w.runMu.Unlock()
	w.dirty.Store(false)
	if err := w.runOnce(ctx); err != nil {
		log.Printf("[Measure] pass failed, keeping heuristic heights: %v", err)
		// If Chromium itself is unavailable, disable permanently for this process.
		if isChromeUnavailable(err) {
			w.disabled.Store(true)
			log.Printf("[Measure] Chromium unavailable; measurement disabled for this run")
		}
	}
}

func isChromeUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "chrome failed to start") ||
		strings.Contains(msg, "exec:")
}

// probeJS measures each plugin's height at spans 1..3 using an offscreen clone,
// mirroring the client's batchMeasureHeights. Returns
// { pluginKey: { "1": h, "2": h, "3": h } }.
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
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	if bin := os.Getenv("CHROME_BIN"); bin != "" {
		opts = append(opts, chromedp.ExecPath(bin))
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	buckets := layout.ViewportBuckets()
	totalBuckets := len(buckets)
	if len(buckets) == 0 {
		return nil
	}

	// Navigate ONCE at the widest bucket so all content/images are present.
	// Subsequent buckets only re-emulate the viewport; the clone-based probe
	// derives each bucket's column width from the live (reflowed) mosaic, so
	// no server re-render is needed per bucket.
	widest := buckets[len(buckets)-1].SampleWidth
	navCtx, navCancel := context.WithTimeout(browserCtx, navBudget)
	err := chromedp.Run(navCtx,
		chromedp.EmulateViewport(int64(widest), 1200),
		chromedp.Navigate(w.baseURL+"/?measure=1"),
		chromedp.WaitVisible(`.mosaic .plugin`, chromedp.ByQuery),
		chromedp.Sleep(settleDelay),
	)
	navCancel()
	if err != nil {
		return err // navigation/launch failed (e.g. Chromium missing)
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
			return firstErr
		}
		return nil
	}

	for name, buckets := range agg {
		w.store.SetPlugin(name, buckets)
	}
	w.store.Flush()
	log.Printf("[Measure] measured %d plugins across %d buckets", len(agg), totalBuckets)
	return firstErr // non-nil if some (but not all) buckets failed; store still updated
}
