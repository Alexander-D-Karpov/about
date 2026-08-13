(function () {
    'use strict';

    const NS = (window.BikeMap = window.BikeMap || {});

    NS.COLORS = [
        '#ff8c42', '#f7c948', '#ff4e6a', '#845ec2',
        '#00c9a7', '#2c73d2', '#ff8066', '#b0a8b9',
        '#00b4d8', '#e76f51', '#2a9d8f', '#e9c46a',
        '#f4a261', '#264653', '#d62828', '#06d6a0',
        '#118ab2', '#ef476f', '#ffd166', '#073b4c',
        '#7209b7', '#3a86ff', '#fb5607', '#8338ec',
        '#ff006e', '#38b000', '#9b5de5', '#f15bb5',
        '#00bbf9', '#fee440', '#00f5d4', '#c77dff'
    ];

    NS.SPEED_STOPS = [
        { v: 0, c: [12, 7, 60] },
        { v: 5, c: [74, 12, 107] },
        { v: 10, c: [125, 27, 108] },
        { v: 15, c: [178, 48, 91] },
        { v: 20, c: [222, 73, 64] },
        { v: 25, c: [246, 121, 33] },
        { v: 30, c: [252, 168, 16] },
        { v: 35, c: [249, 214, 49] },
        { v: 40, c: [252, 255, 164] }
    ];

    NS.SPEED_MAX = 40;
    NS.SPEED_BUCKET = 1.5;
    NS.SPEED_NOISE_CAP = 80;
    // Segment speed (km/h) below which the rider counts as stopped; time under
    // this is excluded from moving-time / average-speed. Matches
    // movingSpeedThresholdKmh in internal/plugins/bike.go.
    NS.MOVE_MIN_KMH = 3;
    NS.CELL_DEG = 0.0003;

    NS.CHART_H = 150;
    NS.PAD_L = 40;
    NS.PAD_R = 12;
    NS.PAD_T = 12;
    NS.PAD_B = 20;

    NS.FALLBACK_CENTER = [20, 0];
    NS.FALLBACK_ZOOM = 2;
    NS.MAX_FIT_RETRIES = 12;
    NS.MIN_EXTENT_SPAN = 0.002;
    NS.SIZE_WAIT_MS = 2500;
    NS.REVEAL_SAFETY_MS = 2000;

    NS.ICONS = {
        fit: '<svg viewBox="0 0 24 24" width="17" height="17" fill="currentColor" aria-hidden="true"><path d="M15 3l2.3 2.3-2.89 2.87 1.42 1.42L18.7 6.7 21 9V3h-6zM3 9l2.3-2.3 2.87 2.89 1.42-1.42L6.7 5.3 9 3H3v6zm6 12l-2.3-2.3 2.89-2.87-1.42-1.42L5.3 17.3 3 15v6h6zm12-6l-2.3 2.3-2.87-2.89-1.42 1.42 2.89 2.87L15 21h6v-6z"/></svg>',
        speed: '<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4.2 17.5a8.5 8.5 0 1 1 15.6 0"/><path d="M12 17.2l4.4-5"/><circle cx="12" cy="17.4" r="1.5" fill="currentColor" stroke="none"/></svg>',
        fullscreen: '<svg viewBox="0 0 24 24" width="17" height="17" fill="currentColor" aria-hidden="true"><path d="M4 9V4h5v2H6v3H4zm11-5h5v5h-2V6h-3V4zM4 15h2v3h3v2H4v-5zm14 0h2v5h-5v-2h3v-3z"/></svg>',
        eye: '<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z"/></svg>'
    };

    function freshState() {
        return {
            map: null,
            section: null,
            container: null,
            rides: [],
            profiles: [],
            plainLines: [],
            arrowMarkers: [],
            hiddenRides: new Set(),
            selectedRideIndex: null,
            hoveredRideIndex: null,
            speedMode: false,
            speedRideCount: 0,
            noSpeedRideCount: 0,
            hoverMarker: null,
            speedLayer: null,
            noSpeedLayer: null,
            speedLayerKey: null,
            speedLayerWeight: 0,
            halo: null,
            speedBtn: null,
            fullscreenBtn: null,
            autoView: true,
            programmaticView: 0,
            chart: null,
            chartHost: null,
            chartRO: null,
            profileBox: null,
            profileTitle: null,
            profileReadout: null,
            mapResizeCleanup: null,
            sizeWaitCancel: null,
            toastTimer: null,
            fitRetryTimer: null,
            fitRetries: 0,
            sizeObserver: null,
            revealed: false,
            revealTimer: null,
            token: 0
        };
    }

    NS.state = freshState();

    NS.resetState = function () {
        const prevToken = NS.state ? NS.state.token : 0;
        NS.state = freshState();
        NS.state.token = prevToken + 1;
        return NS.state;
    };

    NS.clamp = function (v, a, b) {
        return Math.max(a, Math.min(b, v));
    };

    NS.num = function (v) {
        return typeof v === 'number' && isFinite(v);
    };

    NS.rgbStr = function (c) {
        return 'rgb(' + c[0] + ',' + c[1] + ',' + c[2] + ')';
    };

    NS.speedColor = function (v) {
        if (!NS.num(v)) return '#6b7280';
        const stops = NS.SPEED_STOPS;
        if (v <= stops[0].v) return NS.rgbStr(stops[0].c);
        if (v >= stops[stops.length - 1].v) return NS.rgbStr(stops[stops.length - 1].c);
        for (let i = 1; i < stops.length; i++) {
            if (v <= stops[i].v) {
                const a = stops[i - 1];
                const b = stops[i];
                const t = (v - a.v) / (b.v - a.v);
                return NS.rgbStr([
                    Math.round(a.c[0] + (b.c[0] - a.c[0]) * t),
                    Math.round(a.c[1] + (b.c[1] - a.c[1]) * t),
                    Math.round(a.c[2] + (b.c[2] - a.c[2]) * t)
                ]);
            }
        }
        return NS.rgbStr(stops[stops.length - 1].c);
    };

    NS.speedBucket = function (v) {
        if (!NS.num(v)) return -1;
        return Math.round(NS.clamp(v, 0, NS.SPEED_MAX + NS.SPEED_BUCKET) / NS.SPEED_BUCKET);
    };

    NS.cellKey = function (lat, lng) {
        return Math.round(lat / NS.CELL_DEG) + ':' + Math.round(lng / NS.CELL_DEG);
    };

    NS.getDistanceKm = function (a, b) {
        const R = 6371;
        const toRad = Math.PI / 180;
        const dLat = (b[0] - a[0]) * toRad;
        const dLon = (b[1] - a[1]) * toRad;
        const s = Math.sin(dLat / 2) * Math.sin(dLat / 2) +
            Math.cos(a[0] * toRad) * Math.cos(b[0] * toRad) *
            Math.sin(dLon / 2) * Math.sin(dLon / 2);
        return R * 2 * Math.atan2(Math.sqrt(s), Math.sqrt(1 - s));
    };

    NS.getBearing = function (a, b) {
        const toRad = Math.PI / 180;
        const dLon = (b[1] - a[1]) * toRad;
        const lat1 = a[0] * toRad;
        const lat2 = b[0] * toRad;
        const y = Math.sin(dLon) * Math.cos(lat2);
        const x = Math.cos(lat1) * Math.sin(lat2) - Math.sin(lat1) * Math.cos(lat2) * Math.cos(dLon);
        return (Math.atan2(y, x) * 180 / Math.PI + 360) % 360;
    };

    NS.validCoord = function (c) {
        if (!c || !NS.num(c[0]) || !NS.num(c[1])) return false;
        if (c[0] === 0 && c[1] === 0) return false;
        return Math.abs(c[0]) <= 90 && Math.abs(c[1]) <= 180;
    };

    NS.fillGaps = function (arr) {
        const n = arr.length;
        const out = arr.slice();
        let last = null;
        for (let i = 0; i < n; i++) {
            if (!NS.num(out[i])) out[i] = last;
            else last = out[i];
        }
        last = null;
        for (let i = n - 1; i >= 0; i--) {
            if (!NS.num(out[i])) out[i] = last;
            else last = out[i];
        }
        for (let i = 0; i < n; i++) {
            if (!NS.num(out[i])) out[i] = 0;
        }
        return out;
    };

    NS.smooth = function (arr, win) {
        const n = arr.length;
        const out = new Array(n);
        for (let i = 0; i < n; i++) {
            let sum = 0;
            let cnt = 0;
            for (let j = i - win; j <= i + win; j++) {
                if (j < 0 || j >= n) continue;
                if (!NS.num(arr[j])) continue;
                sum += arr[j];
                cnt++;
            }
            out[i] = cnt ? sum / cnt : 0;
        }
        return out;
    };

    NS.fmtClock = function (sec) {
        if (!NS.num(sec)) return '';
        const s = Math.max(0, Math.round(sec));
        const h = Math.floor(s / 3600);
        const m = Math.floor((s % 3600) / 60);
        const ss = s % 60;
        const pad = v => String(v).padStart(2, '0');
        return h > 0 ? h + ':' + pad(m) + ':' + pad(ss) : m + ':' + pad(ss);
    };

    NS.slugify = function (value) {
        return String(value || '')
            .toLowerCase()
            .trim()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-+|-+$/g, '');
    };

    NS.mapReady = function () {
        const map = NS.state.map;
        if (!map) return false;
        if (!map._loaded) return false;
        try {
            return !!map.getContainer() && !!map.getCenter();
        } catch (e) {
            return false;
        }
    };

    NS.containerSize = function () {
        const map = NS.state.map;
        let el = null;
        try {
            el = (map && map.getContainer()) || NS.state.container;
        } catch (e) {
            el = NS.state.container;
        }
        if (!el) return { w: 0, h: 0 };
        return { w: el.clientWidth || 0, h: el.clientHeight || 0 };
    };

    NS.elementSized = function (el) {
        return !!el && el.clientWidth > 20 && el.clientHeight > 20;
    };

    NS.hasUsableSize = function () {
        const s = NS.containerSize();
        return s.w > 20 && s.h > 20;
    };

    NS.whenSized = function (el, cb) {
        if (!el) {
            cb(false);
            return function () {};
        }
        if (NS.elementSized(el)) {
            cb(true);
            return function () {};
        }

        let done = false;
        let ro = null;
        let poll = null;
        let timer = null;

        const finish = function (sized) {
            if (done) return;
            done = true;
            if (ro) ro.disconnect();
            if (poll) clearInterval(poll);
            if (timer) clearTimeout(timer);
            cb(sized);
        };

        timer = setTimeout(function () {
            finish(NS.elementSized(el));
        }, NS.SIZE_WAIT_MS);

        if (typeof ResizeObserver !== 'undefined') {
            ro = new ResizeObserver(function () {
                if (NS.elementSized(el)) finish(true);
            });
            ro.observe(el);
        } else {
            poll = setInterval(function () {
                if (NS.elementSized(el)) finish(true);
            }, 100);
        }

        return function () {
            if (done) return;
            done = true;
            if (ro) ro.disconnect();
            if (poll) clearInterval(poll);
            if (timer) clearTimeout(timer);
        };
    };

    NS.extentToBounds = function (ext) {
        if (!ext || !window.L) return null;

        let minLat = ext.minLat;
        let minLng = ext.minLng;
        let maxLat = ext.maxLat;
        let maxLng = ext.maxLng;

        if (!NS.num(minLat) || !NS.num(minLng) || !NS.num(maxLat) || !NS.num(maxLng)) return null;
        if (minLat > maxLat || minLng > maxLng) return null;

        const span = NS.MIN_EXTENT_SPAN;

        if (maxLat - minLat < span) {
            const c = (minLat + maxLat) / 2;
            minLat = c - span / 2;
            maxLat = c + span / 2;
        }
        if (maxLng - minLng < span) {
            const c = (minLng + maxLng) / 2;
            minLng = c - span / 2;
            maxLng = c + span / 2;
        }

        minLat = NS.clamp(minLat, -85, 85);
        maxLat = NS.clamp(maxLat, -85, 85);
        minLng = NS.clamp(minLng, -180, 180);
        maxLng = NS.clamp(maxLng, -180, 180);

        if (!(maxLat > minLat) || !(maxLng > minLng)) return null;

        try {
            const b = L.latLngBounds([minLat, minLng], [maxLat, maxLng]);
            return (b && b.isValid()) ? b : null;
        } catch (e) {
            return null;
        }
    };

    NS.addLayerSafe = function (layer) {
        const map = NS.state.map;
        if (!layer || !NS.mapReady()) return false;
        try {
            if (map.hasLayer(layer)) return true;
            layer.addTo(map);
            return true;
        } catch (e) {
            console.debug('[bike] addLayer failed:', e);
            return false;
        }
    };

    NS.removeLayerSafe = function (layer) {
        const map = NS.state.map;
        if (!layer || !map) return;
        try {
            if (map.hasLayer(layer)) map.removeLayer(layer);
        } catch (e) {
            console.debug('[bike] removeLayer failed:', e);
        }
    };

    NS.notifyLayout = function () {
        const plugin = document.getElementById('bike-plugin');
        if (!plugin) return;
        if (window.mosaicUtils && window.mosaicUtils.notifyContentChanged) {
            window.mosaicUtils.notifyContentChanged(plugin);
        } else if (window.mosaicUtils && window.mosaicUtils.resizeAll) {
            window.mosaicUtils.resizeAll();
        }
    };

    NS.invalidateMapSize = function () {
        if (!NS.state.map) return;
        [0, 50, 150, 300].forEach(function (delay) {
            setTimeout(function () {
                const map = NS.state.map;
                if (!map) return;
                try {
                    map.invalidateSize({ pan: false, animate: false });
                } catch (e) {}
            }, delay);
        });
    };

    NS.showMapToast = function (text) {
        const host = document.getElementById('bike-map-container');
        if (!host) return;
        let el = host.querySelector('.bike-map-toast');
        if (!el) {
            el = document.createElement('div');
            el.className = 'bike-map-toast';
            host.appendChild(el);
        }
        el.textContent = text;
        el.classList.add('in');
        clearTimeout(NS.state.toastTimer);
        NS.state.toastTimer = setTimeout(function () {
            el.classList.remove('in');
        }, 4000);
    };

    NS.showMapMessage = function (message) {
        const loading = document.getElementById('bike-map-loading');
        if (!loading) return;
        loading.classList.remove('hidden');
        loading.innerHTML = '<span>' + message + '</span>';
    };

    NS.hideMapLoading = function () {
        const loading = document.getElementById('bike-map-loading');
        if (!loading) return;
        loading.classList.add('hidden');
    };

    NS.loadLeaflet = function () {
        return new Promise(function (resolve, reject) {
            if (window.L) {
                resolve();
                return;
            }

            const existing = document.querySelector('script[data-leaflet-loading]');
            if (existing) {
                const wait = setInterval(function () {
                    if (window.L) {
                        clearInterval(wait);
                        resolve();
                    }
                }, 100);
                setTimeout(function () {
                    clearInterval(wait);
                    if (!window.L) reject(new Error('leaflet load timeout'));
                }, 15000);
                return;
            }

            const css = document.createElement('link');
            css.rel = 'stylesheet';
            css.href = '/static/libs/leaflet/leaflet.css';
            css.onerror = function () {
                css.href = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.css';
            };
            document.head.appendChild(css);

            const js = document.createElement('script');
            js.src = '/static/libs/leaflet/leaflet.js';
            js.setAttribute('data-leaflet-loading', '1');
            js.onload = function () { resolve(); };
            js.onerror = function () {
                js.src = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.js';
                js.onload = function () { resolve(); };
                js.onerror = reject;
            };
            document.head.appendChild(js);
        });
    };
})();