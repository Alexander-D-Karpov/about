(function () {
    'use strict';
    function bind() {
        document.querySelectorAll('.recent__item[data-play]').forEach(item => {
            if (item.dataset.bound === '1') return;
            item.dataset.bound = '1';
            item.addEventListener('click', () => {
                if (window.playTrack) window.playTrack(item.dataset.play);
            });
        });
    }
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', bind);
    else bind();
    window.bindRecentTracks = bind;
})();