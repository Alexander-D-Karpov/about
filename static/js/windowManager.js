(function(){
    'use strict';

    // Ensure CSS rules gated by ".js" apply (grid layout next to profile)
    document.documentElement.classList.add('js');

    // --- tiny helpers ---
    const $  = (q, c=document) => c.querySelector(q);
    const $$ = (q, c=document) => Array.from(c.querySelectorAll(q));
    const on = (el, ev, fn, opts) => el && el.addEventListener(ev, fn, opts);
    const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
    const now = () => Date.now();

    const throttle = (fn, ms=100) => {
        let t = 0, to, last;
        return (...args) => {
            const n = now();
            if (n - t > ms) { t = n; fn(...args); }
            else { last = args; clearTimeout(to); to = setTimeout(() => { t = now(); fn(...(last||[])); }, ms); }
        };
    };

    const isInteractive = (node) => !!node.closest('button, a, input, select, textarea, [contenteditable], .plugin-btn');

    // --- bootstrapping containers ---
    const root = $('.container');
    if (!root) return;

    // Ensure profile section stays outside the mosaic and first
    let profile = $('.profile-section', root);

    // If no mosaic in DOM yet, create one and place it right after profile (when present)
    let mosaic = $('.mosaic', root);
    if (!mosaic) {
        mosaic = document.createElement('section');
        mosaic.className = 'mosaic';
        if (profile && profile.parentNode === root) {
            if (profile.nextSibling) root.insertBefore(mosaic, profile.nextSibling);
            else root.appendChild(mosaic);
        } else {
            root.appendChild(mosaic);
        }
    }

    // Move every child except profile + mosaic into the mosaic (these are plugin windows)
    const toMove = [...root.children].filter(el => el !== mosaic && el !== profile);
    toMove.forEach(el => {
        el.classList.add('plugin');
        if (!el.querySelector('.plugin__inner')) {
            const inner = document.createElement('div');
            inner.className = 'plugin__inner';
            while (el.firstChild) inner.appendChild(el.firstChild);
            el.appendChild(inner);
        }
        if (!el.id) {
            const guess = (el.className.match(/([a-z0-9-]+)-section/i) || [,'tile'])[1];
            el.id = `${guess}-${Math.random().toString(36).slice(2,7)}`;
        }
        el.style.gridColumn = el.style.gridRow = '';
        mosaic.appendChild(el);
    });

    // --- defaults, storage ---
    const defaultWidths = {
        'projects-section': 3, 'beatleader-section': 2, 'steam-section': 2,
        'neofetch-section': 2, 'tech-section': 2,
        'code-section': 2, 'meme-section': 1, 'lastfm-section': 2,
        'webring-section': 2, 'social-section': 2, 'visitors-section': 1, 'info-section': 2,
        'services-section': 2
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
        // restore previous scroll position without animation
        window.scrollTo(0, _scrollLockY);
    }


    // --- measurement helpers for packer ---
    const cssNumber = (el, prop) => {
        const v = getComputedStyle(el).getPropertyValue(prop);
        const m = /([0-9.+-]+)/.exec(v); return m ? parseFloat(m[1]) : 0;
    };
    const MIN_COL_FALLBACK = 280;
    const EXTRA = 24;

    function rowMetrics(){
        const rowSize = cssNumber(mosaic, 'grid-auto-rows') || 2;
        const gapRaw = getComputedStyle(mosaic).gap || getComputedStyle(mosaic).gridRowGap;
        const parts = (gapRaw || '3px').trim().split(/\s+/);
        const rowGap = parseFloat(parts.length === 2 ? parts[1] : parts[0]) || 3;
        return { rowSize, rowGap };
    }
    function rowSpanFromPx(h){
        const { rowSize, rowGap } = rowMetrics();
        return Math.max(1, Math.ceil((h + EXTRA + rowGap) / (rowSize + rowGap)));
    }
    function outerHeightPx(el){
        const r = el.getBoundingClientRect();
        const mb = parseFloat(getComputedStyle(el).marginBottom) || 0;
        return Math.ceil(r.height + mb);
    }
    function colCount(){
        const style = getComputedStyle(mosaic);
        const gap = parseFloat((style.columnGap || style.gap || '3px').split(/\s+/)[0]) || 3;
        const minCol = cssNumber(document.documentElement, '--col-min') || cssNumber(mosaic, '--col-min') || MIN_COL_FALLBACK;
        const w = mosaic.clientWidth;
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

    // --- FLIP animation ---
    function flip(update){
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

    // --- packers: quick (prebaked) + precise ---
    function measure(el){
        const w = clamp(+el.dataset.w || 1, 1, Math.max(1, colCount()));
        const hPx = outerHeightPx(el);
        return { el, w, span: rowSpanFromPx(hPx) };
    }

    function packWithGivenSpans(spanGetter){
        clampSpansToCols();
        const cols = colCount();
        const items = $$('.plugin', mosaic);

        // Respect current DOM order; do not reorder here.
        const entries = items.map(el => ({
            el,
            w: Math.min(+el.dataset.w || 1, cols),
            span: clamp(spanGetter(el) || 1, 1, 999)
        }));

        flip(() => {
            const occ = new Array(cols).fill(0);
            entries.forEach(x => {
                let bestC = 0, bestH = Infinity;
                for (let c = 0; c <= cols - x.w; c++){
                    const h = Math.max(...occ.slice(c, c + x.w));
                    if (h < bestH) { bestH = h; bestC = c; }
                }
                const startRow = bestH + 1;
                const endRow = startRow + x.span;
                for (let i=bestC; i<bestC+x.w; i++) occ[i] = endRow;
                x.el.style.gridColumn = `${bestC+1} / span ${x.w}`;
                x.el.style.gridRow    = `${startRow} / span ${x.span}`;
            });
        });
    }


    // quick boot: use last-known spans to avoid jumping
    function quickPackFromSaved(){
        packWithGivenSpans((el) => savedSpans[el.id]);
    }

    // precise pass: measure actual heights and persist spans
    function precisePack(){
        const spansOut = {};
        packWithGivenSpans((el) => {
            const m = measure(el);
            spansOut[el.id] = m.span;
            return m.span;
        });
        localStorage.setItem(spansStoreKey, JSON.stringify(spansOut));
    }

    const packAll = throttle(() => {
        if (document.hidden) return;
        precisePack();
        fitNeofetch();
    }, 80);
    function settlePasses(){
        [80, 220, 600, 1200].forEach(d => setTimeout(packAll, d));
    }


    function fullRepack(){
        clampSpansToCols();
        precisePack();
        fitNeofetch();
        settlePasses();
    }

    function getProjectPlugins(mosaic){
        return Array.from(mosaic.querySelectorAll('.projects-section, .projects-section.plugin'))
            .map(n => n.closest('.plugin') || n)
            .filter(Boolean);
    }


    function getWebringPlugins(mosaic){
        return Array.from(mosaic.querySelectorAll('.webring-section, .webring-section.plugin'))
            .map(n => n.closest('.plugin') || n)
            .filter(Boolean);
    }

    function ensureWebringFirst(mosaic){
        const list = getWebringPlugins(mosaic);
        list.forEach(n => {
            if (n.parentElement === mosaic && n !== mosaic.firstElementChild) {
                mosaic.insertBefore(n, mosaic.firstElementChild);
            }
        });
    }

    function detachProjects(mosaic){
        const list = getProjectPlugins(mosaic);
        list.forEach(n => n.remove());
        return list;
    }

    function ensureProjectsLast(mosaic){
        const list = getProjectPlugins(mosaic);
        list.forEach(n => {
            if (n.parentElement === mosaic && n !== mosaic.lastElementChild) {
                mosaic.appendChild(n);
            }
        });
    }


    (function initialLayout(){
        if (savedOrder.length){
            const byId = Object.fromEntries($$('.plugin', mosaic).map(n => [n.id, n]));
            savedOrder.forEach(id => byId[id] && mosaic.appendChild(byId[id]));
        } else {
            // initial-only constraints; after that user can move anything
            ensurePluginOrdering(mosaic); // e.g. put webring first, projects last (or add neofetch here if you want it special at boot)
        }

        // optional defer
        const deferredProjects = detachProjects(mosaic);

        quickPackFromSaved();

        setTimeout(() => {
            deferredProjects.forEach(n => mosaic.appendChild(n));
            precisePack();
            [120, 420].forEach(d => setTimeout(precisePack, d));
        }, 120);

        settlePasses();
    })();


    function ensurePluginOrdering(mosaic){
        ensureWebringFirst(mosaic);
        ensureProjectsLast(mosaic);
    }

    // --- actions / toolbar ---
    function setWidth(el, w){
        w = clamp(w, 1, 3);
        el.dataset.w = String(w);
        const widths = safeJSON(localStorage.getItem(widthsStoreKey)) || {};
        widths[el.id] = w; localStorage.setItem(widthsStoreKey, JSON.stringify(widths));
        preferredWidths.set(el.id, w);
        packAll(); settlePasses();
    }
    function toggleCollapse(el){
        el.classList.toggle('is-collapsed');
        packAll(); settlePasses();
    }
    function pin(el){
        el.dataset.pinned = el.dataset.pinned === '1' ? '0' : '1';
        const map = safeJSON(localStorage.getItem(pinnedStoreKey)) || {};
        map[el.id] = el.dataset.pinned === '1';
        localStorage.setItem(pinnedStoreKey, JSON.stringify(map));

        // bring pinned to the top, preserving relative order among pinned/unpinned
        const items = $$('.plugin', mosaic);
        const pinned = items.filter(n => n.dataset.pinned === '1');
        const unpinned = items.filter(n => n.dataset.pinned !== '1');
        [...pinned, ...unpinned].forEach(n => mosaic.appendChild(n));

        toast(el.dataset.pinned === '1' ? 'Pinned' : 'Unpinned');
        persistOrder();
        packAll();
        settlePasses();
    }


    const ICONS = { collapse:'▾', 'w-dec':'–', 'w-inc':'+', expand:'⛶' };
    const TITLES = { collapse:'Collapse', 'w-dec':'Narrower', 'w-inc':'Wider', expand:'Expand' };

    function makeDot(action, title){
        const b = document.createElement('button');
        b.className = 'icon-btn plugin-btn'; b.type = 'button';
        b.dataset.action = action; b.setAttribute('aria-label', title); b.title = title;

        // show symbol on hover, hide when leaving
        on(b, 'mouseenter', () => { b.textContent = ICONS[action] || ''; });
        on(b, 'mouseleave', () => { b.textContent = ''; });

        // prevent drag from starting when clicking buttons
        ['pointerdown','mousedown','click'].forEach(ev => {
            on(b, ev, (e) => { e.stopPropagation(); });
        });

        on(b, 'click', (e) => { ripple(e); b.blur(); handleAction(b.closest('.plugin'), action); });

        return b;
    }

    function ensureToolbar(el){
        let titleEl = $('h1,h2,h3,h4', el.querySelector('.plugin__inner'));
        if (!titleEl){
            titleEl = document.createElement('h3');
            titleEl.className = 'plugin-title';
            titleEl.textContent = (el.className.match(/([a-z0-9-]+)-section/i) || [,'Block'])[1].replace(/-/g,' ');
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
            ['collapse','w-dec','w-inc','expand'].forEach(action => bar.append(makeDot(action, TITLES[action])));
            ['pointerdown','mousedown','click'].forEach(ev => bar.addEventListener(ev, e => e.stopPropagation()));
        }

        headerRow.classList.add('drag-handle');
        headerRow.removeAttribute('draggable');
        headerRow.addEventListener('pointerdown', onHeaderPointerDown, { passive:false });
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
        localStorage.setItem('mosaic.order', JSON.stringify(order));
    }

    // --- overlay expand/collapse ---
    let overlay, expanded = null;
    function ensureOverlay(){
        if (overlay) return overlay;
        overlay = document.createElement('div');
        overlay.className = 'plugin-overlay';
        overlay.style.zIndex = '99999'; // guarantee on top
        overlay.addEventListener('click', (e) => { if (e.target === overlay) collapseExpanded(); });
        document.addEventListener('keydown', (e) => { if (e.key === 'Escape') collapseExpanded(); });
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
        ph.style.gridRow    = el.style.gridRow;
        mosaic.insertBefore(ph, el.nextSibling);

        ensureOverlay().classList.add('in');
        lockBodyScroll();              // <— lock here

        el.classList.add('plugin--expanded');
        overlay.appendChild(el);
        expanded = el;
        setTimeout(() => { packAll(); }, 60);

        if (updateURL){
            const pluginName = (el.className.match(/([a-z0-9-]+)-section/i)||[])[1] || el.id;
            history.pushState({ expanded: pluginName }, '', `#${pluginName}`);
        }
    }
    function collapseExpanded(updateURL = true){
        if (!expanded) {
            ensureOverlay().classList.remove('in');
            unlockBodyScroll();          // <— ensure unlocked even if nothing expanded
            return;
        }
        const ph = $('.plugin-placeholder', mosaic);
        expanded.classList.remove('plugin--expanded');
        if (ph && ph.parentNode) ph.parentNode.replaceChild(expanded, ph);
        ensureOverlay().classList.remove('in');
        expanded = null;

        unlockBodyScroll();            // <— unlock here

        packAll(); settlePasses();
        if (updateURL) history.pushState({}, '', window.location.pathname);
    }


    // --- real-WM style dragging with z-elevation + AUTOSCROLL + HOVER-SWAP ---
    let pointerDown = false, startedDrag = false;
    let dragEl = null, placeholderEl = null, handleEl = null;
    let startX = 0, startY = 0, latestX = 0, latestY = 0;
    let dragOffsetX = 0, dragOffsetY = 0;
    let rafPending = false;
    let latestClientY = 0; // viewport-based Y for autoscroll
    let maxZ = 1000;
    let autoScrollRAF = null;
    const DRAG_START_TOL = 6; // px movement before we actually start the drag
    let lastHoverTarget = null, lastSwapAt = 0;

    function bringToFront(e){
        const win = e.currentTarget.closest('.plugin');
        if (!win) return;
        maxZ += 1;
        win.style.zIndex = String(maxZ);
    }

    function onHeaderPointerDown(e){
        if (e.button !== 0) return; // primary only
        if (isInteractive(e.target)) return; // don't drag when clicking buttons/links/etc.
        const win = e.currentTarget.closest('.plugin'); if (!win) return;
        e.preventDefault();

        pointerDown = true; startedDrag = false;
        handleEl = e.currentTarget;
        dragEl = win;

        const rect = dragEl.getBoundingClientRect();
        startX = e.clientX; startY = e.clientY;
        latestX = rect.left; latestY = rect.top;
        latestClientY = e.clientY;

        document.addEventListener('pointermove', onDocPointerMove, { passive:false });
        document.addEventListener('pointerup', onDocPointerUp, { passive:false });
        document.addEventListener('pointercancel', onDocPointerUp, { passive:false });
    }

    function beginDrag(e){
        if (startedDrag || !dragEl) return;
        startedDrag = true;

        const rect = dragEl.getBoundingClientRect();

        // create placeholder occupying grid space (original slot)
        placeholderEl = document.createElement('div');
        placeholderEl.className = dragEl.className + ' plugin-placeholder';
        placeholderEl.style.height = rect.height + 'px';
        placeholderEl.dataset.w = dragEl.dataset.w || '1';
        placeholderEl.style.gridColumn = dragEl.style.gridColumn;
        placeholderEl.style.gridRow    = dragEl.style.gridRow;

        mosaic.replaceChild(placeholderEl, dragEl);

        // elevate and move to fixed coordinates
        dragOffsetX = e.clientX - rect.left;
        dragOffsetY = e.clientY - rect.top;

        bringToFront({ currentTarget: handleEl });

        dragEl.style.position = 'fixed';
        dragEl.style.left = rect.left + 'px';
        dragEl.style.top  = rect.top  + 'px';
        dragEl.style.width  = rect.width  + 'px';
        dragEl.style.height = rect.height + 'px';
        dragEl.classList.add('dragging');
        dragEl.style.pointerEvents = 'none'; // allow elementFromPoint to "see" what's beneath

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

        // start drag only after small movement (prevents "buttons not clickable" issue)
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

        // live swap with hovered plugin
        hoverSwapAt(e.clientX, e.clientY);
    }

    function onDocPointerUp(e){
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
        packAll();

        setTimeout(() => {
            packAll();
        }, 50);
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
        if (autoScrollRAF){ cancelAnimationFrame(autoScrollRAF); autoScrollRAF = null; }
    }

    // --- cosmetics ---
    function ripple(e){
        const el = e.currentTarget; el.classList.add('ripple-host');
        const r = document.createElement('span'); r.className = 'ripple';
        const rect = el.getBoundingClientRect(); const d = Math.max(rect.width, rect.height);
        r.style.width = r.style.height = d + 'px';
        r.style.left = (e.clientX - rect.left - d/2) + 'px';
        r.style.top  = (e.clientY - rect.top  - d/2) + 'px';
        el.appendChild(r); setTimeout(() => r.remove(), 600);
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

    // --- observers / utilities ---
    const io = new IntersectionObserver(entries => {
        entries.forEach(e => e.target.classList.toggle('reveal', e.isIntersecting));
    }, { threshold: 0.08 });
    $$('.plugin', mosaic).forEach(el => io.observe(el));

    if ('fonts' in document && document.fonts.ready) document.fonts.ready.then(() => { packAll(); });

    const ro = new ResizeObserver(() => { packAll(); });
    $$('.plugin', mosaic).forEach(n => ro.observe(n, { box: 'border-box' }));
    $$('.plugin__inner', mosaic).forEach(n => ro.observe(n, { box: 'border-box' }));

    const mo = new MutationObserver(muts => {
        let needs = false;
        muts.forEach(m => {
            m.addedNodes && m.addedNodes.forEach(node => {
                if (node.nodeType === 1){
                    if (node.tagName === 'IMG') needs = true;
                    node.querySelectorAll && node.querySelectorAll('img').forEach(() => needs = true);
                }
            });
            if (m.type === 'attributes' && m.target.tagName === 'IMG' && (m.attributeName === 'src' || m.attributeName === 'srcset')) {
                needs = true;
            }
        });
        if (needs){ packAll(); setTimeout(packAll, 200); }
    });
    mo.observe(mosaic, { childList:true, subtree:true, attributes:true, attributeFilter:['src','srcset'] });

    function restorePreferredWidths(maxCols = colCount()){
        $$('.plugin', mosaic).forEach(el => {
            const pref = preferredWidths.get(el.id);
            if (pref != null) el.dataset.w = String(clamp(pref, 1, Math.min(3, maxCols)));
        });
    }

    let lastCols = colCount();
    on(window, 'resize', () => {
        const nextCols = colCount();
        clampSpansToCols();
        if (nextCols > lastCols) restorePreferredWidths(nextCols);
        lastCols = nextCols;
        fullRepack();
    });

    on(document, 'visibilitychange', () => { if (!document.hidden) { packAll(); settlePasses(); } });
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
            document.fonts.ready.then(() => { packAll(); });
        }

        const ro = new ResizeObserver(() => { packAll(); });
        $$('.plugin', mosaic).forEach(n => ro.observe(n, { box: 'border-box' }));
        $$('.plugin__inner', mosaic).forEach(n => ro.observe(n, { box: 'border-box' }));

        const mo = new MutationObserver(muts => {
            let needs = false;
            muts.forEach(m => {
                m.addedNodes && m.addedNodes.forEach(node => {
                    if (node.nodeType === 1){
                        if (node.tagName === 'IMG') needs = true;
                        node.querySelectorAll && node.querySelectorAll('img').forEach(() => needs = true);
                    }
                });
                if (m.type === 'attributes' && m.target.tagName === 'IMG' && (m.attributeName === 'src' || m.attributeName === 'srcset')) {
                    needs = true;
                }
            });
            if (needs){ packAll(); setTimeout(packAll, 200); }
        });
        mo.observe(mosaic, { childList:true, subtree:true, attributes:true, attributeFilter:['src','srcset'] });

        let lastCols = colCount();
        on(window, 'resize', () => {
            const nextCols = colCount();
            clampSpansToCols();
            if (nextCols > lastCols) restorePreferredWidths(nextCols);
            lastCols = nextCols;
            fullRepack();
        }, { once: false, passive: true });

        on(document, 'visibilitychange', () => {
            if (!document.hidden) { packAll(); settlePasses(); }
        }, { once: false, passive: true });

        on(document, 'keydown', (e) => {
            if (e.altKey && e.key >= '1' && e.key <= '9'){
                e.preventDefault();
                const idx = parseInt(e.key) - 1;
                const plugins = $$('.plugin', mosaic);
                if (plugins[idx]){
                    plugins[idx].scrollIntoView({ behavior:'smooth', block:'center' });
                    setTimeout(() => expand(plugins[idx]), 300);
                }
            }
        }, { once: false });

        $('#js-status')?.classList.add('status-online');
        if ($('#js-text')) $('#js-text').textContent = 'Enabled';
        try {
            localStorage.setItem('_t','1'); localStorage.removeItem('_t');
            $('#storage-status')?.classList.add('status-online');
            $('#storage-text') && ($('#storage-text').textContent = 'Available');
        } catch {
            $('#storage-status')?.classList.add('status-offline');
            $('#storage-text') && ($('#storage-text').textContent = 'Unavailable');
        }

        window.mosaicUtils = {
            resizeAll: packAll,
            fullRepack,
            expand,
            collapseExpanded,
            getMosaic: () => mosaic
        };
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

})();