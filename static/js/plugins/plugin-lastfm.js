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


            item.addEventListener('click', () => {
                const nameEl = item.querySelector('.recent-track-name');
                const artistEl = item.querySelector('.recent-track-artist');

                if (nameEl && artistEl && window.playTrack) {
                    const name = nameEl.textContent.replace(' 🎵', '').trim();
                    const artist = artistEl.textContent.trim();

                    window.playTrack({artist: artist, track: name});
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