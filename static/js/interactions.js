(function () {
    /* ============ small utils ============ */
    const $ = (q, c = document) => c.querySelector(q);
    const $$ = (q, c = document) => Array.from(c.querySelectorAll(q));
    const on = (el, ev, fn, opts) => el && el.addEventListener(ev, fn, opts);
    const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
    const now = () => Date.now();
    const raf = (fn) => requestAnimationFrame(fn);
    const throttle = (fn, ms = 100) => {
        let t = 0, lastArgs = null;
        return (...args) => {
            const n = now();
            if (n - t > ms) { t = n; fn(...args); }
            else { lastArgs = args; clearTimeout(fn._t); fn._t = setTimeout(() => { t = now(); fn(...(lastArgs || [])); }, ms); }
        };
    };
    const storage = {
        get(k, fallback) { try { return JSON.parse(localStorage.getItem(k)) ?? fallback; } catch { return fallback; } },
        set(k, v) { try { localStorage.setItem(k, JSON.stringify(v)); } catch{} },
    };

    /* ============ Physics utilities ============ */
    function distance(a, b) {
        const dx = a.x - b.x;
        const dy = a.y - b.y;
        return Math.sqrt(dx * dx + dy * dy);
    }

    function rectOverlap(rect1, rect2) {
        return !(rect1.right < rect2.left ||
            rect1.left > rect2.right ||
            rect1.bottom < rect2.top ||
            rect1.top > rect2.bottom);
    }

    function getCenter(rect) {
        return {
            x: rect.left + rect.width / 2,
            y: rect.top + rect.height / 2
        };
    }

    /* ============ Toasts ============ */
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

    /* ============ Ripple ============ */
    function ripple(e) {
        const el = e.currentTarget;
        el.classList.add('ripple-host');
        const r = document.createElement('span');
        r.className = 'ripple';
        const rect = el.getBoundingClientRect();
        const d = Math.max(rect.width, rect.height);
        r.style.width = r.style.height = d + 'px';
        r.style.left = (e.clientX - rect.left - d / 2) + 'px';
        r.style.top  = (e.clientY - rect.top  - d / 2) + 'px';
        el.appendChild(r);
        setTimeout(() => r.remove(), 600);
    }

    /* ============ Mosaic builder ============ */
    const root = $('.container');
    if (!root) return;

    // Ensure mosaic column exists right after profile
    let mosaic = $('.mosaic');
    if (!mosaic) {
        mosaic = document.createElement('section');
        mosaic.className = 'mosaic';
        const profile = $('.profile-section', root);
        if (profile && profile.nextSibling) root.insertBefore(mosaic, profile.nextSibling);
        else root.appendChild(mosaic);
    }

    // Move all sections except profile into mosaic and normalize structure
    const toMove = [...root.children].filter(
        el => el !== mosaic && !el.classList.contains('profile-section')
    );
    toMove.forEach(el => {
        el.classList.add('plugin'); // make it a tile
        if (!el.querySelector('.plugin__inner')) {
            const inner = document.createElement('div');
            inner.className = 'plugin__inner';
            while (el.firstChild) inner.appendChild(el.firstChild);
            el.appendChild(inner);
        }
        // Mark as focusable
        el.tabIndex = 0;

        // Give it a stable id for persistence
        if (!el.id) {
            const guess = (el.className.match(/([a-z0-9-]+)-section/i) || [,'tile'])[1];
            el.id = `${guess}-${Math.random().toString(36).slice(2,7)}`;
        }

        mosaic.appendChild(el);
    });

    /* Default column spans for known sections (can be overridden by data-w) */
    const defaultWidths = {
        'projects-section': 2, 'beatleader-section': 2,
        'steam-section': 2, 'neofetch-section': 2,
        'tech-section': 2, 'social-section': 1,
        'code-section': 3, 'meme-section': 1,
        'lastfm-section': 2, 'webring-section': 2, 'visitors-section': 1,  'info-section': 2,
    };
    $$('.plugin', mosaic).forEach(el => {
        const key = Object.keys(defaultWidths).find(k => el.classList.contains(k));
        const saved = storage.get('mosaic.widths', {})[el.id];
        const w = el.dataset.w || saved || (key ? defaultWidths[key] : 1);
        el.dataset.w = String(clamp(+w || 1, 1, 3));
    });

    /* ============ Masonry sizing ============ */
    const getCssNumber = (el, prop) => parseFloat(getComputedStyle(el).getPropertyValue(prop)) || 0;

    const resizeItem = (item) => {
        const inner = item.querySelector('.plugin__inner') || item;
        const rowGap  = getCssNumber(mosaic, 'gap');
        const rowSize = getCssNumber(mosaic, 'grid-auto-rows') || 10;
        const h = inner.getBoundingClientRect().height;
        const rowSpan = Math.ceil((h + rowGap) / (rowSize + rowGap));
        item.style.gridRowEnd = `span ${rowSpan}`;
    };
    const resizeAll = throttle(() => $$('.plugin', mosaic).forEach(resizeItem), 50);

    on(window, 'resize', resizeAll);
    if ('fonts' in document) document.fonts.ready.then(resizeAll);

    const imgs = $$('img', mosaic);
    if (imgs.length) imgs.forEach(img => {
        if (img.complete) return;
        on(img, 'load', resizeAll, { once: true });
        on(img, 'error', resizeAll, { once: true });
    });

    const ro = new ResizeObserver(entries => {
        for (const e of entries) {
            const plugin = e.target.closest('.plugin');
            if (plugin) resizeItem(plugin);
        }
    });
    $$('.plugin__inner', mosaic).forEach(n => ro.observe(n));

    /* ============ Toolbar / header (same line as title) ============ */
    function makeBtn(title, icon, action) {
        const b = document.createElement('button');
        b.className = 'icon-btn plugin-btn';
        b.type = 'button';
        b.setAttribute('aria-label', title);
        b.dataset.action = action;
        b.innerHTML = icon;
        on(b, 'click', (e) => { ripple(e); b.blur(); handleAction(b.closest('.plugin'), action); });
        return b;
    }

    function ensureToolbar(el) {
        // Find or create heading
        let titleEl = $('h3, h2, h4', el);
        if (!titleEl) {
            titleEl = document.createElement('h3');
            titleEl.textContent = (el.className.match(/([a-z0-9-]+)-section/i) || [,'Block'])[1].replace(/-/g,' ');
            el.querySelector('.plugin__inner').prepend(titleEl);
        }

        // Wrap heading and toolbar into a single header row
        let headerRow = $('.plugin-header', el);
        if (!headerRow) {
            headerRow = document.createElement('div');
            headerRow.className = 'plugin-header'; // becomes the "title bar"
            titleEl.before(headerRow);
            headerRow.appendChild(titleEl);
        }

        let bar = $('.plugin-toolbar', headerRow);
        if (!bar) {
            bar = document.createElement('div');
            bar.className = 'plugin-toolbar';
            headerRow.appendChild(bar);
            bar.append(
                makeBtn('Expand',   '⤢',  'expand'),
                makeBtn('Narrower', '−',  'w-dec'),
                makeBtn('Wider',    '+',  'w-inc'),
                makeBtn('Collapse', '▾',  'collapse')
            );
        }

        // Drag handle is HEADER ONLY
        headerRow.classList.add('drag-handle');
        headerRow.setAttribute('draggable', 'true');
    }

    $$('.plugin', mosaic).forEach(ensureToolbar);

    /* Expand overlay */
    let expanded = null, overlay;
    function expand(el) {
        if (expanded === el) return collapseExpanded();
        collapseExpanded();
        overlay ||= (() => {
            const o = document.createElement('div');
            o.className = 'plugin-overlay';
            on(o, 'click', (e) => { if (e.target === o) collapseExpanded(); });
            document.body.appendChild(o);
            on(document, 'keydown', (e) => { if (e.key === 'Escape') collapseExpanded(); });
            return o;
        })();
        expanded = el;
        el.classList.add('plugin--expanded');
        overlay.appendChild(el);
        overlay.classList.add('in');
    }
    function collapseExpanded() {
        if (!expanded) return;
        expanded.classList.remove('plugin--expanded');
        mosaic.appendChild(expanded);
        expanded = null;
        overlay?.classList.remove('in');
        resizeAll();
    }

    /* Collapse toggle */
    function toggleCollapse(el) {
        el.classList.toggle('is-collapsed');
        resizeItem(el);
    }

    /* Pin to the top of mosaic */
    function pin(el) {
        el.dataset.pinned = el.dataset.pinned === '1' ? '0' : '1';
        reorderByPinned();
        persistOrder();
        toast(el.dataset.pinned === '1' ? 'Pinned' : 'Unpinned');
    }
    function reorderByPinned() {
        const items = $$('.plugin', mosaic).sort((a,b) => (b.dataset.pinned||'0').localeCompare(a.dataset.pinned||'0'));
        items.forEach(n => mosaic.appendChild(n));
        resizeAll();
    }

    /* Width control */
    function setWidth(el, w) {
        w = clamp(w, 1, 3);
        el.dataset.w = String(w);
        const widths = storage.get('mosaic.widths', {});
        widths[el.id] = w;
        storage.set('mosaic.widths', widths);
        resizeItem(el);
    }

    function handleAction(el, action) {
        if (!el) return;
        if (action === 'expand')   expand(el);
        if (action === 'collapse') toggleCollapse(el);
        if (action === 'pin')      pin(el);
        if (action === 'w-inc')    setWidth(el, (+el.dataset.w || 1) + 1);
        if (action === 'w-dec')    setWidth(el, (+el.dataset.w || 1) - 1);
    }

    /* ============ Physics-based drag and drop ============ */
    function getId(el){ return el.id; }
    function persistOrder() {
        const order = $$('.plugin', mosaic).map(getId);
        storage.set('mosaic.order', order);
    }
    function applySavedOrder() {
        const order = storage.get('mosaic.order', []);
        if (!order || !order.length) return;
        const map = Object.fromEntries($$('.plugin', mosaic).map(n => [getId(n), n]));
        order.forEach(id => map[id] && mosaic.appendChild(map[id]));
    }
    applySavedOrder();

    let dragEl = null;
    let dragProxy = null;
    let dragOffset = { x: 0, y: 0 };
    let dragVelocity = { x: 0, y: 0 };
    let lastDragPos = { x: 0, y: 0 };
    let dragStartTime = 0;
    let animationFrame = null;
    let isDragging = false;

    // Physics constants
    const BOUNCE_FORCE = 0.3;
    const FRICTION = 0.95;
    const MIN_VELOCITY = 0.5;
    const COLLISION_DISTANCE = 20;

    function createDragProxy(element) {
        const proxy = element.cloneNode(true);
        proxy.className = element.className + ' drag-proxy';
        proxy.style.position = 'fixed';
        proxy.style.pointerEvents = 'none';
        proxy.style.zIndex = '10000';
        proxy.style.transform = 'scale(1.02)';
        proxy.style.opacity = '0.9';
        proxy.style.transition = 'none';
        proxy.style.boxShadow = '0 20px 60px rgba(0,0,0,0.3)';

        // Remove any interactive elements
        proxy.querySelectorAll('button, input, a').forEach(el => {
            el.style.pointerEvents = 'none';
        });

        document.body.appendChild(proxy);
        return proxy;
    }

    function updateDragProxyPosition(e) {
        if (!dragProxy || !isDragging) return;

        const x = e.clientX - dragOffset.x;
        const y = e.clientY - dragOffset.y;

        dragProxy.style.left = x + 'px';
        dragProxy.style.top = y + 'px';

        // Calculate velocity
        const currentTime = now();
        const deltaTime = Math.max(currentTime - dragStartTime, 1);
        dragVelocity.x = (e.clientX - lastDragPos.x) / deltaTime * 1000;
        dragVelocity.y = (e.clientY - lastDragPos.y) / deltaTime * 1000;

        lastDragPos.x = e.clientX;
        lastDragPos.y = e.clientY;

        // Check for collisions
        checkCollisions(e.clientX, e.clientY);
    }

    function checkCollisions(mouseX, mouseY) {
        if (!dragProxy || !dragEl) return;

        const dragRect = dragProxy.getBoundingClientRect();
        const dragCenter = getCenter(dragRect);

        $$('.plugin', mosaic).forEach(plugin => {
            if (plugin === dragEl) return;

            const pluginRect = plugin.getBoundingClientRect();
            const pluginCenter = getCenter(pluginRect);
            const dist = distance(dragCenter, pluginCenter);

            if (dist < COLLISION_DISTANCE || rectOverlap(dragRect, pluginRect)) {
                // Calculate collision vector
                const dx = pluginCenter.x - dragCenter.x;
                const dy = pluginCenter.y - dragCenter.y;
                const magnitude = Math.sqrt(dx * dx + dy * dy) || 1;

                // Normalize and apply bounce force
                const forceX = (dx / magnitude) * BOUNCE_FORCE * Math.abs(dragVelocity.x);
                const forceY = (dy / magnitude) * BOUNCE_FORCE * Math.abs(dragVelocity.y);

                // Create toss effect
                tossPlugin(plugin, forceX, forceY);

                // Apply feedback to drag element
                dragVelocity.x *= -0.3;
                dragVelocity.y *= -0.3;

                // Visual feedback
                plugin.style.transform = 'scale(0.98)';
                setTimeout(() => {
                    plugin.style.transform = '';
                }, 150);
            }
        });
    }

    function tossPlugin(plugin, forceX, forceY) {
        // Prevent multiple tosses on the same element
        if (plugin.classList.contains('being-tossed')) return;

        plugin.classList.add('being-tossed');

        let velocity = { x: forceX, y: forceY };
        let position = { x: 0, y: 0 };

        function animateToss() {
            // Apply physics
            velocity.x *= FRICTION;
            velocity.y *= FRICTION;

            position.x += velocity.x;
            position.y += velocity.y;

            // Apply transform
            plugin.style.transform = `translate(${position.x}px, ${position.y}px) scale(${1 - Math.abs(velocity.x + velocity.y) * 0.001})`;

            // Continue animation if velocity is significant
            if (Math.abs(velocity.x) > MIN_VELOCITY || Math.abs(velocity.y) > MIN_VELOCITY) {
                requestAnimationFrame(animateToss);
            } else {
                // Animation complete
                plugin.style.transform = '';
                plugin.classList.remove('being-tossed');
            }
        }

        animateToss();
    }

    function findDropTarget(x, y) {
        const plugins = $$('.plugin', mosaic).filter(p => p !== dragEl && !p.classList.contains('being-tossed'));
        let closestPlugin = null;
        let closestDistance = Infinity;
        let insertBefore = true;

        plugins.forEach(plugin => {
            const rect = plugin.getBoundingClientRect();
            const center = getCenter(rect);
            const dist = distance({ x, y }, center);

            if (dist < closestDistance) {
                closestDistance = dist;
                closestPlugin = plugin;
                insertBefore = y < center.y;
            }
        });

        return { plugin: closestPlugin, before: insertBefore };
    }

    // Enhanced drag event handlers
    on(mosaic, 'dragstart', (e) => {
        const handle = e.target.closest('.drag-handle');
        if (!handle) { e.preventDefault(); return; }

        dragEl = handle.closest('.plugin');
        if (!dragEl) { e.preventDefault(); return; }

        // Calculate offset from mouse to element top-left
        const rect = dragEl.getBoundingClientRect();
        dragOffset.x = e.clientX - rect.left;
        dragOffset.y = e.clientY - rect.top;

        // Initialize drag state
        dragStartTime = now();
        lastDragPos = { x: e.clientX, y: e.clientY };
        dragVelocity = { x: 0, y: 0 };
        isDragging = true;

        // Create drag proxy
        dragProxy = createDragProxy(dragEl);
        updateDragProxyPosition(e);

        // Style original element
        dragEl.classList.add('dragging');
        dragEl.style.opacity = '0.3';

        // Hide default ghost
        const img = new Image();
        img.src = 'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"></svg>';
        e.dataTransfer.setDragImage(img, 0, 0);
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', getId(dragEl));
    });

    // Global mouse move for smooth proxy following
    on(document, 'dragover', (e) => {
        if (!isDragging || !dragProxy) return;
        e.preventDefault();
        updateDragProxyPosition(e);
    });

    on(mosaic, 'dragover', (e) => {
        if (!dragEl || !isDragging) return;
        e.preventDefault();
    });

    on(mosaic, 'drop', (e) => {
        if (!dragEl || !isDragging) return;
        e.preventDefault();

        const dropTarget = findDropTarget(e.clientX, e.clientY);

        if (dropTarget.plugin) {
            if (dropTarget.before) {
                mosaic.insertBefore(dragEl, dropTarget.plugin);
            } else {
                mosaic.insertBefore(dragEl, dropTarget.plugin.nextSibling);
            }
        }

        cleanupDrag();
        persistOrder();
        resizeAll();
    });

    on(document, 'dragend', (e) => {
        if (!isDragging) return;
        cleanupDrag();
        resizeAll();
    });

    function cleanupDrag() {
        if (dragEl) {
            dragEl.classList.remove('dragging');
            dragEl.style.opacity = '';
        }

        if (dragProxy) {
            dragProxy.remove();
            dragProxy = null;
        }

        if (animationFrame) {
            cancelAnimationFrame(animationFrame);
            animationFrame = null;
        }

        dragEl = null;
        isDragging = false;
        dragVelocity = { x: 0, y: 0 };
    }

    /* ============ Keyboard navigation ============ */
    on(mosaic, 'keydown', (e) => {
        const cur = e.target.closest('.plugin');
        if (!cur) return;
        const items = $$('.plugin', mosaic);
        const idx = items.indexOf(cur);
        if (idx < 0) return;

        const focusItem = (i) => { items[i]?.focus({ preventScroll:false }); };

        if (['ArrowRight','ArrowDown','ArrowLeft','ArrowUp','Home','End','Enter'].includes(e.key)) e.preventDefault();
        if (e.key === 'ArrowRight') focusItem(clamp(idx+1, 0, items.length-1));
        if (e.key === 'ArrowLeft')  focusItem(clamp(idx-1, 0, items.length-1));
        if (e.key === 'ArrowDown')  focusItem(clamp(idx+1, 0, items.length-1));
        if (e.key === 'ArrowUp')    focusItem(clamp(idx-1, 0, items.length-1));
        if (e.key === 'Home')       focusItem(0);
        if (e.key === 'End')        focusItem(items.length-1);
        if (e.key === 'Enter')      expand(cur);
    });

    /* ============ Card hover tilt ============ */
    $$('.plugin', mosaic).forEach(el => {
        let rId = 0;
        on(el, 'pointermove', (e) => {
            if (el.classList.contains('dragging') || el.classList.contains('being-tossed')) return;

            const rect = el.getBoundingClientRect();
            const x = (e.clientX - rect.left) / rect.width  - .5;
            const y = (e.clientY - rect.top ) / rect.height - .5;
            cancelAnimationFrame(rId);
            rId = raf(() => {
                el.style.setProperty('--tiltX', (y * -6).toFixed(2));
                el.style.setProperty('--tiltY', (x *  6).toFixed(2));
                el.classList.add('tilting');
            });
        });
        const reset = () => {
            if (!el.classList.contains('dragging') && !el.classList.contains('being-tossed')) {
                el.classList.remove('tilting');
                el.style.removeProperty('--tiltX');
                el.style.removeProperty('--tiltY');
            }
        };
        on(el, 'pointerleave', reset);
        on(el, 'blur', reset);
    });

    /* ============ Copy-on-click for info rows ============ */
    $$('.info-item').forEach(item => {
        item.title = 'Click to copy value';
        on(item, 'click', async () => {
            const val = $('.info-value', item)?.innerText?.trim();
            if (!val) return;
            try { await navigator.clipboard.writeText(val); toast('Copied: ' + val); }
            catch { toast('Copy failed'); }
        });
    });

    /* ============ Reveal on scroll ============ */
    const io = new IntersectionObserver(entries => {
        entries.forEach(e => e.target.classList.toggle('reveal', e.isIntersecting));
    }, { threshold: 0.08 });
    $$('.plugin', mosaic).forEach(el => io.observe(el));

    /* ============ Status helpers ============ */
    // JS status
    $('#js-status')?.classList.add('status-online');
    if ($('#js-text')) $('#js-text').textContent = 'Enabled';

    // Local storage status
    const hasStorage = (() => {
        try { localStorage.setItem('_t','1'); localStorage.removeItem('_t'); return true; } catch { return false; }
    })();
    if (hasStorage) {
        $('#storage-status')?.classList.add('status-online');
        if ($('#storage-text')) $('#storage-text').textContent = 'Available';
    } else {
        $('#storage-status')?.classList.add('status-offline');
        if ($('#storage-text')) $('#storage-text').textContent = 'Unavailable';
    }

    // "Last updated" clock (updates every second)
    const last = $('#last-updated');
    if (last) {
        const tick = () => {
            const d = new Date();
            last.textContent = [d.getHours(), d.getMinutes(), d.getSeconds()]
                .map(n => String(n).padStart(2,'0')).join(':');
        };
        tick(); setInterval(tick, 1000);
    }

    // Simple uptime ticker if an "Uptime:" row exists and contains minutes/seconds
    const uptimeEl = Array.from($$('.info-item')).find(i => /Uptime:/i.test(i.textContent));
    if (uptimeEl) {
        let secs = 0;
        const val = $('.info-value', uptimeEl);
        setInterval(() => {
            secs++;
            const m = Math.floor(secs/60), s = secs%60, h = Math.floor(m/60), mm = m%60;
            const txt = h ? `${h}h ${mm}m` : (m ? `${m}m ${s}s` : `${s}s`);
            if (val) val.textContent = txt;
        }, 1000);
    }

    /* hook for websocket.js to update ws status safely */
    window.updateWSStatus = function (ok, text) {
        const dot = $('#ws-status'), label = $('#ws-text');
        if (dot) {
            dot.classList.remove('status-online','status-offline','status-loading');
            dot.classList.add(ok === true ? 'status-online' : ok === false ? 'status-offline' : 'status-loading');
        }
        if (label && text) label.textContent = text;
    };

    /* ============ Initial layout pass ============ */
    // Restore pinned flags
    const pinned = storage.get('mosaic.pinned', {});
    $$('.plugin', mosaic).forEach(el => {
        if (pinned[el.id]) el.dataset.pinned = '1';
    });
    reorderByPinned();

    // Buttons ripple
    $$('.plugin-btn, .icon-btn, .meme-refresh-btn, .social-link').forEach(b => on(b, 'click', ripple));

    // Double-click header to collapse
    $$('.plugin .plugin-header').forEach(h => on(h, 'dblclick', () => toggleCollapse(h.closest('.plugin'))));

    // Persist pin/unpin
    on(mosaic, 'click', (e) => {
        const b = e.target.closest('.plugin-btn[data-action="pin"]');
        if (!b) return;
        const el = b.closest('.plugin');
        const map = storage.get('mosaic.pinned', {});
        map[el.id] = el.dataset.pinned === '1';
        storage.set('mosaic.pinned', map);
    });

    // Finally compute layout
    raf(resizeAll);

    /* ============ Coding Stats Collapsible Sections ============ */
    function initCodingStats() {
        // Handle section toggle buttons with event delegation
        on(document, 'click', (e) => {
            const toggleBtn = e.target.closest('.section-toggle');
            if (!toggleBtn) return;

            const targetId = toggleBtn.dataset.target;
            if (!targetId) return;

            const content = document.getElementById(targetId);
            const icon = toggleBtn.querySelector('.toggle-icon');

            if (!content || !icon) return;

            e.preventDefault();
            e.stopPropagation();

            const isCollapsed = content.classList.contains('collapsed');

            if (isCollapsed) {
                // Expand
                content.classList.remove('collapsed');
                toggleBtn.setAttribute('aria-expanded', 'true');
                icon.textContent = '▼';
            } else {
                // Collapse
                content.classList.add('collapsed');
                toggleBtn.setAttribute('aria-expanded', 'false');
                icon.textContent = '▶';
            }

            // Trigger layout recalculation after animation
            setTimeout(() => {
                const plugin = toggleBtn.closest('.plugin');
                if (plugin) {
                    resizeItem(plugin);
                }
            }, 300);
        });
    }

    function initCodingSectionsState() {
        // Initialize all coding stats sections with proper state
        const codeSection = $('.code-section');
        if (!codeSection) return;

        const toggleButtons = $$('.section-toggle', codeSection);
        toggleButtons.forEach(button => {
            const targetId = button.dataset.target;
            const content = document.getElementById(targetId);
            const icon = button.querySelector('.toggle-icon');

            if (!content || !icon) return;

            // Ensure collapsed state is set correctly
            const isCollapsed = content.classList.contains('collapsed');

            button.setAttribute('aria-expanded', isCollapsed ? 'false' : 'true');
            icon.textContent = isCollapsed ? '▶' : '▼';

            // Force the visual state to match
            if (isCollapsed) {
                content.style.maxHeight = '0';
                content.style.paddingTop = '0';
                content.style.opacity = '0';
                content.style.visibility = 'hidden';
            }
        });
    }

// Initialize immediately
    setTimeout(() => {
        initCodingStats();
        initCodingSectionsState();
    }, 100);
})();