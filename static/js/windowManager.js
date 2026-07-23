(function () {
    'use strict';

    document.documentElement.classList.add('js');

    const $ = (q, c = document) => (c ? c.querySelector(q) : null);
    const $$ = (q, c = document) => (c ? Array.from(c.querySelectorAll(q)) : []);
    const on = (el, ev, fn, opts) => el && el.addEventListener(ev, fn, opts);
    const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
    const now = () => Date.now();

    const throttle = (fn, ms = 100) => {
        let t = 0, to, last;
        return (...args) => {
            const n = now();
            if (n - t > ms) {
                t = n;
                fn(...args);
            } else {
                last = args;
                clearTimeout(to);
                to = setTimeout(() => {
                    t = now();
                    fn(...(last || []));
                }, ms);
            }
        };
    };

    const debounce = (fn, ms = 100) => {
        let to;
        return (...args) => {
            clearTimeout(to);
            to = setTimeout(() => fn(...args), ms);
        };
    };

    const isInteractive = (node) =>
        !!node.closest('button, a, input, select, textarea, [contenteditable], .plugin-btn');

    const root = $('.container');
    if (!root) return;

    let mosaic = $('.mosaic', root);
    if (!mosaic) {
        mosaic = document.createElement('section');
        mosaic.className = 'mosaic';
        root.prepend(mosaic);
    }

    const hasPrebakeAtBoot = mosaic.classList.contains('mosaic-prebaked');

    const widthsStoreKey = 'mosaic.widths';
    const orderStoreKey = 'mosaic.order';
    const spansStoreKey = 'mosaic.spans';

    const safeJSON = (s) => {
        try { return JSON.parse(s); } catch { return null; }
    };
    const storageGet = (k) => { try { return localStorage.getItem(k); } catch { return null; } };
    const storageSet = (k, v) => { try { localStorage.setItem(k, v); } catch {} };

    const debouncedPersistOrder = debounce(() => {
        enforceFixedEdgeOrder(mosaic);
        const order = pluginList().map((n) => n.id);
        storageSet(orderStoreKey, JSON.stringify(order));
    }, 200);

    const savedWidths = safeJSON(storageGet(widthsStoreKey)) || {};
    const savedOrder = safeJSON(storageGet(orderStoreKey)) || [];

    const MIN_COL_FALLBACK = 280;
    const ROW_HEIGHT = 1;
    const Z_FRONT = 1500;

    let phase = 'booting';
    let pendingPack = null;
    let scrollIdleTimer = null;

    let pluginCache = null;
    let cachedCols = 0;
    let cachedColsViewport = 0;
    let cachedColsMosaic = 0;
    let lastViewportWidth = window.innerWidth;
    let lastMosaicWidth = mosaic.clientWidth;

    let overlay = null;
    let expanded = null;
    let _scrollLockY = 0;

    const preferredWidths = new Map();

    function pluginList() {
        if (!pluginCache) pluginCache = $$('.plugin', mosaic);
        return pluginCache;
    }
    function invalidatePluginCache() { pluginCache = null; }

    function innerOf(el) {
        return (el && el.querySelector(':scope > .plugin__inner')) || el;
    }

    function isProjectPlugin(el) { return !!el && el.classList?.contains('projects-section'); }
    function isWebringPlugin(el) { return !!el && el.classList?.contains('webring-section'); }
    function isProfilePlugin(el) { return !!el && el.classList?.contains('profile-section'); }

    function pluginNameFromClass(el) {
        for (const cls of el.classList) {
            if (cls.endsWith('-section')) {
                return cls.slice(0, -'-section'.length);
            }
        }
        return 'plugin';
    }

    function ensurePluginId(el) {
        if (!el.id) {
            el.id = `${pluginNameFromClass(el)}-plugin`;
        }
        return el.id;
    }

    function ensureAllPluginIds() {
        pluginList().forEach(ensurePluginId);
    }

    function clearMosaicLayoutState(el) {
        el.style.removeProperty('grid-column');
        el.style.removeProperty('grid-row');
        el.style.removeProperty('grid-row-start');
        el.style.removeProperty('grid-row-end');
        el.style.removeProperty('height');
        el.style.removeProperty('min-height');

        delete el.dataset.currentSpan;
        delete el.dataset.mosaicSpan;
        delete el.dataset.mosaicMw;
        delete el.dataset.mosaicDirty;
    }

    /**
     * Projects всегда находится после .mosaic и вообще не участвует
     * в расчёте колонок, высот, drag-and-drop и сохранённого порядка.
     */
    function mountProjectsTail() {
        const project =
            $('.projects-section.plugin', mosaic) ||
            $('.projects-section.plugin', root);

        if (!project) return null;

        ensurePluginId(project);
        project.dataset.fixedTail = '1';

        const mustMove =
            project.parentElement !== root ||
            project !== root.lastElementChild;

        if (mustMove) {
            clearMosaicLayoutState(project);
            root.appendChild(project);
            invalidatePluginCache();
        }

        return project;
    }

    ensureAllPluginIds();
    mountProjectsTail();

    function enforceFixedEdgeOrder(m = mosaic) {
        const nodes = Array.from(m.children)
            .filter((node) => node.classList?.contains('plugin'));

        const webring = nodes.filter(isWebringPlugin);
        const profile = nodes.filter(isProfilePlugin);

        const middle = nodes.filter((node) =>
            !isWebringPlugin(node) &&
            !isProfilePlugin(node) &&
            !isProjectPlugin(node)
        );

        [
            ...webring,
            ...profile,
            ...middle,
        ].forEach((node) => m.appendChild(node));

        invalidatePluginCache();
        mountProjectsTail();
    }

    function applySavedOrder() {
        if (!savedOrder.length) {
            enforceFixedEdgeOrder(mosaic);
            return;
        }
        const byId = Object.fromEntries(pluginList().map((n) => [n.id, n]));
        savedOrder.forEach((id) => byId[id] && mosaic.appendChild(byId[id]));
        enforceFixedEdgeOrder(mosaic);
    }

    pluginList().forEach((el) => {
        const serverWidth = clamp(+el.dataset.w || 1, 1, 3);
        const preferredWidth = clamp(savedWidths[el.id] || serverWidth, 1, 3);
        el.dataset.w = String(serverWidth);
        preferredWidths.set(el.id, preferredWidth);
        if (el.classList.contains('profile-section')) el.dataset.pinned = '1';
    });

    function readColMin() {
        const fromRoot = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--col-min')) || 0;
        const fromMosaic = parseFloat(getComputedStyle(mosaic).getPropertyValue('--col-min')) || 0;
        return fromRoot || fromMosaic || MIN_COL_FALLBACK;
    }

    function viewportPadding(vw) {
        if (vw <= 780) return 16;
        return clamp(vw * 0.03, 12, 28);
    }

    function mosaicWidthForViewport(vw) {
        return Math.max(0, Math.floor(vw - viewportPadding(vw) * 2));
    }

    function effectiveMosaicWidth() {
        const real = mosaic.clientWidth;
        const fromViewport = mosaicWidthForViewport(window.innerWidth);
        if (real <= 0 || real < fromViewport * 0.75) return fromViewport;
        return real;
    }

    function colCount() {
        const w = effectiveMosaicWidth();
        const vw = window.innerWidth;
        if (w === cachedColsMosaic && vw === cachedColsViewport && cachedCols > 0) return cachedCols;

        const style = getComputedStyle(mosaic);
        const gapStr = style.columnGap || style.gap || '12px';
        const gap = parseFloat(gapStr.split(/\s+/)[0]) || 12;
        const minCol = readColMin();

        cachedCols = w <= 0 ? 1 : Math.max(1, Math.floor((w + gap) / (minCol + gap)));
        cachedColsMosaic = w;
        cachedColsViewport = vw;
        return cachedCols;
    }

    function clampSpansToCols() {
        const cols = colCount();
        pluginList().forEach((el) => {
            let w = clamp(+el.dataset.w || 1, 1, 3);
            if (w > cols) w = cols;
            el.dataset.w = String(w);
        });
    }

    function restorePreferredWidths(maxCols = colCount()) {
        pluginList().forEach((el) => {
            const pref = preferredWidths.get(el.id);
            if (pref != null) el.dataset.w = String(clamp(pref, 1, Math.min(3, maxCols)));
        });
    }

    function getAppliedSpan(el, cols) {
        const explicit = parseInt(el.dataset.currentSpan || '', 10);
        if (Number.isFinite(explicit) && explicit > 0) return clamp(explicit, 1, cols);
        const m = (el.style.gridColumn || '').match(/span\s+(\d+)/i);
        if (!m) return 0;
        const span = parseInt(m[1], 10);
        return Number.isFinite(span) && span > 0 ? clamp(span, 1, cols) : 0;
    }

    function setSpansForCols(items, cols) {
        items.forEach((el) => {
            const baseSpan = clamp(+el.dataset.w || 1, 1, Math.min(3, cols));
            el.dataset.mosaicSpan = String(baseSpan);
        });
    }

    function batchMeasureHeights(items) {
        const heights = new Map();
        let maxH = 0;

        const probe = document.createElement('div');
        probe.style.cssText =
            'position:absolute;left:-99999px;top:0;visibility:hidden;pointer-events:none;contain:layout style;';
        document.body.appendChild(probe);

        const mosaicWidth = effectiveMosaicWidth();
        const style = getComputedStyle(mosaic);
        const gap = parseFloat(style.columnGap || style.gap || '12px') || 12;
        const cols = colCount();
        const colWidth = (mosaicWidth - gap * (cols - 1)) / cols;

        items.forEach((el) => {
            const span = parseInt(el.dataset.mosaicSpan || el.dataset.w || '1', 10) || 1;
            const targetWidth = colWidth * span + gap * (span - 1);

            const cachedH = parseFloat(el.style.height) || 0;
            const cachedW = parseFloat(el.dataset.mosaicMw || '0');
            const dirty = el.dataset.mosaicDirty === '1';

            if (cachedH > 0 && Math.abs(cachedW - targetWidth) < 2 && !dirty) {
                heights.set(el, cachedH);
                if (cachedH > maxH) maxH = cachedH;
                return;
            }

            const clone = el.cloneNode(true);
            clone.style.cssText = `width:${targetWidth}px;height:auto;min-height:0;max-height:none;position:static;display:block;`;
            clone.style.gridColumn = '';
            clone.style.gridRow = '';
            probe.appendChild(clone);

            const h = Math.ceil(clone.getBoundingClientRect().height) || 100;
            heights.set(el, h);
            if (h > maxH) maxH = h;

            el.dataset.mosaicMw = String(targetWidth);
            el.dataset.mosaicDirty = '0';

            probe.removeChild(clone);
        });

        document.body.removeChild(probe);
        return { heights, maxH };
    }

    function expandHorizontally(placements, cols) {
        placements.sort((a, b) => a.row - b.row || a.col - b.col);
        for (let i = 0; i < placements.length; i++) {
            const p = placements[i];
            while (p.col + p.colSpan < cols) {
                const nextCol = p.col + p.colSpan;
                let canExpand = true;
                for (let j = 0; j < placements.length; j++) {
                    if (i === j) continue;
                    const o = placements[j];
                    const overlapsNext = o.col === nextCol || (o.col < nextCol && o.col + o.colSpan > nextCol);
                    if (!overlapsNext) continue;
                    if (!(p.row >= o.row + o.rowSpan || p.row + p.rowSpan <= o.row)) {
                        canExpand = false;
                        break;
                    }
                }
                if (canExpand) p.colSpan++;
                else break;
            }
        }
    }

    function captureScrollAnchor() {
        if (window.scrollY <= 50) return null;
        const vh = window.innerHeight;
        const items = pluginList();
        for (let i = 0; i < items.length; i++) {
            const el = items[i];
            const r = el.getBoundingClientRect();
            if (r.bottom > 0 && r.top < vh && r.top > -50) return { el, top: r.top };
        }
        return null;
    }

    function restoreScrollAnchor(anchor) {
        if (!anchor || !document.body.contains(anchor.el)) return;
        const newTop = anchor.el.getBoundingClientRect().top;
        const delta = newTop - anchor.top;
        if (Math.abs(delta) <= 0.5) return;
        const target = Math.max(0, window.scrollY + delta);
        window.scrollTo(0, target);
    }

    function packMasonry() {
        if (document.hidden || expanded) return;
        if (phase === 'packing') return;

        const prevPhase = phase;
        phase = 'packing';

        const anchor = captureScrollAnchor();

        clampSpansToCols();
        const cols = colCount();
        const items = pluginList();
        const style = getComputedStyle(mosaic);
        const gap = parseFloat(style.columnGap || style.gap || '12px') || 12;

        setSpansForCols(items, cols);
        const { heights } = batchMeasureHeights(items);

        const colHeights = new Array(cols).fill(0);
        const placements = [];

        items.forEach((el) => {
            const w = Math.min(parseInt(el.dataset.mosaicSpan || el.dataset.w || '1', 10) || 1, cols);
            const hPx = heights.get(el) || 100;
            const rowSpan = Math.ceil((hPx + gap) / ROW_HEIGHT);

            let bestCol = 0;
            let bestHeight = Infinity;
            for (let c = 0; c <= cols - w; c++) {
                let maxH = 0;
                for (let i = c; i < c + w; i++) if (colHeights[i] > maxH) maxH = colHeights[i];
                if (maxH < bestHeight) { bestHeight = maxH; bestCol = c; }
            }

            const startRow = Math.ceil(bestHeight / ROW_HEIGHT);
            const newHeight = bestHeight + hPx + gap;
            for (let i = bestCol; i < bestCol + w; i++) colHeights[i] = newHeight;

            placements.push({ el, col: bestCol, row: startRow, colSpan: w, rowSpan, height: hPx });
        });


        const spansOut = {};
        placements.forEach((p) => {
            const newCol = `${p.col + 1} / span ${p.colSpan}`;
            const newRowStart = String(p.row + 1);
            const newRowEnd = `span ${p.rowSpan}`;
            const newHeight = `${p.height}px`;

            if (p.el.style.gridColumn !== newCol) p.el.style.gridColumn = newCol;
            if (p.el.style.gridRowStart !== newRowStart) p.el.style.gridRowStart = newRowStart;
            if (p.el.style.gridRowEnd !== newRowEnd) p.el.style.gridRowEnd = newRowEnd;
            if (p.el.style.height !== newHeight) p.el.style.height = newHeight;
            p.el.dataset.currentSpan = String(p.colSpan);

            spansOut[p.el.id] = p.rowSpan;
        });

        storageSet(spansStoreKey, JSON.stringify(spansOut));

        restoreScrollAnchor(anchor);

        phase = prevPhase === 'packing' ? 'idle' : prevPhase;
        if (phase === 'booting') phase = 'idle';
    }

    let packScheduled = false;
    function packAll(force = false) {
        if (document.hidden || expanded) return;
        if (phase === 'scrolling') {
            pendingPack = pendingPack || { force };
            return;
        }
        if (packScheduled || phase === 'packing') return;

        if (force) {
            pluginList().forEach((p) => {
                p.dataset.mosaicDirty = '1';
                p.dataset.mosaicMw = '0';
            });
        }

        packScheduled = true;
        requestAnimationFrame(() => {
            packScheduled = false;
            packMasonry();
        });
    }
    const debouncedPack = debounce(() => packAll(false), 150);

    function fullRepack() {
        invalidatePluginCache();
        cachedCols = 0;
        if (phase === 'scrolling') { pendingPack = { force: true, full: true }; return; }
        clampSpansToCols();
        packMasonry();
    }

    function flushPendingPack() {
        if (!pendingPack || expanded || document.hidden) return;
        const { full } = pendingPack;
        pendingPack = null;
        if (full) fullRepack();
        else packAll();
    }

    on(window, 'scroll', () => {
        if (document.body.classList.contains('scroll-locked')) return;
        phase = 'scrolling';
        clearTimeout(scrollIdleTimer);
        scrollIdleTimer = setTimeout(() => {
            phase = 'idle';
            flushPendingPack();
        }, 220);
    }, { passive: true });

    function lockBodyScroll() {
        if (document.body.classList.contains('scroll-locked')) return;
        _scrollLockY = window.scrollY || 0;
        document.body.style.position = 'fixed';
        document.body.style.top = `-${_scrollLockY}px`;
        document.body.style.left = '0';
        document.body.style.right = '0';
        document.body.style.width = '100%';
        document.body.classList.add('scroll-locked');
    }
    function unlockBodyScroll() {
        if (!document.body.classList.contains('scroll-locked')) return;
        document.body.classList.remove('scroll-locked');
        document.body.style.position = '';
        document.body.style.top = '';
        document.body.style.left = '';
        document.body.style.right = '';
        document.body.style.width = '';
        window.scrollTo(0, _scrollLockY);
    }

    function setWidth(el, w) {
        ensurePluginId(el);

        const cols = colCount();
        const next = clamp(w, 1, Math.min(3, cols));

        el.dataset.w = String(next);
        el.dataset.currentSpan = String(next);
        el.dataset.mosaicSpan = String(next);
        el.dataset.mosaicDirty = '1';
        el.dataset.mosaicMw = '0';

        el.style.height = '';
        el.style.minHeight = '';
        el.style.gridColumn = '';
        el.style.gridRowStart = '';
        el.style.gridRowEnd = '';

        const widths = safeJSON(storageGet(widthsStoreKey)) || {};
        widths[el.id] = next;
        storageSet(widthsStoreKey, JSON.stringify(widths));

        preferredWidths.set(el.id, next);

        cachedCols = 0;
        cachedColsViewport = 0;
        cachedColsMosaic = 0;

        packAll(true);
    }

    function toggleCollapse(el) {
        el.classList.toggle('is-collapsed');
        el.dataset.mosaicDirty = '1';
        packMasonry();
    }

    function ensureOverlay() {
        if (overlay) return overlay;
        overlay = document.createElement('div');
        overlay.className = 'plugin-overlay';
        overlay.style.zIndex = '1000000';
        overlay.addEventListener('click', (e) => { if (e.target === overlay) collapseExpanded(); });
        document.addEventListener('keydown', (e) => { if (e.key === 'Escape') collapseExpanded(); });
        document.body.appendChild(overlay);
        return overlay;
    }

    function expand(el, updateURL = true) {
        if (expanded === el) return collapseExpanded();
        collapseExpanded(false);

        const rect = el.getBoundingClientRect();
        const ph = document.createElement('div');
        ph.className = 'plugin plugin-placeholder';
        ph.dataset.w = el.dataset.w || '1';
        ph.style.gridColumn = el.style.gridColumn;
        ph.style.gridRowStart = el.style.gridRowStart;
        ph.style.gridRowEnd = el.style.gridRowEnd;
        ph.style.height = rect.height + 'px';
        mosaic.insertBefore(ph, el.nextSibling);

        ensureOverlay().classList.add('in');
        lockBodyScroll();

        el.style.height = '';
        el.style.minHeight = '';
        el.style.gridColumn = '';
        el.style.gridRowStart = '';
        el.style.gridRowEnd = '';
        el.classList.add('plugin--expanded');
        overlay.appendChild(el);
        expanded = el;

        if (updateURL) {
            const name = (el.className.match(/([a-z0-9-]+)-section/i) || [, el.id])[1] || el.id;
            history.pushState({ expanded: name }, '', `#${name}`);
        }
    }

    function collapseExpanded(updateURL = true) {
        if (!expanded) {
            ensureOverlay().classList.remove('in');
            unlockBodyScroll();
            return;
        }
        const ph = $('.plugin-placeholder', mosaic);
        expanded.classList.remove('plugin--expanded');
        if (ph && ph.parentNode) ph.parentNode.replaceChild(expanded, ph);
        ensureOverlay().classList.remove('in');
        expanded = null;
        unlockBodyScroll();
        invalidatePluginCache();
        packAll(true);
        if (updateURL) history.pushState({}, '', window.location.pathname);
    }

    const ICONS = { collapse: '▾', 'w-dec': '–', 'w-inc': '+', expand: '⛶' };
    const TITLES = { collapse: 'Collapse', 'w-dec': 'Narrower', 'w-inc': 'Wider', expand: 'Expand' };

    function makeDot(action, title) {
        const b = document.createElement('button');
        b.className = 'icon-btn plugin-btn';
        b.type = 'button';
        b.dataset.action = action;
        b.setAttribute('aria-label', title);
        b.title = title;

        on(b, 'mouseenter', () => { b.textContent = ICONS[action] || ''; });
        on(b, 'mouseleave', () => { b.textContent = ''; });
        ['pointerdown', 'mousedown', 'click'].forEach((ev) => on(b, ev, (e) => e.stopPropagation()));

        on(b, 'click', (e) => {
            ripple(e);
            b.blur();
            handleAction(b.closest('.plugin'), action);
        });
        return b;
    }

    function ensureToolbar(el) {
        if (el.classList.contains('profile-section')) return;

        let titleEl = $('h1,h2,h3,h4', el.querySelector('.plugin__inner'));
        if (!titleEl) {
            titleEl = document.createElement('h3');
            titleEl.className = 'plugin-title';
            titleEl.textContent = (el.className.match(/([a-z0-9-]+)-section/i) || [, 'Block'])[1].replace(/-/g, ' ');
        } else {
            titleEl.classList.add('plugin-title');
        }

        let headerRow = $('.plugin-header', el);
        if (!headerRow) {
            headerRow = document.createElement('div');
            headerRow.className = 'plugin-header';
            headerRow.appendChild(titleEl);
            el.querySelector('.plugin__inner').prepend(headerRow);
        }

        let bar = $('.plugin-toolbar', headerRow);
        if (!bar) {
            bar = document.createElement('div');
            bar.className = 'plugin-toolbar';
            headerRow.appendChild(bar);
            const actions = ['collapse', 'w-dec', 'w-inc', 'expand'];
            actions.forEach((a) => bar.append(makeDot(a, TITLES[a])));
            ['pointerdown', 'mousedown', 'click'].forEach((ev) =>
                bar.addEventListener(ev, (e) => e.stopPropagation()));
        }

        bar.style.display = 'flex';
        bar.style.visibility = 'visible';
        bar.style.opacity = '1';

        headerRow.classList.add('drag-handle');
        headerRow.removeAttribute('draggable');
        headerRow.addEventListener('pointerdown', onHeaderPointerDown, { passive: false });
        headerRow.addEventListener('mousedown', bringToFront);
    }

    function handleAction(el, action) {
        if (!el) return;
        if (action === 'expand' || action === 'view') expand(el);
        else if (action === 'collapse') toggleCollapse(el);
        else if (action === 'w-inc') setWidth(el, (+el.dataset.w || 1) + 1);
        else if (action === 'w-dec') setWidth(el, (+el.dataset.w || 1) - 1);
    }

    function bringToFront(e) {
        const win = e.currentTarget.closest('.plugin');
        if (!win) return;
        pluginList().forEach((p) => p.classList.remove('is-front'));
        win.classList.add('is-front');
    }

    let pointerDown = false, startedDrag = false;
    let dragEl = null, placeholderEl = null, handleEl = null;
    let startX = 0, startY = 0, latestX = 0, latestY = 0;
    let dragOffsetX = 0, dragOffsetY = 0;
    let rafPending = false;
    let latestClientY = 0;
    let autoScrollRAF = null;
    let longPressTimer = null;
    let lastHoverTarget = null, lastSwapAt = 0;
    const DRAG_START_TOL = 6;
    const LONG_PRESS_DELAY = 400;

    function isMobileOrToolbarHidden() {
        if (window.innerWidth <= 780) return true;
        const tb = document.querySelector('.plugin-toolbar');
        return tb && getComputedStyle(tb).display === 'none';
    }

    function onHeaderPointerDown(e) {
        if (e.button !== 0) return;
        if (isInteractive(e.target)) return;

        const win = e.currentTarget.closest('.plugin');
        if (!win) return;

        if (!isMobileOrToolbarHidden()) {
            e.preventDefault();
            pointerDown = true;
            startedDrag = false;
            handleEl = e.currentTarget;
            dragEl = win;
            const rect = dragEl.getBoundingClientRect();
            startX = e.clientX;
            startY = e.clientY;
            latestX = rect.left;
            latestY = rect.top;
            latestClientY = e.clientY;
            document.addEventListener('pointermove', onDocPointerMove, { passive: false });
            document.addEventListener('pointerup', onDocPointerUp, { passive: false });
            document.addEventListener('pointercancel', onDocPointerUp, { passive: false });
            return;
        }

        const lpStartX = e.clientX, lpStartY = e.clientY;
        handleEl = e.currentTarget;
        dragEl = win;

        const teardownLP = () => {
            document.removeEventListener('pointermove', lpMoveCheck);
            document.removeEventListener('pointerup', cancelLP);
            document.removeEventListener('pointercancel', cancelLP);
        };
        const cancelLP = () => {
            if (longPressTimer) { clearTimeout(longPressTimer); longPressTimer = null; }
            teardownLP();
            dragEl = null;
            handleEl = null;
        };
        const lpMoveCheck = (ev) => {
            if (Math.hypot(ev.clientX - lpStartX, ev.clientY - lpStartY) > 10) cancelLP();
        };

        document.addEventListener('pointermove', lpMoveCheck, { passive: true });
        document.addEventListener('pointerup', cancelLP, { once: true });
        document.addEventListener('pointercancel', cancelLP, { once: true });

        longPressTimer = setTimeout(() => {
            longPressTimer = null;
            teardownLP();
            if (!dragEl) return;

            if (navigator.vibrate) navigator.vibrate(50);

            const rect = win.getBoundingClientRect();
            pointerDown = true;
            startedDrag = false;
            startX = lpStartX;
            startY = lpStartY;
            latestX = rect.left;
            latestY = rect.top;
            latestClientY = lpStartY;

            beginDrag({ clientX: lpStartX, clientY: lpStartY });

            document.addEventListener('pointermove', onDocPointerMove, { passive: false });
            document.addEventListener('pointerup', onDocPointerUp, { passive: false });
            document.addEventListener('pointercancel', onDocPointerUp, { passive: false });
        }, LONG_PRESS_DELAY);
    }

    function beginDrag(e) {
        if (startedDrag || !dragEl) return;
        startedDrag = true;

        const rect = dragEl.getBoundingClientRect();

        placeholderEl = document.createElement('div');
        placeholderEl.className = dragEl.className + ' plugin-placeholder';
        placeholderEl.style.height = rect.height + 'px';
        placeholderEl.dataset.w = dragEl.dataset.w || '1';
        placeholderEl.style.gridColumn = dragEl.style.gridColumn;
        placeholderEl.style.gridRowStart = dragEl.style.gridRowStart;
        placeholderEl.style.gridRowEnd = dragEl.style.gridRowEnd;

        mosaic.replaceChild(placeholderEl, dragEl);
        invalidatePluginCache();

        dragOffsetX = e.clientX - rect.left;
        dragOffsetY = e.clientY - rect.top;

        dragEl.classList.add('is-front');
        dragEl.style.position = 'fixed';
        dragEl.style.left = rect.left + 'px';
        dragEl.style.top = rect.top + 'px';
        dragEl.style.width = rect.width + 'px';
        dragEl.style.height = rect.height + 'px';
        dragEl.classList.add('dragging');
        dragEl.style.pointerEvents = 'none';

        document.body.appendChild(dragEl);
        document.body.classList.add('dragging-cursor');

        startAutoScrollLoop();
    }

    function hoverSwapAt(x, y) {
        if (!startedDrag || !placeholderEl) return;
        const under = document.elementFromPoint(x, y);
        const target = under && under.closest('.plugin');
        if (!target || target === dragEl || target === placeholderEl) return;

        const t = now();
        if (target === lastHoverTarget && t - lastSwapAt < 60) return;
        lastHoverTarget = target;
        lastSwapAt = t;

        const a = placeholderEl, b = target;
        if (!a.parentNode || !b.parentNode) return;
        const aNext = a.nextSibling, bNext = b.nextSibling;
        a.parentNode.insertBefore(b, aNext);
        b.parentNode.insertBefore(a, bNext);
        invalidatePluginCache();
    }

    function onDocPointerMove(e) {
        if (!pointerDown) return;
        latestClientY = e.clientY;

        if (!startedDrag) {
            const dx = e.clientX - startX, dy = e.clientY - startY;
            if (Math.hypot(dx, dy) < DRAG_START_TOL) return;
            beginDrag(e);
        }

        e.preventDefault();
        latestX = e.clientX - dragOffsetX;
        latestY = e.clientY - dragOffsetY;

        if (!rafPending) {
            rafPending = true;
            requestAnimationFrame(() => {
                rafPending = false;
                if (dragEl) {
                    dragEl.style.left = latestX + 'px';
                    dragEl.style.top = latestY + 'px';
                }
            });
        }
        hoverSwapAt(e.clientX, e.clientY);
    }

    function onDocPointerUp() {
        document.removeEventListener('pointermove', onDocPointerMove);
        document.removeEventListener('pointerup', onDocPointerUp);
        document.removeEventListener('pointercancel', onDocPointerUp);
        stopAutoScrollLoop();

        if (!startedDrag) {
            pointerDown = false;
            dragEl = null;
            handleEl = null;
            return;
        }

        if (dragEl && placeholderEl && placeholderEl.parentNode === mosaic) {
            mosaic.replaceChild(dragEl, placeholderEl);
            invalidatePluginCache();
        }
        placeholderEl?.remove();
        placeholderEl = null;

        if (dragEl) {
            dragEl.classList.remove('dragging');
            dragEl.style.position = dragEl.style.left = dragEl.style.top = '';
            dragEl.style.width = dragEl.style.height = '';
            dragEl.style.pointerEvents = '';
        }

        pointerDown = false;
        startedDrag = false;
        dragEl = null;
        handleEl = null;
        lastHoverTarget = null;

        document.body.classList.remove('dragging-cursor');

        debouncedPersistOrder();
        packAll(true);
    }

    function startAutoScrollLoop() {
        const EDGE = 28;
        const MAX_STEP = 24;
        const step = (d) => clamp(Math.floor(d * 1.2), 6, MAX_STEP);
        const loop = () => {
            if (!startedDrag) { autoScrollRAF = null; return; }
            autoScrollRAF = requestAnimationFrame(loop);
            const toTop = latestClientY;
            const toBottom = window.innerHeight - latestClientY;
            if (toTop < EDGE) window.scrollBy(0, -step(EDGE - toTop));
            else if (toBottom < EDGE) window.scrollBy(0, step(EDGE - toBottom));
        };
        if (!autoScrollRAF) autoScrollRAF = requestAnimationFrame(loop);
    }
    function stopAutoScrollLoop() {
        if (autoScrollRAF) { cancelAnimationFrame(autoScrollRAF); autoScrollRAF = null; }
    }

    function ripple(e) {
        const el = e.currentTarget;
        el.classList.add('ripple-host');
        const r = document.createElement('span');
        r.className = 'ripple';
        const rect = el.getBoundingClientRect();
        const d = Math.max(rect.width, rect.height);
        r.style.width = r.style.height = d + 'px';
        r.style.left = (e.clientX - rect.left - d / 2) + 'px';
        r.style.top = (e.clientY - rect.top - d / 2) + 'px';
        el.appendChild(r);
        setTimeout(() => r.remove(), 600);
    }

    pluginList().forEach((el) => ensureToolbar(el));

    const lastObservedHeights = new WeakMap();
    let pendingHeightUpdate = false;

    function markDirtyAndPack(plugin) {
        if (!plugin || plugin.dataset.fixedTail === '1') return;
        plugin.dataset.mosaicDirty = '1';
        if (phase === 'booting' || expanded || phase === 'packing') return;
        if (phase === 'scrolling') {
            pendingPack = pendingPack || { force: false };
            return;
        }
        debouncedPack();
    }

    const ro = new ResizeObserver((entries) => {
        if (phase === 'booting' || expanded || phase === 'packing') return;

        const changed = [];
        for (const entry of entries) {
            const target = entry.target;
            if (!document.body.contains(target)) { ro.unobserve(target); continue; }

            const plugin = target.closest('.plugin');
            if (!plugin) { ro.unobserve(target); continue; }

            const newHeight = entry.contentRect.height;
            const lastHeight = lastObservedHeights.get(target);

            if (lastHeight === undefined) {
                lastObservedHeights.set(target, newHeight);
                continue;
            }
            if (Math.abs(newHeight - lastHeight) > 8) {
                changed.push(plugin);
                lastObservedHeights.set(target, newHeight);
            }
        }

        if (!changed.length || pendingHeightUpdate) return;
        pendingHeightUpdate = true;
        requestAnimationFrame(() => {
            pendingHeightUpdate = false;
            changed.forEach((p) => markDirtyAndPack(p));
        });
    });

    function observeElement(el) {
        if (!el) return;
        ro.observe(innerOf(el), { box: 'border-box' });
    }
    pluginList().forEach((n) => observeElement(n));

    document.addEventListener('load', (e) => {
        const t = e.target;
        if (!(t instanceof HTMLImageElement)) return;
        const plugin = t.closest ? t.closest('.plugin') : null;
        if (!plugin) return;
        markDirtyAndPack(plugin);
    }, true);

    document.addEventListener('error', (e) => {
        const t = e.target;
        if (!(t instanceof HTMLImageElement)) return;
        const plugin = t.closest ? t.closest('.plugin') : null;
        if (!plugin) return;
        markDirtyAndPack(plugin);
    }, true);

    function notifyContentChanged(elOrSelector) {
        let el = elOrSelector;
        if (typeof elOrSelector === 'string') {
            el = document.querySelector(elOrSelector);
        }
        if (!el) return;

        if (!el.classList.contains('plugin')) {
            el = el.closest('.plugin');
        }
        if (!el) return;

        markDirtyAndPack(el);
    }

    function notifyPluginAdded(el) {
        if (!el || !el.classList.contains('plugin')) return;
        invalidatePluginCache();
        observeElement(el);
        ensureToolbar(el);
        markDirtyAndPack(el);
    }

    function notifyPluginReplaced(oldEl, newEl) {
        if (oldEl) {
            const oldInner = innerOf(oldEl);
            ro.unobserve(oldInner);
            lastObservedHeights.delete(oldInner);
        }
        notifyPluginAdded(newEl);
    }

    on(window, 'resize', throttle(() => {
        if (phase === 'packing') return;

        const nextViewportWidth = window.innerWidth;
        const nextMosaicWidth = mosaic.clientWidth;
        cachedCols = 0;
        const nextCols = colCount();

        const widthChanged =
            Math.abs(nextViewportWidth - lastViewportWidth) > 1 ||
            Math.abs(nextMosaicWidth - lastMosaicWidth) > 1;

        if (!widthChanged) return;

        pluginList().forEach((el) => { el.dataset.mosaicDirty = '1'; });

        clampSpansToCols();
        if (nextCols > 1) restorePreferredWidths(nextCols);
        lastViewportWidth = nextViewportWidth;
        lastMosaicWidth = nextMosaicWidth;

        clampSpansToCols();
        packMasonry();
    }, 200));

    on(document, 'visibilitychange', () => {
        if (!document.hidden && phase === 'idle') packAll(false);
    });

    function handleHashChange() {
        const hash = window.location.hash.slice(1);
        if (hash) {
            const el = $(`.${hash}-section.plugin`);
            if (el && !expanded) setTimeout(() => expand(el, false), 100);
        } else if (expanded) {
            collapseExpanded(false);
        }
    }
    on(window, 'hashchange', handleHashChange);
    on(window, 'popstate', (e) => {
        if (e.state && e.state.expanded) {
            const el = $(`.${e.state.expanded}-section.plugin`);
            if (el) expand(el, false);
        } else if (expanded) {
            collapseExpanded(false);
        }
    });

    on(document, 'keydown', (e) => {
        if (e.altKey && e.key >= '1' && e.key <= '9') {
            e.preventDefault();
            const idx = parseInt(e.key, 10) - 1;
            const items = pluginList();
            if (items[idx]) {
                items[idx].scrollIntoView({ behavior: 'smooth', block: 'center' });
                setTimeout(() => expand(items[idx]), 300);
            }
        }
    });

    function ready(fn) {
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', fn, { once: true });
        } else {
            fn();
        }
    }

    function finishBoot() {
        document.documentElement.classList.remove('js-loading');
        document.documentElement.classList.add('js-loaded');
    }

    (function initialLayout() {
        const layoutVersionKey = 'mosaic.layout.version';
        const layoutVersion = '2026-07-12-skebob'

        if (storageGet(layoutVersionKey) !== layoutVersion) {
            try {
                localStorage.removeItem('mosaic.spans');
                localStorage.removeItem('mosaic.order');
                localStorage.removeItem('mosaic.widths');
                localStorage.setItem(layoutVersionKey, layoutVersion);
            } catch {}
        }

        const finalizeFromPrebake = () => {
            ensureAllPluginIds();

            const items = pluginList();

            const snapshot = items.map((el) => {
                const cs = getComputedStyle(el);
                const computedHeight = parseFloat(cs.height) || 0;
                const naturalHeight = el.scrollHeight || 0;
                const h = Math.max(computedHeight, naturalHeight, 120);

                return {
                    el,
                    gridColumn: cs.gridColumn,
                    gridRowStart: cs.gridRowStart,
                    gridRowEnd: cs.gridRowEnd,
                    height: `${Math.ceil(h)}px`,
                    minHeight: `${Math.ceil(h)}px`,
                };
            });

            mosaic.classList.remove('mosaic-prebaked');

            snapshot.forEach((s) => {
                s.el.style.gridColumn = s.gridColumn;
                s.el.style.gridRowStart = s.gridRowStart;
                s.el.style.gridRowEnd = s.gridRowEnd;
                s.el.style.height = s.height;
                s.el.style.minHeight = s.minHeight;

                s.el.dataset.mosaicDirty = '0';
                s.el.dataset.mosaicMw = '0';

                const m = s.gridColumn.match(/span\s+(\d+)/i);
                if (m) {
                    s.el.dataset.currentSpan = String(parseInt(m[1], 10));
                    s.el.dataset.mosaicSpan = String(parseInt(m[1], 10));
                }
            });

            applySavedOrder();
            ensurePluginIdsAfterReorder();
            clampSpansToCols();

            phase = 'idle';
            finishBoot();

            requestAnimationFrame(() => {
                pluginList().forEach((el) => {
                    el.dataset.mosaicDirty = '1';
                    el.dataset.mosaicMw = '0';
                });

                packAll(true);
            });

            if (window.location.hash) {
                setTimeout(handleHashChange, 50);
            }
        };

        const finalizeClassic = () => {
            applySavedOrder();
            ensurePluginIdsAfterReorder();
            restorePreferredWidths(colCount());
            clampSpansToCols();
            packMasonry();

            phase = 'idle';
            finishBoot();

            if (window.location.hash) {
                setTimeout(handleHashChange, 50);
            }
        };

        function ensurePluginIdsAfterReorder() {
            invalidatePluginCache();
            ensureAllPluginIds();
        }

        if (hasPrebakeAtBoot) {
            if (document.readyState === 'loading') {
                document.addEventListener('DOMContentLoaded', () => {
                    requestAnimationFrame(finalizeFromPrebake);
                }, { once: true });
            } else {
                requestAnimationFrame(finalizeFromPrebake);
            }
        } else {
            if (document.readyState === 'loading') {
                document.addEventListener('DOMContentLoaded', () => {
                    requestAnimationFrame(finalizeClassic);
                }, { once: true });
            } else {
                requestAnimationFrame(finalizeClassic);
            }
        }
    })();

    if ('fonts' in document && document.fonts.ready) {
        document.fonts.ready.then(() => {
            if (phase === 'idle') {
                pluginList().forEach((el) => { el.dataset.mosaicDirty = '1'; });
                packAll(false);
            }
        });
    }

    window.mosaicUtils = {
        resizeAll: () => packAll(true),
        fullRepack,
        expand,
        collapseExpanded,
        getMosaic: () => mosaic,
        observe: observeElement,
        notifyContentChanged,
        notifyPluginAdded,
        notifyPluginReplaced
    };
})();