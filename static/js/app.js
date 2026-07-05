(function () {
    'use strict';

    const page = document.getElementById('page');
    const root = document.querySelector('.root');

    function readJSON(id, fallback) {
        const el = document.getElementById(id);
        if (!el) return fallback;
        try { return JSON.parse(el.textContent || '{}'); } catch { return fallback; }
    }

    const Gol = (() => {
        let canvas, ctx, cell = 16, cols = 0, rows = 0, grid = null;
        let w = 0, h = 0, running = true, full = false, gen = 0, popHist = [], timer = null, raf = 0, bands = null;
        const theme = () => page.getAttribute('data-theme') || 'dark';

        const patterns = () => ({
            glider: [[1, 0], [2, 1], [0, 2], [1, 2], [2, 2]],
            lwss: [[0, 0], [3, 0], [4, 1], [0, 2], [4, 2], [1, 3], [2, 3], [3, 3], [4, 3]],
            toad: [[1, 0], [2, 0], [3, 0], [0, 1], [1, 1], [2, 1]],
            beacon: [[0, 0], [1, 0], [0, 1], [3, 2], [2, 3], [3, 3]],
            blinker: [[0, 0], [1, 0], [2, 0]],
            pulsar: [[2, 0], [3, 0], [4, 0], [8, 0], [9, 0], [10, 0], [0, 2], [5, 2], [7, 2], [12, 2], [0, 3], [5, 3], [7, 3], [12, 3], [0, 4], [5, 4], [7, 4], [12, 4], [2, 5], [3, 5], [4, 5], [8, 5], [9, 5], [10, 5], [2, 7], [3, 7], [4, 7], [8, 7], [9, 7], [10, 7], [0, 8], [5, 8], [7, 8], [12, 8], [0, 9], [5, 9], [7, 9], [12, 9], [0, 10], [5, 10], [7, 10], [12, 10], [2, 12], [3, 12], [4, 12], [8, 12], [9, 12], [10, 12]]
        });
        const idx = (x, y) => ((y + rows) % rows) * cols + ((x + cols) % cols);
        const newGrid = () => new Uint8Array(cols * rows);

        function stampGun(ox, oy) {
            const g = [[0, 4], [0, 5], [1, 4], [1, 5], [10, 4], [10, 5], [10, 6], [11, 3], [11, 7], [12, 2], [12, 8], [13, 2], [13, 8], [14, 5], [15, 3], [15, 7], [16, 4], [16, 5], [16, 6], [17, 5], [20, 2], [20, 3], [20, 4], [21, 2], [21, 3], [21, 4], [22, 1], [22, 5], [24, 0], [24, 1], [24, 5], [24, 6], [34, 2], [34, 3], [35, 2], [35, 3]];
            g.forEach(p => { grid[idx(ox + p[0], oy + p[1])] = 1; });
        }
        function stamp(cells, ox, oy, t) {
            t = t || 0;
            cells.forEach(p => {
                let x = p[0], y = p[1];
                if (t & 1) { const s = x; x = y; y = s; }
                if (t & 2) x = -x;
                if (t & 4) y = -y;
                grid[idx(ox + x, oy + y)] = 1;
            });
        }
        function seed() {
            grid = newGrid();
            const P = patterns(), keys = Object.keys(P);
            stampGun(3, 4);
            if (cols > 90) stampGun(cols - 42, rows - 16);
            for (let cy = 2; cy < rows - 14; cy += 18)
                for (let cx = 2; cx < cols - 16; cx += 22) {
                    if ((cy < 16 && cx < 40) || (cx > cols - 60 && cy > rows - 32)) continue;
                    if (Math.random() < 0.4) continue;
                    stamp(P[keys[(Math.random() * keys.length) | 0]], cx + ((Math.random() * 6) | 0), cy + ((Math.random() * 5) | 0), (Math.random() * 8) | 0);
                }
            gen = 0; popHist = [];
        }
        function step() {
            const n = newGrid(); let pop = 0;
            for (let y = 0; y < rows; y++) for (let x = 0; x < cols; x++) {
                let s = 0;
                for (let dy = -1; dy <= 1; dy++) for (let dx = -1; dx <= 1; dx++) if (dx || dy) s += grid[idx(x + dx, y + dy)];
                const a = grid[idx(x, y)];
                const v = (a && (s === 2 || s === 3)) || (!a && s === 3) ? 1 : 0;
                n[y * cols + x] = v; pop += v;
            }
            grid = n; popHist.push(pop); if (popHist.length > 36) popHist.shift();
        }
        function buildBands() {
            const HUES = { Profile: 252, Music: 142, Code: 212, Games: 280, Travel: 38, Hosting: 175, Machines: 210, Projects: 330 };
            const ptop = canvas.getBoundingClientRect().top;
            bands = [];
            root.querySelectorAll('section').forEach(s => {
                const r = s.getBoundingClientRect();
                const l = s.getAttribute('data-screen-label');
                bands.push({ c: (r.top + r.bottom) / 2 - ptop, hue: HUES[l] != null ? HUES[l] : 200 });
            });
        }
        function hueAt(y) {
            if (!bands || !bands.length) return 200;
            if (y <= bands[0].c) return bands[0].hue;
            if (y >= bands[bands.length - 1].c) return bands[bands.length - 1].hue;
            for (let i = 0; i < bands.length - 1; i++) {
                if (y >= bands[i].c && y < bands[i + 1].c) {
                    const t = (y - bands[i].c) / (bands[i + 1].c - bands[i].c || 1);
                    let h0 = bands[i].hue, d = bands[i + 1].hue - h0;
                    if (d > 180) d -= 360; else if (d < -180) d += 360;
                    return (h0 + d * t + 360) % 360;
                }
            }
            return bands[0].hue;
        }
        function resize() {
            const parent = canvas.parentElement;
            const nw = parent.clientWidth || window.innerWidth;
            const prev = canvas.style.height; canvas.style.height = '0px';
            const nh = Math.max(parent.scrollHeight, window.innerHeight);
            canvas.style.height = prev || '0px';
            const dpr = window.devicePixelRatio || 1;
            canvas.style.width = nw + 'px'; canvas.style.height = nh + 'px';
            canvas.width = Math.round(nw * dpr); canvas.height = Math.round(nh * dpr);
            ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
            w = nw; h = nh;
            const c = Math.ceil(nw / cell), r = Math.ceil(nh / cell);
            if (!grid || c !== cols || r !== rows) { cols = c; rows = r; seed(); }
            buildBands(); draw();
        }
        function draw() {
            if (!grid) return;
            buildBands();
            const light = theme() === 'light';
            const pageRGB = light ? '230,234,240' : '0,0,0';
            const pad = 240, top = Math.max(0, (window.scrollY || 0) - pad), bot = (window.scrollY || 0) + window.innerHeight + pad;
            const y0 = Math.max(0, (top / cell) | 0), y1 = Math.min(rows, ((bot / cell) | 0) + 1);
            if (running) { ctx.fillStyle = 'rgba(' + pageRGB + ',' + (full ? '.34' : '.28') + ')'; ctx.fillRect(0, y0 * cell, w, (y1 - y0) * cell); }
            else { ctx.fillStyle = 'rgb(' + pageRGB + ')'; ctx.fillRect(0, y0 * cell, w, (y1 - y0) * cell); }
            const alpha = full ? (light ? .82 : .85) : (light ? .18 : .24), lum = light ? 52 : 64, sat = light ? 70 : 78;
            for (let y = y0; y < y1; y++) {
                ctx.fillStyle = 'hsla(' + hueAt(y * cell + cell / 2).toFixed(0) + ',' + sat + '%,' + lum + '%,' + alpha + ')';
                const base = y * cols, ry = y * cell + 1, rs = cell - 2;
                for (let x = 0; x < cols; x++) if (grid[base + x]) ctx.fillRect(x * cell + 1, ry, rs, rs);
            }
        }
        function tick() {
            step(); gen++;
            if (gen % 24 === 0 && popHist.length >= 12) {
                const mn = Math.min(...popHist), mx = Math.max(...popHist);
                if (popHist[popHist.length - 1] < cols * rows * .014 || (mx - mn) < 4)
                    stampGun(2 + ((Math.random() * (cols - 40)) | 0), 2 + ((Math.random() * Math.max(1, rows - 12)) | 0));
            }
            if (gen % 22 === 0) {
                const P = patterns(), keys = Object.keys(P);
                stamp(P[keys[(Math.random() * keys.length) | 0]], 2 + ((Math.random() * (cols - 18)) | 0), 2 + ((Math.random() * (rows - 16)) | 0), (Math.random() * 8) | 0);
            }
            draw();
        }
        function apply() { clearInterval(timer); if (running) timer = setInterval(tick, 95); else draw(); canvas.style.cursor = full ? 'crosshair' : 'default'; }

        return {
            init() {
                canvas = document.getElementById('gol'); if (!canvas) return;
                ctx = canvas.getContext('2d');
                resize(); apply();
                canvas.addEventListener('click', e => {
                    const r = canvas.getBoundingClientRect();
                    const x = Math.floor((e.clientX - r.left) / cell), y = Math.floor((e.clientY - r.top) / cell);
                    stamp([[1, 0], [2, 1], [0, 2], [1, 2], [2, 2]], x, y, 0); draw();
                });
                window.addEventListener('resize', resize);
                window.addEventListener('scroll', () => { if (raf) return; raf = requestAnimationFrame(() => { raf = 0; draw(); }); }, { passive: true });
                [250, 700, 1500, 3000].forEach(t => setTimeout(resize, t));
            },
            toggleRun() { running = !running; document.getElementById('gol-run').textContent = running ? '❚❚' : '▶'; apply(); },
            randomize() { seed(); draw(); },
            toggleFull() { full = !full; page.classList.toggle('gol-full', full); const b = document.getElementById('gol-full'); b.classList.toggle('is-on', full); b.textContent = full ? 'show profile' : 'view life'; apply(); },
            redraw() { draw(); }
        };
    })();

    function initTheme() {
        const saved = localStorage.getItem('theme');
        if (saved) page.setAttribute('data-theme', saved);
        document.getElementById('theme-toggle').addEventListener('click', () => {
            const next = page.getAttribute('data-theme') === 'light' ? 'dark' : 'light';
            page.setAttribute('data-theme', next);
            localStorage.setItem('theme', next);
            Gol.redraw();
        });
    }

    function initGolControls() {
        document.getElementById('gol-run').addEventListener('click', () => Gol.toggleRun());
        document.getElementById('gol-rand').addEventListener('click', () => Gol.randomize());
        document.getElementById('gol-full').addEventListener('click', () => Gol.toggleFull());
    }

    function initExpand() {
        const backdrop = document.getElementById('expand-backdrop');
        const close = () => { document.querySelectorAll('.is-expanded').forEach(e => e.classList.remove('is-expanded')); backdrop.classList.remove('in'); document.body.style.overflow = ''; };
        document.querySelectorAll('.expand-btn').forEach(b => b.addEventListener('click', () => {
            const el = document.getElementById(b.dataset.expand); if (!el) return;
            el.classList.add('is-expanded'); backdrop.classList.add('in'); document.body.style.overflow = 'hidden';
        }));
        backdrop.addEventListener('click', e => { if (e.target === backdrop) close(); });
        document.getElementById('expand-close').addEventListener('click', close);
        document.addEventListener('keydown', e => { if (e.key === 'Escape') close(); });
    }

    function initClock() {
        const clock = document.getElementById('clock'), uptimeEl = document.getElementById('uptime');
        const base = parseInt(uptimeEl.dataset.uptime || '0', 10), mount = Date.now();
        const fmt = n => String(n).padStart(2, '0');
        setInterval(() => {
            clock.textContent = new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit', timeZone: 'Europe/Moscow', hour12: false }).format(new Date());
            const up = base + Math.floor((Date.now() - mount) / 1000);
            uptimeEl.textContent = fmt(Math.floor(up / 3600)) + ':' + fmt(Math.floor((up % 3600) / 60)) + ':' + fmt(up % 60);
        }, 1000);
    }

    function drawWeekly() {
        const c = document.getElementById('weekly'); if (!c) return;
        const vals = readJSON('weekly-data', [0, 0, 0, 0, 0, 0, 0]);
        const ctx = c.getContext('2d'); ctx.clearRect(0, 0, c.width, c.height);
        const max = Math.max(...vals, 1), n = 7, gw = c.width / n, bw = gw * 0.5, pad = 16;
        const peak = vals.indexOf(Math.max(...vals));
        for (let i = 0; i < n; i++) {
            const hh = vals[i] / max * (c.height - pad), x = i * gw + (gw - bw) / 2, y = c.height - hh, rad = Math.min(7, bw / 2);
            ctx.fillStyle = i === peak ? '#10d060' : 'rgba(16,208,96,.32)';
            ctx.beginPath(); ctx.moveTo(x, c.height); ctx.lineTo(x, y + rad); ctx.arcTo(x, y, x + rad, y, rad);
            ctx.lineTo(x + bw - rad, y); ctx.arcTo(x + bw, y, x + bw, y + rad, rad); ctx.lineTo(x + bw, c.height); ctx.closePath(); ctx.fill();
        }
    }
    function drawPlatform() {
        const c = document.getElementById('platform'); if (!c) return;
        const data = readJSON('platform-data', []);
        if (!data.length) return;
        const ctx = c.getContext('2d'); ctx.clearRect(0, 0, c.width, c.height);
        const cx = c.width / 2, cy = c.height / 2, r = c.width / 2 - 10, ir = r * 0.58;
        let a = -Math.PI / 2, tot = data.reduce((s, d) => s + d[0], 0);
        data.forEach(d => { const ang = d[0] / tot * Math.PI * 2; ctx.beginPath(); ctx.moveTo(cx, cy); ctx.arc(cx, cy, r, a, a + ang); ctx.closePath(); ctx.fillStyle = d[1]; ctx.fill(); a += ang; });
        ctx.globalCompositeOperation = 'destination-out'; ctx.beginPath(); ctx.arc(cx, cy, ir, 0, Math.PI * 2); ctx.fill(); ctx.globalCompositeOperation = 'source-over';
    }

    function initMachineTabs() {
        document.querySelectorAll('.neofetch-section').forEach(sec => {
            sec.querySelectorAll('.machine-btn').forEach(btn => btn.addEventListener('click', () => {
                sec.querySelectorAll('.machine-btn').forEach(b => b.removeAttribute('data-active'));
                btn.setAttribute('data-active', 'true');
                sec.querySelectorAll('.neofetch-output').forEach(o => o.style.display = 'none');
                const out = sec.querySelector('#neofetch-' + btn.dataset.machine); if (out) out.style.display = 'block';
            }));
        });
    }

    function initMeme() {
        const btn = document.getElementById('meme-shuffle');
        if (!btn) return;
        btn.addEventListener('click', () => {
            if (btn.disabled) return;
            const orig = btn.innerHTML;
            btn.disabled = true;
            fetch('/api/meme/refresh', { method: 'POST', headers: { 'Content-Type': 'application/json' } })
                .then(r => r.json())
                .then(d => { if (d && d.meme) renderMeme(d.meme); })
                .catch(() => {})
                .finally(() => { btn.disabled = false; btn.innerHTML = orig; });
        });
    }

    function renderMeme(meme) {
        const box = document.querySelector('.memebox__img');
        if (!box || !meme) return;
        if (meme.image) {
            const img = document.createElement('img');
            img.src = meme.image;
            img.alt = '';
            box.replaceChildren(img);
        } else {
            const p = document.createElement('p');
            p.textContent = meme.text || '';
            box.replaceChildren(p);
        }
    }

    function start() {
        initTheme(); Gol.init(); initGolControls(); initExpand(); initClock();
        drawWeekly(); drawPlatform(); initMachineTabs(); initMeme();
        window.drawWeekly = drawWeekly;
        window.drawPlatform = drawPlatform;
        window.renderMeme = renderMeme;
    }
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
    else start();
})();