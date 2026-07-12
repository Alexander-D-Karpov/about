(function () {
    'use strict';

    function initCodeToggles() {
        const section = document.querySelector('.code-section');
        if (!section) return;

        section.querySelectorAll('details').forEach(d => {
            if (d.dataset.codeToggleBound === '1') return;
            d.dataset.codeToggleBound = '1';
            d.addEventListener('toggle', () => {
                if (window.mosaicUtils && window.mosaicUtils.notifyContentChanged) {
                    window.mosaicUtils.notifyContentChanged(section);
                } else if (window.mosaicUtils) {
                    window.mosaicUtils.resizeAll();
                }
            });
        });
    }

    window.initCodeToggles = initCodeToggles;

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => setTimeout(initCodeToggles, 100));
    } else {
        setTimeout(initCodeToggles, 100);
    }
})();