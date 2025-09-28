(function() {
    'use strict';

    const $ = (q, c = document) => c.querySelector(q);
    const $$ = (q, c = document) => Array.from(c.querySelectorAll(q));
    const on = (el, ev, fn, opts) => el && el.addEventListener(ev, fn, opts);

    function initTechStackFiltering() {
        const techSection = $('.tech-section');
        if (!techSection) return;

        const projectsSection = $('.projects-section');
        if (!projectsSection) return;

        const techItems = $$('.tech-item', techSection);
        const projectCards = $$('.project-card', projectsSection);

        techItems.forEach(item => {
            const techName = item.querySelector('.tech-name')?.textContent ||
                item.title ||
                item.querySelector('img')?.alt || '';

            item.style.cursor = 'pointer';
            item.addEventListener('click', () => {
                techItems.forEach(t => t.classList.remove('filtered'));
                item.classList.add('filtered');

                projectCards.forEach(card => {
                    const techTags = $$('.tech-tag', card);
                    const hasTech = techTags.some(tag =>
                        tag.textContent.toLowerCase().includes(techName.toLowerCase())
                    );

                    card.style.opacity = hasTech ? '1' : '0.3';
                    card.style.transform = hasTech ? 'scale(1)' : 'scale(0.95)';
                    card.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
                });

                setTimeout(() => {
                    techItems.forEach(t => t.classList.remove('filtered'));
                    projectCards.forEach(card => {
                        card.style.opacity = '1';
                        card.style.transform = 'scale(1)';
                    });
                }, 3000);
            });
        });
    }

    function initProjectTechFiltering() {
        const projectsSection = $('.projects-section');
        if (!projectsSection) return;

        $$('.tech-tag', projectsSection).forEach(tag => {
            tag.addEventListener('click', (e) => {
                e.stopPropagation();
                const techName = tag.textContent;
                const projectCards = $$('.project-card', projectsSection);

                projectCards.forEach(card => {
                    const techTags = $$('.tech-tag', card);
                    const hasTech = techTags.some(t => t.textContent === techName);

                    if (!hasTech) {
                        card.style.opacity = '0.3';
                        card.style.transform = 'scale(0.95)';
                    } else {
                        card.style.opacity = '1';
                        card.style.transform = 'scale(1.02)';
                    }
                    card.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
                });

                setTimeout(() => {
                    projectCards.forEach(card => {
                        card.style.opacity = '1';
                        card.style.transform = 'scale(1)';
                    });
                }, 2500);
            });
        });
    }

    function initCodeSectionToggles() {
        const codeSection = $('.code-section');
        if (!codeSection) return;

        $$('.section-toggle', codeSection).forEach(toggle => {
            toggle.addEventListener('click', () => {
                const target = toggle.dataset.target;
                const content = $(`#${target}`, codeSection);
                const icon = $('.toggle-icon', toggle);

                if (content) {
                    const isCollapsed = content.classList.contains('collapsed');

                    if (isCollapsed) {
                        content.classList.remove('collapsed');
                        icon.textContent = '▼';
                        toggle.setAttribute('aria-expanded', 'true');
                    } else {
                        content.classList.add('collapsed');
                        icon.textContent = '▶';
                        toggle.setAttribute('aria-expanded', 'false');
                    }

                    setTimeout(() => {
                        if (window.mosaicUtils) window.mosaicUtils.resizeAll();
                    }, 300);
                }
            });
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
    function initMemeRefresh() {
        const memeSection = document.querySelector('.meme-section');
        if (!memeSection) return;

        let refreshBtn = document.querySelector('.meme-refresh-btn', memeSection);
        if (!refreshBtn) {
            const memeHeader = memeSection.querySelector('.meme-header');
            if (memeHeader) {
                refreshBtn = document.createElement('button');
                refreshBtn.className = 'btn btn-sm meme-refresh-btn';
                refreshBtn.textContent = '🎲 New Meme';
                refreshBtn.type = 'button';
                refreshBtn.style.marginLeft = 'auto';
                memeHeader.appendChild(refreshBtn);
            }
        }

        if (refreshBtn) {
            refreshBtn.addEventListener('click', async (e) => {
                e.preventDefault();
                refreshBtn.disabled = true;
                const originalText = refreshBtn.textContent;
                refreshBtn.textContent = 'Loading...';

                try {
                    const response = await fetch('/api/meme/refresh', {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                        }
                    });

                    if (response.ok) {
                        const data = await response.json();

                        if (data.success && data.html) {
                            const memeContent = memeSection.querySelector('.meme-content');
                            if (memeContent) {
                                memeContent.style.opacity = '0';
                                memeContent.style.transition = 'opacity 0.2s ease';

                                setTimeout(() => {
                                    memeContent.innerHTML = '';
                                    const tempDiv = document.createElement('div');
                                    tempDiv.innerHTML = data.html;
                                    const newContent = tempDiv.querySelector('.meme-content');
                                    if (newContent) {
                                        memeContent.innerHTML = newContent.innerHTML;
                                    }

                                    setTimeout(() => {
                                        memeContent.style.opacity = '1';
                                    }, 50);
                                }, 200);
                            }
                        }
                    } else {
                        console.error('Failed to refresh meme');
                    }
                } catch (error) {
                    console.error('Error refreshing meme:', error);
                } finally {
                    refreshBtn.disabled = false;
                    refreshBtn.textContent = originalText;
                }
            });
        }

        const memeContent = memeSection.querySelector('.meme-content');
        if (memeContent) {
            memeContent.style.cursor = 'pointer';
            memeContent.addEventListener('click', (e) => {
                if (e.target.tagName === 'BUTTON') return;

                if (refreshBtn) {
                    refreshBtn.click();
                }
            });
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