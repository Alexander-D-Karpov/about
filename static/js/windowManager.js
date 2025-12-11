(function(){
    'use strict';

    document.documentElement.classList.add('js-loading');
    document.documentElement.classList.add('js');

    const $ = (q, c = document) => c ? c.querySelector(q) : null;
    const $$ = (q, c = document) => c ? Array.from(c.querySelectorAll(q)) : [];
    const on = (el, ev, fn, opts) => el && el.addEventListener(ev, fn, opts);
    const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
    const now = () => Date.now();

    const throttle = (fn, ms = 100) => {
        let t = 0, to, last;
        return (...args) => {
            const n = now();
            if (n - t > ms) { t = n; fn(...args); } else {
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

    const toMove = [...root.children].filter(el => el !== mosaic);

    toMove.forEach(el => {
        el.classList.add('plugin');
        if (!el.querySelector('.plugin__inner')) {
            const inner = document.createElement('div');
            inner.className = 'plugin__inner';
            while (el.firstChild) inner.appendChild(el.firstChild);
            el.appendChild(inner);
        }
        if (!el.id) {
            const guess = (el.className.match(/([a-z0-9-]+)-section/i) || [, 'tile'])[1];
            el.id = `${guess}-${Math.random().toString(36).slice(2, 7)}`;
        }
        el.style.gridColumn = el.style.gridRow = '';
        mosaic.appendChild(el);
    });

    const defaultWidths = {
        'projects-section': 3,
        'beatleader-section': 2,
        'steam-section': 2,
        'neofetch-section': 2,
        'tech-section': 1,
        'code-section': 2,
        'meme-section': 1,
        'lastfm-section': 2,
        'webring-section': 2,
        'social-section': 1,
        'visitors-section': 1,
        'info-section': 2,
        'services-section': 2,
        'places-section': 2,
        'profile-section': 2,
        'health-section': 1,
        'personal-section': 2,
    };

    const preferredWidths = new Map();
    const spansStoreKey = 'mosaic.spans';
    const widthsStoreKey = 'mosaic.widths';
    const orderStoreKey  = 'mosaic.order';
    const pinnedStoreKey = 'mosaic.pinned';

    const savedSpans  = safeJSON(localStorage.getItem(spansStoreKey)) || {};
    const savedWidths = safeJSON(localStorage.getItem(widthsStoreKey)) || {};
    const savedOrder  = safeJSON(localStorage.getItem(orderStoreKey))  || [];
    const savedPinned = safeJSON(localStorage.getItem(pinnedStoreKey)) || {};

    function safeJSON(s){ try { return JSON.parse(s); } catch { return null; } }

    $$('.plugin', mosaic).forEach(el => {
        const key = Object.keys(defaultWidths).find(k => el.classList.contains(k));
        const w = clamp(+el.dataset.w || savedWidths[el.id] || (key ? defaultWidths[key] : 1), 1, 3);
        el.dataset.w = String(w);
        preferredWidths.set(el.id, w);
        if (savedPinned[el.id]) el.dataset.pinned = '1';
        if (el.classList.contains('profile-section')) el.dataset.pinned = '1';
    });

    let _scrollLockY = 0;

    function lockBodyScroll(){
        if (document.body.classList.contains('scroll-locked')) return;
        _scrollLockY = window.scrollY || document.documentElement.scrollTop || 0;
        document.body.style.position = 'fixed';
        document.body.style.top = `-${_scrollLockY}px`;
        document.body.style.left = '0';
        document.body.style.right = '0';
        document.body.style.width = '100%';
        document.body.classList.add('scroll-locked');
    }

    function unlockBodyScroll(){
        if (!document.body.classList.contains('scroll-locked')) return;
        document.body.classList.remove('scroll-locked');
        document.body.style.position = '';
        document.body.style.top = '';
        document.body.style.left = '';
        document.body.style.right = '';
        document.body.style.width = '';
        window.scrollTo(0, _scrollLockY);
    }

    const MIN_COL_FALLBACK = 280;
    const ROW_HEIGHT = 1;
    const VERTICAL_GAP = 12;

    function getContentHeight(el) {
        el.style.height = 'auto';
        el.style.gridRowEnd = 'auto';

        const rect = el.getBoundingClientRect();
        return Math.ceil(rect.height);
    }

    function colCount(){
        const style = getComputedStyle(mosaic);
        const gapStr = style.columnGap || style.gap || '12px';
        const gap = parseFloat(gapStr.split(/\s+/)[0]) || 12;

        const minColRoot = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--col-min')) || 0;
        const minColMosaic = parseFloat(getComputedStyle(mosaic).getPropertyValue('--col-min')) || 0;
        const minCol = minColRoot || minColMosaic || MIN_COL_FALLBACK;

        const w = mosaic.clientWidth;
        if (w <= 0) return 1;

        return Math.max(1, Math.floor((w + gap) / (minCol + gap)));
    }

    function clampSpansToCols(){
        const cols = colCount();
        $$('.plugin', mosaic).forEach(el => {
            let w = clamp(+el.dataset.w || 1, 1, 3);
            if (w > cols) w = cols;
            el.dataset.w = String(w);
        });
    }

    function packMasonry(animate = true) {
        clampSpansToCols();
        const cols = colCount();
        const items = $$('.plugin', mosaic);

        const style = getComputedStyle(mosaic);
        const gap = parseFloat(style.columnGap || style.gap || '12px') || 12;

        items.forEach(el => {
            el.style.position = '';
            el.style.top = '';
            el.style.left = '';
            el.style.width = '';
            el.style.height = 'auto';
            el.style.gridRowEnd = 'auto';
        });

        void mosaic.offsetHeight;

        const heights = new Map();
        items.forEach(el => {
            const h = getContentHeight(el);
            heights.set(el, h);
        });

        const colHeights = new Array(cols).fill(0);

        const spansOut = {};

        items.forEach(el => {
            const w = Math.min(+el.dataset.w || 1, cols);
            const hPx = heights.get(el) || 100;

            let bestCol = 0;
            let bestHeight = Infinity;

            for (let c = 0; c <= cols - w; c++) {
                const maxH = Math.max(...colHeights.slice(c, c + w));
                if (maxH < bestHeight) {
                    bestHeight = maxH;
                    bestCol = c;
                }
            }

            const span = Math.ceil(hPx / ROW_HEIGHT) + Math.ceil(gap / ROW_HEIGHT);

            el.style.gridColumn = `${bestCol + 1} / span ${w}`;
            el.style.gridRowStart = String(Math.ceil(bestHeight / ROW_HEIGHT) + 1);
            el.style.gridRowEnd = `span ${span}`;

            const newHeight = bestHeight + hPx + gap;
            for (let i = bestCol; i < bestCol + w; i++) {
                colHeights[i] = newHeight;
            }

            spansOut[el.id] = span;
        });

        localStorage.setItem(spansStoreKey, JSON.stringify(spansOut));
    }

    function quickPackFromSaved() {
        packMasonry(false);
    }

    let packScheduled = false;
    let packAnimated = true;

    function packAll(animate = true) {
        if (document.hidden) return;
        if (packScheduled) return;

        packScheduled = true;
        packAnimated = animate;

        requestAnimationFrame(() => {
            packScheduled = false;
            packMasonry(packAnimated);
            fitNeofetch();
        });
    }

    const debouncedPack = debounce(() => packAll(true), 100);

    function settlePasses(){
        setTimeout(() => packAll(false), 100);
        setTimeout(() => packAll(false), 300);
        setTimeout(() => packAll(false), 600);
    }

    function fullRepack(){
        clampSpansToCols();
        packMasonry(true);
        fitNeofetch();
        settlePasses();
    }

    function getProjectPlugins(m) {
        return Array.from(m.querySelectorAll('.projects-section, .projects-section.plugin'))
            .map(n => n.closest('.plugin') || n)
            .filter(Boolean);
    }

    function getWebringPlugins(m) {
        return Array.from(m.querySelectorAll('.webring-section, .webring-section.plugin'))
            .map(n => n.closest('.plugin') || n)
            .filter(Boolean);
    }

    function ensureWebringFirst(m) {
        const list = getWebringPlugins(m);
        list.forEach(n => {
            if (n.parentElement === m && n !== m.firstElementChild) {
                m.insertBefore(n, m.firstElementChild);
            }
        });
    }

    function detachProjects(m) {
        const list = getProjectPlugins(m);
        list.forEach(n => n.remove());
        return list;
    }

    function ensureProjectsLast(m) {
        const list = getProjectPlugins(m);
        list.forEach(n => {
            if (n.parentElement === m && n !== m.lastElementChild) {
                m.appendChild(n);
            }
        });
    }

    function ensurePluginOrdering(m) {
        ensureWebringFirst(m);
        ensureProjectsLast(m);
    }

    (function initialLayout(){
        if (savedOrder.length){
            const byId = Object.fromEntries($$('.plugin', mosaic).map(n => [n.id, n]));
            savedOrder.forEach(id => byId[id] && mosaic.appendChild(byId[id]));
        } else {
            ensurePluginOrdering(mosaic);
        }

        const deferredProjects = detachProjects(mosaic);

        quickPackFromSaved();

        setTimeout(() => {
            deferredProjects.forEach(n => mosaic.appendChild(n));
            packMasonry(false);
            settlePasses();
        }, 50);

        window.addEventListener('load', () => {
            setTimeout(() => packAll(false), 100);
            setTimeout(() => packAll(false), 500);
            setTimeout(() => packAll(false), 1000);
        });
    })();

    function setWidth(el, w){
        w = clamp(w, 1, 3);
        el.dataset.w = String(w);
        const widths = safeJSON(localStorage.getItem(widthsStoreKey)) || {};
        widths[el.id] = w;
        localStorage.setItem(widthsStoreKey, JSON.stringify(widths));
        preferredWidths.set(el.id, w);
        packAll(true);
    }

    function toggleCollapse(el){
        el.classList.toggle('is-collapsed');
        setTimeout(() => packAll(true), 50);
    }

    function pin(el){
        el.dataset.pinned = el.dataset.pinned === '1' ? '0' : '1';
        const map = safeJSON(localStorage.getItem(pinnedStoreKey)) || {};
        map[el.id] = el.dataset.pinned === '1';
        localStorage.setItem(pinnedStoreKey, JSON.stringify(map));

        const items = $$('.plugin', mosaic);
        const pinned = items.filter(n => n.dataset.pinned === '1');
        const unpinned = items.filter(n => n.dataset.pinned !== '1');
        [...pinned, ...unpinned].forEach(n => mosaic.appendChild(n));

        toast(el.dataset.pinned === '1' ? 'Pinned' : 'Unpinned');
        persistOrder();
        packAll(true);
    }

    const ICONS = {collapse: '▾', 'w-dec': '–', 'w-inc': '+', expand: '⛶'};
    const TITLES = { collapse:'Collapse', 'w-dec':'Narrower', 'w-inc':'Wider', expand:'Expand' };

    function makeDot(action, title){
        const b = document.createElement('button');
        b.className = 'icon-btn plugin-btn';
        b.type = 'button';
        b.dataset.action = action;
        b.setAttribute('aria-label', title);
        b.title = title;

        on(b, 'mouseenter', () => { b.textContent = ICONS[action] || ''; });
        on(b, 'mouseleave', () => { b.textContent = ''; });

        ['pointerdown', 'mousedown', 'click'].forEach(ev => {
            on(b, ev, (e) => { e.stopPropagation(); });
        });

        on(b, 'click', (e) => {
            ripple(e);
            b.blur();
            handleAction(b.closest('.plugin'), action);
        });

        return b;
    }

    function ensureToolbar(el){
        if (el.classList.contains('profile-section')) return;

        let titleEl = $('h1,h2,h3,h4', el.querySelector('.plugin__inner'));
        if (!titleEl){
            titleEl = document.createElement('h3');
            titleEl.className = 'plugin-title';
            titleEl.textContent = (el.className.match(/([a-z0-9-]+)-section/i) || [, 'Block'])[1].replace(/-/g, ' ');
        } else {
            titleEl.classList.add('plugin-title');
        }

        let headerRow = $('.plugin-header', el);
        if (!headerRow){
            headerRow = document.createElement('div');
            headerRow.className = 'plugin-header';
            headerRow.appendChild(titleEl);
            el.querySelector('.plugin__inner').prepend(headerRow);
        }

        let bar = $('.plugin-toolbar', headerRow);
        if (!bar){
            bar = document.createElement('div');
            bar.className = 'plugin-toolbar';
            headerRow.appendChild(bar);
            ['collapse', 'w-dec', 'w-inc', 'expand'].forEach(action => {
                bar.append(makeDot(action, TITLES[action]));
            });
            ['pointerdown', 'mousedown', 'click'].forEach(ev =>
                bar.addEventListener(ev, e => e.stopPropagation())
            );
        }

        headerRow.classList.add('drag-handle');
        headerRow.removeAttribute('draggable');
        headerRow.addEventListener('pointerdown', onHeaderPointerDown, {passive: false});
        headerRow.addEventListener('mousedown', bringToFront);
    }

    $$('.plugin', mosaic).forEach(el => ensureToolbar(el));

    function handleAction(el, action){
        if (!el) return;
        if (action === 'expand' || action === 'view') expand(el);
        if (action === 'collapse') toggleCollapse(el);
        if (action === 'pin')      pin(el);
        if (action === 'w-inc')    setWidth(el, (+el.dataset.w || 1) + 1);
        if (action === 'w-dec')    setWidth(el, (+el.dataset.w || 1) - 1);
    }

    function persistOrder(){
        const order = $$('.plugin', mosaic).map(n => n.id);
        localStorage.setItem(orderStoreKey, JSON.stringify(order));
    }

    let overlay, expanded = null;

    function ensureOverlay(){
        if (overlay) return overlay;
        overlay = document.createElement('div');
        overlay.className = 'plugin-overlay';
        overlay.style.zIndex = '99999';
        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) collapseExpanded();
        });
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') collapseExpanded();
        });
        document.body.appendChild(overlay);
        return overlay;
    }

    function expand(el, updateURL = true){
        if (expanded === el) return collapseExpanded();

        collapseExpanded(false);

        const ph = document.createElement('div');
        ph.className = 'plugin plugin-placeholder';
        ph.dataset.w = el.dataset.w || '1';
        ph.style.gridColumn = el.style.gridColumn;
        ph.style.gridRowStart = el.style.gridRowStart;
        ph.style.gridRowEnd = el.style.gridRowEnd;
        mosaic.insertBefore(ph, el.nextSibling);

        ensureOverlay().classList.add('in');
        lockBodyScroll();

        el.classList.add('plugin--expanded');
        overlay.appendChild(el);
        expanded = el;

        if (updateURL){
            const pluginName = (el.className.match(/([a-z0-9-]+)-section/i) || [, el.id])[1] || el.id;
            history.pushState({ expanded: pluginName }, '', `#${pluginName}`);
        }
    }

    function collapseExpanded(updateURL = true){
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

        packAll(true);
        if (updateURL) history.pushState({}, '', window.location.pathname);
    }

    let pointerDown = false, startedDrag = false;
    let dragEl = null, placeholderEl = null, handleEl = null;
    let startX = 0, startY = 0, latestX = 0, latestY = 0;
    let dragOffsetX = 0, dragOffsetY = 0;
    let rafPending = false;
    let latestClientY = 0;
    let maxZ = 1000;
    let autoScrollRAF = null;
    const DRAG_START_TOL = 6;
    let lastHoverTarget = null, lastSwapAt = 0;

    function bringToFront(e){
        const win = e.currentTarget.closest('.plugin');
        if (!win) return;
        maxZ += 1;
        win.style.zIndex = String(maxZ);
    }

    function isMobileOrToolbarHidden() {
        if (window.innerWidth <= 780) return true;

        const toolbar = document.querySelector('.plugin-toolbar');
        if (toolbar) {
            const style = getComputedStyle(toolbar);
            if (style.display === 'none') return true;
        }
        return false;
    }

    function onHeaderPointerDown(e){
        if (e.button !== 0) return;
        if (isInteractive(e.target)) return;

        if (isMobileOrToolbarHidden()) return;

        const win = e.currentTarget.closest('.plugin');
        if (!win) return;
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

        document.addEventListener('pointermove', onDocPointerMove, {passive: false});
        document.addEventListener('pointerup', onDocPointerUp, {passive: false});
        document.addEventListener('pointercancel', onDocPointerUp, {passive: false});
    }

    function beginDrag(e){
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

        dragOffsetX = e.clientX - rect.left;
        dragOffsetY = e.clientY - rect.top;

        bringToFront({ currentTarget: handleEl });

        dragEl.style.position = 'fixed';
        dragEl.style.left = rect.left + 'px';
        dragEl.style.top  = rect.top  + 'px';
        dragEl.style.width  = rect.width  + 'px';
        dragEl.style.height = rect.height + 'px';
        dragEl.classList.add('dragging');
        dragEl.style.pointerEvents = 'none';

        document.body.appendChild(dragEl);
        document.body.classList.add('dragging-cursor');

        startAutoScrollLoop();
    }

    function hoverSwapAt(clientX, clientY){
        if (!startedDrag || !placeholderEl) return;

        const under = document.elementFromPoint(clientX, clientY);
        let target = under && under.closest('.plugin');
        if (!target || target === dragEl || target === placeholderEl) return;

        const t = now();
        if (target === lastHoverTarget && (t - lastSwapAt) < 60) return;
        lastHoverTarget = target;
        lastSwapAt = t;

        swapNodes(placeholderEl, target);
    }

    function swapNodes(a, b){
        if (!a || !b || !a.parentNode || !b.parentNode) return;
        const aNext = a.nextSibling;
        const bNext = b.nextSibling;
        const aParent = a.parentNode;
        const bParent = b.parentNode;
        aParent.insertBefore(b, aNext);
        bParent.insertBefore(a, bNext);
    }

    function onDocPointerMove(e){
        if (!pointerDown) return;
        latestClientY = e.clientY;

        if (!startedDrag){
            const dx = e.clientX - startX;
            const dy = e.clientY - startY;
            if (Math.hypot(dx, dy) < DRAG_START_TOL) return;
            beginDrag(e);
        }

        e.preventDefault();

        latestX = e.clientX - dragOffsetX;
        latestY = e.clientY - dragOffsetY;

        if (!rafPending){
            rafPending = true;
            requestAnimationFrame(() => {
                rafPending = false;
                if (dragEl){
                    dragEl.style.left = latestX + 'px';
                    dragEl.style.top  = latestY + 'px';
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

        if (!startedDrag){
            pointerDown = false;
            dragEl = null;
            handleEl = null;
            return;
        }

        if (dragEl && placeholderEl && placeholderEl.parentNode === mosaic){
            mosaic.replaceChild(dragEl, placeholderEl);
        }
        placeholderEl?.remove();
        placeholderEl = null;

        if (dragEl){
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

        persistOrder();
        packAll(true);
    }

    function startAutoScrollLoop(){
        const EDGE = 28;
        const MAX_STEP = 24;
        const step = (delta) => clamp(Math.floor(delta * 1.2), 6, MAX_STEP);

        function loop(){
            if (!startedDrag) return (autoScrollRAF = null);
            autoScrollRAF = requestAnimationFrame(loop);

            const toTop = latestClientY;
            const toBottom = window.innerHeight - latestClientY;

            if (toTop < EDGE){
                window.scrollBy(0, -step(EDGE - toTop));
            } else if (toBottom < EDGE){
                window.scrollBy(0, step(EDGE - toBottom));
            }
        }

        if (!autoScrollRAF) autoScrollRAF = requestAnimationFrame(loop);
    }

    function stopAutoScrollLoop(){
        if (autoScrollRAF) {
            cancelAnimationFrame(autoScrollRAF);
            autoScrollRAF = null;
        }
    }

    function ripple(e){
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

    let toastRoot;
    function toast(msg){
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

    const io = new IntersectionObserver(entries => {
        entries.forEach(e => e.target.classList.toggle('reveal', e.isIntersecting));
    }, { threshold: 0.08 });
    $$('.plugin', mosaic).forEach(el => io.observe(el));

    if ('fonts' in document && document.fonts.ready) {
        document.fonts.ready.then(() => {
            packAll(false);
        });
    }

    let resizeTimeout = null;
    const ro = new ResizeObserver((entries) => {
        clearTimeout(resizeTimeout);
        resizeTimeout = setTimeout(() => {
            packAll(false);
        }, 50);
    });

    $$('.plugin', mosaic).forEach(n => ro.observe(n, { box: 'border-box' }));

    const mo = new MutationObserver(debounce(() => {
        packAll(false);
    }, 150));

    mo.observe(mosaic, {
        childList: true,
        subtree: true,
        attributes: true,
        attributeFilter: ['src', 'srcset', 'class']
    });

    function restorePreferredWidths(maxCols = colCount()){
        $$('.plugin', mosaic).forEach(el => {
            const pref = preferredWidths.get(el.id);
            if (pref != null) el.dataset.w = String(clamp(pref, 1, Math.min(3, maxCols)));
        });
    }

    let lastCols = colCount();
    on(window, 'resize', throttle(() => {
        const nextCols = colCount();
        clampSpansToCols();
        if (nextCols > lastCols) restorePreferredWidths(nextCols);
        lastCols = nextCols;
        fullRepack();
    }, 200));

    on(document, 'visibilitychange', () => {
        if (!document.hidden) {
            packAll(false);
        }
    });

    let hashNavigationInitialized = false;

    function handleHashChange(){
        const hash = window.location.hash.slice(1);
        if (hash){
            const el = $(`.${hash}-section.plugin`);
            if (el && !expanded) setTimeout(() => expand(el, false), 100);
        } else if (expanded) {
            collapseExpanded(false);
        }
    }

    function initHashNavigation() {
        if (hashNavigationInitialized) return;
        hashNavigationInitialized = true;

        on(window, 'hashchange', handleHashChange, { once: false });
        on(window, 'popstate', (e) => {
            if (e.state && e.state.expanded){
                const el = $(`.${e.state.expanded}-section.plugin`);
                if (el) expand(el, false);
            } else if (expanded) {
                collapseExpanded(false);
            }
        }, { once: false });

        if (window.location.hash) setTimeout(handleHashChange, 400);
    }

    let windowManagerInitialized = false;

    function initWindowManager() {
        if (windowManagerInitialized) return;
        windowManagerInitialized = true;

        initHashNavigation();

        if ('fonts' in document && document.fonts.ready) {
            document.fonts.ready.then(() => {
                packAll(false);
            });
        }

        on(document, 'keydown', (e) => {
            if (e.altKey && e.key >= '1' && e.key <= '9'){
                e.preventDefault();
                const idx = parseInt(e.key, 10) - 1;
                const plugins = $$('.plugin', mosaic);
                if (plugins[idx]){
                    plugins[idx].scrollIntoView({behavior: 'smooth', block: 'center'});
                    setTimeout(() => expand(plugins[idx]), 300);
                }
            }
        }, { once: false });

        window.mosaicUtils = {
            resizeAll: () => packAll(false),
            fullRepack,
            expand,
            collapseExpanded,
            getMosaic: () => mosaic
        };

        setTimeout(() => {
            document.documentElement.classList.remove('js-loading');
            document.documentElement.classList.add('js-loaded');
        }, 100);
    }

    initWindowManager();

    function fitNeofetch(){
        document.querySelectorAll('.neofetch-section .terminal').forEach(term => {
            const pre = term.querySelector('.neofetch-pre');
            if (!pre) return;
            const style = getComputedStyle(term);
            const scale = parseFloat(style.getPropertyValue('--neo-scale')) || 1;
            const headerH = (term.querySelector('.terminal-header')?.offsetHeight || 0);
            const paletteH = (term.querySelector('.color-palette')?.offsetHeight || 0);
            const bodyPadding = 20;
            const scaledPreH = Math.ceil(pre.scrollHeight * scale);
            term.style.height = (headerH + bodyPadding + scaledPreH + paletteH + 8) + 'px';
        });
    }

    document.querySelectorAll('.section-toggle').forEach(toggle => {
        toggle.addEventListener('click', () => {
            setTimeout(() => packAll(false), 100);
        });
    });

    document.querySelectorAll('img').forEach(img => {
        if (img.complete) return;
        img.addEventListener('load', () => debouncedPack(), {once: true});
        img.addEventListener('error', () => debouncedPack(), {once: true});
    });

})();