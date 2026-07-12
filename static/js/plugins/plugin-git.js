(function () {
    'use strict';

    const cache = new Map();
    let tooltip = null;
    let activeCell = null;
    let pinned = false;
    let hideTimer = null;
    let showTimer = null;

    const disabledSources = new Set(loadDisabled());

    function loadDisabled() {
        try {
            return JSON.parse(sessionStorage.getItem('git.disabledSources') || '[]');
        } catch {
            return [];
        }
    }

    function saveDisabled() {
        try {
            sessionStorage.setItem('git.disabledSources', JSON.stringify([...disabledSources]));
        } catch {}
    }

    function ensureTooltip() {
        if (tooltip) return tooltip;
        tooltip = document.createElement('div');
        tooltip.className = 'git-tooltip';
        tooltip.hidden = true;
        document.body.appendChild(tooltip);
        return tooltip;
    }

    function esc(s) {
        const d = document.createElement('div');
        d.textContent = s || '';
        return d.innerHTML;
    }

    function fmtDate(iso) {
        const d = new Date(iso + 'T00:00:00');
        if (isNaN(d)) return iso;
        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    }

    function notifyResize(section) {
        if (!section) return;
        if (window.mosaicUtils && window.mosaicUtils.notifyContentChanged) {
            window.mosaicUtils.notifyContentChanged(section);
        } else if (window.mosaicUtils) {
            window.mosaicUtils.resizeAll();
        }
    }

    function position(cell) {
        if (!tooltip || !cell) return;
        const rect = cell.getBoundingClientRect();
        const tw = tooltip.offsetWidth;
        const th = tooltip.offsetHeight;

        let left = rect.left + rect.width / 2 - tw / 2;
        left = Math.max(8, Math.min(left, window.innerWidth - tw - 8));

        let top = rect.top - th - 8;
        if (top < 8) top = Math.min(rect.bottom + 8, window.innerHeight - th - 8);
        if (top < 8) top = 8;

        tooltip.style.left = left + 'px';
        tooltip.style.top = top + 'px';
    }

    function renderBasic(date, count) {
        ensureTooltip();
        const label = count === 0 ? 'No contributions' : count + ' contribution' + (count === 1 ? '' : 's');
        tooltip.innerHTML = '<div class="git-tooltip-head"><span>' + label + ' · ' + esc(fmtDate(date)) + '</span>' +
            (pinned ? '<button type="button" class="git-tooltip-close" aria-label="Close">✕</button>' : '') + '</div>';
        tooltip.hidden = false;
    }

    const TYPE_ICONS = {
        push: '<svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor"><path d="M11.93 8.5a4.002 4.002 0 0 1-7.86 0H.75a.75.75 0 0 1 0-1.5h3.32a4.002 4.002 0 0 1 7.86 0h3.32a.75.75 0 0 1 0 1.5Zm-1.43-.75a2.5 2.5 0 1 0-5 0 2.5 2.5 0 0 0 5 0Z"/></svg>',
        pr: '<svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor"><path d="M5.45 5.154A4.25 4.25 0 0 0 9.25 7.5h1.378a2.251 2.251 0 1 1 0 1.5H9.25A5.734 5.734 0 0 1 5 7.123v3.505a2.25 2.25 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.95-.218ZM4.25 13.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5Zm8.5-4.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5ZM5 3.25a.75.75 0 1 0-1.5 0 .75.75 0 0 0 1.5 0Z"/></svg>',
        issue: '<svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor"><path d="M8 9.5a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3Z"/><path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0ZM1.5 8a6.5 6.5 0 1 0 13 0 6.5 6.5 0 0 0-13 0Z"/></svg>',
        create: '<svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor"><path d="M2 2.5A2.5 2.5 0 0 1 4.5 0h8.75a.75.75 0 0 1 .75.75v12.5a.75.75 0 0 1-.75.75h-2.5a.75.75 0 0 1 0-1.5h1.75v-2h-8a1 1 0 0 0-.714 1.7.75.75 0 1 1-1.072 1.05A2.495 2.495 0 0 1 2 11.5Zm10.5-1h-8a1 1 0 0 0-1 1v6.708A2.486 2.486 0 0 1 4.5 9h8Z"/></svg>',
        star: '<svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor"><path d="M8 .25a.75.75 0 0 1 .673.418l1.882 3.815 4.21.612a.75.75 0 0 1 .416 1.279l-3.046 2.97.719 4.192a.751.751 0 0 1-1.088.791L8 12.347l-3.766 1.98a.75.75 0 0 1-1.088-.79l.72-4.194L.818 6.374a.75.75 0 0 1 .416-1.28l4.21-.611L7.327.668A.75.75 0 0 1 8 .25Z"/></svg>',
        release: '<svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor"><path d="M1 7.775V2.75C1 1.784 1.784 1 2.75 1h5.025c.464 0 .91.184 1.238.513l6.25 6.25a1.75 1.75 0 0 1 0 2.474l-5.026 5.026a1.75 1.75 0 0 1-2.474 0l-6.25-6.25A1.752 1.752 0 0 1 1 7.775ZM6 5a1 1 0 1 0-2 0 1 1 0 0 0 2 0Z"/></svg>'
    };
    TYPE_ICONS.merge = TYPE_ICONS.pr;

    function typeIcon(t) {
        return TYPE_ICONS[t] || '<svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor"><circle cx="8" cy="8" r="3"/></svg>';
    }

    function renderData(data, full) {
        ensureTooltip();
        let html = '<div class="git-tooltip-head"><span>' + data.total + ' contribution' + (data.total === 1 ? '' : 's') +
            ' · ' + esc(fmtDate(data.date)) + '</span>' +
            (full ? '<button type="button" class="git-tooltip-close" aria-label="Close">✕</button>' : '') + '</div>';

        if (Array.isArray(data.sources) && data.sources.length > 0) {
            html += '<div class="git-tooltip-sources">';
            data.sources.forEach(s => {
                html += '<span class="git-tooltip-src" style="--gc:' + esc(s.color) + '"><i></i>' +
                    esc(s.name) + ' ' + s.count + '</span>';
            });
            html += '</div>';
        }

        const acts = Array.isArray(data.activities) ? data.activities : [];
        const list = full ? acts : acts.slice(0, 8);

        list.forEach(a => {
            let diff = '';
            if (a.additions > 0 || a.deletions > 0) {
                diff = '<span class="git-tooltip-diff"><em class="git-add">+' + a.additions +
                    '</em> <em class="git-del">−' + a.deletions + '</em></span>';
            }
            const title = a.url
                ? '<a class="git-tooltip-title" href="' + esc(a.url) + '" target="_blank" rel="noopener">' + esc(a.title) + '</a>'
                : '<span class="git-tooltip-title">' + esc(a.title) + '</span>';
            html += '<div class="git-tooltip-item" style="--gc:' + esc(a.color) + '" data-type="' + esc(a.type) + '">' +
                '<i class="git-tooltip-ico">' + typeIcon(a.type) + '</i>' +
                '<span class="git-tooltip-body">' + title +
                '<span class="git-tooltip-meta">' + esc(a.repo || '') +
                (a.ref ? ' · ' + esc(a.ref) : '') +
                (a.time ? ' · ' + esc(a.time) : '') + diff +
                (a.source ? ' · ' + esc(a.source) : '') + '</span>' +
                '</span></div>';
        });

        if (!full && acts.length > 8) {
            html += '<div class="git-tooltip-more">… and ' + (acts.length - 8) + ' more — click day to see all</div>';
        }
        if (data.private > 0) {
            html += '<div class="git-tooltip-private"><svg viewBox="0 0 16 16" width="11" height="11" fill="currentColor" style="vertical-align:-1px;margin-right:4px"><path d="M4 4a4 4 0 0 1 8 0v2h.25c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 12.25 15h-8.5A1.75 1.75 0 0 1 2 13.25v-5.5C2 6.784 2.784 6 3.75 6H4Zm8.25 3.5h-8.5a.25.25 0 0 0-.25.25v5.5c0 .138.112.25.25.25h8.5a.25.25 0 0 0 .25-.25v-5.5a.25.25 0 0 0-.25-.25ZM10.5 6V4a2.5 2.5 0 1 0-5 0v2Z"/></svg>' +
                data.private + ' private contribution' + (data.private === 1 ? '' : 's') + '</div>';
        }
        tooltip.innerHTML = html;
        tooltip.hidden = false;
    }

    async function fetchDay(date) {
        if (cache.has(date)) return cache.get(date);
        try {
            const res = await fetch('/api/git/day?date=' + encodeURIComponent(date));
            if (!res.ok) return null;
            const data = await res.json();
            cache.set(date, data);
            return data;
        } catch {
            return null;
        }
    }

    async function show(cell) {
        if (pinned) return;
        const date = cell.dataset.date;
        if (!date) return;

        activeCell = cell;
        const count = parseInt(cell.dataset.count || '0', 10);

        renderBasic(date, count);
        position(cell);

        if (count === 0) return;

        if (cache.has(date)) {
            renderData(cache.get(date), false);
            position(cell);
            return;
        }

        tooltip.innerHTML += '<div class="git-tooltip-loading">loading…</div>';
        position(cell);

        const data = await fetchDay(date);
        if (data && activeCell === cell && !pinned) {
            renderData(data, false);
            position(cell);
        }
    }

    async function pin(cell) {
        const date = cell.dataset.date;
        if (!date) return;

        pinned = true;
        activeCell = cell;
        ensureTooltip();
        tooltip.classList.add('git-tooltip--pinned');

        const count = parseInt(cell.dataset.count || '0', 10);
        renderBasic(date, count);
        position(cell);

        if (count === 0) return;

        if (!cache.has(date)) {
            tooltip.innerHTML += '<div class="git-tooltip-loading">loading…</div>';
            position(cell);
        }

        const data = await fetchDay(date);
        if (data && activeCell === cell && pinned) {
            renderData(data, true);
            position(cell);
        }
    }

    function hide() {
        activeCell = null;
        pinned = false;
        if (tooltip) {
            tooltip.hidden = true;
            tooltip.classList.remove('git-tooltip--pinned');
        }
    }

    function parseHex(c) {
        c = (c || '').trim().replace('#', '');
        if (c.length === 3) c = c[0] + c[0] + c[1] + c[1] + c[2] + c[2];
        if (c.length !== 6) return null;
        const v = parseInt(c, 16);
        if (isNaN(v)) return null;
        return [(v >> 16) & 255, (v >> 8) & 255, v & 255];
    }

    function blendColors(counts, colors) {
        let r = 0, g = 0, b = 0, total = 0;
        for (const [k, c] of Object.entries(counts)) {
            if (c <= 0) continue;
            const rgb = parseHex(colors[k]) || [0x4d, 0x9f, 0xff];
            r += rgb[0] * c;
            g += rgb[1] * c;
            b += rgb[2] * c;
            total += c;
        }
        if (!total) return '#4d9fff';
        const h = n => Math.round(n / total).toString(16).padStart(2, '0');
        return '#' + h(r) + h(g) + h(b);
    }

    function sourceColors() {
        const map = {};
        document.querySelectorAll('.git-legend .git-src[data-key]').forEach(el => {
            map[el.dataset.key] = (el.style.getPropertyValue('--gc') || '').trim() || '#4d9fff';
        });
        return map;
    }

    function cellSrc(cell) {
        if (cell._src !== undefined) return cell._src;
        try {
            cell._src = cell.dataset.src ? JSON.parse(cell.dataset.src) : null;
        } catch {
            cell._src = null;
        }
        return cell._src;
    }

    function recomputeHeatmap(hoverKey) {
        const colors = sourceColors();
        document.querySelectorAll('.git-heatmap').forEach(hm => {
            const cells = hm.querySelectorAll('.git-day:not(.git-day--scale)');
            let max = 0;
            const totals = new Map();

            cells.forEach(cell => {
                const src = cellSrc(cell);
                let total = 0;
                let filtered = null;
                if (src) {
                    filtered = {};
                    for (const [k, v] of Object.entries(src)) {
                        if (disabledSources.has(k)) continue;
                        if (hoverKey && k !== hoverKey) continue;
                        filtered[k] = v;
                        total += v;
                    }
                }
                totals.set(cell, { total, filtered });
                if (total > max) max = total;
            });

            cells.forEach(cell => {
                const { total, filtered } = totals.get(cell) || { total: 0, filtered: null };
                let level = 0;
                if (total > 0 && max > 0) {
                    level = Math.ceil(total / max * 4);
                    if (level < 1) level = 1;
                    if (level > 4) level = 4;
                }
                cell.dataset.level = String(level);
                if (total > 0 && filtered) {
                    cell.style.setProperty('--gc', blendColors(filtered, colors));
                }
            });
        });
    }

    function applyFeedFilter(hoverLabel) {
        const disabledLabels = new Set();
        document.querySelectorAll('.git-legend .git-src[data-key]').forEach(el => {
            if (disabledSources.has(el.dataset.key)) disabledLabels.add(el.dataset.label);
        });
        document.querySelectorAll('.git-feed .git-item').forEach(item => {
            const s = item.dataset.source;
            item.classList.toggle('git-item--off', disabledLabels.has(s));
            item.classList.toggle('git-item--dim', !!hoverLabel && s !== hoverLabel && !disabledLabels.has(s));
        });
    }

    function applyStates() {
        document.querySelectorAll('.git-legend .git-src[data-key]').forEach(el => {
            el.classList.toggle('git-src--off', disabledSources.has(el.dataset.key));
        });
        if (disabledSources.size > 0) {
            recomputeHeatmap(null);
        }
        applyFeedFilter(null);
    }

    function revealFeed(feed, count) {
        if (!feed) return;
        const hidden = Array.from(feed.querySelectorAll('.git-item--hidden'));
        hidden.slice(0, count).forEach(el => el.classList.remove('git-item--hidden'));
        const btn = feed.querySelector('[data-git-feed-more]');
        const remaining = feed.querySelectorAll('.git-item--hidden').length;
        if (btn) {
            if (remaining > 0) btn.textContent = 'Show more (' + remaining + ')';
            else btn.remove();
        }
        notifyResize(feed.closest('.plugin'));
    }

    function revealExpandedFeeds() {
        document.querySelectorAll('.plugin--expanded [data-git-feed]').forEach(feed => {
            feed.open = true;
            if (feed.querySelector('.git-item--hidden')) revealFeed(feed, Infinity);
        });
    }

    function restoreFeedState() {
        let open = false;
        try {
            open = sessionStorage.getItem('git.feedOpen') === '1';
        } catch {}
        document.querySelectorAll('[data-git-feed]').forEach(d => {
            d.open = open;
        });
    }

    function sizeHeatmaps() {
        document.querySelectorAll('.git-heatmap-block').forEach(block => {
            const hm = block.querySelector('.git-heatmap');
            if (!hm) return;
            const weeks = Math.ceil(hm.querySelectorAll('.git-day').length / 7);
            if (!weeks) return;

            const gap = 3;
            const avail = block.clientWidth - 26;
            if (avail <= 0) return;

            const expanded = !!block.closest('.plugin--expanded');
            const mobile = window.innerWidth <= 780;

            let targetWeeks = Math.min(weeks, 53);
            if (expanded) targetWeeks = weeks;
            else if (mobile) targetWeeks = Math.min(weeks, 26);

            let cell = Math.floor(avail / targetWeeks) - gap;
            cell = Math.max(8, Math.min(cell, 11));

            block.style.setProperty('--cell', cell + 'px');
            block.style.setProperty('--cgap', gap + 'px');
        });
    }

    function scrollHeatmapToEnd(el) {
        if (!el) return;
        requestAnimationFrame(() => {
            el.scrollLeft = el.scrollWidth > el.clientWidth ? el.scrollWidth : 0;
        });
    }

    function scrollHeatmapsToEnd() {
        document.querySelectorAll('.git-heatmap-scroll').forEach(scrollHeatmapToEnd);
    }

    function init() {
        document.addEventListener('mouseover', e => {
            const src = e.target.closest('.git-legend .git-src[data-key]');
            if (src) {
                recomputeHeatmap(src.dataset.key);
                applyFeedFilter(src.dataset.label);
                return;
            }
            const cell = e.target.closest('.git-day');
            if (!cell || cell.classList.contains('git-day--scale')) return;

            clearTimeout(hideTimer);
            clearTimeout(showTimer);
            showTimer = setTimeout(() => show(cell), 120);
        });

        document.addEventListener('mouseout', e => {
            const src = e.target.closest('.git-legend .git-src[data-key]');
            if (src) {
                recomputeHeatmap(null);
                applyFeedFilter(null);
                return;
            }
            const cell = e.target.closest('.git-day');
            if (!cell) return;
            clearTimeout(showTimer);
            if (!pinned) hideTimer = setTimeout(hide, 150);
        });

        document.addEventListener('click', e => {
            if (e.target.closest('.git-tooltip-close')) {
                hide();
                return;
            }
            if (e.target.closest('.git-tooltip')) {
                return;
            }

            const legendSrc = e.target.closest('.git-legend .git-src[data-key]');
            if (legendSrc) {
                e.preventDefault();
                const key = legendSrc.dataset.key;
                if (disabledSources.has(key)) disabledSources.delete(key);
                else disabledSources.add(key);
                saveDisabled();
                legendSrc.classList.toggle('git-src--off', disabledSources.has(key));
                recomputeHeatmap(null);
                applyFeedFilter(null);
                return;
            }

            const moreBtn = e.target.closest('[data-git-feed-more]');
            if (moreBtn) {
                revealFeed(moreBtn.closest('.git-feed'), 15);
                return;
            }

            const cell = e.target.closest('.git-day');
            if (cell && !cell.classList.contains('git-day--scale')) {
                if (parseInt(cell.dataset.count || '0', 10) === 0) return;
                clearTimeout(hideTimer);
                clearTimeout(showTimer);
                if (pinned && activeCell === cell) {
                    hide();
                } else {
                    pin(cell);
                }
                return;
            }

            if (pinned) hide();
        });

        document.addEventListener('keydown', e => {
            if (e.key === 'Escape') hide();
        });

        window.addEventListener('scroll', () => {
            if (activeCell) position(activeCell);
        }, { passive: true });

        sizeHeatmaps();
        scrollHeatmapsToEnd();
        applyStates();
        restoreFeedState();

        const mo = new MutationObserver(muts => {
            let heatmapAdded = false;
            let classChanged = false;
            let justExpanded = null;
            for (const m of muts) {
                if (m.type === 'attributes') {
                    classChanged = true;
                    const t = m.target;
                    if (t.nodeType === 1 &&
                        t.classList.contains('plugin--expanded') &&
                        !(m.oldValue || '').split(/\s+/).includes('plugin--expanded')) {
                        (justExpanded || (justExpanded = [])).push(t);
                    }
                    continue;
                }
                for (const n of m.addedNodes) {
                    if (n.nodeType === 1 && (n.matches?.('.git-heatmap-scroll, .git-activity') || n.querySelector?.('.git-heatmap-scroll, .git-activity'))) {
                        heatmapAdded = true;
                    }
                }
            }
            if (heatmapAdded) {
                sizeHeatmaps();
                scrollHeatmapsToEnd();
                applyStates();
            }
            if (classChanged) {
                revealExpandedFeeds();
                sizeHeatmaps();
            }
            if (justExpanded) {
                justExpanded.forEach(el =>
                    el.querySelectorAll('.git-heatmap-scroll').forEach(scrollHeatmapToEnd)
                );
            }
        });
        mo.observe(document.body, {
            childList: true,
            subtree: true,
            attributes: true,
            attributeOldValue: true,
            attributeFilter: ['class']
        });
        let resizeTimer = null;
        window.addEventListener('resize', () => {
            clearTimeout(resizeTimer);
            resizeTimer = setTimeout(sizeHeatmaps, 100);
        }, { passive: true });
        document.addEventListener('toggle', e => {
            const feed = e.target.closest?.('[data-git-feed]') || (e.target.matches?.('[data-git-feed]') ? e.target : null);
            if (!feed) return;
            try {
                sessionStorage.setItem('git.feedOpen', feed.open ? '1' : '0');
            } catch {}
            notifyResize(feed.closest('.plugin'));
        }, true);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();