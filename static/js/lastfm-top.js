(function () {
    'use strict';

    var GRADS = [
        'linear-gradient(135deg,#3a7bd5,#2a5298)',
        'linear-gradient(135deg,#9046d8,#6a2cb0)',
        'linear-gradient(135deg,#1a9e54,#0f7a3f)',
        'linear-gradient(135deg,#cf8420,#a86510)',
        'linear-gradient(135deg,#cf4a6e,#a02f50)',
        'linear-gradient(135deg,#1d9e92,#127870)'
    ];

    function esc(s) {
        return String(s == null ? '' : s)
            .replace(/&/g, '&amp;').replace(/</g, '&lt;')
            .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    function render(grid, items) {
        grid.innerHTML = items.map(function (it, i) {
            var bg = GRADS[i % GRADS.length];
            var img = it.image ? '<img src="' + esc(it.image) + '" alt="">' : '';
            return '<div class="cover" style="background:' + bg + '">' + img +
                '<div class="cover__shade"></div>' +
                '<div class="cover__cap"><div class="cover__name">' + esc(it.name) +
                '</div><div class="cover__plays">' + esc(it.plays) + '</div></div></div>';
        }).join('');
    }

    async function load(sel) {
        var kind = sel.dataset.lfmPeriod;
        var period = sel.value;
        var grid = document.querySelector('[data-lfm-grid="' + kind + '"]');
        if (!grid) return;
        grid.dataset.loading = '1';
        sel.disabled = true;
        try {
            var res = await fetch('/api/lastfm/top?type=' + encodeURIComponent(kind) + '&period=' + encodeURIComponent(period));
            if (!res.ok) throw new Error('bad status');
            var data = await res.json();
            render(grid, data.items || []);
        } catch (e) {
        } finally {
            delete grid.dataset.loading;
            sel.disabled = false;
        }
    }

    document.addEventListener('change', function (e) {
        var sel = e.target && e.target.closest ? e.target.closest('.lfm-period') : null;
        if (sel) load(sel);
    });
})();