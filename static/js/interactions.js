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
        const home = $('.webring-home', sec);
        if (home) on(home, 'click', (e) => {
            e.preventDefault();
            const base = sec.dataset.baseUrl;
            if (base) window.open(base, '_blank');
        });

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
                    initAnimatedCounters();
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
