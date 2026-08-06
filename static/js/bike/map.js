(function () {
    'use strict';

    const NS = (window.BikeMap = window.BikeMap || {});

    function makeArrowIcon(bearing, color, size) {
        return L.divIcon({
            className: 'bike-arrow',
            html: '<svg width="' + size + '" height="' + size + '" viewBox="0 0 20 20" style="transform:rotate(' + bearing + 'deg)">' +
                '<circle cx="10" cy="10" r="9" fill="' + color + '" stroke="rgba(0,0,0,.4)" stroke-width="1"/>' +
                '<path d="M10 4 L15 12 L10 10 L5 12 Z" fill="white"/></svg>',
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
        if (!NS.mapReady()) return;
        let zoom;
        try {
            zoom = NS.state.map.getZoom();
        } catch (e) {
            return;
        }
        if (!NS.num(zoom)) return;
        const size = getArrowSize(zoom);
        NS.state.arrowMarkers.forEach(function (m) {
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

    function placeArrows(latlngs, color, intervalKm, size) {
        if (latlngs.length < 2) return [];

        const markers = [];
        let accumulated = 0;
        let nextAt = intervalKm * 0.4;

        for (let i = 1; i < latlngs.length; i++) {
            const segDist = NS.getDistanceKm(
                [latlngs[i - 1][0], latlngs[i - 1][1]],
                [latlngs[i][0], latlngs[i][1]]
            );
            accumulated += segDist;

            if (accumulated >= nextAt) {
                const bearing = NS.getBearing(
                    [latlngs[i - 1][0], latlngs[i - 1][1]],
                    [latlngs[i][0], latlngs[i][1]]
                );
                const t = segDist > 0 ? Math.min(1, (accumulated - nextAt + segDist) / segDist) : 0.5;
                const lat = latlngs[i - 1][0] + (latlngs[i][0] - latlngs[i - 1][0]) * t;
                const lng = latlngs[i - 1][1] + (latlngs[i][1] - latlngs[i - 1][1]) * t;

                const m = L.marker([lat, lng], {
                    icon: makeArrowIcon(bearing, color, size),
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

    function ensureHoverMarker() {
        const st = NS.state;
        if (st.hoverMarker || !window.L) return st.hoverMarker;

        st.hoverMarker = L.marker([0, 0], {
            icon: L.divIcon({
                className: 'bike-hover-dot',
                html: '<span></span>',
                iconSize: [18, 18],
                iconAnchor: [9, 9]
            }),
            interactive: false,
            keyboard: false,
            zIndexOffset: 10000
        });
        return st.hoverMarker;
    }

    NS.showHoverPoint = function (rideIdx, i) {
        if (!NS.mapReady()) return;

        const prof = NS.state.profiles[rideIdx];
        if (!prof) return;

        const p = prof.points[i];
        if (!p || !NS.num(p.lat) || !NS.num(p.lng)) return;

        const m = ensureHoverMarker();
        if (!m) return;

        m.setLatLng([p.lat, p.lng]);
        NS.addLayerSafe(m);

        const el = m.getElement();
        const dot = el && el.firstElementChild;
        if (dot) {
            dot.style.background = (NS.state.speedMode && prof.hasTime)
                ? NS.speedColor(prof.speedSeries[i])
                : NS.COLORS[rideIdx % NS.COLORS.length];
        }
    };

    NS.clearHoverPoint = function () {
        NS.removeLayerSafe(NS.state.hoverMarker);
    };

    function fitPadding() {
        const s = NS.containerSize();
        const base = Math.min(s.w || 0, s.h || 0);
        if (!base) return [12, 12];
        const p = NS.clamp(Math.round(base * 0.035), 6, 24);
        return [p, p];
    }

    function rideExtent(i) {
        const p = NS.state.profiles[i];
        if (!p) return null;
        return { minLat: p.minLat, minLng: p.minLng, maxLat: p.maxLat, maxLng: p.maxLng };
    }

    NS.rideExtent = rideExtent;

    function ridesExtent(onlyVisible) {
        const st = NS.state;
        let minLat = Infinity;
        let maxLat = -Infinity;
        let minLng = Infinity;
        let maxLng = -Infinity;
        let found = false;

        st.rides.forEach(function (_, i) {
            if (onlyVisible && st.hiddenRides.has(i)) return;
            const p = st.profiles[i];
            if (!p) return;
            found = true;
            if (p.minLat < minLat) minLat = p.minLat;
            if (p.maxLat > maxLat) maxLat = p.maxLat;
            if (p.minLng < minLng) minLng = p.minLng;
            if (p.maxLng > maxLng) maxLng = p.maxLng;
        });

        if (!found) return null;
        if (!NS.num(minLat) || !NS.num(maxLat) || !NS.num(minLng) || !NS.num(maxLng)) return null;
        return { minLat: minLat, minLng: minLng, maxLat: maxLat, maxLng: maxLng };
    }

    NS.ridesExtent = ridesExtent;

    function beginProgrammaticView() {
        NS.state.programmaticView++;
        NS.state.autoView = true;
        setTimeout(function () {
            NS.state.programmaticView = Math.max(0, NS.state.programmaticView - 1);
        }, 700);
    }

    function scheduleFitRetry(extent, maxZoom, animate) {
        const st = NS.state;
        if (st.fitRetryTimer || st.fitRetries >= NS.MAX_FIT_RETRIES) return;

        const token = st.token;
        st.fitRetries++;
        st.fitRetryTimer = setTimeout(function () {
            const cur = NS.state;
            if (!cur || cur.token !== token) return;
            cur.fitRetryTimer = null;
            fitExtent(extent, maxZoom, animate);
        }, 120);
    }

    function fitExtent(extent, maxZoom, animate) {
        const st = NS.state;
        if (!st.map || !extent) return false;

        if (!NS.hasUsableSize()) {
            scheduleFitRetry(extent, maxZoom, animate);
            return false;
        }

        const bounds = NS.extentToBounds(extent);
        if (!bounds) return false;

        try {
            st.map.invalidateSize({ pan: false, animate: false });
            beginProgrammaticView();
            st.map.fitBounds(bounds, {
                padding: fitPadding(),
                maxZoom: maxZoom,
                animate: !!animate
            });
            st.fitRetries = 0;
            return true;
        } catch (e) {
            console.debug('[bike] fitBounds failed:', e);
            return false;
        }
    }

    NS.fitExtent = fitExtent;

    NS.fitAllRides = function (animate) {
        const ext = ridesExtent(true) || ridesExtent(false);
        if (!ext) return false;
        return fitExtent(ext, 16, animate);
    };

    NS.activeRideIndex = function () {
        const st = NS.state;
        if (st.hoveredRideIndex != null) return st.hoveredRideIndex;
        if (st.selectedRideIndex != null) return st.selectedRideIndex;
        return null;
    };

    function styleRide(i, state) {
        const st = NS.state;
        const plain = st.plainLines[i];
        if (!plain || !NS.mapReady()) return;

        try {
            if (!st.map.hasLayer(plain)) return;

            if (st.speedMode) {
                plain.setStyle({ weight: 14, opacity: 0 });
                return;
            }
            if (state === 'active') {
                plain.setStyle({ weight: 5, opacity: 1 });
                plain.bringToFront();
            } else if (state === 'dim') {
                plain.setStyle({ weight: 2, opacity: 0.2 });
            } else {
                plain.setStyle({ weight: 3, opacity: 0.7 });
            }
        } catch (e) {
            console.debug('[bike] styleRide failed:', e);
        }
    }

    NS.syncRideHighlight = function (opts) {
        const st = NS.state;
        const scrollIntoView = opts && opts.scrollIntoView;
        const activeIdx = NS.activeRideIndex();

        st.rides.forEach(function (_, i) {
            if (st.hiddenRides.has(i)) return;
            if (activeIdx == null) styleRide(i, 'normal');
            else styleRide(i, i === activeIdx ? 'active' : 'dim');
        });

        st.arrowMarkers.forEach(function (m) {
            if (st.hiddenRides.has(m._bikeIndex)) return;
            const el = m.getElement();
            if (!el) return;
            if (activeIdx == null) el.style.opacity = '0.85';
            else el.style.opacity = m._bikeIndex === activeIdx ? '1' : '0.15';
        });

        NS.updateHalo();

        let activeItem = null;
        document.querySelectorAll('.bike-ride-item').forEach(function (item) {
            const isActive = parseInt(item.dataset.ride, 10) === activeIdx;
            item.classList.toggle('active', isActive);
            if (isActive) activeItem = item;
        });

        if (scrollIntoView && activeItem) {
            requestAnimationFrame(function () {
                activeItem.scrollIntoView({ block: 'nearest', inline: 'nearest', behavior: 'smooth' });
            });
        }
    };

    NS.applyMapMode = function () {
        const st = NS.state;
        if (!NS.mapReady()) return;

        if (st.speedMode) NS.buildSpeedLayer();

        st.rides.forEach(function (_, i) {
            const plain = st.plainLines[i];
            if (!plain) return;
            if (!st.hiddenRides.has(i)) NS.addLayerSafe(plain);
            else NS.removeLayerSafe(plain);
        });

        if (st.speedLayer) {
            if (st.speedMode) NS.addLayerSafe(st.speedLayer);
            else NS.removeLayerSafe(st.speedLayer);
        }
        if (st.noSpeedLayer) {
            if (st.speedMode) NS.addLayerSafe(st.noSpeedLayer);
            else NS.removeLayerSafe(st.noSpeedLayer);
        }

        if (st.speedMode && st.speedLayer) {
            try {
                st.speedLayer.eachLayer(function (l) {
                    if (l.bringToFront) l.bringToFront();
                    if (l.redraw) l.redraw();
                });
            } catch (e) {
                console.debug('[bike] speed layer refresh failed:', e);
            }
        }

        st.arrowMarkers.forEach(function (m) {
            const want = !st.hiddenRides.has(m._bikeIndex) && !st.speedMode;
            if (want) NS.addLayerSafe(m);
            else NS.removeLayerSafe(m);
        });

        const legend = document.getElementById('bike-speed-legend');
        if (legend) legend.hidden = !st.speedMode;
        if (st.speedBtn) st.speedBtn.setAttribute('aria-pressed', st.speedMode ? 'true' : 'false');
        if (st.section) st.section.classList.toggle('speed-on', st.speedMode);

        NS.syncRideHighlight();
    };

    NS.selectRide = function (idx, options) {
        const st = NS.state;
        const opts = options || {};
        const toggle = opts.toggle !== false;
        const fit = !!opts.fit;
        const openPopup = !!opts.openPopup;
        const scrollIntoView = opts.scrollIntoView !== false;
        const updateURL = opts.updateURL !== false;

        if (idx == null || idx < 0 || idx >= st.rides.length) return;
        if (st.hiddenRides.has(idx)) return;
        if (!st.profiles[idx]) return;

        if (toggle && st.selectedRideIndex === idx) {
            st.selectedRideIndex = null;
            if (st.map) {
                try { st.map.closePopup(); } catch (e) {}
            }
            if (updateURL) NS.updateRideURL();
            NS.hideProfile();
            NS.syncRideHighlight();
            return;
        }

        st.selectedRideIndex = idx;
        if (updateURL) NS.updateRideURL();

        if (fit) fitExtent(rideExtent(idx), 16, true);

        const line = st.plainLines[idx];
        if (openPopup && line && NS.mapReady() && !st.speedMode) {
            try {
                if (st.map.hasLayer(line)) line.openPopup();
            } catch (e) {}
        }

        NS.showProfile(idx);
        NS.syncRideHighlight({ scrollIntoView: scrollIntoView });
    };

    NS.toggleRideVisibility = function (idx) {
        const st = NS.state;
        const item = document.querySelector('.bike-ride-item[data-ride="' + idx + '"]');
        const btn = item && item.querySelector('.bike-toggle-btn');

        if (st.hiddenRides.has(idx)) {
            st.hiddenRides.delete(idx);
            if (item) item.classList.remove('ride-hidden');
            if (btn) btn.setAttribute('aria-pressed', 'false');
        } else {
            st.hiddenRides.add(idx);
            if (item) item.classList.add('ride-hidden');
            if (btn) btn.setAttribute('aria-pressed', 'true');

            if (st.selectedRideIndex === idx) {
                st.selectedRideIndex = null;
                NS.updateRideURL();
                NS.hideProfile();
            }
            if (st.hoveredRideIndex === idx) st.hoveredRideIndex = null;
        }

        st.speedLayerKey = null;
        NS.applyMapMode();
    };

    NS.highlightRide = function (idx) {
        NS.state.hoveredRideIndex = idx;
        NS.syncRideHighlight({ scrollIntoView: true });
    };

    NS.resetHighlight = function () {
        NS.state.hoveredRideIndex = null;
        NS.syncRideHighlight();
    };

    function bootExtent() {
        const idx = NS.rideIndexFromURL();
        if (idx != null && NS.state.profiles[idx]) {
            const ext = rideExtent(idx);
            if (ext) return { extent: ext, rideIndex: idx };
        }
        return { extent: ridesExtent(false), rideIndex: null };
    }

    function revealMap() {
        const st = NS.state;
        if (st.revealed) return;
        st.revealed = true;
        if (st.revealTimer) {
            clearTimeout(st.revealTimer);
            st.revealTimer = null;
        }
        NS.hideMapLoading();
        NS.notifyLayout();
    }

    function observeContainerSize(target) {
        if (typeof ResizeObserver === 'undefined') return;

        NS.state.sizeObserver = new ResizeObserver(function () {
            if (!NS.mapReady()) return;
            try {
                NS.state.map.invalidateSize({ pan: false, animate: false });
            } catch (e) {}
            if (NS.state.autoView && NS.state.selectedRideIndex == null && NS.state.fitRetries > 0) {
                NS.fitAllRides(false);
            }
        });
        NS.state.sizeObserver.observe(target);
    }

    function mountMap(container, fullscreenTarget, sized) {
        const st = NS.state;
        st.container = container;

        st.map = L.map(container, {
            zoomControl: true,
            attributionControl: true,
            preferCanvas: true,
            zoomSnap: 0,
            zoomDelta: 0.5,
            wheelPxPerZoomLevel: 90
        });

        // On mobile, one-finger drag should scroll the page, not pan the map
        // (pinch-zoom and the zoom buttons still work).
        if (L.Browser && L.Browser.mobile) {
            st.map.dragging.disable();
            if (st.map.tap) st.map.tap.disable();
            container.style.touchAction = 'pan-y';
        }

        const boot = bootExtent();
        const bounds = sized ? NS.extentToBounds(boot.extent) : null;
        let viewSet = false;

        if (bounds) {
            try {
                st.map.fitBounds(bounds, {
                    padding: fitPadding(),
                    maxZoom: 16,
                    animate: false
                });
                viewSet = true;
            } catch (e) {
                console.debug('[bike] initial fitBounds failed:', e);
            }
        }

        if (!viewSet) {
            const seed = NS.firstValidLatLng();
            st.map.setView(seed || NS.FALLBACK_CENTER, seed ? 10 : NS.FALLBACK_ZOOM);
        }

        st.map.on('dragstart', function () { NS.state.autoView = false; });
        st.map.on('zoomstart', function () {
            if (!NS.state.programmaticView) NS.state.autoView = false;
        });

        const isDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
        L.tileLayer(
            isDark
                ? 'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png'
                : 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',
            { maxZoom: 18 }
        ).addTo(st.map);

        let arrowSize = 14;
        try {
            arrowSize = getArrowSize(st.map.getZoom());
        } catch (e) {}

        st.rides.forEach(function (ride, i) {
            const prof = st.profiles[i];
            if (!prof) {
                st.hiddenRides.add(i);
                return;
            }

            if (prof.hasTime) st.speedRideCount++;
            else st.noSpeedRideCount++;

            const color = NS.COLORS[i % NS.COLORS.length];
            const latlngs = prof.points.map(p => [p.lat, p.lng]);

            const line = L.polyline(latlngs, {
                color: color,
                weight: 3,
                opacity: 0.7,
                smoothFactor: 1
            });
            line._bikeIndex = i;

            line.bindPopup(
                '<strong>' + (ride.name || 'Ride') + '</strong><br>' +
                (NS.num(ride.distance_km) ? ride.distance_km.toFixed(1) : '?') + ' km · ↑' +
                (NS.num(ride.elevation_gain_m) ? ride.elevation_gain_m.toFixed(0) : '?') + 'm' +
                (ride.date ? '<br><small>' + ride.date + '</small>' : ''),
                { maxWidth: 220 }
            );

            line.on('mouseover', function () { NS.highlightRide(i); });
            line.on('mouseout', function () { NS.resetHighlight(); });
            line.on('click', function () {
                NS.selectRide(i, {
                    toggle: true,
                    fit: true,
                    openPopup: true,
                    scrollIntoView: true,
                    updateURL: true
                });
                NS.invalidateMapSize();
            });

            NS.addLayerSafe(line);
            st.plainLines[i] = line;

            const arrows = placeArrows(latlngs, color, getArrowInterval(ride.distance_km || 0), arrowSize);
            arrows.forEach(function (m) {
                m._bikeIndex = i;
                NS.addLayerSafe(m);
            });
            st.arrowMarkers.push.apply(st.arrowMarkers, arrows);

            const colorEl = document.querySelector('.bike-ride-color[data-ride-color="' + i + '"]');
            if (colorEl) colorEl.style.background = color;
        });

        NS.buildLegend();
        NS.buildMapControls(fullscreenTarget);
        NS.decorateRideList();

        st.map.on('zoomend', updateArrowSizes);
        st.map.on('popupopen', NS.invalidateMapSize);
        st.map.on('popupclose', NS.invalidateMapSize);

        NS.setupRideListScrolling();

        if (st.mapResizeCleanup) {
            st.mapResizeCleanup();
            st.mapResizeCleanup = null;
        }
        st.mapResizeCleanup = NS.setupMapResizeHandling(fullscreenTarget);
        observeContainerSize(fullscreenTarget);

        st.revealTimer = setTimeout(revealMap, NS.REVEAL_SAFETY_MS);

        requestAnimationFrame(function () {
            if (!NS.state.map || NS.state.map !== st.map) return;

            try {
                st.map.invalidateSize({ pan: false, animate: false });
            } catch (e) {}

            if (boot.rideIndex != null) {
                fitExtent(boot.extent, 16, false);
                NS.selectRide(boot.rideIndex, {
                    toggle: false,
                    fit: false,
                    openPopup: true,
                    scrollIntoView: true,
                    updateURL: false
                });
            } else {
                NS.fitAllRides(false);
            }

            updateArrowSizes();
            NS.syncRideHighlight();

            requestAnimationFrame(revealMap);
        });
    }

    function createMap() {
        const st = NS.state;
        const container = document.getElementById('bike-map');
        if (!container) return;

        if (!NS.hasAnyProfile()) {
            NS.showMapMessage('No usable GPS tracks in these rides');
            return;
        }

        const fullscreenTarget = container.closest('.bike-map-container') || container;
        const token = st.token;

        st.sizeWaitCancel = NS.whenSized(container, function (sized) {
            const cur = NS.state;
            if (!cur || cur.token !== token) return;
            cur.sizeWaitCancel = null;
            mountMap(container, fullscreenTarget, sized);
        });
    }

    function init() {
        const dataEl = document.getElementById('bike-data');
        if (!dataEl) return;

        let rides;
        try {
            rides = JSON.parse(dataEl.textContent || '[]');
        } catch (e) {
            console.debug('[bike] failed to parse ride data:', e);
            return;
        }
        if (!Array.isArray(rides) || !rides.length) return;

        const st = NS.state;
        st.rides = rides;
        st.profiles = NS.buildProfiles(rides);
        st.plainLines = new Array(rides.length);

        NS.bindProfileEvents();

        NS.loadLeaflet().then(createMap).catch(function (err) {
            console.debug('[bike] leaflet failed to load:', err);
            NS.showMapMessage('Map failed to load');
        });
    }

    NS.init = init;

    window.reinitBikeMap = function () {
        const st = NS.state;

        if (st.sizeWaitCancel) st.sizeWaitCancel();
        if (st.mapResizeCleanup) st.mapResizeCleanup();
        if (st.chartRO) st.chartRO.disconnect();
        if (st.sizeObserver) st.sizeObserver.disconnect();
        if (st.fitRetryTimer) clearTimeout(st.fitRetryTimer);
        if (st.revealTimer) clearTimeout(st.revealTimer);
        if (st.map) {
            try { st.map.remove(); } catch (e) {}
        }

        document.querySelectorAll('.bike-ride-item').forEach(function (item) {
            delete item.dataset.bikeDecorated;
        });

        NS.resetState();
        init();
    };

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        setTimeout(init, 50);
    }
})();