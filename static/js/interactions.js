(function(){
    'use strict';

    const $  = (q, c=document) => c.querySelector(q);
    const $$ = (q, c=document) => Array.from(c.querySelectorAll(q));
    const on = (el, ev, fn, opts) => el && el.addEventListener(ev, fn, opts);

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

    // --- Tech filtering (persistent) ---
    let currentFilter = null;
    let filterPopup;

    function createFilterPopup(){
        if (filterPopup) return filterPopup;

        filterPopup = document.createElement('div');
        filterPopup.className = 'tech-filter-popup';

        // Inline styles so it works without any extra CSS
        Object.assign(filterPopup.style, {
            position: 'fixed',
            top: '10px',
            left: '50%',
            transform: 'translateX(-50%)',
            zIndex: '9999',
            display: 'none',
            alignItems: 'center',
            gap: '10px',
            padding: '10px 14px',
            borderRadius: '10px',
            background: 'rgba(20,20,24,0.9)',
            color: 'var(--fg, #e5e7eb)',
            boxShadow: '0 8px 28px rgba(0,0,0,.35)',
            backdropFilter: 'blur(6px)',
            WebkitBackdropFilter: 'blur(6px)',
            border: '1px solid rgba(255,255,255,.08)',
            fontWeight: '600'
        });

        filterPopup.innerHTML = `
        <span class="filter-icon" aria-hidden="true">🔧</span>
        <span class="filter-text">Filtering projects by:</span>
        <strong class="filter-tech" style="color: var(--accent, #7aa2ff);"></strong>
        <button class="clear-filter-btn" type="button" aria-label="Clear project filter"
            style="margin-left:8px;padding:6px 10px;border-radius:8px;border:1px solid rgba(255,255,255,.15);background:transparent;color:inherit;cursor:pointer;">
            Clear
        </button>
    `;

        filterPopup.querySelector('.clear-filter-btn').addEventListener('click', clearTechFilter, { passive: true });
        document.body.appendChild(filterPopup);
        return filterPopup;
    }
    function showFilterPopup(name){
        const p = createFilterPopup();
        const label = p.querySelector('.filter-tech');
        if (label) label.textContent = name;

        // show with a tiny animation
        p.style.display = 'flex';
        p.style.opacity = '0';
        p.style.transition = 'opacity 160ms ease, transform 160ms ease';
        p.style.transform = 'translateX(-50%) translateY(-6px)';
        requestAnimationFrame(() => {
            p.style.opacity = '1';
            p.style.transform = 'translateX(-50%) translateY(0)';
        });
    }

    function matchesTech(tags, name){
        const re = new RegExp(`\\b${name.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')}\\b`, 'i');
        return tags.some(t => re.test(t.textContent.trim()));
    }
    function clearTechFilter(){
        const projectsSection = document.querySelector('.projects-section');
        if (!projectsSection) return;

        projectsSection.querySelectorAll('.project-card').forEach(card => {
            card.style.opacity = '1';
            card.style.transform = 'scale(1)';
            card.style.filter = 'none';
            card.style.transition = 'all 0.3s ease';
            card.style.outline = '';
            card.style.outlineOffset = '';
        });

        document.querySelectorAll('.tech-item.filtered').forEach(x => x.classList.remove('filtered'));
        currentFilter = null;

        if (filterPopup) {
            filterPopup.style.opacity = '0';
            filterPopup.style.transform = 'translateX(-50%) translateY(-6px)';
            setTimeout(() => { filterPopup.style.display = 'none'; }, 160);
        }

        if (window.mosaicUtils) window.mosaicUtils.resizeAll();
    }

    function applyTechFilter(name){
        const techSection = document.querySelector('.tech-section');
        const projects = document.querySelector('.projects-section');
        if (!techSection || !projects) return;

        techSection.querySelectorAll('.tech-item').forEach(it => {
            const label = it.querySelector('.tech-name')?.textContent || it.title || it.querySelector('img')?.alt || '';
            if (label.toLowerCase() === name.toLowerCase()) it.classList.add('filtered');
            else it.classList.remove('filtered');
        });

        let any = false;
        const matchingCards = [];

        projects.querySelectorAll('.project-card').forEach(card => {
            const tags = card.querySelectorAll('.tech-tag');
            const hit = Array.from(tags).some(t => new RegExp(`\\b${name.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\$&')}\\b`, 'i').test(t.textContent.trim()));

            card.style.transition = 'all 0.3s ease';
            card.style.opacity   = hit ? '1' : '0.2';
            card.style.transform = hit ? 'scale(1)' : 'scale(0.95)';
            card.style.filter    = hit ? 'none' : 'grayscale(80%)';

            if (hit) { any = true; matchingCards.push(card); }
        });

        if (any) {
            showFilterPopup(name);

            projects.scrollIntoView({ behavior:'smooth', block:'start' });

            setTimeout(() => {
                matchingCards.forEach(card => {
                    card.style.outline = '2px solid var(--accent)';
                    card.style.outlineOffset = '4px';
                });
                setTimeout(() => {
                    matchingCards.forEach(card => { card.style.outline = ''; card.style.outlineOffset = ''; });
                }, 2000);
            }, 450);
        } else {
            clearTechFilter();
        }

        if (window.mosaicUtils) window.mosaicUtils.resizeAll();
    }

    function initSteamHover(){
        const sec = $('.steam-section'); if (!sec) return;
        $$('.game-item', sec).forEach(item => {
            item.style.cursor = 'default';
            item.addEventListener('mouseenter', () => {
                item.style.transform = 'translateY(-2px)';
                item.style.boxShadow = '0 4px 12px rgba(0,0,0,.25)';
            });
            item.addEventListener('mouseleave', () => {
                item.style.transform = ''; item.style.boxShadow = '';
            });
        });
    }

    // --- Code, Music, Services and misc plugin interactions ---
    function initCodeToggles(){
        const sec = $('.code-section');
        if (!sec) return;

        $$('.section-toggle', sec).forEach(toggle => {
            if (toggle.dataset.listenerAttached === '1') return; // prevent double-binding from multiple initializers
            toggle.dataset.listenerAttached = '1';

            toggle.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();

                const id = toggle.dataset.target;
                if (!id) return;

                const content = sec.querySelector('#' + id);
                const icon = toggle.querySelector('.toggle-icon');
                if (!content || !icon) return;

                const willCollapse = !content.classList.contains('collapsed');
                content.classList.toggle('collapsed', willCollapse);
                icon.textContent = willCollapse ? '▶' : '▼';
                toggle.setAttribute('aria-expanded', willCollapse ? 'false' : 'true');

                setTimeout(() => {
                    if (window.mosaicUtils) window.mosaicUtils.resizeAll();
                }, 280);
            }, { passive: false });
        });
    }

    function initTechFiltering(){
        const tech = document.querySelector('.tech-section');
        const projects = document.querySelector('.projects-section');
        if (!tech || !projects) return;

        // Bind clicks on the main tech stack list
        tech.querySelectorAll('.tech-item').forEach(item => {
            if (item.dataset.listenerAttached === '1') return;
            item.dataset.listenerAttached = '1';

            item.style.cursor = 'pointer';
            item.addEventListener('click', () => {
                const name = item.querySelector('.tech-name')?.textContent
                    || item.title
                    || item.querySelector('img')?.alt
                    || '';
                if (!name) return;

                projects.scrollIntoView({ behavior: 'smooth', block: 'start' });
                setTimeout(() => {
                    currentFilter = name; // uses the existing module-scoped variable
                    applyTechFilter(name);
                }, 450);
            }, { passive: true });
        });

        // Bind clicks on tech tags inside each project card
        projects.querySelectorAll('.tech-tag').forEach(tag => {
            if (tag.dataset.listenerAttached === '1') return;
            tag.dataset.listenerAttached = '1';

            tag.style.cursor = 'pointer';
            tag.addEventListener('click', (e) => {
                e.stopPropagation();
                const name = tag.textContent.trim();
                projects.scrollIntoView({ behavior: 'smooth', block: 'start' });
                setTimeout(() => {
                    currentFilter = name;
                    applyTechFilter(name);
                }, 450);
            }, { passive: false });
        });

        // Expose for other scripts (plugins.js) to reuse the persistent UI instead of duplicating logic
        if (!window.applyTechFilter) window.applyTechFilter = applyTechFilter;
        if (!window.clearTechFilter) window.clearTechFilter = clearTechFilter;
    }


    function initLastFM(){
        const sec = $('.lastfm-section'); if (!sec) return;
        $$('.recent-track-item', sec).forEach(item => {
            item.style.cursor = 'pointer';
            on(item, 'mouseenter', () => item.style.background = 'rgba(255,255,255,.024)');
            on(item, 'mouseleave', () => item.style.background = '');
            on(item, 'click', () => {
                const t = $('.recent-track-name', item)?.textContent || '';
                const a = $('.recent-track-artist', item)?.textContent || '';
                if (t && a && window.playTrack) window.playTrack(`${a} ${t}`);
            });
        });
    }

    // Smooth meme refresh: hold height so layout doesn't jump while image swaps.
    function lockSectionHeight(sectionEl){
        const plugin = sectionEl.closest('.plugin') || sectionEl;
        if (!plugin) return () => {};
        const rect = plugin.getBoundingClientRect();
        const prevMin = plugin.style.minHeight;
        plugin.style.minHeight = rect.height + 'px';

        // Try to keep the viewport anchored around the meme during the swap.
        const preTop = plugin.getBoundingClientRect().top;
        const release = () => {
            plugin.style.minHeight = prevMin || '';
            // Re-anchor after update to compensate for subtle grid changes.
            const postTop = plugin.getBoundingClientRect().top;
            const dy = postTop - preTop;
            if (Math.abs(dy) > 1) window.scrollBy(0, dy);
            if (window.mosaicUtils) window.mosaicUtils.resizeAll();
        };

        // Safety release in case no images load event fires
        const t = setTimeout(release, 1000);
        return () => { clearTimeout(t); release(); };
    }


    function initMeme(){
        const sec = document.querySelector('.meme-section');
        if (!sec) return;

        let btn = sec.querySelector('.meme-refresh-btn');
        if (!btn){
            const header = sec.querySelector('.meme-header');
            if (header){
                btn = document.createElement('button');
                btn.className = 'btn btn-sm meme-refresh-btn';
                btn.type = 'button';
                btn.textContent = '🎲';
                header.appendChild(btn);
            }
        }

        if (btn){
            if (btn.hasAttribute('onclick')) btn.removeAttribute('onclick');
            if (btn.dataset.listenerAttached !== '1'){
                btn.dataset.listenerAttached = '1';
                btn.addEventListener('click', (e) => {
                    e.preventDefault();
                    if (typeof window.refreshMeme === 'function') window.refreshMeme();
                }, { passive: false });
            }
        }

        const content = sec.querySelector('.meme-content');
        if (content && content.dataset.listenerAttached !== '1'){
            content.dataset.listenerAttached = '1';
            content.style.cursor = 'pointer';
            content.addEventListener('click', (e) => {
                if (e.target.closest('button')) return;
                btn && btn.click();
            }, { passive: true });
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

            card.addEventListener('mouseenter', () => {
                if (!overlay) card.style.transform = 'translateY(-1px)';
            });
            card.addEventListener('mouseleave', () => {
                if (!overlay) card.style.transform = '';
            });

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
        const home = $('.webring-home', sec);
        if (home) on(home, 'click', (e) => { e.preventDefault(); const base = sec.dataset.baseUrl; if (base) window.open(base, '_blank'); });
    }
    function initNeofetchSwitch(){
        const sec = document.querySelector('.neofetch-section');
        if (!sec) return;

        autoScaleAllNeofetch();

        sec.querySelectorAll('.machine-btn').forEach(btn => {
            btn.addEventListener('click', () => {
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

                requestAnimationFrame(autoScaleAllNeofetch);
                setTimeout(() => { window.mosaicUtils && window.mosaicUtils.resizeAll(); }, 50);
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

    function throttle(fn, wait){
        let t=0; return function(){ const now=Date.now(); if (now-t>wait){ t=now; fn(); } };
    }
    function initAnimatedCounters(){
        const anim = (el) => {
            const text = el.textContent;
            const n = parseFloat(text.replace(/[^\d.]/g,''));
            if (isNaN(n)) return;
            const suffix = text.replace(/[\d.,]/g,'');
            const dur = 1000, steps = 30, stepVal = n/steps;
            let i = 0, cur = 0;
            const t = setInterval(() => {
                i++; cur += stepVal;
                if (i >= steps){ cur=n; clearInterval(t); }
                el.textContent = Math.floor(cur).toLocaleString() + suffix;
            }, dur/steps);
        };
        $$('.visitor-number, .stat-value').forEach(el => {
            const io = new IntersectionObserver(ents => {
                ents.forEach(en => { if (en.isIntersecting){ anim(el); io.unobserve(el); } });
            });
            io.observe(el);
        });
    }

    function init(){
        const waitForMosaic = () => {
            if (!window.mosaicUtils?.getMosaic()) {
                setTimeout(waitForMosaic, 50);
                return;
            }

            setTimeout(() => {
                ensureProjectsAlwaysLast();

                initTechFiltering();
                initCodeToggles();
                initSteamHover();
                initLastFM();
                initMeme();
                initVisitors();
                initServices();
                initWebring();
                initNeofetchSwitch();
                initAnimatedCounters();

                setTimeout(() => {
                    ensureProjectsAlwaysLast();
                    window.mosaicUtils && window.mosaicUtils.resizeAll();
                }, 120);

                window.mosaicUtils && window.mosaicUtils.resizeAll();
            }, 80);

        };

        waitForMosaic();
    }

    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();

})();
