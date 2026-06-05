(function () {
    'use strict';

    let map = null;
    let heatLayer = null;
    let markersLayer = null;
    let markers = [];
    let places = [];
    let config = {};
    let isHeatmapMode = true;
    let mapResizeCleanup = null;

    let interactionTimeout = null;
    let initialFitDone = false;

    function clamp(v, min, max) {
        return Math.max(min, Math.min(max, v));
    }

    function initPlacesMap() {
        const container = document.getElementById('places-map');
        const dataEl = document.getElementById('places-data');
        const configEl = document.getElementById('places-config');

        if (!container || !dataEl) return;

        // Force container dimensions for potato mode
        if (document.body.classList.contains('potato-mode')) {
            container.style.height = '280px';
            container.style.minHeight = '280px';
            container.style.display = 'block';
        }


        try {
            places = JSON.parse(dataEl.textContent || '[]');
            config = JSON.parse(configEl?.textContent || '{}');
        } catch (e) {
            console.error('Failed to parse places data:', e);
            return;
        }

        if (!Array.isArray(places) || places.length === 0) {
            const loadingEl = document.getElementById('map-loading');
            if (loadingEl) {
                loadingEl.innerHTML = '<span style="color:var(--muted)">No places to display</span>';
            }
            return;
        }

        loadLeaflet()
            .then(() => {
                createMap(container);
            })
            .catch((err) => {
                console.error('Failed to load Leaflet:', err);
            });
    }

    function loadLeaflet() {
        return new Promise((resolve, reject) => {
            if (window.L) {
                loadHeatPlugin().then(resolve).catch(reject);
                return;
            }

            const cssLink = document.createElement('link');
            cssLink.rel = 'stylesheet';
            cssLink.href = '/static/libs/leaflet/leaflet.css';
            cssLink.onerror = () => {
                cssLink.href = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.css';
            };
            document.head.appendChild(cssLink);

            const script = document.createElement('script');
            script.src = '/static/libs/leaflet/leaflet.js';
            script.onload = () => {
                loadHeatPlugin().then(resolve).catch(reject);
            };
            script.onerror = () => {
                script.src = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.js';
                script.onload = () => {
                    loadHeatPlugin().then(resolve).catch(reject);
                };
            };
            document.head.appendChild(script);
        });
    }

    function loadHeatPlugin() {
        return new Promise((resolve, reject) => {
            if (window.L && window.L.heatLayer) {
                resolve();
                return;
            }

            const heatScript = document.createElement('script');
            heatScript.src = '/static/libs/leaflet/leaflet-heat.js';
            heatScript.onload = resolve;
            heatScript.onerror = () => {
                heatScript.src = 'https://unpkg.com/leaflet.heat@0.2.0/dist/leaflet-heat.js';
                heatScript.onload = resolve;
                heatScript.onerror = reject;
            };
            document.head.appendChild(heatScript);
        });
    }

    function createMap(container) {
        const defaultLat = config.defaultLat ?? 25;
        const defaultLng = config.defaultLng ?? 0;
        const defaultZoom = config.defaultZoom ?? 3;

        map = L.map(container, {
            center: [defaultLat, defaultLng],
            zoom: defaultZoom,
            zoomControl: true,
            attributionControl: true,
            minZoom: 2,
            maxZoom: 18,
            preferCanvas: true // OPTIMIZATION: Use Canvas instead of SVGs for vector layers
        });

        const isDark = window.matchMedia('(prefers-color-scheme: dark)').matches;

        let tileUrl;
        let tileAttribution;

        if (isDark) {
            tileUrl = 'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png';
            tileAttribution =
                '&copy; <a href="https://www.openstreetmap.org/copyright">OSM</a> &copy; <a href="https://carto.com/attributions">CARTO</a>';
        } else {
            tileUrl = 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png';
            tileAttribution =
                '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>';
        }

        L.tileLayer(tileUrl, {
            attribution: tileAttribution,
            maxZoom: 18
        }).addTo(map);

        createHeatLayer();
        createMarkersLayer();

        // Heatmap is default
        isHeatmapMode = true;
        if (heatLayer) {
            heatLayer.addTo(map);
            ensureHeatCanvasConfig();
        }

        fitAllPlaces(true);

        const loadingEl = document.getElementById('map-loading');
        if (loadingEl) {
            loadingEl.classList.add('hidden');
        }

        const fullscreenTarget = container.closest('.places-map-container') || container;

        setupControls();
        setupInteractionHandling();
        addPlacesFullscreenControl(fullscreenTarget);

        if (mapResizeCleanup) {
            mapResizeCleanup();
            mapResizeCleanup = null;
        }
        mapResizeCleanup = setupMapResizeHandling(fullscreenTarget);

        window
            .matchMedia('(prefers-color-scheme: dark)')
            .addEventListener('change', () => {
                location.reload();
            });
    }

    function computeHeatData() {
        const cellCounts = new Map();

        places.forEach((p) => {
            const lat = Number(p.lat ?? p.Lat);
            const lng = Number(p.lng ?? p.Lng);
            if (!isFinite(lat) || !isFinite(lng)) return;

            const key = `${lat.toFixed(3)},${lng.toFixed(3)}`;
            cellCounts.set(key, (cellCounts.get(key) || 0) + 1);
        });

        let maxCount = 0;
        cellCounts.forEach((v) => {
            if (v > maxCount) maxCount = v;
        });

        const minAlpha = 0.55;
        const maxAlpha = 1.0;
        const gamma = 0.65;

        const heatData = [];
        cellCounts.forEach((count, key) => {
            const [latStr, lngStr] = key.split(',');
            const lat = parseFloat(latStr);
            const lng = parseFloat(lngStr);
            if (!isFinite(lat) || !isFinite(lng)) return;

            const norm = maxCount ? count / maxCount : 1;
            const weight = minAlpha + Math.pow(norm, gamma) * (maxAlpha - minAlpha);

            heatData.push([lat, lng, weight]);
        });

        return heatData;
    }

    function createHeatLayer() {
        const heatData = computeHeatData();
        if (!heatData.length) return;

        const baseRadius = config.heatmapRadius || 35;

        heatLayer = L.heatLayer(heatData, {
            radius: baseRadius,
            blur: Math.round(baseRadius * 0.9),
            maxZoom: 10,
            max: 1.0,
            minOpacity: 0.35,
            gradient: {
                0.0: '#0b1d33',
                0.25: '#194a8a',
                0.5: '#4c87ff',
                0.75: '#a7c7ff',
                1.0: '#ffffff'
            }
        });
    }

    function markerStyle(radius) {
        return {
            radius,
            fillColor: '#7aa2ff',
            color: '#ffffff',
            weight: 2,
            opacity: 1,
            fillOpacity: 0.9
        };
    }

    function createMarkersLayer() {
        markersLayer = L.layerGroup();
        markers = [];

        const baseRadius = config.markerRadius || 8;

        places.forEach((place) => {
            const lat = Number(place.lat ?? place.Lat);
            const lng = Number(place.lng ?? place.Lng);
            if (!isFinite(lat) || !isFinite(lng)) return;

            const marker = L.circleMarker([lat, lng], markerStyle(baseRadius));
            marker._baseRadius = baseRadius;
            markers.push(marker);

            let popupContent =
                '<div class="place-popup"><div class="place-popup-content">';
            popupContent += `<div class="place-popup-name">${escapeHtml(
                place.name || place.Name || ''
            )}</div>`;

            const city = place.city || place.City || '';
            const country = place.country || place.Country || '';

            if (city || country) {
                popupContent += `<div class="place-popup-location">📍 ${escapeHtml(
                    [city, country].filter(Boolean).join(', ')
                )}</div>`;
            }

            if (place.description || place.Description) {
                popupContent += `<div class="place-popup-description">${escapeHtml(
                    place.description || place.Description
                )}</div>`;
            }

            const visited = place.visited_date || place.VisitedDate || '';
            if (visited) {
                const dateRu = formatDateRu(String(visited));
                popupContent += `<span class="place-popup-date">${escapeHtml(
                    dateRu
                )}</span>`;
            }

            if (place.category || place.Category) {
                popupContent += `<span class="place-popup-category">${escapeHtml(
                    place.category || place.Category
                )}</span>`;
            }

            popupContent += '</div></div>';

            marker.bindPopup(popupContent, {
                maxWidth: 280,
                minWidth: 200
            });

            markersLayer.addLayer(marker);
        });

        updateMarkerSizes();
    }

    function invalidatePlacesMapSize() {
        if (!map) return;
        [0, 50, 150, 300].forEach(delay => {
            setTimeout(() => {
                if (map) map.invalidateSize({ pan: false, animate: false });
            }, delay);
        });
    }

    function setupMapResizeHandling(targetEl) {
        if (!targetEl) return () => {};

        const refresh = () => invalidatePlacesMapSize();
        const onVisibility = () => { if (!document.hidden) refresh(); };

        window.addEventListener('resize', refresh);
        window.addEventListener('orientationchange', refresh);
        document.addEventListener('visibilitychange', onVisibility);
        document.addEventListener('fullscreenchange', refresh);

        let resizeObserver = null;
        if (typeof ResizeObserver !== 'undefined') {
            resizeObserver = new ResizeObserver(() => refresh());
            resizeObserver.observe(targetEl);
        }

        refresh();

        return () => {
            window.removeEventListener('resize', refresh);
            window.removeEventListener('orientationchange', refresh);
            document.removeEventListener('visibilitychange', onVisibility);
            document.removeEventListener('fullscreenchange', refresh);
            if (resizeObserver) resizeObserver.disconnect();
        };
    }

    function addPlacesFullscreenControl(fullscreenTarget) {
        if (!map || !window.L || !fullscreenTarget) return;

        const PlacesFullscreenControl = L.Control.extend({
            options: { position: 'topright' },

            onAdd() {
                const wrapper = L.DomUtil.create('div', 'leaflet-bar places-fullscreen-control');
                const btn = L.DomUtil.create('button', 'map-control-btn places-fullscreen-btn', wrapper);

                btn.type = 'button';
                btn.title = 'Toggle fullscreen';
                btn.setAttribute('aria-label', 'Toggle fullscreen');
                btn.innerHTML = `<svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor" aria-hidden="true">
                <path d="M7 14H5v5h5v-2H7v-3zm0-4h2V7h3V5H5v5zm10 7h-3v2h5v-5h-2v3zm0-12V5h-5v2h3v3h2V5z"/>
            </svg>`;

                const syncState = () => {
                    btn.classList.toggle('active', document.fullscreenElement === fullscreenTarget);
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
                        invalidatePlacesMapSize();
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

        map.addControl(new PlacesFullscreenControl());
    }

    function updateMarkerSizes() {
        if (!map || !markers.length) return;
        const z = map.getZoom();

        const minR = 4;
        const maxR = 13;
        const zClamped = clamp(z, 2, 10);
        const t = (zClamped - 2) / 8;
        const radius = maxR - t * (maxR - minR);

        markers.forEach((m) => {
            m.setStyle({radius});
        });
    }

    function fitAllPlaces(isInitial = false) {
        if (!map || !places.length) return;
        if (isInitial && initialFitDone) return;

        if (places.length === 1) {
            const p = places[0];
            const lat = Number(p.lat ?? p.Lat);
            const lng = Number(p.lng ?? p.Lng);
            if (!isFinite(lat) || !isFinite(lng)) return;
            map.setView([lat, lng], 8);
        } else {
            const bounds = L.latLngBounds(
                places
                    .map((p) => [Number(p.lat ?? p.Lat), Number(p.lng ?? p.Lng)])
                    .filter(([lat, lng]) => isFinite(lat) && isFinite(lng))
            );
            if (bounds.isValid()) {
                map.fitBounds(bounds, {
                    padding: [40, 40],
                    maxZoom: 6
                });
            }
        }

        if (isInitial) initialFitDone = true;
    }

    function setupControls() {
        const toggleBtn = document.getElementById('toggle-heatmap');
        const fitBtn = document.getElementById('fit-bounds');

        if (toggleBtn) {
            toggleBtn.classList.add('active');

            toggleBtn.addEventListener('click', () => {
                setMode(!isHeatmapMode, toggleBtn);
            });
        }

        if (fitBtn) {
            fitBtn.addEventListener('click', () => fitAllPlaces(false));
        }

        const observer = new MutationObserver(() => {
            setTimeout(() => {
                if (map) map.invalidateSize();
            }, 300);
        });

        const pluginEl = document.getElementById('places-plugin');
        if (pluginEl) {
            observer.observe(pluginEl, {
                attributes: true,
                attributeFilter: ['class']
            });
        }

        const overlay = document.querySelector('.plugin-overlay');
        if (overlay) {
            observer.observe(overlay, {
                attributes: true,
                attributeFilter: ['class']
            });
        }
    }

    function setMode(heatMode, toggleBtn) {
        isHeatmapMode = heatMode;
        if (!map) return;

        if (heatMode) {
            if (markersLayer && map.hasLayer(markersLayer)) {
                map.removeLayer(markersLayer);
            }
            if (heatLayer && !map.hasLayer(heatLayer)) {
                heatLayer.addTo(map);
                ensureHeatCanvasConfig();
            }
            toggleBtn && toggleBtn.classList.add('active');
        } else {
            if (heatLayer && map.hasLayer(heatLayer)) {
                map.removeLayer(heatLayer);
            }
            if (markersLayer && !map.hasLayer(markersLayer)) {
                markersLayer.addTo(map);
            }
            updateMarkerSizes();
            toggleBtn && toggleBtn.classList.remove('active');
        }
    }

    function getHeatCanvas() {
        if (!heatLayer || !heatLayer._heat) return null;
        return heatLayer._heat._canvas || null;
    }

    function ensureHeatCanvasConfig() {
        const canvas = getHeatCanvas();
        if (!canvas) return;
        canvas.style.pointerEvents = 'none';
        canvas.style.transition = canvas.style.transition || 'opacity 80ms ease-out';
        if (!canvas.style.opacity) canvas.style.opacity = '1';
    }

    function setHeatVisible(visible) {
        const canvas = getHeatCanvas();
        if (!canvas) return;
        canvas.style.opacity = visible ? '1' : '0';
    }

    function setupInteractionHandling() {
        if (!map) return;

        function interactionStart() {
            if (!isHeatmapMode || !heatLayer) return;
            ensureHeatCanvasConfig();
            setHeatVisible(false);
        }

        function interactionEnd() {
            if (!isHeatmapMode || !heatLayer) return;
            clearTimeout(interactionTimeout);
            interactionTimeout = setTimeout(() => {
                ensureHeatCanvasConfig();
                setHeatVisible(true);
            }, 60);
        }

        map.on('movestart', interactionStart);
        map.on('moveend', interactionEnd);
        map.on('zoomstart', interactionStart);
        map.on('zoomend', () => {
            interactionEnd();
            updateMarkerSizes();
        });
    }

    function formatDateRu(dateStr) {
        if (!dateStr) return '';
        const [y, m, d] = dateStr.split('-').map(Number);
        if (!y || !m || !d) return dateStr;
        const dd = String(d).padStart(2, '0');
        const mm = String(m).padStart(2, '0');
        return `${dd}.${mm}.${y}`;
    }

    function escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initPlacesMap);
    } else {
        setTimeout(initPlacesMap, 50);
    }

    window.reinitPlacesMap = function () {
        if (mapResizeCleanup) {
            mapResizeCleanup();
            mapResizeCleanup = null;
        }
        if (map) {
            map.remove();
            map = null;
        }
        heatLayer = null;
        markersLayer = null;
        markers = [];
        isHeatmapMode = true;
        initialFitDone = false;
        initPlacesMap();
    };

    setTimeout(() => {
        if (map) {
            map.invalidateSize();
        }
    }, 100);
})();