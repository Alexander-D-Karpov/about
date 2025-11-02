(function() {
    'use strict';

    const $ = (q, c = document) => c.querySelector(q);
    const $$ = (q, c = document) => Array.from(c.querySelectorAll(q));
    const on = (el, ev, fn, opts) => el && el.addEventListener(ev, fn, opts);

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

        $$('.stat-item', beatLeaderSection).forEach(statItem => {
            statItem.style.cursor = 'pointer';
        });
    }

    function initLastFMTrackActions() {
        const lastFMSection = $('.lastfm-section');
        if (!lastFMSection) return;

        let isUpdating = false;

        const observer = new MutationObserver(() => {
            if (isUpdating) return;
            isUpdating = true;
            setupRecentTracksHandlers(lastFMSection);
            setTimeout(() => {
                isUpdating = false;
            }, 100);
        });

        const recentTracksList = lastFMSection.querySelector('.recent-tracks-list');
        if (recentTracksList) {
            observer.observe(recentTracksList, {
                childList: true,
                subtree: false
            });
        }

        setupRecentTracksHandlers(lastFMSection);
    }

    function setupRecentTracksHandlers(lastFMSection) {
        if (!lastFMSection) return;

        const trackItems = lastFMSection.querySelectorAll('.recent-track-item');

        trackItems.forEach(trackItem => {
            if (trackItem.dataset.handlerAttached === 'true') {
                return;
            }

            if (trackItem.classList.contains('now-playing')) {
                trackItem.style.cursor = 'default';
                trackItem.dataset.handlerAttached = 'true';
                return;
            }

            trackItem.style.cursor = 'pointer';

            const trackNameEl = trackItem.querySelector('.recent-track-name');
            const trackArtistEl = trackItem.querySelector('.recent-track-artist');

            if (!trackNameEl || !trackArtistEl) return;

            const trackName = trackNameEl.textContent.replace(' 🎵', '').trim();
            const trackArtist = trackArtistEl.textContent.trim();

            const clickHandler = function (e) {
                e.preventDefault();
                e.stopPropagation();

                if (trackName && trackArtist && window.playTrack) {
                    const searchQuery = `${trackArtist} ${trackName}`;
                    window.playTrack(searchQuery);
                }
            };

            trackItem.addEventListener('click', clickHandler);

            trackItem.addEventListener('mouseenter', () => {
                trackItem.style.background = 'rgba(255,255,255,.024)';
            });

            trackItem.addEventListener('mouseleave', () => {
                trackItem.style.background = '';
            });

            trackItem.dataset.handlerAttached = 'true';
        });
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
        return;
    }

    function animateCounter(element) {
        if (element.dataset.animated === 'true' || element.dataset.animating === 'true') {
            return;
        }

        element.dataset.animating = 'true';

        const text = element.textContent;
        const rawValue = element.dataset.rawValue;

        let number;
        if (rawValue) {
            number = parseFloat(rawValue);
        } else {
            number = parseFloat(text.replace(/[^\d.]/g, ''));
        }

        if (isNaN(number) || number === 0) {
            element.dataset.animating = '';
            return;
        }

        const suffix = text.replace(/[\d.,]/g, '');
        const duration = 800;
        const steps = 20;
        const increment = number / steps;
        let current = 0;
        let step = 0;

        const timer = setInterval(() => {
            current += increment;
            step++;

            if (step >= steps) {
                current = number;
                clearInterval(timer);
                element.dataset.animating = '';
            }

            if (suffix.includes('K') || suffix.includes('M')) {
                element.textContent = formatNumber(Math.floor(current));
            } else {
                const formatted = Math.floor(current).toLocaleString();
                element.textContent = formatted + suffix;
            }
        }, duration / steps);
    }

    function formatNumber(n) {
        if (n < 1000) {
            return n.toString();
        } else if (n < 1000000) {
            return (n / 1000).toFixed(1) + 'K';
        } else {
            return (n / 1000000).toFixed(1) + 'M';
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    function init() {
        setTimeout(() => {
            initSteamGameInteractions();
            initBeatLeaderInteractions();
            initLastFMTrackActions();
            initVisitorsInteractions();
            initServicesInteractions();
            initWebringInteractions();
            initKeyboardShortcuts();
            initAnimatedCounters();
        }, 100);
    }

})();