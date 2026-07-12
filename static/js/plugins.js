(function() {
    'use strict';

    const $ = (q, c = document) => c.querySelector(q);
    const $$ = (q, c = document) => Array.from(c.querySelectorAll(q));

    function initSteamGameInteractions() {
        const steamSection = $('.steam-section');
        if (!steamSection) return;

        $$('.game-item', steamSection).forEach(gameItem => {
            gameItem.style.cursor = 'default';

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

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    function init() {
        setTimeout(() => {
            initSteamGameInteractions();
            initBeatLeaderInteractions();
            initVisitorsInteractions();
            initServicesInteractions();
            initWebringInteractions();
            initKeyboardShortcuts();
        }, 100);
    }
})();