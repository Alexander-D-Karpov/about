(function () {
    const SECTION_SEL = {
        topbar: '.topbar',
        section_profile: '[data-section="profile"]',
        section_health: '[data-section="health"]',
        section_music: '#sec-music',
        section_code: '#sec-code',
        section_tech: '[data-section="techstack"]',
        section_games: '#sec-games',
        section_travel: '#sec-travel',
        section_hosting: '#sec-hosting',
        section_machines: '#sec-machines',
        section_projects: '#sec-projects',
    };

    let ws = null;
    let backoff = 1000;
    let pingTimer = null;
    let npTimer = null;

    function connect() {
        const proto = location.protocol === 'https:' ? 'wss' : 'ws';
        ws = new WebSocket(`${proto}://${location.host}/ws`);

        ws.onopen = () => {
            backoff = 1000;
            setStatus(true);
            send({ type: 'get_client_count' });
            clearInterval(pingTimer);
            pingTimer = setInterval(() => send({ type: 'ping' }), 25000);
        };

        ws.onmessage = (ev) => {
            let msg;
            try { msg = JSON.parse(ev.data); } catch { return; }
            handle(msg);
        };

        ws.onclose = () => {
            setStatus(false);
            clearInterval(pingTimer);
            setTimeout(connect, backoff);
            backoff = Math.min(backoff * 1.6, 15000);
        };

        ws.onerror = () => { try { ws.close(); } catch {} };
    }

    function send(obj) {
        if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj));
    }

    function handle(msg) {
        const d = msg.data || {};
        switch (msg.type) {
            case 'section_rendered':
                swapSection(d.name, d.html);
                break;
            case 'visitors_update':
                setText('#connected-clients', d.today);
                break;
            case 'meme_update':
                if (typeof window.renderMeme === 'function' && d.meme) window.renderMeme(d.meme);
                break;
            case 'webring_update':
                updateWebring(d);
                break;
            case 'client_count_update':
            case 'heartbeat':
            case 'pong':
                break;
        }
    }

    function swapSection(name, html) {
        const sel = SECTION_SEL[name];
        if (!sel || !html) return;
        const el = document.querySelector(sel);
        if (!el) return;

        const tmp = document.createElement('div');
        tmp.innerHTML = html.trim();
        const next = tmp.firstElementChild;
        if (!next) return;

        el.replaceWith(next);
        afterSwap();
    }

    function afterSwap() {
        if (typeof window.bindRecentTracks === 'function') { try { window.bindRecentTracks(); } catch {} }
        if (typeof window.drawWeekly === 'function') { try { window.drawWeekly(); } catch {} }
        if (typeof window.drawPlatform === 'function') { try { window.drawPlatform(); } catch {} }
        startNowPlayingTicker();
    }

    function updateWebring(d) {
        const map = { prev: '[data-ring="prev"]', next: '[data-ring="next"]' };
        Object.keys(map).forEach((k) => {
            const peer = d[k];
            const a = document.querySelector(map[k]);
            if (!peer || !a) return;
            if (peer.url) a.href = peer.url;
            const name = a.querySelector('.ring__name');
            if (name && peer.name) name.textContent = peer.name;
            const fav = a.querySelector('.ring__fav');
            if (fav && peer.favicon) fav.src = peer.favicon;
        });
    }

    function startNowPlayingTicker() {
        clearInterval(npTimer);
        npTimer = setInterval(tickNowPlaying, 1000);
        tickNowPlaying();
    }

    function tickNowPlaying() {
        const np = document.querySelector('.current-track .np[data-playing="true"]');
        if (!np) return;
        const started = parseInt(np.dataset.started || '0', 10);
        const duration = parseInt(np.dataset.duration || '0', 10);
        if (!started) return;

        let elapsed = Math.floor(Date.now() / 1000) - started;
        if (elapsed < 0) elapsed = 0;
        if (duration > 0 && elapsed > duration) elapsed = duration;

        const fill = np.parentElement.querySelector('.np__fill') || document.querySelector('.np__fill');
        if (fill && duration > 0) {
            fill.style.width = ((elapsed / duration) * 100).toFixed(1) + '%';
        }
        const times = document.querySelectorAll('.np__time span');
        if (times.length >= 1) times[0].textContent = fmtTime(elapsed);
        if (times.length >= 2 && duration > 0) times[1].textContent = fmtTime(duration);
    }

    function fmtTime(s) {
        const m = Math.floor(s / 60);
        const sec = s % 60;
        return m + ':' + String(sec).padStart(2, '0');
    }

    function setText(sel, v) {
        if (v == null) return;
        const el = document.querySelector(sel);
        if (el) el.textContent = String(v);
    }

    function setStatus(ok) {
        const dot = document.getElementById('connection-status');
        const txt = document.getElementById('status-text');
        if (dot) dot.className = 'status-indicator ' + (ok ? 'status-online' : 'status-offline');
        if (txt) txt.textContent = ok ? 'Live' : 'Reconnecting…';
    }

    function start() {
        connect();
        startNowPlayingTicker();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', start);
    } else {
        start();
    }
})();