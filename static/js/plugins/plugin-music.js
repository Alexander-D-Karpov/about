(function () {
    'use strict';

    const HEART_SVG = '<svg viewBox="0 0 24 24" width="13" height="13" aria-hidden="true"><path fill="currentColor" d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"/></svg>';
    const SPOTIFY_SVG = '<svg viewBox="0 0 24 24" width="13" height="13" aria-hidden="true"><path fill="currentColor" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm4.59 14.44c-.18.29-.56.38-.85.2-2.33-1.42-5.27-1.75-8.72-.96-.33.08-.66-.13-.74-.46-.08-.33.13-.66.46-.74 3.78-.86 7.02-.48 9.64 1.11.29.18.38.56.21.85zm1.22-2.72c-.22.36-.7.48-1.06.26-2.67-1.64-6.73-2.11-9.89-1.16-.41.13-.85-.1-.98-.51-.12-.41.11-.84.51-.97 3.61-1.1 8.09-.57 11.15 1.31.36.22.48.7.27 1.07zm.11-2.84C14.5 8.61 9.4 8.43 6.24 9.39c-.49.15-1.01-.13-1.16-.62-.15-.49.13-1.01.62-1.16 3.63-1.1 9.26-.89 12.98 1.32.44.26.59.83.33 1.27-.26.44-.83.59-1.27.33z"/></svg>';

    let lastEndPolledStarted = 0;

    function fmtMMSS(s) {
        s = Math.max(0, Math.floor(s));
        return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
    }

    function todayWeeklyIndexJS(period) {
        if (period === '7day') return 6;
        return (new Date().getDay() + 6) % 7;
    }

    function formatCountJS(n) {
        n = Number(n) || 0;
        return n.toLocaleString('en-US');
    }

    function updateMusicNow(data) {
        const section = document.getElementById('music-section');
        if (!section) return;
        const now = section.querySelector('[data-music-now]');
        if (!now) return;

        now.classList.toggle('is-playing', !!data.isPlaying);

        const status = now.querySelector('[data-music-status]');
        if (status) status.textContent = data.statusText || '';

        const nameEl = now.querySelector('[data-music-name]');
        if (nameEl) {
            nameEl.textContent = data.name || '';
            if (data.url) nameEl.href = data.url;
        }

        const artistEl = now.querySelector('[data-music-artist]');
        if (artistEl) artistEl.textContent = data.artist || '';

        const cover = now.querySelector('[data-music-cover]');
        if (cover && data.image) {
            let img = cover.querySelector('img');
            if (!img) {
                img = document.createElement('img');
                img.loading = 'lazy';
                cover.insertBefore(img, cover.firstChild);
            }
            img.src = data.image;
        }

        const loved = now.querySelector('[data-music-loved]');
        if (loved) loved.classList.toggle('is-off', !data.loved);
        const liked = now.querySelector('[data-music-liked]');
        if (liked) liked.classList.toggle('is-off', !data.liked);

        const lovedCountEl = section.querySelector('[data-music-loved-count]');
        const likedCountEl = section.querySelector('[data-music-liked-count]');
        if (lovedCountEl && data.lovedCount != null) lovedCountEl.textContent = data.lovedCount;
        if (likedCountEl && data.likedCount != null) likedCountEl.textContent = data.likedCount;

        const hidName = document.getElementById('lastfm-track-name');
        const hidArtist = document.getElementById('lastfm-track-artist');
        const hidImage = document.getElementById('lastfm-track-image');
        if (hidName) hidName.textContent = data.name || '';
        if (hidArtist) hidArtist.textContent = data.artist || '';
        if (hidImage) hidImage.textContent = data.image || '';

        const progress = now.querySelector('[data-music-progress]');
        if (progress) {
            const meta = progress.parentElement;
            const elEl = meta && meta.querySelector('[data-music-elapsed]');
            const totEl = meta && meta.querySelector('[data-music-total]');
            if (data.isPlaying && data.started) {
                progress.dataset.started = String(data.started);
                progress.dataset.duration = String(data.duration || 0);
                if (totEl && data.duration) totEl.textContent = fmtMMSS(data.duration);
            } else {
                progress.dataset.started = '0';
                progress.dataset.duration = '0';
                const fill = progress.querySelector('[data-music-fill]');
                if (fill) fill.style.width = '0%';
                if (elEl) elEl.textContent = '0:00';
                if (totEl) totEl.textContent = '0:00';
            }
        }

        renderMusicRecent(section, data.recentTracks || []);
    }

    async function musicChangePeriod(period) {
        const section = document.getElementById('music-section');
        if (!section) return;
        const sel = section.querySelector('[data-music-period]');
        if (sel) sel.disabled = true;
        try {
            const res = await fetch('/api/music/stats?period=' + encodeURIComponent(period));
            if (!res.ok) throw new Error('stats fetch failed');
            const data = await res.json();
            applyMusicStats(section, data);
        } catch (e) {
            console.error('period change failed', e);
        } finally {
            if (sel) sel.disabled = false;
            if (window.mosaicUtils && window.mosaicUtils.resizeAll) window.mosaicUtils.resizeAll();
        }
    }

    function applyMusicStats(section, data) {
        const totalsWrap = section.querySelector('.music-totals');
        if (totalsWrap && Array.isArray(data.totals)) {
            totalsWrap.querySelectorAll('.music-total').forEach((card, i) => {
                const t = data.totals[i];
                if (!t) return;
                const val = card.querySelector('.music-total__value');
                if (val) val.textContent = t.Value;
                let delta = card.querySelector('.music-total__delta');
                if (t.Delta) {
                    if (!delta) {
                        delta = document.createElement('div');
                        delta.className = 'music-total__delta';
                        val.after(delta);
                    }
                    delta.textContent = t.Delta;
                    delta.classList.toggle('up', !!t.DeltaPos);
                    delta.classList.toggle('down', !t.DeltaPos);
                } else if (delta) {
                    delta.remove();
                }
            });
        }
        renderCovers(section, '.music-sub[data-role="artists"] .music-covers', data.topArtists);
        renderCovers(section, '.music-sub[data-role="albums"] .music-covers', data.topAlbums);
        renderTagsBlock(section, data.tags);
        renderWeekly(section, data.weeklyBars, data.weeklyDays, data.weeklyPeak,
            data.todayIndex != null ? data.todayIndex : todayWeeklyIndexJS(data.period));
    }

    function renderCovers(section, selector, items) {
        const wrap = section.querySelector(selector);
        if (!wrap || !Array.isArray(items)) return;
        wrap.innerHTML = '';
        items.forEach(it => {
            const card = document.createElement('div');
            card.className = 'music-cover';
            if (it.Grad) card.style.setProperty('--grad', it.Grad);
            if (it.Image) {
                const img = document.createElement('img');
                img.loading = 'lazy';
                img.src = it.Image;
                card.appendChild(img);
            } else {
                const ph = document.createElement('div');
                ph.className = 'music-cover__ph';
                ph.textContent = it.Initial || '';
                card.appendChild(ph);
            }
            const name = document.createElement('div');
            name.className = 'music-cover__name';
            name.textContent = it.Name || '';
            const plays = document.createElement('div');
            plays.className = 'music-cover__plays';
            plays.textContent = (it.Plays || 0) + ' plays';
            card.appendChild(name);
            card.appendChild(plays);
            wrap.appendChild(card);
        });
    }

    function renderTagsBlock(section, tags) {
        const wrap = section.querySelector('.music-tags');
        if (!wrap || !Array.isArray(tags)) return;
        wrap.innerHTML = '';
        tags.forEach(t => {
            const span = document.createElement('span');
            span.className = 'music-tag';
            span.style.fontSize = (t.Size || 13) + 'px';
            span.style.color = t.Color || '';
            span.textContent = t.Name || '';
            const sup = document.createElement('sup');
            sup.className = 'music-tag__count';
            sup.textContent = t.Count || '';
            span.appendChild(sup);
            wrap.appendChild(span);
        });
    }

    function renderWeekly(section, bars, days, peak, todayIdx) {
        const wrap = section.querySelector('.music-weekly__bars');
        if (!wrap || !Array.isArray(bars)) return;
        wrap.innerHTML = '';
        bars.forEach((h, i) => {
            const col = document.createElement('div');
            col.className = 'music-weekly__col' + (i === todayIdx ? ' is-today' : '');
            const track = document.createElement('div');
            track.className = 'music-weekly__track';
            const bar = document.createElement('div');
            bar.className = 'music-weekly__bar';
            bar.style.height = h + '%';
            track.appendChild(bar);
            const day = document.createElement('span');
            day.className = 'music-weekly__day';
            day.textContent = (days && days[i]) || '';
            col.appendChild(track);
            col.appendChild(day);
            wrap.appendChild(col);
        });
        const peakEl = section.querySelector('.music-weekly__peak');
        if (peakEl && peak) peakEl.textContent = 'busiest · ' + peak;
    }

    function renderMusicRecent(section, items) {
        const list = section.querySelector('[data-music-recent]');
        if (!list) return;
        list.innerHTML = '';
        items.slice(0, 5).forEach(function (it) {
            const li = document.createElement('li');
            li.className = 'music-recent__item';

            const cover = document.createElement('span');
            cover.className = 'music-recent__cover';
            if (it.image) {
                const img = document.createElement('img');
                img.loading = 'lazy';
                img.src = it.image;
                cover.appendChild(img);
            } else {
                const ph = document.createElement('span');
                ph.className = 'music-recent__ph';
                if (it.color) ph.style.background = it.color;
                ph.textContent = it.initial || '';
                cover.appendChild(ph);
            }

            const text = document.createElement('span');
            text.className = 'music-recent__text';
            const nm = document.createElement('span');
            nm.className = 'music-recent__name';
            nm.textContent = it.name || '';
            const ar = document.createElement('span');
            ar.className = 'music-recent__artist';
            ar.textContent = it.artist || '';
            text.appendChild(nm);
            text.appendChild(ar);

            const badges = document.createElement('span');
            badges.className = 'music-recent__badges';
            if (it.loved) {
                const b = document.createElement('span');
                b.className = 'badge loved';
                b.innerHTML = HEART_SVG;
                badges.appendChild(b);
            }
            if (it.liked) {
                const b = document.createElement('span');
                b.className = 'badge liked';
                b.innerHTML = SPOTIFY_SVG;
                badges.appendChild(b);
            }

            const time = document.createElement('span');
            time.className = 'music-recent__time';
            time.textContent = it.relativeTime || '';

            li.appendChild(cover);
            li.appendChild(text);
            li.appendChild(badges);
            li.appendChild(time);
            list.appendChild(li);
        });
    }

    function updateMusicStats(data) {
        const section = document.getElementById('music-section');
        if (!section) return;

        const sel = section.querySelector('[data-music-period]');
        const visible = sel ? sel.value : 'overall';
        if (data.period && data.period === visible && data.scrobbles) {
            const first = section.querySelector('.music-total');
            if (first) {
                const val = first.querySelector('.music-total__value');
                const cur = (data.scrobbles.cur != null) ? data.scrobbles.cur : data.scrobbles.Cur;
                if (val && cur != null) val.textContent = formatCountJS(cur);
            }
        }

        const weekly = Array.isArray(data.weekly) ? data.weekly : [];
        if (weekly.length) {
            const max = Math.max(1, ...weekly);
            const todayIdx = todayWeeklyIndexJS(data.period);
            section.querySelectorAll('.music-weekly__col').forEach((col, i) => {
                const bar = col.querySelector('.music-weekly__bar');
                if (bar) bar.style.height = Math.round(((weekly[i] || 0) / max) * 100) + '%';
                col.classList.toggle('is-today', i === todayIdx);
            });
        }
        if (data.peakDay) {
            const peakEl = section.querySelector('.music-weekly__peak');
            if (peakEl) peakEl.textContent = 'busiest · ' + data.peakDay;
        }
    }

    function setupRecentTracksHandlers(section) {
        if (!section) return;

        section.querySelectorAll('.recent-track-item').forEach(item => {
            if (item.dataset.lastfmBound === '1') return;
            item.dataset.lastfmBound = '1';

            if (item.classList.contains('now-playing')) {
                item.style.cursor = 'default';
                return;
            }

            const trackName = item.querySelector('.recent-track-name')?.textContent?.replace(' 🎵', '').trim() || '';
            const trackArtist = item.querySelector('.recent-track-artist')?.textContent?.trim() || '';

            if (!trackName || !trackArtist || !window.playTrack) {
                item.style.cursor = 'default';
                return;
            }

            item.style.cursor = 'pointer';
            item.addEventListener('click', function () {
                window.playTrack(`${trackArtist} ${trackName}`);
            });
        });
    }

    function initLastFMTrackActions() {
        const section = document.querySelector('.music-section');
        if (!section) return;

        let isUpdating = false;
        const observer = new MutationObserver(() => {
            if (isUpdating) return;
            isUpdating = true;
            setupRecentTracksHandlers(section);
            setTimeout(() => {
                isUpdating = false;
            }, 100);
        });

        const recentTracksList = section.querySelector('.recent-tracks-list');
        if (recentTracksList) {
            observer.observe(recentTracksList, { childList: true, subtree: false });
        }

        setupRecentTracksHandlers(section);
    }

    function initMusicInteractions() {
        const section = document.getElementById('music-section') || document.querySelector('.music-section');
        if (!section) return;

        if (section.dataset.musicClickBound !== '1') {
            section.dataset.musicClickBound = '1';
            section.addEventListener('click', function (e) {
                const item = e.target.closest('.music-recent__item');
                if (!item) return;
                const name = item.querySelector('.music-recent__name')?.textContent?.trim() || '';
                const artist = item.querySelector('.music-recent__artist')?.textContent?.trim() || '';
                if (name && artist && window.playTrack) window.playTrack(artist + ' ' + name);
            });
        }

        section.querySelectorAll('details').forEach(function (d) {
            if (d.dataset.musicToggleBound === '1') return;
            d.dataset.musicToggleBound = '1';
            d.addEventListener('toggle', function () {
                if (window.mosaicUtils && window.mosaicUtils.resizeAll) {
                    window.mosaicUtils.resizeAll();
                }
            });
        });
    }

    async function pollCurrentTrack() {
        try {
            const res = await fetch('/api/music/now');
            if (!res.ok) return;
            const data = await res.json();
            if (data && data.hasTrack) {
                updateMusicNow(data);
            }
        } catch (e) {
        }
    }

    function tickMusicProgress() {
        document.querySelectorAll('[data-music-progress]').forEach(function (p) {
            const started = parseInt(p.dataset.started || '0', 10);
            const duration = parseInt(p.dataset.duration || '0', 10);
            const fill = p.querySelector('[data-music-fill]');
            if (!fill || !started || !duration) return;
            let elapsed = Math.floor(Date.now() / 1000) - started;
            if (elapsed < 0) elapsed = 0;
            if (elapsed > duration) elapsed = duration;
            fill.style.width = ((elapsed / duration) * 100).toFixed(1) + '%';
            const meta = p.parentElement;
            const el = meta && meta.querySelector('[data-music-elapsed]');
            const tot = meta && meta.querySelector('[data-music-total]');
            if (el) el.textContent = fmtMMSS(elapsed);
            if (tot) tot.textContent = fmtMMSS(duration);
            if (elapsed >= duration && lastEndPolledStarted !== started) {
                lastEndPolledStarted = started;
                setTimeout(pollCurrentTrack, 1000);
            }
        });
    }

    setInterval(tickMusicProgress, 1000);

    window.updateMusicNow = updateMusicNow;
    window.updateMusicStats = updateMusicStats;
    window.initMusicInteractions = initMusicInteractions;
    window.musicChangePeriod = musicChangePeriod;
    window.applyMusicStats = applyMusicStats;
    window.setupRecentTracksHandlers = setupRecentTracksHandlers;

    function init() {
        initMusicInteractions();
        initLastFMTrackActions();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => setTimeout(init, 100));
    } else {
        setTimeout(init, 100);
    }
})();