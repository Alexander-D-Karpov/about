(function () {
    'use strict';

    function initCodeToggles() {
        const section = document.querySelector('.code-section');
        if (!section) return;

        section.querySelectorAll('.section-toggle').forEach(toggle => {
            if (toggle.dataset.listenerAttached) return;
            toggle.dataset.listenerAttached = '1';

            const id = toggle.dataset.target;
            if (!id) return;

            const content = section.querySelector('#' + id);
            const icon = toggle.querySelector('.toggle-icon');
            if (!content || !icon) return;

            if (id === 'wakatime-langs') {
                content.classList.remove('collapsed');
                icon.textContent = '▼';
                toggle.setAttribute('aria-expanded', 'true');
            }

            toggle.addEventListener('click', (e) => {
                e.preventDefault();
                const willCollapse = !content.classList.contains('collapsed');
                content.classList.toggle('collapsed', willCollapse);
                icon.textContent = willCollapse ? '▶' : '▼';
                toggle.setAttribute('aria-expanded', !willCollapse);

                if (window.mosaicUtils) {
                    window.mosaicUtils.resizeAll();
                    setTimeout(() => window.mosaicUtils.resizeAll(), 150);
                }
            });
        });
    }

    window.initCodeToggles = initCodeToggles;

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initCodeToggles);
    } else {
        setTimeout(initCodeToggles, 100);
    }
})();