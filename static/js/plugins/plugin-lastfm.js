(function () {
    'use strict';

    function initLastFM() {
        const section = document.querySelector('.lastfm-section');
        if (!section) return;

        section.querySelectorAll('.recent-track-item').forEach(item => {
            if (item.classList.contains('now-playing')) {
                item.style.cursor = 'default';
                return;
            }

            item.style.cursor = 'pointer';

            item.addEventListener('mouseenter', () => {
                item.style.background = 'rgba(255,255,255,.024)';
            });

            item.addEventListener('mouseleave', () => {
                item.style.background = '';
            });

            item.addEventListener('click', () => {
                const name = item.querySelector('.recent-track-name')?.textContent?.replace(' 🎵', '').trim();
                const artist = item.querySelector('.recent-track-artist')?.textContent?.trim();
                if (name && artist && window.playTrack) {
                    window.playTrack(`${artist} ${name}`);
                }
            });
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initLastFM);
    } else {
        setTimeout(initLastFM, 100);
    }

    window.initLastFMInteractions = initLastFM;
})();