(function () {
    'use strict';

    function initSteam() {
        const section = document.querySelector('.steam-section');
        if (!section) return;

        section.querySelectorAll('.game-item').forEach(item => {
            item.style.cursor = 'default';

            item.addEventListener('mouseenter', () => {
                item.style.transform = 'translateY(-2px)';
                item.style.boxShadow = '0 4px 12px rgba(0,0,0,.25)';
            });

            item.addEventListener('mouseleave', () => {
                item.style.transform = '';
                item.style.boxShadow = '';
            });
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initSteam);
    } else {
        setTimeout(initSteam, 100);
    }

    window.initSteamInteractions = initSteam;
})();