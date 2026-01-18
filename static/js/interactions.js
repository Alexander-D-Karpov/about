(function(){
    'use strict';

    let isShuffling = false;

    const $  = (q, c=document) => c.querySelector(q);
    const $$ = (q, c=document) => Array.from(c.querySelectorAll(q));
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

    let currentFilter = null;
    let filterPopup;

    function createFilterPopup(){
        if (filterPopup) return filterPopup;

        filterPopup = document.createElement('div');
        filterPopup.className = 'tech-filter-popup';

        filterPopup.innerHTML = `
        <span class="filter-icon" style="font-size:18px;" aria-hidden="true">🔧</span>
        <span class="filter-text">Filtering:</span>
        <strong class="filter-tech" style="color:#7aa2ff;"></strong>
        <button class="clear-filter-btn" type="button" aria-label="Clear project filter"
            style="margin-left:4px;padding:6px 12px;border-radius:8px;border:1px solid rgba(255,255,255,.2);background:rgba(255,107,107,.9);color:white;cursor:pointer;font-weight:700;font-size:12px;transition:all .2s ease;">
            Clear ✕
        </button>
    `;

        const clearBtn = filterPopup.querySelector('.clear-filter-btn');
        clearBtn.addEventListener('mouseover', () => {
            clearBtn.style.background = 'rgba(220,38,38,1)';
            clearBtn.style.transform = 'scale(1.05)';
        });
        clearBtn.addEventListener('mouseout', () => {
            clearBtn.style.background = 'rgba(255,107,107,.9)';
            clearBtn.style.transform = 'scale(1)';
        });
        clearBtn.addEventListener('click', (e) => {
            e.preventDefault();
            e.stopPropagation();
            clearTechFilter();
        }, {passive: false});

        document.body.appendChild(filterPopup);
        return filterPopup;
    }

    function showFilterPopup(name){
        const p = createFilterPopup();
        const label = p.querySelector('.filter-tech');
        if (label) label.textContent = name;

        p.classList.add('show');
        p.style.transition = 'opacity 200ms ease, transform 200ms ease';
        p.style.transform = 'translateX(-50%) translateY(0)';
    }

    function hideFilterPopup() {
        if (!filterPopup) return;
        filterPopup.classList.remove('show');
    }

    function clearTechFilter(){
        const projectsSection = document.querySelector('.projects-section, .projects-section.plugin, .plugin.projects-section');
        if (!projectsSection) {
            console.warn('Projects section not found for clearing');
            return;
        }

        projectsSection.querySelectorAll('.project-card').forEach(card => {
            card.style.transition = 'all 0.3s ease';
            card.style.opacity = '1';
            card.style.transform = 'scale(1)';
            card.style.filter = 'none';
            card.style.outline = '';
            card.style.outlineOffset = '';
        });

        document.querySelectorAll('.tech-item.filtered').forEach(x => x.classList.remove('filtered'));
        currentFilter = null;

        hideFilterPopup();

        if (window.mosaicUtils) window.mosaicUtils.resizeAll();
    }

    function applyTechFilter(name){
        if (!name) return;

        const techSection = document.querySelector('.tech-section, .tech-section.plugin, .plugin.tech-section');
        const projectsSection = document.querySelector('.projects-section, .projects-section.plugin, .plugin.projects-section');

        if (!projectsSection) {
            console.warn('Projects section not found');
            return;
        }

        console.log(`Applying filter for: ${name}`);
        currentFilter = name;

        if (techSection) {
            techSection.querySelectorAll('.tech-item').forEach(item => {
                const label = item.querySelector('.tech-name')?.textContent ||
                    item.title ||
                    item.querySelector('img')?.alt || '';

                if (label.toLowerCase() === name.toLowerCase()) {
                    item.classList.add('filtered');
                } else {
                    item.classList.remove('filtered');
                }
            });
        }

        let matchCount = 0;
        const matchingCards = [];

        projectsSection.querySelectorAll('.project-card').forEach(card => {
            const tags = Array.from(card.querySelectorAll('.tech-tag'));
            const tagTexts = tags.map(t => t.textContent.trim().toLowerCase());
            const searchName = name.toLowerCase();

            const isMatch = tagTexts.some(tag => tag === searchName);

            card.style.transition = 'all 0.3s ease';

            if (isMatch) {
                card.style.opacity = '1';
                card.style.transform = 'scale(1)';
                card.style.filter = 'none';
                matchingCards.push(card);
                matchCount++;
            } else {
                card.style.opacity = '0.25';
                card.style.transform = 'scale(0.96)';
                card.style.filter = 'grayscale(70%)';
            }
        });

        console.log(`Found ${matchCount} matching projects`);

        if (matchCount > 0) {
            showFilterPopup(name);

            projectsSection.scrollIntoView({
                behavior: 'smooth',
                block: 'start'
            });

            setTimeout(() => {
                matchingCards.forEach(card => {
                    card.style.outline = '2px solid var(--accent)';
                    card.style.outlineOffset = '4px';
                });

                setTimeout(() => {
                    matchingCards.forEach(card => {
                        card.style.outline = '';
                        card.style.outlineOffset = '';
                    });
                }, 2000);
            }, 500);
        } else {
            console.warn(`No projects found matching "${name}"`);
            clearTechFilter();
        }

        if (window.mosaicUtils) {
            setTimeout(() => window.mosaicUtils.resizeAll(), 100);
        }
    }

    function initTechFiltering() {
        const techSection = document.querySelector('.tech-section, .tech-section.plugin, .plugin.tech-section');
        const projectsSection = document.querySelector('.projects-section, .projects-section.plugin, .plugin.projects-section');

        if (!techSection) {
            console.warn('Tech section not found. Available sections:',
                Array.from(document.querySelectorAll('[class*="section"]')).map(el => el.className));
            return;
        }

        if (!projectsSection) {
            console.warn('Projects section not found. Available sections:',
                Array.from(document.querySelectorAll('[class*="section"]')).map(el => el.className));
            return;
        }

        techSection.querySelectorAll('.tech-item').forEach(item => {
            if (item.dataset.techFilterAttached === '1') return;
            item.dataset.techFilterAttached = '1';

            const name = item.querySelector('.tech-name')?.textContent ||
                item.title ||
                item.querySelector('img')?.alt || '';

            if (!name) {
                console.warn('Tech item without name found:', item);
                return;
            }

            item.style.cursor = 'pointer';

            item.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                console.log(`Tech item clicked: ${name}`);
                currentFilter = name;
                applyTechFilter(name);
            }, {passive: false});

            item.addEventListener('mouseenter', () => {
                if (!item.classList.contains('filtered')) {
                    item.style.transform = 'translateY(-2px)';
                }
            });

            item.addEventListener('mouseleave', () => {
                if (!item.classList.contains('filtered')) {
                    item.style.transform = '';
                }
            });
        });

        projectsSection.querySelectorAll('.tech-tag').forEach(tag => {
            if (tag.dataset.techFilterAttached === '1') return;
            tag.dataset.techFilterAttached = '1';

            tag.style.cursor = 'pointer';

            tag.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();

                const name = tag.textContent.trim();
                console.log(`Tech tag clicked: ${name}`);

                currentFilter = name;

                projectsSection.scrollIntoView({
                    behavior: 'smooth',
                    block: 'start'
                });

                setTimeout(() => {
                    applyTechFilter(name);
                }, 300);
            }, {passive: false});

            tag.addEventListener('mouseenter', () => {
                tag.style.transform = 'scale(1.05)';
            });

            tag.addEventListener('mouseleave', () => {
                tag.style.transform = 'scale(1)';
            });
        });

        if (!window.applyTechFilter) window.applyTechFilter = applyTechFilter;
        if (!window.clearTechFilter) window.clearTechFilter = clearTechFilter;
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

    function initCodeToggles(){
        const sec = $('.code-section');
        if (!sec) return;

        $$('.section-toggle', sec).forEach(toggle => {
            if (toggle.dataset.listenerAttached === '1') return;
            toggle.dataset.listenerAttached = '1';

            const id = toggle.dataset.target;
            if (!id) return;

            const content = sec.querySelector('#' + id);
            const icon = toggle.querySelector('.toggle-icon');
            if (!content || !icon) return;

            if (id === 'wakatime-langs') {
                content.classList.remove('collapsed');
                icon.textContent = '▼';
                toggle.setAttribute('aria-expanded', 'true');
            }

            toggle.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();

                const willCollapse = !content.classList.contains('collapsed');
                content.classList.toggle('collapsed', willCollapse);
                icon.textContent = willCollapse ? '▶' : '▼';
                toggle.setAttribute('aria-expanded', willCollapse ? 'false' : 'true');

                if (window.mosaicUtils) {
                    window.mosaicUtils.resizeAll();
                    setTimeout(() => window.mosaicUtils.resizeAll(), 50);
                    setTimeout(() => window.mosaicUtils.resizeAll(), 150);
                    setTimeout(() => window.mosaicUtils.resizeAll(), 350);
                }
            }, { passive: false });
        });
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

    function initHealthInteractions() {
        const healthSection = document.querySelector('.health-section');
        if (!healthSection) return;

        healthSection.querySelectorAll('.health-card').forEach(card => {
            card.style.cursor = 'default';
        });
    }

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

                    // Create SVG icon for dice
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

        if (btn){
            if (btn.hasAttribute('onclick')) btn.removeAttribute('onclick');
            if (btn.dataset.listenerAttached !== '1'){
                btn.dataset.listenerAttached = '1';
                btn.addEventListener('click', (e) => {
                    e.preventDefault();
                    e.stopPropagation();

                    // Add loading state
                    const originalHTML = btn.innerHTML;
                    btn.disabled = true;
                    btn.innerHTML = '<svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor" class="loading"><circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2" fill="none" opacity="0.25"/><path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" stroke-width="2" fill="none" stroke-linecap="round"/></svg>';

                    if (typeof window.refreshMeme === 'function') {
                        window.refreshMeme();

                        setTimeout(() => {
                            btn.disabled = false;
                            btn.innerHTML = originalHTML;
                        }, 1500);
                    }
                }, { passive: false });
            }
        }

        const content = sec.querySelector('.meme-content');
        if (content) {
            content.style.cursor = 'default';
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
        if (home) on(home, 'click', (e) => {
            e.preventDefault();
            const base = sec.dataset.baseUrl;
            if (base) window.open(base, '_blank');
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

                requestAnimationFrame(autoScaleAllNeofetch);
                setTimeout(() => {
                    if (window.mosaicUtils) window.mosaicUtils.resizeAll();
                }, 50);
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

    function initEasterEgg() {
        const statusIndicator = document.getElementById('connection-status');
        if (!statusIndicator) return;

        statusIndicator.addEventListener('click', function (e) {
            if (this.classList.contains('status-online')) {
                e.preventDefault();
                e.stopPropagation();
                shufflePlugins();
            }
        });

        statusIndicator.style.cursor = 'pointer';
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initEasterEgg);
    } else {
        setTimeout(initEasterEgg, 100);
    }

    window.shufflePlugins = shufflePlugins;

    function init(){
        const waitForMosaic = () => {
            if (!window.mosaicUtils?.getMosaic()) {
                setTimeout(waitForMosaic, 50);
                return;
            }

            const waitForSections = (attempt = 0) => {
                const techSection = document.querySelector('.tech-section, .tech-section.plugin, .plugin.tech-section');
                const projectsSection = document.querySelector('.projects-section, .projects-section.plugin, .plugin.projects-section');

                if (!techSection || !projectsSection) {
                    if (attempt < 20) {
                        console.log(`Waiting for sections... (attempt ${attempt + 1})`);
                        setTimeout(() => waitForSections(attempt + 1), 100);
                        return;
                    } else {
                        console.warn('Sections not found after 20 attempts');
                    }
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
                    initHealthInteractions();

                    setTimeout(() => {
                        ensureProjectsAlwaysLast();
                        window.mosaicUtils && window.mosaicUtils.resizeAll();
                    }, 120);

                    window.mosaicUtils && window.mosaicUtils.resizeAll();
                }, 80);
            };

            waitForSections();
        };

        waitForMosaic();
    }

    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();

    window.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && currentFilter) {
            clearTechFilter();
        }
    });

})();