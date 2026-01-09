(function () {
    'use strict';

    const $ = (q, c = document) => c ? c.querySelector(q) : null;
    const $$ = (q, c = document) => c ? Array.from(c.querySelectorAll(q)) : [];

    const root = $('.container');
    if (!root) return;

    let mosaic = $('.mosaic', root);
    if (!mosaic) {
        mosaic = document.createElement('section');
        mosaic.className = 'mosaic';
        root.prepend(mosaic);
    }

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

        el.style.height = 'auto';
        el.style.gridColumn = '';
        el.style.gridRowStart = '';
        el.style.gridRowEnd = '';
    });

    window.mosaicUtils = {
        resizeAll: () => {
        },
        fullRepack: () => {
        },
        expand: () => {
        },
        collapseExpanded: () => {
        },
        getMosaic: () => mosaic
    };

    document.documentElement.classList.remove('js-loading');
    document.documentElement.classList.add('js-loaded');
})();