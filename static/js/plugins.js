(function() {
    'use strict';

    const $ = (q, c = document) => c.querySelector(q);
    const $$ = (q, c = document) => Array.from(c.querySelectorAll(q));
    const on = (el, ev, fn, opts) => el && el.addEventListener(ev, fn, opts);

    function initCodeSectionToggles(){
        const codeSection = $('.code-section');
        if (!codeSection) return;

        // If another initializer already wired the toggles, reuse it.
        if (typeof window.initCodeToggles === 'function') {
            window.initCodeToggles();
            return;
        }

        const toggles = $$('.section-toggle', codeSection);
        toggles.forEach(toggle => {
            if (toggle.dataset.listenerAttached === '1') return;
            toggle.dataset.listenerAttached = '1';

            toggle.addEventListener('click', () => {
                const target = toggle.dataset.target;
                const content = codeSection.querySelector('#' + target);
                const icon = toggle.querySelector('.toggle-icon');
                if (!content || !icon) return;

                const isCollapsed = content.classList.contains('collapsed');
                content.classList.toggle('collapsed', !isCollapsed);
                icon.textContent = isCollapsed ? '▼' : '▶';
                toggle.setAttribute('aria-expanded', isCollapsed ? 'true' : 'false');

                setTimeout(() => {
                    if (window.mosaicUtils) window.mosaicUtils.resizeAll();
                }, 300);
            }, { passive: true });
        });
    }

    function initProjectTechFiltering(){
        const projectsSection = $('.projects-section');
        if (!projectsSection) return;

        $$('.tech-tag', projectsSection).forEach(tag => {
            if (tag.dataset.listenerAttached === '1') return;
            tag.dataset.listenerAttached = '1';

            tag.style.cursor = 'pointer';
            tag.addEventListener('click', (e) => {
                e.stopPropagation();
                const name = tag.textContent.trim();
                projectsSection.scrollIntoView({ behavior: 'smooth', block: 'start' });
                setTimeout(() => {
                    if (typeof window.applyTechFilter === 'function') window.applyTechFilter(name);
                }, 450);
            }, { passive: false });
        });
    }

    function initCodeSectionToggles(){
        const codeSection = $('.code-section');
        if (!codeSection) return;

        // If another initializer already wired the toggles, reuse it.
        if (typeof window.initCodeToggles === 'function') {
            window.initCodeToggles();
            return;
        }

        const toggles = $$('.section-toggle', codeSection);
        toggles.forEach(toggle => {
            if (toggle.dataset.listenerAttached === '1') return;
            toggle.dataset.listenerAttached = '1';

            toggle.addEventListener('click', () => {
                const target = toggle.dataset.target;
                const content = codeSection.querySelector('#' + target);
                const icon = toggle.querySelector('.toggle-icon');
                if (!content || !icon) return;

                const isCollapsed = content.classList.contains('collapsed');
                content.classList.toggle('collapsed', !isCollapsed);
                icon.textContent = isCollapsed ? '▼' : '▶';
                toggle.setAttribute('aria-expanded', isCollapsed ? 'true' : 'false');

                setTimeout(() => {
                    if (window.mosaicUtils) window.mosaicUtils.resizeAll();
                }, 300);
            }, { passive: true });
        });
    }

    function initSteamGameInteractions() {
        const steamSection = $('.steam-section');
        if (!steamSection) return;

        $$('.game-item', steamSection).forEach(gameItem => {
            gameItem.style.cursor = 'default';

            gameItem.removeEventListener('click', gameItem._clickHandler);

            gameItem.addEventListener('mouseenter', () => {
                gameItem.style.transform = 'translateY(-2px)';
                gameItem.style.boxShadow = '0 4px 12px rgba(0,0,0,0.25)';
            });

            gameItem.addEventListener('mouseleave', () => {
                gameItem.style.transform = '';
                gameItem.style.boxShadow = '';
            });
        });
    }

    function initBeatLeaderInteractions() {
        const beatLeaderSection = $('.beatleader-section');
        if (!beatLeaderSection) return;

        $$('.map-item', beatLeaderSection).forEach(mapItem => {
            mapItem.addEventListener('click', () => {
                const mapName = $('.map-name', mapItem)?.textContent;
                if (mapName) {
                    const searchUrl = `https://beatsaver.com/search?q=${encodeURIComponent(mapName)}`;
                    window.open(searchUrl, '_blank');
                }
            });

            mapItem.style.cursor = 'pointer';
        });

        $$('.stat-item', beatLeaderSection).forEach(statItem => {
            statItem.style.cursor = 'pointer';
        });
    }

    function initLastFMTrackActions() {
        const lastFMSection = $('.lastfm-section');
        if (!lastFMSection) return;

        $$('.recent-track-item', lastFMSection).forEach(trackItem => {
            trackItem.addEventListener('click', () => {
                const trackName = $('.recent-track-name', trackItem)?.textContent;
                const artistName = $('.recent-track-artist', trackItem)?.textContent;

                if (trackName && artistName && window.playTrack) {
                    const searchQuery = `${artistName} ${trackName}`;
                    window.playTrack(searchQuery);
                }
            });

            trackItem.style.cursor = 'pointer';
            trackItem.addEventListener('mouseenter', () => {
                trackItem.style.background = 'rgba(255,255,255,.024)';
            });

            trackItem.addEventListener('mouseleave', () => {
                trackItem.style.background = '';
            });
        });
    }
    function initTechStackFiltering(){
        const techSection = $('.tech-section');
        const projectsSection = $('.projects-section');
        if (!techSection || !projectsSection) return;

        const go = (name) => {
            projectsSection.scrollIntoView({ behavior: 'smooth', block: 'start' });
            setTimeout(() => {
                if (typeof window.applyTechFilter === 'function') {
                    window.applyTechFilter(name);
                } else {
                    // graceful fallback: dim non-matching cards
                    $$('.project-card', projectsSection).forEach(card => {
                        const tags = $$('.tech-tag', card);
                        const hit = tags.some(t => t.textContent.trim().toLowerCase() === name.toLowerCase());
                        card.style.opacity = hit ? '1' : '0.2';
                        card.style.transform = hit ? 'scale(1)' : 'scale(0.95)';
                    });
                }
            }, 450);
        };

        $$('.tech-item', techSection).forEach(item => {
            if (item.dataset.listenerAttached === '1') return;
            item.dataset.listenerAttached = '1';

            const name = item.querySelector('.tech-name')?.textContent || item.title || item.querySelector('img')?.alt || '';
            if (!name) return;

            item.style.cursor = 'pointer';
            item.addEventListener('click', () => go(name), { passive: true });
        });
    }

    function initMemeRefresh(){
        const memeSection = document.querySelector('.meme-section');
        if (!memeSection) return;

        let refreshBtn = memeSection.querySelector('.meme-refresh-btn');
        if (!refreshBtn){
            const header = memeSection.querySelector('.meme-header');
            if (header){
                refreshBtn = document.createElement('button');
                refreshBtn.className = 'btn btn-sm meme-refresh-btn';
                refreshBtn.type = 'button';
                refreshBtn.textContent = '🎲';
                header.appendChild(refreshBtn);
            }
        }

        if (refreshBtn){
            if (refreshBtn.hasAttribute('onclick')) refreshBtn.removeAttribute('onclick');
            if (refreshBtn.dataset.listenerAttached !== '1'){
                refreshBtn.dataset.listenerAttached = '1';
                refreshBtn.addEventListener('click', (e) => {
                    e.preventDefault();
                    if (typeof window.refreshMeme === 'function') window.refreshMeme();
                }, { passive: false });
            }
        }

        const memeContent = memeSection.querySelector('.meme-content');
        if (memeContent && memeContent.dataset.listenerAttached !== '1'){
            memeContent.dataset.listenerAttached = '1';
            memeContent.style.cursor = 'pointer';
            memeContent.addEventListener('click', (e) => {
                if (e.target.closest('button')) return;
                refreshBtn && refreshBtn.click();
            }, { passive: true });
        }
    }

    function initVisitorsInteractions() {
        const visitorsSection = $('.visitors-section');
        if (!visitorsSection) return;

        $$('.visitor-stat', visitorsSection).forEach(stat => {
            stat.style.cursor = 'pointer';
        });
    }

    function initServicesInteractions() {
        const servicesSection = $('.services-section');
        if (!servicesSection) return;

        $$('.service-item', servicesSection).forEach(serviceItem => {
            const serviceUrl = serviceItem.dataset.url;

            if (serviceUrl) {
                serviceItem.addEventListener('click', () => {
                    window.open(serviceUrl, '_blank');
                });

                serviceItem.style.cursor = 'pointer';
                serviceItem.addEventListener('mouseenter', () => {
                    serviceItem.style.transform = 'translateY(-1px)';
                });

                serviceItem.addEventListener('mouseleave', () => {
                    serviceItem.style.transform = '';
                });
            }
        });
    }

    function initWebringInteractions() {
        const webringSection = $('.webring-section');
        if (!webringSection) return;

        const webringHome = $('.webring-home', webringSection);
        if (webringHome) {
            webringHome.addEventListener('click', (e) => {
                e.preventDefault();

                const baseUrl = webringSection.dataset.baseUrl;
                if (baseUrl) {
                    window.open(baseUrl, '_blank');
                }
            });
        }
    }

    function initKeyboardShortcuts() {
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
                        if (window.mosaicUtils) {
                            window.mosaicUtils.expand(plugins[pluginIndex]);
                        }
                    }, 300);
                }
            }

            if (e.key === 'Escape' && window.mosaicUtils) {
                window.mosaicUtils.collapseExpanded();
            }
        });
    }

    function initAnimatedCounters() {
        $$('.visitor-number, .stat-value').forEach(counter => {
            const observer = new IntersectionObserver((entries) => {
                entries.forEach(entry => {
                    if (entry.isIntersecting) {
                        animateCounter(entry.target);
                        observer.unobserve(entry.target);
                    }
                });
            });
            observer.observe(counter);
        });
    }

    function animateCounter(element) {
        const text = element.textContent;
        const number = parseFloat(text.replace(/[^\d.]/g, ''));

        if (isNaN(number)) return;

        const suffix = text.replace(/[\d.,]/g, '');
        const duration = 1000;
        const steps = 30;
        const increment = number / steps;
        let current = 0;
        let step = 0;

        const timer = setInterval(() => {
            current += increment;
            step++;

            if (step >= steps) {
                current = number;
                clearInterval(timer);
            }

            const formatted = Math.floor(current).toLocaleString();
            element.textContent = formatted + suffix;
        }, duration / steps);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    function init() {
        setTimeout(() => {
            initTechStackFiltering();
            initProjectTechFiltering();
            initCodeSectionToggles();
            initSteamGameInteractions();
            initBeatLeaderInteractions();
            initLastFMTrackActions();
            initMemeRefresh();
            initVisitorsInteractions();
            initServicesInteractions();
            initWebringInteractions();
            initKeyboardShortcuts();
            initAnimatedCounters();
        }, 100);
    }

})();