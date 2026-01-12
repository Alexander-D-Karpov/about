(function () {
    'use strict';

    const STORAGE_KEY = 'potato_mode_suggested';
    const PERF_SAMPLES_KEY = 'perf_samples';

    function isPotatoMode() {
        return document.body.classList.contains('potato-mode') ||
            document.cookie.includes('potato_mode=1');
    }

    function togglePotatoMode() {
        if (isPotatoMode()) {
            disablePotatoMode();
        } else {
            enablePotatoMode();
        }
    }

    function enablePotatoMode() {
        document.cookie = 'potato_mode=1;path=/;max-age=31536000;SameSite=Lax';
        const url = new URL(window.location);
        url.searchParams.delete('potato');
        window.history.replaceState({}, '', url);
        location.reload();
    }

    function disablePotatoMode() {
        document.cookie = 'potato_mode=0;path=/;max-age=31536000;SameSite=Lax';
        const url = new URL(window.location);
        url.searchParams.delete('potato');
        window.history.replaceState({}, '', url);
        location.reload();
    }

    function checkURLParam() {
        const url = new URL(window.location);
        if (url.searchParams.has('potato')) {
            const val = url.searchParams.get('potato');
            if (val === '0' || val === 'false') {
                if (isPotatoMode()) {
                    disablePotatoMode();
                }
            } else if (!isPotatoMode()) {
                enablePotatoMode();
            }
        }
    }

    function detectSlowDevice() {
        if (isPotatoMode()) return;

        const dominated = localStorage.getItem(STORAGE_KEY);
        if (dominated === 'dismissed') return;

        if (navigator.hardwareConcurrency && navigator.hardwareConcurrency <= 2) {
            suggestPotatoMode('low CPU cores detected');
            return;
        }

        if (navigator.deviceMemory && navigator.deviceMemory <= 2) {
            suggestPotatoMode('low memory detected');
            return;
        }

        measurePerformance();
    }

    function measurePerformance() {
        let frames = 0;
        let lastTime = performance.now();
        const samples = [];
        const LOW_FPS_THRESHOLD = 25;
        const SAMPLE_COUNT = 5;

        function measure(now) {
            frames++;
            if (now - lastTime >= 1000) {
                samples.push(frames);
                frames = 0;
                lastTime = now;

                if (samples.length >= SAMPLE_COUNT) {
                    const avgFps = samples.reduce((a, b) => a + b, 0) / samples.length;

                    const stored = JSON.parse(localStorage.getItem(PERF_SAMPLES_KEY) || '[]');
                    stored.push(avgFps);
                    if (stored.length > 10) stored.shift();
                    localStorage.setItem(PERF_SAMPLES_KEY, JSON.stringify(stored));

                    if (avgFps < LOW_FPS_THRESHOLD) {
                        const historicalAvg = stored.reduce((a, b) => a + b, 0) / stored.length;
                        if (historicalAvg < LOW_FPS_THRESHOLD) {
                            suggestPotatoMode(`low FPS detected (${Math.round(avgFps)})`);
                        }
                    }
                    return;
                }
            }
            requestAnimationFrame(measure);
        }

        requestAnimationFrame(measure);
    }

    function suggestPotatoMode(reason) {
        if (isPotatoMode()) return;
        if (document.querySelector('.potato-suggestion')) return;

        console.log('Performance issue:', reason);

        const banner = document.createElement('div');
        banner.className = 'potato-suggestion';
        banner.style.cssText = `
            position: fixed; bottom: 60px; left: 50%; transform: translateX(-50%);
            background: var(--paper, #1a1a1a); color: var(--ink, #e0e0e0);
            padding: 12px 20px; z-index: 100000; border-radius: 12px;
            display: flex; align-items: center; gap: 12px;
            font-size: 13px; font-weight: 500; box-shadow: 0 4px 20px rgba(0,0,0,0.3);
            border: 1px solid var(--edge-1, rgba(255,255,255,0.1));
            max-width: calc(100vw - 40px);
        `;

        banner.innerHTML = `
            <span> Slow performance? Try Lite Mode</span>
            <button id="enable-potato" style="padding:6px 14px;background:var(--accent, #6a9fff);border:none;border-radius:6px;color:#fff;cursor:pointer;font-weight:600;font-size:12px;">Enable</button>
            <button id="dismiss-potato" style="padding:6px 10px;background:transparent;border:1px solid var(--edge-1, rgba(255,255,255,0.2));border-radius:6px;color:var(--ink, #e0e0e0);cursor:pointer;font-size:12px;">✕</button>
        `;

        document.body.appendChild(banner);

        document.getElementById('enable-potato').onclick = enablePotatoMode;
        document.getElementById('dismiss-potato').onclick = () => {
            localStorage.setItem(STORAGE_KEY, 'dismissed');
            banner.remove();
        };

        setTimeout(() => {
            if (banner.parentNode) banner.remove();
        }, 15000);
    }

    function updateToggleButton() {
        const btn = document.getElementById('potato-toggle');
        if (btn) {
            btn.classList.toggle('active', isPotatoMode());
            btn.title = isPotatoMode() ? 'Switch to Full Mode' : 'Switch to Lite Mode';
        }
    }

    window.enablePotatoMode = enablePotatoMode;
    window.disablePotatoMode = disablePotatoMode;
    window.togglePotatoMode = togglePotatoMode;
    window.isPotatoMode = isPotatoMode;

    checkURLParam();

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => {
            updateToggleButton();
            if (!isPotatoMode()) {
                setTimeout(detectSlowDevice, 3000);
            }
        });
    } else {
        updateToggleButton();
        if (!isPotatoMode()) {
            setTimeout(detectSlowDevice, 3000);
        }
    }
})();