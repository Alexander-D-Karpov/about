(function () {
    'use strict';

    const STORAGE_KEY = 'potato_mode_suggested';

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
        location.reload();
    }

    function disablePotatoMode() {
        document.cookie = 'potato_mode=0;path=/;max-age=31536000;SameSite=Lax';
        location.reload();
    }

    function detectSlowDevice() {
        if (isPotatoMode()) return;

        const dominated = localStorage.getItem(STORAGE_KEY);
        if (dominated === 'dismissed') return;

        if (navigator.hardwareConcurrency && navigator.hardwareConcurrency <= 2) {
            suggestPotatoMode('low CPU cores detected');
            return;
        }

        let frames = 0;
        let lastTime = performance.now();
        const samples = [];
        const LOW_FPS_THRESHOLD = 25;

        function measure(now) {
            frames++;
            if (now - lastTime >= 1000) {
                samples.push(frames);
                frames = 0;
                lastTime = now;

                if (samples.length >= 3) {
                    const avgFps = samples.reduce((a, b) => a + b, 0) / samples.length;
                    if (avgFps < LOW_FPS_THRESHOLD) {
                        suggestPotatoMode(`low FPS detected (${Math.round(avgFps)})`);
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

        return; // disable suggestion banner for now

        const banner = document.createElement('div');
        banner.className = 'potato-suggestion';
        banner.style.cssText = `
            position: fixed; top: 0; left: 0; right: 0;
            background: linear-gradient(135deg, #6a9fff, #a080ff);
            color: #fff; padding: 12px 20px; z-index: 100000;
            display: flex; align-items: center; justify-content: center; gap: 16px;
            font-size: 14px; font-weight: 500;
        `;

        banner.innerHTML = `
            <span>🥔 Slow performance detected. Enable Lite Mode for better experience?</span>
            <button id="enable-potato" style="padding:6px 16px;background:rgba(255,255,255,.25);border:none;border-radius:6px;color:#fff;cursor:pointer;font-weight:600;">Enable</button>
            <button id="dismiss-potato" style="padding:6px 12px;background:transparent;border:1px solid rgba(255,255,255,.4);border-radius:6px;color:#fff;cursor:pointer;">Dismiss</button>
        `;

        document.body.prepend(banner);

        document.getElementById('enable-potato').onclick = () => {
            enablePotatoMode();
        };

        document.getElementById('dismiss-potato').onclick = () => {
            localStorage.setItem(STORAGE_KEY, 'dismissed');
            banner.remove();
        };
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

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => {
            updateToggleButton();
            if (!isPotatoMode()) {
                setTimeout(detectSlowDevice, 2000);
            }
        });
    } else {
        updateToggleButton();
        if (!isPotatoMode()) {
            setTimeout(detectSlowDevice, 2000);
        }
    }
})();