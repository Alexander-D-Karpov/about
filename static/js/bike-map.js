(function () {
    'use strict';

    const COLORS = [
        '#ff8c42', '#f7c948', '#ff4e6a', '#845ec2',
        '#00c9a7', '#2c73d2', '#ff8066', '#b0a8b9',
        '#00b4d8', '#e76f51', '#2a9d8f', '#e9c46a',
        '#f4a261', '#264653', '#d62828', '#06d6a0',
        '#118ab2', '#ef476f', '#ffd166', '#073b4c',
        '#7209b7', '#3a86ff', '#fb5607', '#8338ec',
        '#ff006e', '#38b000', '#9b5de5', '#f15bb5',
        '#00bbf9', '#fee440', '#00f5d4', '#c77dff',
    ];

    let arrowMarkers = [];
    let hiddenRides = new Set();

    let map = null;
    let polylines = [];
    let rides = [];
    let mapResizeCleanup = null;
    let selectedRideIndex = null;
    let hoveredRideIndex = null;

    function drawSparklineSVG(elevData, width, height) {
        if (elevData.length < 2) return '';

        const min = Math.min(...elevData);
        const max = Math.max(...elevData);
        const range = max - min || 1;

        const pts = elevData.map((e, i) => {
            const x = (i / (elevData.length - 1)) * width;
            const y = height - ((e - min) / range) * (height - 2) - 1;
            return `${x.toFixed(1)},${y.toFixed(1)}`;
        });

        const pathD = `M ${pts.join(' L ')}`;
        const fillD = `M 0,${height} L ${pts.join(' L ')} L ${width},${height} Z`;

        return `<svg width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" aria-hidden="true">
        <path d="${fillD}" fill="rgba(255,140,66,.18)"/>
        <path d="${pathD}" fill="none" stroke="#ff8c42" stroke-width="1.5" stroke-linejoin="round"/>
    </svg>`;
    }

    function getElevationData(ride) {
        if (!ride.coordinates || ride.coordinates.length < 2) return [];
        return ride.coordinates
            .filter(c => c.length >= 3 && c[2] != null && c[2] !== 0)
            .map(c => c[2]);
    }

    function slugifyRidePart(value) {
        return String(value || '')
            .toLowerCase()
            .trim()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-+|-+$/g, '');
    }

    function getRideShareKey(idx) {
        const ride = rides[idx];
        if (!ride) return String(idx);

        const date = slugifyRidePart(ride.date || 'unknown-date');
        const name = slugifyRidePart(ride.name || 'ride');
        const dist = Number(ride.distance_km || 0).toFixed(1).replace('.', '-');

        return `${date}-${name}-${dist}km`;
    }

    function getRideIndexFromURL() {
        const url = new URL(window.location.href);
        const raw = url.searchParams.get('ride');
        if (!raw) return null;

        if (/^\d+$/.test(raw)) {
            const numeric = parseInt(raw, 10);
            return numeric >= 0 && numeric < rides.length ? numeric : null;
        }

        const idx = rides.findIndex((_, i) => getRideShareKey(i) === raw);
        return idx >= 0 ? idx : null;
    }

    function updateRideURL() {
        const url = new URL(window.location.href);

        if (selectedRideIndex == null) {
            url.searchParams.delete('ride');
        } else {
            url.searchParams.set('ride', getRideShareKey(selectedRideIndex));
        }

        history.replaceState({}, '', url.toString());
    }

    function getCurrentActiveRideIndex() {
        if (hoveredRideIndex != null) return hoveredRideIndex;
        if (selectedRideIndex != null) return selectedRideIndex;
        return null;
    }

    function syncRideHighlight({ scrollIntoView = false } = {}) {
        const activeIdx = getCurrentActiveRideIndex();

        polylines.forEach(line => {
            if (hiddenRides.has(line._bikeIndex)) return;

            if (activeIdx == null) {
                line.setStyle({ weight: 3, opacity: 0.7 });
                return;
            }

            if (line._bikeIndex === activeIdx) {
                line.setStyle({ weight: 5, opacity: 1 });
                line.bringToFront();
            } else {
                line.setStyle({ weight: 2, opacity: 0.2 });
            }
        });

        arrowMarkers.forEach(m => {
            if (hiddenRides.has(m._bikeIndex)) return;
            const el = m.getElement();
            if (!el) return;

            if (activeIdx == null) {
                el.style.opacity = '0.85';
            } else {
                el.style.opacity = m._bikeIndex === activeIdx ? '1' : '0.15';
            }
        });

        let activeItem = null;
        document.querySelectorAll('.bike-ride-item').forEach(item => {
            const isActive = parseInt(item.dataset.ride, 10) === activeIdx;
            item.classList.toggle('active', isActive);
            if (isActive) activeItem = item;
        });

        if (scrollIntoView && activeItem) {
            requestAnimationFrame(() => {
                activeItem.scrollIntoView({
                    block: 'nearest',
                    inline: 'nearest',
                    behavior: 'smooth'
                });
            });
        }
    }

    function selectRide(idx, {
        toggle = true,
        fit = false,
        openPopup = false,
        scrollIntoView = true,
        updateURL = true
    } = {}) {
        if (idx == null || idx < 0 || idx >= rides.length) return;
        if (hiddenRides.has(idx)) return;

        if (toggle && selectedRideIndex === idx) {
            selectedRideIndex = null;
            if (map) map.closePopup();
            if (updateURL) updateRideURL();
            syncRideHighlight();
            return;
        }

        selectedRideIndex = idx;

        if (updateURL) updateRideURL();

        const line = polylines.find(l => l._bikeIndex === idx);
        if (line && map) {
            if (fit) {
                map.fitBounds(line.getBounds(), { padding: [40, 40], maxZoom: 14 });
            }
            if (openPopup) {
                line.openPopup();
            }
        }

        syncRideHighlight({ scrollIntoView });
    }

    function applyRideSelectionFromURL() {
        const idx = getRideIndexFromURL();
        if (idx == null) return;

        selectRide(idx, {
            toggle: false,
            fit: true,
            openPopup: true,
            scrollIntoView: true,
            updateURL: false
        });
    }

    function toggleRideVisibility(idx) {
        const line = polylines.find(l => l._bikeIndex === idx);
        const rideArrows = arrowMarkers.filter(m => m._bikeIndex === idx);
        const item = document.querySelector(`.bike-ride-item[data-ride="${idx}"]`);
        const btn = item && item.querySelector('.bike-toggle-btn');

        if (hiddenRides.has(idx)) {
            hiddenRides.delete(idx);
            if (line && map) line.addTo(map);
            rideArrows.forEach(m => { if (map) m.addTo(map); });
            if (item) item.classList.remove('ride-hidden');
            if (btn) btn.setAttribute('aria-pressed', 'false');
        } else {
            hiddenRides.add(idx);
            if (line && map) map.removeLayer(line);
            rideArrows.forEach(m => { if (map) map.removeLayer(m); });
            if (item) item.classList.add('ride-hidden');
            if (btn) btn.setAttribute('aria-pressed', 'true');

            if (selectedRideIndex === idx) {
                selectedRideIndex = null;
                updateRideURL();
            }
            if (hoveredRideIndex === idx) {
                hoveredRideIndex = null;
            }
        }

        syncRideHighlight();
    }

    function getBearing(a, b) {
        const toRad = Math.PI / 180;
        const dLon = (b[1] - a[1]) * toRad;
        const lat1 = a[0] * toRad;
        const lat2 = b[0] * toRad;
        const y = Math.sin(dLon) * Math.cos(lat2);
        const x = Math.cos(lat1) * Math.sin(lat2) - Math.sin(lat1) * Math.cos(lat2) * Math.cos(dLon);
        return (Math.atan2(y, x) * 180 / Math.PI + 360) % 360;
    }

    function getDistanceKm(a, b) {
        const R = 6371;
        const toRad = Math.PI / 180;
        const dLat = (b[0] - a[0]) * toRad;
        const dLon = (b[1] - a[1]) * toRad;
        const s = Math.sin(dLat / 2) * Math.sin(dLat / 2) +
            Math.cos(a[0] * toRad) * Math.cos(b[0] * toRad) *
            Math.sin(dLon / 2) * Math.sin(dLon / 2);
        return R * 2 * Math.atan2(Math.sqrt(s), Math.sqrt(1 - s));
    }

    function placeArrows(latlngs, color, intervalKm) {
        if (latlngs.length < 2) return [];

        const markers = [];
        let accumulated = 0;
        let nextAt = intervalKm * 0.4;

        for (let i = 1; i < latlngs.length; i++) {
            const segDist = getDistanceKm(
                [latlngs[i - 1][0], latlngs[i - 1][1]],
                [latlngs[i][0], latlngs[i][1]]
            );
            accumulated += segDist;

            if (accumulated >= nextAt) {
                const bearing = getBearing(
                    [latlngs[i - 1][0], latlngs[i - 1][1]],
                    [latlngs[i][0], latlngs[i][1]]
                );

                const t = segDist > 0 ? Math.min(1, (accumulated - nextAt + segDist) / segDist) : 0.5;
                const lat = latlngs[i - 1][0] + (latlngs[i][0] - latlngs[i - 1][0]) * t;
                const lng = latlngs[i - 1][1] + (latlngs[i][1] - latlngs[i - 1][1]) * t;

                const m = L.marker([lat, lng], {
                    icon: makeArrowIcon(bearing, color, 14),
                    interactive: false,
                    keyboard: false
                });
                m._bearing = bearing;
                m._color = color;
                markers.push(m);
                nextAt = accumulated + intervalKm;
            }
        }

        return markers;
    }

    function makeArrowIcon(bearing, color, size) {
        return L.divIcon({
            className: 'bike-arrow',
            html: `<svg width="${size}" height="${size}" viewBox="0 0 20 20" style="transform:rotate(${bearing}deg)">
            <circle cx="10" cy="10" r="9" fill="${color}" stroke="rgba(0,0,0,.4)" stroke-width="1"/>
            <path d="M10 4 L15 12 L10 10 L5 12 Z" fill="white"/>
        </svg>`,
            iconSize: [size, size],
            iconAnchor: [size / 2, size / 2]
        });
    }

    function getArrowSize(zoom) {
        if (zoom <= 8) return 6;
        if (zoom <= 10) return 8;
        if (zoom <= 12) return 12;
        if (zoom <= 14) return 16;
        return 20;
    }

    function updateArrowSizes() {
        if (!map) return;
        const size = getArrowSize(map.getZoom());
        arrowMarkers.forEach(m => {
            m.setIcon(makeArrowIcon(m._bearing, m._color, size));
        });
    }

    function getArrowInterval(distKm) {
        if (distKm < 10) return 1.5;
        if (distKm < 30) return 3;
        if (distKm < 60) return 5;
        if (distKm < 100) return 8;
        return 12;
    }

    function invalidateBikeMapSize() {
        if (!map) return;

        [0, 50, 150, 300].forEach(delay => {
            setTimeout(() => {
                if (!map) return;
                map.invalidateSize({ pan: false, animate: false });
            }, delay);
        });
    }

    function setupMapResizeHandling(targetEl) {
        if (!targetEl) return () => {};

        const refresh = () => invalidateBikeMapSize();

        const onWindowResize = () => refresh();
        const onOrientationChange = () => refresh();
        const onVisibilityChange = () => {
            if (!document.hidden) refresh();
        };
        const onFullscreenChange = () => refresh();

        window.addEventListener('resize', onWindowResize);
        window.addEventListener('orientationchange', onOrientationChange);
        document.addEventListener('visibilitychange', onVisibilityChange);
        document.addEventListener('fullscreenchange', onFullscreenChange);

        let resizeObserver = null;
        if (typeof ResizeObserver !== 'undefined') {
            resizeObserver = new ResizeObserver(() => refresh());
            resizeObserver.observe(targetEl);
        }

        refresh();

        return () => {
            window.removeEventListener('resize', onWindowResize);
            window.removeEventListener('orientationchange', onOrientationChange);
            document.removeEventListener('visibilitychange', onVisibilityChange);
            document.removeEventListener('fullscreenchange', onFullscreenChange);
            if (resizeObserver) resizeObserver.disconnect();
        };
    }

    function addBikeFullscreenControl(fullscreenTarget) {
        if (!map || !window.L || !fullscreenTarget) return;

        const BikeFullscreenControl = L.Control.extend({
            options: { position: 'topright' },

            onAdd() {
                const wrapper = L.DomUtil.create('div', 'leaflet-bar bike-fullscreen-control');
                const btn = L.DomUtil.create('button', 'bike-fullscreen-btn', wrapper);

                btn.type = 'button';
                btn.title = 'Toggle fullscreen';
                btn.setAttribute('aria-label', 'Toggle fullscreen');
                btn.innerHTML = `<svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor" aria-hidden="true">
                <path d="M7 14H5v5h5v-2H7v-3zm0-4h2V7h3V5H5v5zm10 7h-3v2h5v-5h-2v3zm0-12V5h-5v2h3v3h2V5z"/>
            </svg>`;

                const syncState = () => {
                    const isFullscreen = document.fullscreenElement === fullscreenTarget;
                    btn.setAttribute('aria-pressed', isFullscreen ? 'true' : 'false');
                };

                L.DomEvent.disableClickPropagation(wrapper);
                L.DomEvent.disableScrollPropagation(wrapper);

                L.DomEvent.on(btn, 'click', async (e) => {
                    L.DomEvent.stop(e);
                    try {
                        if (document.fullscreenElement === fullscreenTarget) {
                            await document.exitFullscreen();
                        } else if (fullscreenTarget.requestFullscreen) {
                            await fullscreenTarget.requestFullscreen();
                        }
                    } catch (err) {
                        console.error('Fullscreen toggle failed:', err);
                    } finally {
                        syncState();
                        invalidateBikeMapSize();
                    }
                });

                document.addEventListener('fullscreenchange', syncState);
                this._syncState = syncState;
                syncState();

                return wrapper;
            },

            onRemove() {
                if (this._syncState) {
                    document.removeEventListener('fullscreenchange', this._syncState);
                }
            }
        });

        map.addControl(new BikeFullscreenControl());
    }

    function setupRideListScrolling() {
        const list = document.querySelector('.bike-rides-list');
        if (!list) return;
        list.style.overflowY = 'auto';
        list.style.overflowX = 'hidden';
        list.style.scrollBehavior = 'smooth';
        list.style.webkitOverflowScrolling = 'touch';
    }

    function init() {
        const dataEl = document.getElementById('bike-data');
        if (!dataEl) return;

        try {
            rides = JSON.parse(dataEl.textContent || '[]');
        } catch {
            return;
        }

        if (!rides.length) return;

        loadLeaflet().then(createMap).catch(console.error);
    }

    function loadLeaflet() {
        return new Promise((resolve, reject) => {
            if (window.L) {
                resolve();
                return;
            }

            const css = document.createElement('link');
            css.rel = 'stylesheet';
            css.href = '/static/libs/leaflet/leaflet.css';
            css.onerror = () => { css.href = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.css'; };
            document.head.appendChild(css);

            const js = document.createElement('script');
            js.src = '/static/libs/leaflet/leaflet.js';
            js.onload = resolve;
            js.onerror = () => {
                js.src = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.js';
                js.onload = resolve;
                js.onerror = reject;
            };
            document.head.appendChild(js);
        });
    }

    function createMap() {
        const container = document.getElementById('bike-map');
        if (!container) return;

        const fullscreenTarget = container.closest('.bike-map-container') || container;

        map = L.map(container, { zoomControl: true, attributionControl: true, preferCanvas: true });

        const isDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
        L.tileLayer(
            isDark
                ? 'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png'
                : 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',
            { maxZoom: 18 }
        ).addTo(map);

        const allCoords = [];

        rides.forEach((ride, i) => {
            if (!ride.coordinates || ride.coordinates.length < 2) return;

            const color = COLORS[i % COLORS.length];
            const latlngs = ride.coordinates.map(c => [c[0], c[1]]);
            allCoords.push(...latlngs);

            const line = L.polyline(latlngs, {
                color: color,
                weight: 3,
                opacity: 0.7,
                smoothFactor: 1
            }).addTo(map);

            line._bikeIndex = i;
            line._bikeColor = color;

            line.bindPopup(
                `<strong>${ride.name || 'Ride'}</strong><br>` +
                `${ride.distance_km?.toFixed(1) || '?'} km · ↑${ride.elevation_gain_m?.toFixed(0) || '?'}m` +
                (ride.date ? `<br><small>${ride.date}</small>` : ''),
                { maxWidth: 220 }
            );

            line.on('mouseover', () => highlightRide(i));
            line.on('mouseout', () => resetHighlight());
            line.on('click', () => {
                selectRide(i, {
                    toggle: true,
                    fit: true,
                    openPopup: true,
                    scrollIntoView: true,
                    updateURL: true
                });
                invalidateBikeMapSize();
            });

            polylines.push(line);

            const interval = getArrowInterval(ride.distance_km || 0);
            const arrows = placeArrows(latlngs, color, interval);
            arrows.forEach(m => {
                m.addTo(map);
                m._bikeIndex = i;
            });
            arrowMarkers.push(...arrows);

            const colorEl = document.querySelector(`.bike-ride-color[data-ride-color="${i}"]`);
            if (colorEl) colorEl.style.background = color;
        });

        if (allCoords.length) {
            map.fitBounds(L.latLngBounds(allCoords), { padding: [30, 30], maxZoom: 13 });
        }

        const loading = document.getElementById('bike-map-loading');
        if (loading) loading.classList.add('hidden');

        const fitBtn = document.getElementById('bike-fit-bounds');
        if (fitBtn) {
            fitBtn.addEventListener('click', () => {
                if (allCoords.length) {
                    map.fitBounds(L.latLngBounds(allCoords), { padding: [30, 30], maxZoom: 13 });
                    invalidateBikeMapSize();
                }
            });
        }

        map.on('zoomend', updateArrowSizes);
        map.on('popupopen', invalidateBikeMapSize);
        map.on('popupclose', invalidateBikeMapSize);
        updateArrowSizes();

        addBikeFullscreenControl(fullscreenTarget);
        setupRideListScrolling();

        if (mapResizeCleanup) {
            mapResizeCleanup();
            mapResizeCleanup = null;
        }
        mapResizeCleanup = setupMapResizeHandling(fullscreenTarget);

        invalidateBikeMapSize();

        [120, 350, 700].forEach(d => setTimeout(() => {
            if (map && allCoords.length && selectedRideIndex == null) {
                map.invalidateSize({ animate: false });
                map.fitBounds(L.latLngBounds(allCoords), { padding: [30, 30], maxZoom: 13 });
            }
        }, d));

        document.querySelectorAll('.bike-ride-item').forEach(item => {
            const idx = parseInt(item.dataset.ride, 10);

            const toggleBtn = document.createElement('button');
            toggleBtn.className = 'bike-toggle-btn';
            toggleBtn.type = 'button';
            toggleBtn.title = 'Toggle visibility';
            toggleBtn.setAttribute('aria-pressed', 'false');
            toggleBtn.setAttribute('aria-label', 'Toggle ride visibility');
            toggleBtn.innerHTML = `<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor">
            <path d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z"/>
        </svg>`;
            toggleBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                toggleRideVisibility(idx);
            });

            const chips = item.querySelector('.bike-ride-chips');
            if (chips) chips.after(toggleBtn);
            else item.appendChild(toggleBtn);

            item.addEventListener('mouseenter', () => {
                if (!hiddenRides.has(idx)) highlightRide(idx);
            });
            item.addEventListener('mouseleave', () => resetHighlight());
            item.addEventListener('click', (e) => {
                if (e.target.closest('.bike-toggle-btn')) return;
                selectRide(idx, {
                    toggle: true,
                    fit: true,
                    openPopup: true,
                    scrollIntoView: true,
                    updateURL: true
                });
                invalidateBikeMapSize();
            });
        });

        applyRideSelectionFromURL();
    }

    function highlightRide(idx) {
        hoveredRideIndex = idx;
        syncRideHighlight({ scrollIntoView: true });
    }

    function resetHighlight() {
        hoveredRideIndex = null;
        syncRideHighlight();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        setTimeout(init, 50);
    }

    window.reinitBikeMap = function () {
        if (mapResizeCleanup) {
            mapResizeCleanup();
            mapResizeCleanup = null;
        }
        if (map) {
            map.remove();
            map = null;
        }
        polylines = [];
        arrowMarkers = [];
        hiddenRides = new Set();
        selectedRideIndex = null;
        hoveredRideIndex = null;
        init();
    };
})();