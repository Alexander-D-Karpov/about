(function(){
    'use strict';

    let isShuffling = false;

    const $  = (q, c=document) => c.querySelector(q);
    const $$ = (q, c=document) => Array.from(c.querySelectorAll(q));
    const on = (el, ev, fn, opts) => el && el.addEventListener(ev, fn, opts);

    // Capture image errors, including those from cards inserted over websocket.
    // Limit retries per element so a permanently missing image does not loop.
    function retryProjectImage(img) {
        if (!(img instanceof HTMLImageElement) || !img.classList.contains('project-image__photo')) return;
        const attempt = Number(img.dataset.retries || 0);
        if (attempt >= 3) return;
        img.dataset.retries = String(attempt + 1);
        setTimeout(() => {
            if (!img.isConnected) return;
            const url = new URL(img.src, document.baseURI);
            url.searchParams.set('_retry', String(Date.now()));
            img.src = url.href;
        }, 1000 * Math.pow(2, attempt));
    }
    document.addEventListener('error', (event) => retryProjectImage(event.target), true);
    const retryAlreadyFailedImages = () => document.querySelectorAll('.project-image__photo').forEach((img) => {
        if (img.complete && img.naturalWidth === 0) retryProjectImage(img);
    });
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', retryAlreadyFailedImages);
    else retryAlreadyFailedImages();

    // Animate from the card's own image, without changing the masonry layout.
    // Request the original only on interaction; display the thumbnail meanwhile.
    let projectPreview, projectPreviewOwner, projectPreviewVersion = 0;
    function setPreviewRect(preview, rect) {
        preview.style.left = `${rect.left}px`;
        preview.style.top = `${rect.top}px`;
        preview.style.width = `${rect.width}px`;
        preview.style.height = `${rect.height}px`;
    }
    function hideProjectPreview() {
        projectPreviewVersion++;
        const preview = projectPreview;
        const owner = projectPreviewOwner;
        projectPreviewOwner = null;
        projectPreview = null;
        if (!preview) return;
        if (owner?.isConnected) setPreviewRect(preview, owner.getBoundingClientRect());
        preview.style.opacity = '0';
        setTimeout(() => preview.remove(), 250);
    }
    function showProjectPreview(owner) {
        if (!owner || owner === projectPreviewOwner) return;
        hideProjectPreview();
        const photo = owner.querySelector('.project-image__photo');
        if (!photo) return;
        let original;
        try {
            original = new URL(owner.dataset.original || photo.src, document.baseURI);
            if (!['http:', 'https:'].includes(original.protocol)) return;
        } catch { return; }
        projectPreviewOwner = owner;
        projectPreview = document.createElement('div');
        projectPreview.className = 'project-image-preview';
        projectPreview.setAttribute('aria-hidden', 'true');
        const img = document.createElement('img');
        img.alt = '';
        img.src = photo.currentSrc || photo.src;
        projectPreview.appendChild(img);
        document.body.appendChild(projectPreview);
        const preview = projectPreview;
        const rect = owner.getBoundingClientRect();
        setPreviewRect(preview, rect);
        // Grow around the original card, clamping only when near a screen edge.
        const ratio = photo.naturalWidth && photo.naturalHeight ? photo.naturalWidth / photo.naturalHeight : rect.width / rect.height;
        let width = Math.min(rect.width * 1.9, 720, window.innerWidth - 24);
        let height = width / ratio;
        if (height > window.innerHeight * .72) {
            height = window.innerHeight * .72;
            width = height * ratio;
        }
        const left = Math.max(12, Math.min(rect.left + (rect.width - width) / 2, window.innerWidth - width - 12));
        const top = Math.max(12, Math.min(rect.top + (rect.height - height) / 2, window.innerHeight - height - 12));
        requestAnimationFrame(() => requestAnimationFrame(() => {
            if (projectPreview === preview) setPreviewRect(preview, {left, top, width, height});
        }));
        const version = projectPreviewVersion;
        const full = new Image();
        full.onload = () => { if (projectPreviewVersion === version) img.src = full.src; };
        full.src = original.href;
    }
    document.addEventListener('pointerover', (event) => {
        if (event.pointerType !== 'mouse' && event.pointerType !== 'pen') return;
        showProjectPreview(event.target.closest?.('.project-image'));
    });
    document.addEventListener('pointerout', (event) => {
        if (projectPreviewOwner && projectPreviewOwner.contains(event.target) && !projectPreviewOwner.contains(event.relatedTarget)) hideProjectPreview();
    });
    document.addEventListener('focusin', (event) => {
        if (event.target.matches?.('.project-image:focus-visible')) showProjectPreview(event.target);
    });
    document.addEventListener('focusout', (event) => { if (event.target === projectPreviewOwner) hideProjectPreview(); });
    document.addEventListener('click', (event) => {
        const owner = event.target.closest?.('.project-image');
        if (!owner) { hideProjectPreview(); return; }
        if (window.matchMedia('(hover: none)').matches) {
            if (owner === projectPreviewOwner) hideProjectPreview();
            else showProjectPreview(owner);
        }
    });
    document.addEventListener('keydown', (event) => { if (event.key === 'Escape') hideProjectPreview(); });
    window.addEventListener('scroll', hideProjectPreview, {passive: true});
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

    const isInteractive = (node) => !!node.closest('button, a, input, select, textarea, [contenteditable], .plugin-btn');

    function ensureProjectsAlwaysLast(){
        const mosaic = window.mosaicUtils?.getMosaic();
        if (!mosaic) return;

        const items = Array.from(mosaic.querySelectorAll('.projects-section, .projects-section.plugin'))
            .map(n => n.closest('.plugin') || n);

        items.forEach(node => {
            if (node.parentElement === mosaic && node !== mosaic.lastElementChild) {
                mosaic.appendChild(node);
            }
        });

        $$('.plugin', mosaic).forEach((plugin, idx) => {
            if (plugin.classList.contains('projects-section')) plugin.dataset.order = '9999';
            else if (!plugin.dataset.order || +plugin.dataset.order >= 9999) plugin.dataset.order = String(idx);
        });
    }

    function initHealthInteractions() {
        const healthSection = document.querySelector('.health-section');
        if (!healthSection) return;

        healthSection.querySelectorAll('.health-card').forEach(card => {
            card.style.cursor = 'default';
        });
    }

    // Dice button / inline onclick="refreshMeme()" both call this. It asks the
    // server for a new meme; the server broadcasts `meme_update` over the
    // websocket, and websocket.js swaps the image and re-packs the masonry.
    // A single in-flight guard plus the button's own 1.5s disable throttle it.
    let memeRefreshInFlight = false;
    window.refreshMeme = function refreshMeme(){
        if (memeRefreshInFlight) return;
        memeRefreshInFlight = true;
        fetch('/api/meme/refresh', { method: 'POST', headers: { 'Accept': 'application/json' } })
            .catch(() => {})
            .finally(() => { memeRefreshInFlight = false; });
    };

    function initMeme(){
        const sec = document.querySelector('.meme-section');
        if (!sec) return;

        let btn = sec.querySelector('.meme-refresh-btn');
        if (!btn){
            const header = sec.querySelector('.plugin-header');
            if (header){
                const toolbar = header.querySelector('.plugin-toolbar');
                if (toolbar) {
                    btn = document.createElement('button');
                    btn.className = 'icon-btn plugin-btn meme-refresh-btn';
                    btn.type = 'button';
                    btn.title = 'Random Meme';
                    btn.setAttribute('aria-label', 'Get random meme');
                    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
                    svg.setAttribute('viewBox', '0 0 24 24');
                    svg.setAttribute('width', '16');
                    svg.setAttribute('height', '16');
                    svg.setAttribute('fill', 'currentColor');
                    svg.innerHTML = '<path d="M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zm7 4a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3zm-4 4a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3zm8 0a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3zm-4 4a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3z"/>';
                    btn.appendChild(svg);
                    toolbar.appendChild(btn);
                }
            }
        }

        const doRefresh = () => {
            if (typeof window.refreshMeme === 'function') window.refreshMeme();
        };

        if (btn && btn.dataset.listenerAttached !== '1') {
            btn.dataset.listenerAttached = '1';
            btn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                if (btn.disabled) return;
                btn.disabled = true;
                const svg = btn.querySelector('svg');
                if (svg) svg.style.animation = 'spin 0.8s linear infinite';
                doRefresh();
                setTimeout(() => {
                    btn.disabled = false;
                    if (svg) svg.style.animation = '';
                }, 1500);
            }, {passive: false});
        }
    }

    function initVisitors(){ $$('.visitors-section .visitor-stat').forEach(s => s.style.cursor='pointer'); }

    function initServices(){
        const sec = $('.services-section');
        if (!sec) return;

        $$('.service-item', sec).forEach(card => {
            const url = card.dataset.url;
            if (!url) return;

            const overlay = card.querySelector('.card-overlay');
            if (overlay) {
                overlay.addEventListener('click', (e) => {
                    e.stopPropagation();
                });
            }

            // Hover handled by CSS

            if (!overlay) {
                card.style.cursor = 'pointer';
                card.addEventListener('click', (e) => {
                    if (e.target.closest('a, button')) return;
                    window.open(url, '_blank');
                });
            }
        });
    }

    function initWebring(){
        const sec = $('.webring-section'); if (!sec) return;

        // Deliberately no click handler on .webring-home: the anchor has a real
        // href and target="_blank", so the browser opens it. plugins.js bound an
        // identical preventDefault+window.open here, and with both live the ring
        // home opened in two tabs.

        document.addEventListener('keydown', (e) => {
            if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.isContentEditable) return;

            const sec = document.querySelector('.webring-section');
            if (!sec) return;

            const rect = sec.getBoundingClientRect();
            const inView = rect.top < window.innerHeight && rect.bottom > 0;
            if (!inView) return;

            e.preventDefault();
            const link = sec.querySelector(e.key === 'ArrowLeft' ? '.webring-prev' : '.webring-next');
            if (link && link.href) {
                window.location.href = link.href;
            }
        });
    }

    function initNeofetchSwitch(){
        const sec = document.querySelector('.neofetch-section');
        if (!sec) return;

        autoScaleAllNeofetch();

        sec.querySelectorAll('.machine-btn').forEach(btn => {
            if (btn.dataset.listenerAttached === '1') return;
            btn.dataset.listenerAttached = '1';

            btn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();

                sec.querySelectorAll('.machine-btn').forEach(b => {
                    b.classList.remove('active');
                    b.removeAttribute('data-active');
                });
                btn.classList.add('active');
                btn.setAttribute('data-active','true');

                const idx = btn.dataset.machine || '0';
                sec.querySelectorAll('.neofetch-output').forEach(o => o.style.display = 'none');
                const out = sec.querySelector(`#neofetch-${idx}`);
                if (out) out.style.display = 'block';

                // Size the terminal and repack in the same frame as the click;
                // the old rAF + 50ms + debounced-observer chain resized the
                // card in visible steps a beat after the switch.
                autoScaleAllNeofetch();
                if (window.mosaicUtils) {
                    window.mosaicUtils.notifyContentChanged(sec);
                    window.mosaicUtils.fullRepack();
                }
            });
        });

        window.addEventListener('resize', throttle(autoScaleAllNeofetch, 100));
    }

    function autoScaleAllNeofetch(){
        document.querySelectorAll('.neofetch-output').forEach(out => {
            if (out.style.display === 'none') return;
            const term = out.querySelector('.terminal');
            const pre = out.querySelector('.neofetch-pre');
            if (!term || !pre) return;

            pre.style.transform = '';

            const maxW = term.clientWidth - 20;
            const needW = pre.scrollWidth;
            const scale = needW > maxW ? Math.max(0.6, maxW/needW) : 1;
            term.style.setProperty('--neo-scale', String(scale));

            const headerH = (out.querySelector('.terminal-header')?.offsetHeight || 0);
            const paletteH = (out.querySelector('.color-palette')?.offsetHeight || 0);
            const bodyPad = 20;
            const scaledH = Math.ceil(pre.scrollHeight * scale);
            term.style.height = (headerH + bodyPad + scaledH + paletteH + 8) + 'px';
        });
    }



    function shufflePlugins() {
        if (isShuffling) return;

        const mosaic = document.querySelector('.mosaic');
        if (!mosaic) return;

        const plugins = Array.from(mosaic.querySelectorAll('.plugin'));
        if (plugins.length < 2) return;

        isShuffling = true;

        const originalPositions = plugins.map(plugin => {
            const rect = plugin.getBoundingClientRect();
            return {el: plugin, x: rect.left, y: rect.top};
        });

        for (let i = plugins.length - 1; i > 0; i--) {
            const j = Math.floor(Math.random() * (i + 1));
            [plugins[i], plugins[j]] = [plugins[j], plugins[i]];
        }

        originalPositions.forEach(({el, x, y}) => {
            const newRect = el.getBoundingClientRect();
            const deltaX = x - newRect.left;
            const deltaY = y - newRect.top;

            el.style.transition = 'none';
            el.style.transform = `translate(${deltaX}px, ${deltaY}px)`;
        });

        requestAnimationFrame(() => {
            plugins.forEach(plugin => mosaic.appendChild(plugin));

            requestAnimationFrame(() => {
                originalPositions.forEach(({el}) => {
                    el.style.transition = 'transform 0.6s cubic-bezier(0.34, 1.56, 0.64, 1)';
                    el.style.transform = '';
                });

                setTimeout(() => {
                    originalPositions.forEach(({el}) => {
                        el.style.transition = '';
                    });

                    if (window.mosaicUtils) {
                        window.mosaicUtils.resizeAll();
                    }

                    isShuffling = false;
                }, 700);
            });
        });
    }

    // The connection dot resets the window layout: drops the order and widths
    // saved from dragging/resizing (windowManager.js) and reloads, so every
    // window is back where and how wide the server puts it.
    function resetWindowLayout() {
        try {
            ['mosaic.order', 'mosaic.widths', 'mosaic.spans'].forEach((k) => localStorage.removeItem(k));
        } catch {}
        location.reload();
    }

    function initEasterEgg() {
        const statusIndicator = document.getElementById('connection-status');
        if (!statusIndicator) return;

        statusIndicator.addEventListener('click', function (e) {
            e.preventDefault();
            e.stopPropagation();
            resetWindowLayout();
        });

        statusIndicator.style.cursor = 'pointer';
        statusIndicator.title = 'Reset window layout';
        statusIndicator.setAttribute('role', 'button');
        statusIndicator.setAttribute('aria-label', 'Reset window layout');
        statusIndicator.tabIndex = 0;
        statusIndicator.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                resetWindowLayout();
            }
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initEasterEgg);
    } else {
        setTimeout(initEasterEgg, 100);
    }

    window.shufflePlugins = shufflePlugins;
    window.resetWindowLayout = resetWindowLayout;

    function init(){
        const waitForMosaic = () => {
            if (!window.mosaicUtils?.getMosaic()) {
                setTimeout(waitForMosaic, 50);
                return;
            }

            const waitForSections = (attempt = 0) => {
                const techSection = document.querySelector('.tech-section');
                const projectsSection = document.querySelector('.projects-section');

                if ((!techSection || !projectsSection) && attempt < 20) {
                    setTimeout(() => waitForSections(attempt + 1), 100);
                    return;
                }

                setTimeout(() => {
                    ensureProjectsAlwaysLast();
                    initMeme();
                    initVisitors();
                    initServices();
                    initWebring();
                    initNeofetchSwitch();
                    initHealthInteractions();
                }, 80);
            };

            waitForSections();
        };

        waitForMosaic();
    }

    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();
})();

// Balanced grids: auto-fill left lone items on the last row (six photo
// folders in five columns, twelve links as 7 + 5). For each grid below this
// picks the column count that splits the items into the fewest, evenest rows
// that fit the current width (six -> 3 + 3, twelve -> 6 + 6) and exposes it
// as a CSS variable the grid's template reads.
(function () {
    'use strict';
    if (!window.ResizeObserver) return;

    const phone = () => window.innerWidth <= 720;
    const GRIDS = [
        {sel: '.photos-grid', prop: '--photo-cols', min: () => 112, gap: () => 14},
        {sel: '.social-section .social-links', prop: '--link-cols', min: () => (phone() ? 44 : 36), gap: () => (phone() ? 8 : 6)},
    ];

    function balance(grid) {
        const spec = GRIDS.find((g) => grid.matches(g.sel));
        if (!spec) return;
        const n = grid.children.length;
        const w = grid.clientWidth;
        if (!n || !w) return;
        const gap = spec.gap(), min = spec.min();
        const maxCols = Math.max(1, Math.floor((w + gap) / (min + gap)));
        const rows = Math.ceil(n / maxCols);
        const cols = Math.ceil(n / rows);
        if (grid.style.getPropertyValue(spec.prop) !== String(cols)) {
            grid.style.setProperty(spec.prop, String(cols));
        }
    }

    const ro = new ResizeObserver((entries) => entries.forEach((e) => balance(e.target)));

    function attach() {
        GRIDS.forEach((spec) => document.querySelectorAll(spec.sel).forEach((g) => {
            if (g.dataset.balanced) return;
            g.dataset.balanced = '1';
            ro.observe(g);
            balance(g);
        }));
    }

    function init() {
        attach();
        // sections are swapped wholesale on live updates
        const mosaic = document.querySelector('.mosaic') || document.body;
        new MutationObserver(attach).observe(mosaic, {childList: true});
    }

    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();
})();
