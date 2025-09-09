(function () {
    'use strict';

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

    const root = $('.container'); if (!root) return;

    let mosaic = $('.mosaic');
    if (!mosaic) {
        mosaic = document.createElement('section');
        mosaic.className = 'mosaic';
        const profile = $('.profile-section', root);
        if (profile && profile.nextSibling) root.insertBefore(mosaic, profile.nextSibling);
        else root.appendChild(mosaic);
    }

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
        el.style.gridRow = el.style.gridColumn = el.style.gridRowEnd = '';
        mosaic.appendChild(el);
    });

    const defaultWidths = {
        'projects-section': 3, 'beatleader-section': 2, 'steam-section': 2,
        'neofetch-section': 2, 'tech-section': 2, 'social-section': 1,
        'code-section': 2, 'meme-section': 1, 'lastfm-section': 2,
        'webring-section': 2, 'visitors-section': 1, 'info-section': 2,
        'services-section': 2,
    };

    // Track intended/preferred widths so we can restore them after a shrink -> grow
    const preferredWidths = new Map();

    $$('.plugin', mosaic).forEach(el => {
        const saved = storage.get('mosaic.widths', {})[el.id];
        const key = Object.keys(defaultWidths).find(k => el.classList.contains(k));
        const w = el.dataset.w || saved || (key ? defaultWidths[key] : 1);
        const clamped = clamp(+w || 1, 1, 3);
        el.dataset.w = String(clamped);
        preferredWidths.set(el.id, clamped);
    });

    const cssNumber = (el, prop) => {
        const v = getComputedStyle(el).getPropertyValue(prop);
        const m = /([\d.]+)/.exec(v); return m ? parseFloat(m[1]) : 0;
    };

    const rowMetrics = () => {
        const rowSize = cssNumber(mosaic, 'grid-auto-rows') || 2;
        const gapRaw = getComputedStyle(mosaic).gap || getComputedStyle(mosaic).gridRowGap;
        const parts = (gapRaw || '3px').trim().split(/\s+/);
        const rowGap = parseFloat(parts.length === 2 ? parts[1] : parts[0]) || 3;
        return { rowSize, rowGap };
    };

    const EXTRA = 24;

    function rowSpanFromPx(h) {
        const { rowSize, rowGap } = rowMetrics();
        return Math.max(1, Math.ceil((h + EXTRA + rowGap) / (rowSize + rowGap)));
    }

    function outerHeightPx(plugin) {
        const r = plugin.getBoundingClientRect();
        const mb = parseFloat(getComputedStyle(plugin).marginBottom) || 0;
        return Math.ceil(r.height + mb);
    }

    // Use CSS variable --col-min so breakpoints & JS stay in sync
    const MIN_COL_FALLBACK = 280;
    function colCount() {
        const style = getComputedStyle(mosaic);
        const gap = parseFloat((style.columnGap || style.gap || '3px').split(/\s+/)[0]) || 3;
        const minCol = cssNumber(document.documentElement, '--col-min') || cssNumber(mosaic, '--col-min') || MIN_COL_FALLBACK;
        const w = mosaic.clientWidth;
        return Math.max(1, Math.floor((w + gap) / (minCol + gap)));
    }

    function clampSpansToCols() {
        const cols = colCount();
        $$('.plugin', mosaic).forEach(el => {
            let w = clamp(+el.dataset.w || 1, 1, 3);
            if (w > cols) w = cols;
            el.dataset.w = String(w);
        });
    }

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

    function priority(el) {
        if (el.dataset.pinned === '1') return 2;
        if (el.classList.contains('projects-section')) return -1;
        return 0;
    }

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

        m.sort((a, b) =>
            (b._pri - a._pri) ||
            (b.w - a.w) ||
            (a._ord - b._ord)
        );

        flip(() => {
            const occ = new Array(cols).fill(0);

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

    const packAll = throttle(() => {
        if (document.hidden) return;
        layoutPacker();
        fitNeofetch();
    }, 80);

    function settlePasses() {
        [0, 60, 180, 420, 900, 1600].forEach(d => setTimeout(packAll, d));
    }

    function fullRepack() {
        // Force recalculation of column count and layout
        clampSpansToCols();
        layoutPacker();
        fitNeofetch();
        settlePasses();
    }

    (function initialLayout() {
        const items = $$('.plugin', mosaic);

        const order = storage.get('mosaic.order', []);
        if (order && order.length) {
            const map = Object.fromEntries(items.map(n => [n.id, n]));
            order.forEach(id => map[id] && mosaic.appendChild(map[id]));
        } else {
            $$('.projects-section.plugin', mosaic).forEach(n => mosaic.appendChild(n));
        }

        const pinned = storage.get('mosaic.pinned', {});
        items.forEach(el => { if (pinned[el.id]) el.dataset.pinned = '1'; });
        $$('.plugin', mosaic)
            .sort((a,b) => (b.dataset.pinned||'0').localeCompare(a.dataset.pinned||'0'))
            .forEach(n => mosaic.appendChild(n));

        raf(() => { packAll(); settlePasses(); });
    })();

    function setWidth(el, w) {
        w = clamp(w, 1, 3);
        el.dataset.w = String(w);
        // persist + remember intended width
        const widths = storage.get('mosaic.widths', {}); widths[el.id] = w; storage.set('mosaic.widths', widths);
        preferredWidths.set(el.id, w);
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
            const actions = ['collapse', 'w-dec', 'w-inc', 'expand'];
            const titles = ['Collapse', 'Narrower', 'Wider', 'Expand'];
            actions.forEach((action, i) => bar.append(makeDot(action, titles[i])));
        }
        headerRow.classList.add('drag-handle');
        headerRow.setAttribute('draggable','true');
    }

    $$('.plugin', mosaic).forEach(ensureToolbar);

    let expanded = null, overlay, slotMap = new Map();

    function ensureOverlay(){
        overlay ||= (() => {
            const o = document.createElement('div');
            o.className = 'plugin-overlay';
            o.style.display = 'flex';
            o.style.alignItems = 'center';
            o.style.justifyContent = 'center';
            o.style.padding = '20px';
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

    function expand(el, updateURL = true){
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

        if (updateURL) {
            const pluginName = getPluginName(el);
            history.pushState({ expanded: pluginName }, '', `#${pluginName}`);
        }
    }

    function collapseExpanded(updateURL = true){
        if (!expanded) return;
        const ph = slotMap.get(expanded);
        expanded.classList.remove('plugin--expanded');
        if (ph && ph.parentNode) ph.parentNode.replaceChild(expanded, ph);
        slotMap.delete(expanded);
        ensureOverlay().classList.remove('in');
        expanded = null;
        packAll(); settlePasses(); fitNeofetch();

        if (updateURL) {
            history.pushState({}, '', window.location.pathname);
        }
    }

    function getPluginName(el) {
        for (const cls of el.classList) {
            if (cls.endsWith('-section')) {
                return cls.replace('-section', '');
            }
        }
        return el.id || 'plugin';
    }

    function handleAction(el, action) {
        if (!el) return;
        if (action === 'expand')   expand(el);
        if (action === 'view')     expand(el);
        if (action === 'collapse') toggleCollapse(el);
        if (action === 'pin')      pin(el);
        if (action === 'w-inc')    setWidth(el, (+el.dataset.w || 1) + 1);
        if (action === 'w-dec')    setWidth(el, (+el.dataset.w || 1) - 1);
    }

    function persistOrder() { storage.set('mosaic.order', $$('.plugin', mosaic).map(n => n.id)); }

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

    let filterPopup = null;
    let currentFilter = null;

    function createFilterPopup() {
        if (filterPopup) return filterPopup;

        filterPopup = document.createElement('div');
        filterPopup.className = 'tech-filter-popup';
        filterPopup.innerHTML = `
            <span class="filter-icon">🔧</span>
            <span class="filter-text">Filtering projects by:</span>
            <strong class="filter-tech"></strong>
            <button class="clear-filter-btn" type="button">Clear Filter</button>
        `;
        document.body.appendChild(filterPopup);

        $('.clear-filter-btn', filterPopup).addEventListener('click', clearTechFilter);
        return filterPopup;
    }

    function showFilterPopup(techName) {
        const popup = createFilterPopup();
        $('.filter-tech', popup).textContent = techName;
        popup.classList.add('show');
    }

    function clearTechFilter() {
        const projectsSection = $('.projects-section');
        if (!projectsSection) return;

        $$('.project-card', projectsSection).forEach(card => {
            card.style.opacity = '1';
            card.style.transform = 'scale(1)';
            card.style.filter = 'none';
            card.style.transition = 'all 0.3s ease';
        });

        $$('.tech-item').forEach(item => item.classList.remove('filtered'));

        if (filterPopup) {
            filterPopup.classList.remove('show');
        }

        currentFilter = null;
    }

    function matchesTechnology(techTags, techName) {
        // Create regex pattern for exact word matching (case insensitive)
        const pattern = new RegExp(`\\b${techName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\b`, 'i');

        return techTags.some(tag => {
            const tagText = tag.textContent.trim();
            return pattern.test(tagText);
        });
    }

    function initTechFiltering() {
        const techSection = $('.tech-section');
        const projectsSection = $('.projects-section');
        if (!techSection || !projectsSection) return;

        const techItems = $$('.tech-item', techSection);
        const projectCards = $$('.project-card', projectsSection);

        techItems.forEach(item => {
            const techName = item.querySelector('.tech-name')?.textContent ||
                item.title ||
                item.querySelector('img')?.alt || '';

            item.style.cursor = 'pointer';
            item.addEventListener('click', () => {
                if (currentFilter === techName) {
                    clearTechFilter();
                    return;
                }

                projectsSection.scrollIntoView({
                    behavior: 'smooth',
                    block: 'start'
                });

                setTimeout(() => {
                    techItems.forEach(t => t.classList.remove('filtered'));
                    item.classList.add('filtered');
                    currentFilter = techName;

                    let hasMatchingProjects = false;

                    projectCards.forEach(card => {
                        const techTags = $$('.tech-tag', card);
                        const hasTech = matchesTechnology(techTags, techName);

                        card.style.transition = 'all 0.3s ease';
                        if (!hasTech) {
                            card.style.opacity = '0.2';
                            card.style.transform = 'scale(0.95)';
                            card.style.filter = 'grayscale(80%)';
                        } else {
                            card.style.opacity = '1';
                            card.style.transform = 'scale(1)';
                            card.style.filter = 'none';
                            hasMatchingProjects = true;
                        }
                    });

                    if (hasMatchingProjects) {
                        showFilterPopup(techName);
                    } else {
                        toast(`No projects found using ${techName}`);
                        clearTechFilter();
                    }
                }, 500);
            });
        });
    }

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

    const io = new IntersectionObserver(entries => {
        entries.forEach(e => e.target.classList.toggle('reveal', e.isIntersecting));
    }, { threshold: 0.08 });
    $$('.plugin', mosaic).forEach(el => io.observe(el));

    if ('fonts' in document && document.fonts.ready) document.fonts.ready.then(() => { packAll(); settlePasses(); });

    const ro = new ResizeObserver(() => { packAll(); });
    $$('.plugin', mosaic).forEach(n => ro.observe(n, { box: 'border-box' }));
    $$('.plugin__inner', mosaic).forEach(n => ro.observe(n, { box: 'border-box' }));

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

    // --- Width restore helper: behave like reload after growing columns ---
    function restorePreferredWidths(maxCols = colCount()) {
        $$('.plugin', mosaic).forEach(el => {
            const pref = preferredWidths.get(el.id);
            if (pref != null) {
                el.dataset.w = String(clamp(pref, 1, Math.min(3, maxCols)));
            } else {
                const saved = storage.get('mosaic.widths', {})[el.id];
                const key = Object.keys(defaultWidths).find(k => el.classList.contains(k));
                const base = el.dataset.w || saved || (key ? defaultWidths[key] : 1);
                el.dataset.w = String(clamp(+base || 1, 1, Math.min(3, maxCols)));
                preferredWidths.set(el.id, clamp(+base || 1, 1, 3));
            }
        });
    }

    // Enhanced window resize handling with proper reflow + width restore
    let resizeTimer = null;
    let lastCols = colCount();

    on(window, 'resize', () => {
        if (resizeTimer) clearTimeout(resizeTimer);

        const nextCols = colCount();

        // Always clamp first to avoid overflows while shrinking
        clampSpansToCols();

        // If we gained columns, restore original (preferred) widths like after a reload
        if (nextCols > lastCols) {
            restorePreferredWidths(nextCols);
        }

        lastCols = nextCols;

        // Schedule full repack after resize stops
        resizeTimer = setTimeout(() => {
            fullRepack();
            resizeTimer = null;
        }, 150);
    });

    on(document, 'visibilitychange', () => { if (!document.hidden) { packAll(); settlePasses(); } });

    document.addEventListener('toggle', (e) => {
        if (e.target.closest('.code-section') && e.target.tagName === 'DETAILS') setTimeout(packAll, 50);
    }, true);

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

    function fitNeofetch() {
        const terms = document.querySelectorAll('.neofetch-section .terminal');
        terms.forEach(t => {
            const pre = t.querySelector('pre'); if (!pre) return;
            t.style.overflowX = 'auto'; t.style.overflowY = 'auto';

            t.style.removeProperty('--neo-scale'); pre.style.removeProperty('transform'); pre.style.transform = 'scale(1)';

            setTimeout(() => {
                const containerWidth = t.clientWidth - 24;
                const contentWidth = pre.scrollWidth;

                let scale = 1;
                const inExpanded = !!t.closest('.plugin--expanded');
                if (!inExpanded && contentWidth > containerWidth && containerWidth > 0) {
                    scale = Math.max(0.3, containerWidth / contentWidth);
                }
                t.style.setProperty('--neo-scale', String(scale));
                pre.style.transform = `scale(${scale})`;

                const scaledHeight = pre.scrollHeight * scale;
                t.style.height = Math.max(200, scaledHeight + 24) + 'px';
            }, 10);
        });
    }

    function handleHashChange() {
        const hash = window.location.hash.slice(1);
        if (hash) {
            const plugin = $(`.${hash}-section`);
            if (plugin && !expanded) {
                setTimeout(() => expand(plugin, false), 100);
            }
        } else if (expanded) {
            collapseExpanded(false);
        }
    }

    on(window, 'hashchange', handleHashChange);
    on(window, 'popstate', (e) => {
        if (e.state && e.state.expanded) {
            const plugin = $(`.${e.state.expanded}-section`);
            if (plugin) expand(plugin, false);
        } else if (expanded) {
            collapseExpanded(false);
        }
    });

    if (window.location.hash) {
        setTimeout(handleHashChange, 500);
    }

    (function init(){
        initClicks();
        initStatus();
        initTechFiltering();

        document.addEventListener('keydown', (e) => {
            if (e.altKey && e.key >= '1' && e.key <= '9') {
                e.preventDefault();
                const pluginIndex = parseInt(e.key) - 1;
                const plugins = $$('.plugin');

                if (plugins[pluginIndex]) {
                    plugins[pluginIndex].scrollIntoView({
                        behavior: 'smooth',
                        block: 'center'
                    });

                    setTimeout(() => {
                        expand(plugins[pluginIndex]);
                    }, 300);
                }
            }
        });
    })();

    window.mosaicUtils = { resizeAll: packAll, toast, expand, collapseExpanded, fullRepack };
    on(document, 'plugin-updated', () => setTimeout(() => { packAll(); fitNeofetch(); }, 50));
    window.addEventListener('lastfm_update', () => setTimeout(() => { packAll(); fitNeofetch(); }, 50));
    window.addEventListener('code_stats_update', () => setTimeout(() => { packAll(); fitNeofetch(); }, 50));
    window.addEventListener('plugins_updated', () => setTimeout(() => location.reload(), 900));

})();