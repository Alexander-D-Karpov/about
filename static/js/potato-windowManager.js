(function () {
    'use strict';

    const $ = (q, c = document) => c ? c.querySelector(q) : null;
    const $$ = (q, c = document) => c ? Array.from(c.querySelectorAll(q)) : [];

    const root = $('.container');
    if (!root) return;

    // Remove any inline styles on container
    root.removeAttribute('style');

    let mosaic = $('.mosaic', root);
    if (!mosaic) {
        mosaic = document.createElement('section');
        mosaic.className = 'mosaic';
        root.prepend(mosaic);
    }

    // Remove any inline styles on mosaic
    mosaic.removeAttribute('style');

    const toMove = [...root.children].filter(el => el !== mosaic);
    toMove.forEach(el => {
        el.classList.add('plugin');
        if (!el.querySelector('.plugin__inner')) {
            const inner = document.createElement('div');
            inner.className = 'plugin__inner';
            while (el.firstChild) inner.appendChild(el.firstChild);
            el.appendChild(inner);
        }
        mosaic.appendChild(el);
    });

    $$('.plugin', mosaic).forEach(el => {
        let header = $('.plugin-header', el);
        if (!header) {
            const titleEl = $('h1,h2,h3,h4', el.querySelector('.plugin__inner'));
            if (titleEl) {
                header = document.createElement('div');
                header.className = 'plugin-header';
                const title = document.createElement('h3');
                title.className = 'plugin-title';
                title.textContent = titleEl.textContent;
                header.appendChild(title);
                el.querySelector('.plugin__inner').prepend(header);
            }
        }

        // Remove ALL inline styles completely
        el.removeAttribute('style');

        // Also remove from inner
        const inner = el.querySelector('.plugin__inner');
        if (inner) {
            inner.removeAttribute('style');
        }
    });

    // Move profile to first position
    const profile = mosaic.querySelector('.profile-section');
    if (profile && profile !== mosaic.firstElementChild) {
        mosaic.insertBefore(profile, mosaic.firstElementChild);
    }

    // Move projects to last position
    const projects = mosaic.querySelector('.projects-section');
    if (projects) {
        mosaic.appendChild(projects);
    }

    // Stub out mosaicUtils so other scripts don't error
    window.mosaicUtils = {
        resizeAll: () => {
        },
        fullRepack: () => {
        },
        expand: (el) => {
            // Simple expand - just scroll to element
            if (el) el.scrollIntoView({behavior: 'smooth', block: 'start'});
        },
        collapseExpanded: () => {
        },
        getMosaic: () => mosaic
    };

    document.documentElement.classList.remove('js-loading');
    document.documentElement.classList.add('js-loaded');
})();