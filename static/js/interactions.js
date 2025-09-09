(function () {
    'use strict';

    // ---------- helpers ----------
    const $  = (q, c = document) => c.querySelector(q);
    const $$ = (q, c = document) => Array.from(c.querySelectorAll(q));
    const on = (el, ev, fn, opts) => el && el.addEventListener(ev, fn, opts);
    const raf = (fn) => requestAnimationFrame(fn);
    const now = () => Date.now();
    const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
    const throttle = (fn, ms = 100) => {
        let t = 0, lastArgs = null, to;
        return (...args) => {
            const n = now();
            if (n - t > ms) { t = n; fn(...args); }
            else { lastArgs = args; clearTimeout(to); to = setTimeout(() => { t = now(); fn(...(lastArgs || [])); }, ms); }
        };
    };
    const storage = {
        get(k, fallback) { try { return JSON.parse(localStorage.getItem(k)) ?? fallback; } catch { return fallback; } },
        set(k, v) { try { localStorage.setItem(k, JSON.stringify(v)); } catch {} },
    };

    document.documentElement.classList.add('js');

    // ---------- toast + ripple ----------
    let toastRoot;
    function toast(msg) {
        toastRoot ||= (() => {
            const r = document.createElement('div');
            r.className = 'toast-root';
            document.body.appendChild(r);
            return r;
        })();
        const n = document.createElement('div');
        n.className = 'toast';
        n.textContent = msg;
        toastRoot.appendChild(n);
        requestAnimationFrame(() => n.classList.add('in'));
        setTimeout(() => n.classList.remove('in'), 1600);
        setTimeout(() => n.remove(), 1900);
    }
    function ripple(e) {
        const el = e.currentTarget; el.classList.add('ripple-host');
        const r = document.createElement('span'); r.className = 'ripple';
        const rect = el.getBoundingClientRect(); const d = Math.max(rect.width, rect.height);
        r.style.width = r.style.height = d + 'px';
        r.style.left = (e.clientX - rect.left - d/2) + 'px';
        r.style.top  = (e.clientY - rect.top  - d/2) + 'px';
        el.appendChild(r); setTimeout(() => r.remove(), 600);
    }

    // ---------- boot ----------
    const root = $('.container'); if (!root) return;

    // ensure mosaic section exists
    let mosaic = $('.mosaic');
    if (!mosaic) {
        mosaic = document.createElement('section');
        mosaic.className = 'mosaic';
        const profile = $('.profile-section', root);
        if (profile && profile.nextSibling) root.insertBefore(mosaic, profile.nextSibling);
        else root.appendChild(mosaic);
    }

    // move sections into mosaic
    const toMove = [...root.children].filter(el => el !== mosaic && !el.classList.contains('profile-section'));
    toMove.forEach(el => {
        el.classList.add('plugin');
        if (!el.querySelector('.plugin__inner')) {
            const inner = document.createElement('div'); inner.className = 'plugin__inner';
            while (el.firstChild) inner.appendChild(el.firstChild);
            el.appendChild(inner);
        }
        if (!el.id) {
            const guess = (el.className.match(/([a-z0-9-]+)-section/i) || [,'tile'])[1];
            el.id = `${guess}-${Math.random().toString(36).slice(2,7)}`;
        }
        // wipe any stale inline placement from older builds
        el.style.gridRow = el.style.gridColumn = el.style.gridRowEnd = '';
        mosaic.appendChild(el);
    });

    // ---------- default widths ----------
    const defaultWidths = {
        'projects-section': 3, 'beatleader-section': 2, 'steam-section': 2,
        'neofetch-section': 2, 'tech-section': 2, 'social-section': 1,
        'code-section': 2, 'meme-section': 1, 'lastfm-section': 2,
        'webring-section': 2, 'visitors-section': 1, 'info-section': 2,
    };
    $$('.plugin', mosaic).forEach(el => {
        const saved = storage.get('mosaic.widths', {})[el.id];
        const key = Object.keys(defaultWidths).find(k => el.classList.contains(k));
        const w = el.dataset.w || saved || (key ? defaultWidths[key] : 1);
        el.dataset.w = String(clamp(+w || 1, 1, 3));
    });

    // ---------- sizing ----------
    const cssNumber = (el, prop) => {
        const v = getComputedStyle(el).getPropertyValue(prop);
        const m = /([\d.]+)/.exec(v); return m ? parseFloat(m[1]) : 0;
    };
    const rowMetrics = () => {
        const rowSize = cssNumber(mosaic, 'grid-auto-rows') || 8;
        const gapRaw = getComputedStyle(mosaic).gap || getComputedStyle(mosaic).gridRowGap;
        const parts = (gapRaw || '24px').trim().split(/\s+/);
        const rowGap = parseFloat(parts.length === 2 ? parts[1] : parts[0]) || 24;
        return { rowSize, rowGap };
    };

    // generous cushion so a tiny async change never causes overflow into the next tile
    const EXTRA = 24;

    function rowSpanFromPx(h) {
        const { rowSize, rowGap } = rowMetrics();
        return Math.max(1, Math.ceil((h + EXTRA + rowGap) / (rowSize + rowGap)));
    }

    // measure the **outer** plugin box (includes padding, header, etc.)
    function outerHeightPx(plugin) {
        const r = plugin.getBoundingClientRect();
        const mb = parseFloat(getComputedStyle(plugin).marginBottom) || 0;
        return Math.ceil(r.height + mb);
    }

    // ---------- columns ----------
    const MIN_COL = 280;
    function colCount() {
        const style = getComputedStyle(mosaic);
        const gap = parseFloat((style.columnGap || style.gap || '24px').split(/\s+/)[0]) || 24;
        const w = mosaic.clientWidth;
        return Math.max(1, Math.floor((w + gap) / (MIN_COL + gap)));
    }
    function clampSpansToCols() {
        const cols = colCount();
        $$('.plugin', mosaic).forEach(el => {
            let w = clamp(+el.dataset.w || 1, 1, 3);
            if (w > cols) w = cols;
            el.dataset.w = String(w);
        });
    }

    // ---------- FLIP ----------
    function flip(update) {
        const items = $$('.plugin', mosaic);
        const first = new Map(items.map(el => [el, el.getBoundingClientRect()]));
        update();
        const last  = new Map(items.map(el => [el, el.getBoundingClientRect()]));
        items.forEach(el => {
            const f = first.get(el), l = last.get(el); if (!f || !l) return;
            const dx = f.left - l.left, dy = f.top - l.top;
            const sx = f.width ? f.width / l.width : 1, sy = f.height ? f.height / l.height : 1;
            if (Math.abs(dx)<1 && Math.abs(dy)<1 && Math.abs(sx-1)<.01 && Math.abs(sy-1)<.01) return;
            el.animate(
                [{ transform:`translate(${dx}px,${dy}px) scale(${sx},${sy})` },
                    { transform:'translate(0,0) scale(1,1)' }],
                { duration:260, easing:'cubic-bezier(.2,.7,.2,1)', fill:'both' }
            );
        });
    }

    // ---------- priorities (pin top; projects bottom) ----------
    function priority(el) {
        if (el.dataset.pinned === '1') return 2;
        if (el.classList.contains('projects-section')) return -1;
        return 0;
    }

    // ---------- packer (explicit placement; wide-first inside priority) ----------
    function measure(el) {
        const w = clamp(+el.dataset.w || 1, 1, Math.max(1, colCount()));
        const hPx = outerHeightPx(el);
        return { el, w, span: rowSpanFromPx(hPx) };
    }

    function layoutPacker() {
        if (!mosaic.isConnected) return;
        clampSpansToCols();

        const cols = colCount();
        const items = $$('.plugin', mosaic);
        const orderIndex = new Map(items.map((el, i) => [el, i]));
        const m = items.map(el => {
            const o = measure(el);
            o._pri = priority(el);
            o._ord = orderIndex.get(el);
            return o;
        });

        // Wide-first within priority to reduce “pits”
        m.sort((a, b) =>
            (b._pri - a._pri) ||
            (b.w - a.w) ||
            (a._ord - b._ord)
        );

        flip(() => {
            const occ = new Array(cols).fill(0); // column skyline

            function bestStart(w) {
                let bestC = 1, bestH = Infinity;
                for (let c = 1; c <= cols - w + 1; c++) {
                    const idx = c - 1;
                    const h = Math.max(...occ.slice(idx, idx + w));
                    if (h < bestH) { bestH = h; bestC = c; }
                }
                return { c: bestC, h: bestH };
            }

            m.forEach(x => {
                const w = Math.min(x.w, cols);
                const { c, h } = bestStart(w);
                const startRow = h + 1;
                const endRow   = startRow + x.span;

                for (let i = c - 1; i < c - 1 + w; i++) occ[i] = endRow;

                x.el.style.gridColumn = `${c} / span ${w}`;
                x.el.style.gridRow    = `${startRow} / span ${x.span}`;
            });
        });
    }

    // ---------- pack triggers ----------
    const packAll = throttle(() => {
        if (document.hidden) return;
        layoutPacker();
        fitNeofetch();
    }, 80);

    function settlePasses() {
        // a few extra passes to catch slow images/fonts without jank
        [0, 60, 180, 420, 900, 1600].forEach(d => setTimeout(packAll, d));
    }

    // ---------- first pass + ordering ----------
    (function initialLayout() {
        const items = $$('.plugin', mosaic);

        // restore order if present; otherwise push projects to bottom
        const order = storage.get('mosaic.order', []);
        if (order && order.length) {
            const map = Object.fromEntries(items.map(n => [n.id, n]));
            order.forEach(id => map[id] && mosaic.appendChild(map[id]));
        } else {
            $$('.projects-section.plugin', mosaic).forEach(n => mosaic.appendChild(n));
        }

        // restore pinned
        const pinned = storage.get('mosaic.pinned', {});
        items.forEach(el => { if (pinned[el.id]) el.dataset.pinned = '1'; });
        $$('.plugin', mosaic)
            .sort((a,b) => (b.dataset.pinned||'0').localeCompare(a.dataset.pinned||'0'))
            .forEach(n => mosaic.appendChild(n));

        raf(() => { packAll(); settlePasses(); });
    })();

    // ---------- width/pin/collapse/expand ----------
    function setWidth(el, w) {
        w = clamp(w, 1, 3);
        el.dataset.w = String(w);
        const widths = storage.get('mosaic.widths', {}); widths[el.id] = w; storage.set('mosaic.widths', widths);
        packAll(); settlePasses();
    }
    function toggleCollapse(el) {
        el.classList.toggle('is-collapsed');
        packAll(); settlePasses();
    }
    function pin(el) {
        el.dataset.pinned = el.dataset.pinned === '1' ? '0' : '1';
        $$('.plugin', mosaic).sort((a,b) => (b.dataset.pinned||'0').localeCompare(a.dataset.pinned||'0')).forEach(n => mosaic.appendChild(n));
        const map = storage.get('mosaic.pinned', {}); map[el.id] = el.dataset.pinned === '1'; storage.set('mosaic.pinned', map);
        storage.set('mosaic.order', $$('.plugin', mosaic).map(n => n.id));
        toast(el.dataset.pinned === '1' ? 'Pinned' : 'Unpinned');
        packAll(); settlePasses();
    }

    function makeDot(action, title) {
        const b = document.createElement('button');
        b.className = 'icon-btn plugin-btn'; b.type = 'button'; b.dataset.action = action; b.setAttribute('aria-label', title);
        on(b, 'click', (e) => { ripple(e); b.blur(); handleAction(b.closest('.plugin'), action); });
        return b;
    }
    function ensureToolbar(el) {
        let titleEl = $('h3, h2, h4', el);
        if (!titleEl) {
            titleEl = document.createElement('h3');
            titleEl.className = 'plugin-title';
            titleEl.textContent = (el.className.match(/([a-z0-9-]+)-section/i) || [,'Block'])[1].replace(/-/g,' ');
            el.querySelector('.plugin__inner').prepend(titleEl);
        } else { titleEl.className = 'plugin-title'; }
        let headerRow = $('.plugin-header', el);
        if (!headerRow) {
            headerRow = document.createElement('div'); headerRow.className = 'plugin-header';
            headerRow.appendChild(titleEl); el.querySelector('.plugin__inner').prepend(headerRow);
        }
        let bar = $('.plugin-toolbar', headerRow);
        if (!bar) {
            bar = document.createElement('div'); bar.className = 'plugin-toolbar'; headerRow.appendChild(bar);
            bar.append(makeDot('collapse','Collapse'), makeDot('w-dec','Narrower'), makeDot('w-inc','Wider'), makeDot('expand','Expand'));
        }
        headerRow.classList.add('drag-handle');
        headerRow.setAttribute('draggable','true');
    }
    $$('.plugin', mosaic).forEach(ensureToolbar);

    // expand with overlay
    let expanded = null, overlay, slotMap = new Map();
    function ensureOverlay(){
        overlay ||= (() => {
            const o = document.createElement('div'); o.className = 'plugin-overlay';
            on(o, 'click', (e) => { if (e.target === o) collapseExpanded(); });
            document.body.appendChild(o);
            on(document, 'keydown', (e) => { if (e.key === 'Escape') collapseExpanded(); });
            return o;
        })();
        return overlay;
    }
    function makePlaceholder(fromEl){
        const p = document.createElement('div');
        p.className = 'plugin plugin-placeholder';
        p.dataset.w = fromEl.dataset.w || '1';
        p.style.gridRowEnd = `span ${rowSpanFromPx(outerHeightPx(fromEl))}`;
        return p;
    }
    function expand(el){
        if (expanded === el) return collapseExpanded();
        collapseExpanded();
        const ph = makePlaceholder(el);
        slotMap.set(el, ph);
        mosaic.insertBefore(ph, el.nextSibling);
        ensureOverlay().classList.add('in');
        el.classList.add('plugin--expanded');
        ensureOverlay().appendChild(el);
        setTimeout(() => { packAll(); fitNeofetch(); }, 60);
        expanded = el;
    }
    function collapseExpanded(){
        if (!expanded) return;
        const ph = slotMap.get(expanded);
        expanded.classList.remove('plugin--expanded');
        if (ph && ph.parentNode) ph.parentNode.replaceChild(expanded, ph);
        slotMap.delete(expanded);
        ensureOverlay().classList.remove('in');
        expanded = null;
        packAll(); settlePasses(); fitNeofetch();
    }
    function handleAction(el, action) {
        if (!el) return;
        if (action === 'expand')   expand(el);
        if (action === 'collapse') toggleCollapse(el);
        if (action === 'pin')      pin(el);
        if (action === 'w-inc')    setWidth(el, (+el.dataset.w || 1) + 1);
        if (action === 'w-dec')    setWidth(el, (+el.dataset.w || 1) - 1);
    }

    // ---------- persistence ----------
    function persistOrder() { storage.set('mosaic.order', $$('.plugin', mosaic).map(n => n.id)); }

    // ---------- drag & drop ----------
    let dragEl = null, dragProxy = null, dragOffset = {x:0,y:0}, isDragging = false;
    function createProxy(element, rect) {
        const proxy = element.cloneNode(true);
        proxy.className = element.className + ' drag-proxy';
        Object.assign(proxy.style, {
            position:'fixed', left:rect.left+'px', top:rect.top+'px',
            width:rect.width+'px', height:rect.height+'px', pointerEvents:'none',
            zIndex:'10000', transform:'translateZ(0)', opacity:'0.94', transition:'none',
            boxShadow:'0 20px 60px rgba(0,0,0,0.28)', overflow:'hidden'
        });
        proxy.querySelectorAll('*').forEach(n => n.style.pointerEvents = 'none');
        document.body.appendChild(proxy);
        return proxy;
    }
    on(mosaic, 'dragstart', (e) => {
        const handle = e.target.closest('.drag-handle'); if (!handle) { e.preventDefault(); return; }
        dragEl = handle.closest('.plugin'); if (!dragEl) { e.preventDefault(); return; }
        const rect = dragEl.getBoundingClientRect();
        dragOffset.x = e.clientX - rect.left; dragOffset.y = e.clientY - rect.top;
        dragProxy = createProxy(dragEl, rect); isDragging = true;
        dragEl.classList.add('dragging'); dragEl.style.opacity = '0.3'; document.body.classList.add('dragging-cursor');
        const img = new Image(); img.src = 'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"></svg>';
        e.dataTransfer.setDragImage(img, 0, 0); e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', dragEl.id);
    });
    function moveProxy(e) {
        if (!dragProxy || !isDragging) return;
        let x = e.clientX - dragOffset.x, y = e.clientY - dragOffset.y;
        x = clamp(x, 0, Math.max(0, window.innerWidth  - dragProxy.offsetWidth));
        y = clamp(y, 0, Math.max(0, window.innerHeight - dragProxy.offsetHeight));
        dragProxy.style.left = x + 'px'; dragProxy.style.top  = y + 'px';
    }
    on(document, 'dragover', (e) => { if (!isDragging || !dragProxy) return; e.preventDefault(); moveProxy(e); });
    function nearestByY(y) {
        const plugins = $$('.plugin', mosaic).filter(p => p !== dragEl);
        let closest = null, before = true, best = Infinity;
        plugins.forEach(p => {
            const r = p.getBoundingClientRect(); const centerY = r.top + r.height/2; const d = Math.abs(y - centerY);
            if (d < best) { best = d; closest = p; before = y < centerY; }
        });
        return { plugin: closest, before };
    }
    on(mosaic, 'drop', (e) => {
        if (!dragEl || !isDragging) return; e.preventDefault();
        const { plugin, before } = nearestByY(e.clientY);
        if (plugin) mosaic.insertBefore(dragEl, before ? plugin : plugin.nextSibling);
        cleanupDrag(); persistOrder(); packAll(); settlePasses();
    });
    on(document, 'dragend', () => { if (isDragging) { cleanupDrag(); packAll(); settlePasses(); } });
    function cleanupDrag() {
        dragEl?.classList.remove('dragging'); if (dragEl) dragEl.style.opacity = '';
        dragProxy?.remove(); dragProxy = null; dragEl = null; isDragging = false;
        document.body.classList.remove('dragging-cursor');
    }

    // ---------- misc UX ----------
    function initClicks() {
        $$('.plugin-btn, .icon-btn, .meme-refresh-btn, .social-link').forEach(b => on(b, 'click', ripple));
        $$('.plugin .plugin-header').forEach(h => on(h, 'dblclick', () => toggleCollapse(h.closest('.plugin'))));
    }
    function initStatus(){
        $('#js-status')?.classList.add('status-online');
        if ($('#js-text')) $('#js-text').textContent = 'Enabled';
        const ok = (() => { try { localStorage.setItem('_t','1'); localStorage.removeItem('_t'); return true; } catch { return false; } })();
        if (ok) { $('#storage-status')?.classList.add('status-online'); $('#storage-text') && ($('#storage-text').textContent = 'Available'); }
        else    { $('#storage-status')?.classList.add('status-offline'); $('#storage-text') && ($('#storage-text').textContent = 'Unavailable'); }
    }

    // copy info values
    $$('.info-item').forEach(item => {
        item.title = 'Click to copy value';
        on(item, 'click', async () => {
            const val = $('.info-value', item)?.innerText?.trim(); if (!val) return;
            try { await navigator.clipboard.writeText(val); toast('Copied: ' + val); } catch { toast('Copy failed'); }
        });
    });

    // Intersection reveal
    const io = new IntersectionObserver(entries => {
        entries.forEach(e => e.target.classList.toggle('reveal', e.isIntersecting));
    }, { threshold: 0.08 });
    $$('.plugin', mosaic).forEach(el => io.observe(el));

    // Fonts/images -> pack
    if ('fonts' in document && document.fonts.ready) document.fonts.ready.then(() => { packAll(); settlePasses(); });

    // Observe size changes on BOTH the plugin and the inner (border-box)
    const ro = new ResizeObserver(() => { packAll(); });
    $$('.plugin', mosaic).forEach(n => ro.observe(n, { box: 'border-box' }));
    $$('.plugin__inner', mosaic).forEach(n => ro.observe(n, { box: 'border-box' }));

    // Observe for NEW images and attach load listeners
    const mo = new MutationObserver(muts => {
        let queued = false;
        const hook = (img) => {
            if (!img || img.tagName !== 'IMG') return;
            queued = true;
            if (img.complete) { setTimeout(packAll, 0); return; }
            img.addEventListener('load',  packAll, { once: true });
            img.addEventListener('error', packAll, { once: true });
        };
        muts.forEach(m => {
            m.addedNodes && m.addedNodes.forEach(node => {
                if (node.nodeType === 1) {
                    hook(node);
                    node.querySelectorAll && node.querySelectorAll('img').forEach(hook);
                }
            });
            if (m.type === 'attributes' && m.target.tagName === 'IMG' && (m.attributeName === 'src' || m.attributeName === 'srcset')) {
                hook(m.target);
            }
        });
        if (queued) settlePasses();
    });
    mo.observe(mosaic, { childList: true, subtree: true, attributes: true, attributeFilter: ['src','srcset'] });

    // window / tab visibility
    on(window, 'resize', throttle(() => { packAll(); }, 140));
    on(document, 'visibilitychange', () => { if (!document.hidden) { packAll(); settlePasses(); } });

    // details/summary in code blocks
    document.addEventListener('toggle', (e) => {
        if (e.target.closest('.code-section') && e.target.tagName === 'DETAILS') setTimeout(packAll, 50);
    }, true);

    // Neofetch switcher
    document.addEventListener('click', (e) => {
        const btn = e.target.closest('.machine-btn'); if (!btn) return;
        e.preventDefault();
        const target = btn.dataset.machine;
        const sec = btn.closest('.neofetch-section'); if (!sec || !target) return;

        sec.querySelectorAll('.machine-btn').forEach(b => { b.removeAttribute('data-active'); b.classList.remove('active'); });
        btn.setAttribute('data-active','true'); btn.classList.add('active');

        sec.querySelectorAll('.neofetch-output').forEach(o => o.style.display = 'none');
        const out = sec.querySelector(`#neofetch-${target}`); if (out) out.style.display = 'block';

        setTimeout(() => { packAll(); fitNeofetch(); }, 30);
    });

    // visitors uptime (client)
    (function uptime(){
        const els = $$('[data-uptime]'); if (!els.length) return;
        storage.set('client_start_time', storage.get('client_start_time', Date.now()));
        function tick() {
            const start = storage.get('client_start_time', Date.now());
            const elapsed = Date.now() - start;
            const s = Math.floor(elapsed/1000), m = Math.floor(s/60), h = Math.floor(m/60), d = Math.floor(h/24);
            let txt = d>0 ? `${d}d ${h%24}h ${m%60}m` : h>0 ? `${h}h ${m%60}m` : `${m}m`;
            els.forEach(el => { if (el.dataset.uptime === 'client' || !el.dataset.uptime) el.textContent = txt; });
        }
        tick(); setInterval(tick, 1000);
    })();

    // ---------- neofetch fit ----------
    function fitNeofetch() {
        const terms = document.querySelectorAll('.neofetch-section .terminal');
        terms.forEach(t => {
            const pre = t.querySelector('pre'); if (!pre) return;
            t.style.overflowX = 'auto'; t.style.overflowY = 'auto';

            // reset
            t.style.removeProperty('--neo-scale'); pre.style.removeProperty('transform'); pre.style.transform = 'scale(1)';

            setTimeout(() => {
                const containerWidth = t.clientWidth - 24;
                const contentWidth = pre.scrollWidth;
                const contentHeight = pre.scrollHeight;

                let scale = 1;
                const inExpanded = !!t.closest('.plugin--expanded');
                if (!inExpanded && contentWidth > containerWidth && containerWidth > 0) {
                    scale = Math.max(0.3, containerWidth / contentWidth);
                }
                t.style.setProperty('--neo-scale', String(scale));
                pre.style.transform = `scale(${scale})`;

                const scaledHeight = contentHeight * scale;
                t.style.height = Math.max(200, scaledHeight + 24) + 'px';
            }, 10);
        });
    }

    // ---------- init ----------
    (function init(){ initClicks(); initStatus(); })();

    // hooks
    window.mosaicUtils = { resizeAll: packAll, toast, expand, collapseExpanded };
    on(document, 'plugin-updated', () => setTimeout(() => { packAll(); fitNeofetch(); }, 50));
    window.addEventListener('lastfm_update', () => setTimeout(() => { packAll(); fitNeofetch(); }, 50));
    window.addEventListener('code_stats_update', () => setTimeout(() => { packAll(); fitNeofetch(); }, 50));
    window.addEventListener('plugins_updated', () => setTimeout(() => location.reload(), 900));
})();
