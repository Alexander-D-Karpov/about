(function () {
    'use strict';

    const ISO3 = {
        AF: 'AFG', AL: 'ALB', DZ: 'DZA', AO: 'AGO', AR: 'ARG', AM: 'ARM', AU: 'AUS', AT: 'AUT',
        AZ: 'AZE', BS: 'BHS', BD: 'BGD', BY: 'BLR', BE: 'BEL', BZ: 'BLZ', BJ: 'BEN', BT: 'BTN',
        BO: 'BOL', BA: 'BIH', BW: 'BWA', BR: 'BRA', BN: 'BRN', BG: 'BGR', BF: 'BFA', BI: 'BDI',
        KH: 'KHM', CM: 'CMR', CA: 'CAN', CF: 'CAF', TD: 'TCD', CL: 'CHL', CN: 'CHN', CO: 'COL',
        CG: 'COG', CD: 'COD', CR: 'CRI', CI: 'CIV', HR: 'HRV', CU: 'CUB', CY: 'CYP', CZ: 'CZE',
        DK: 'DNK', DJ: 'DJI', DO: 'DOM', EC: 'ECU', EG: 'EGY', SV: 'SLV', GQ: 'GNQ', ER: 'ERI',
        EE: 'EST', ET: 'ETH', FJ: 'FJI', FI: 'FIN', FR: 'FRA', GF: 'GUF', GA: 'GAB', GM: 'GMB',
        GE: 'GEO', DE: 'DEU', GH: 'GHA', GR: 'GRC', GL: 'GRL', GT: 'GTM', GN: 'GIN', GW: 'GNB',
        GY: 'GUY', HT: 'HTI', HN: 'HND', HU: 'HUN', IS: 'ISL', IN: 'IND', ID: 'IDN', IR: 'IRN',
        IQ: 'IRQ', IE: 'IRL', IL: 'ISR', IT: 'ITA', JM: 'JAM', JP: 'JPN', JO: 'JOR', KZ: 'KAZ',
        KE: 'KEN', KP: 'PRK', KR: 'KOR', KW: 'KWT', KG: 'KGZ', LA: 'LAO', LV: 'LVA', LB: 'LBN',
        LS: 'LSO', LR: 'LBR', LY: 'LBY', LT: 'LTU', LU: 'LUX', MK: 'MKD', MG: 'MDG', MW: 'MWI',
        MY: 'MYS', ML: 'MLI', MT: 'MLT', MR: 'MRT', MX: 'MEX', MD: 'MDA', MN: 'MNG', ME: 'MNE',
        MA: 'MAR', MZ: 'MOZ', MM: 'MMR', NA: 'NAM', NP: 'NPL', NL: 'NLD', NC: 'NCL', NZ: 'NZL',
        NI: 'NIC', NE: 'NER', NG: 'NGA', NO: 'NOR', OM: 'OMN', PK: 'PAK', PS: 'PSE', PA: 'PAN',
        PG: 'PNG', PY: 'PRY', PE: 'PER', PH: 'PHL', PL: 'POL', PT: 'PRT', PR: 'PRI', QA: 'QAT',
        RO: 'ROU', RU: 'RUS', RW: 'RWA', SA: 'SAU', SN: 'SEN', RS: 'SRB', SL: 'SLE', SG: 'SGP',
        SK: 'SVK', SI: 'SVN', SB: 'SLB', SO: 'SOM', ZA: 'ZAF', SS: 'SSD', ES: 'ESP', LK: 'LKA',
        SD: 'SDN', SR: 'SUR', SZ: 'SWZ', SE: 'SWE', CH: 'CHE', SY: 'SYR', TW: 'TWN', TJ: 'TJK',
        TZ: 'TZA', TH: 'THA', TL: 'TLS', TG: 'TGO', TT: 'TTO', TN: 'TUN', TR: 'TUR', TM: 'TKM',
        UG: 'UGA', UA: 'UKR', AE: 'ARE', GB: 'GBR', US: 'USA', UY: 'URY', UZ: 'UZB', VU: 'VUT',
        VE: 'VEN', VN: 'VNM', YE: 'YEM', ZM: 'ZMB', ZW: 'ZWE', XK: 'KOS', HK: 'CHN', MO: 'CHN'
    };

    const FILL_OPACITY = [0, 0.18, 0.4, 0.65, 0.9];
    const EMPTY_RETRY_DELAY = 60000;
    const MAX_EMPTY_RETRIES = 10;
    const REFRESH_INTERVAL = 10 * 60 * 1000;

    let geoPromise = null;
    let geoLayer = null;
    let mapEl = null;
    let countsByISO3 = {};
    let maxCount = 0;
    let emptyRetries = 0;
    let retryTimer = null;
    let refreshTimer = null;
    let fetchInFlight = false;

    function cssVar(name, fallback) {
        const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
        return v || fallback;
    }

    function levelFor(count, max) {
        if (count <= 0 || max <= 0) return 0;
        const t = Math.log(count + 1) / Math.log(max + 1);
        return Math.max(1, Math.min(4, Math.ceil(t * 4)));
    }

    function polygonCenter(poly) {
        const ring = poly[0];
        let x = 0, y = 0;
        ring.forEach(p => {
            x += p[0];
            y += p[1];
        });
        return [x / ring.length, y / ring.length];
    }

    function fixCrimea(geo) {
        const rus = geo.features.find(f => f.id === 'RUS');
        const ukr = geo.features.find(f => f.id === 'UKR');
        if (!rus || !ukr || rus.geometry.type !== 'MultiPolygon') return;

        const isCrimea = poly => {
            const [lng, lat] = polygonCenter(poly);
            return lng > 31.5 && lng < 37 && lat > 44 && lat < 46.5;
        };

        const crimea = rus.geometry.coordinates.filter(isCrimea);
        if (!crimea.length) return;

        rus.geometry.coordinates = rus.geometry.coordinates.filter(p => !isCrimea(p));

        if (ukr.geometry.type === 'Polygon') {
            ukr.geometry = {
                type: 'MultiPolygon',
                coordinates: [ukr.geometry.coordinates, ...crimea]
            };
        } else {
            ukr.geometry.coordinates.push(...crimea);
        }
    }

    function loadLeaflet(cb) {
        if (window.L) {
            cb();
            return;
        }
        if (document.querySelector('script[data-leaflet-loading]')) {
            const wait = setInterval(() => {
                if (window.L) {
                    clearInterval(wait);
                    cb();
                }
            }, 100);
            setTimeout(() => clearInterval(wait), 15000);
            return;
        }
        const css = document.createElement('link');
        css.rel = 'stylesheet';
        css.href = '/static/libs/leaflet/leaflet.css';
        document.head.appendChild(css);

        const script = document.createElement('script');
        script.src = '/static/libs/leaflet/leaflet.js';
        script.setAttribute('data-leaflet-loading', '1');
        script.onload = cb;
        script.onerror = () => {
            css.href = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.css';
            const fallback = document.createElement('script');
            fallback.src = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.js';
            fallback.onload = cb;
            document.head.appendChild(fallback);
        };
        document.head.appendChild(script);
    }

    function loadGeo() {
        if (geoPromise) return geoPromise;
        geoPromise = fetch('/static/libs/geo/countries.geo.json')
            .then(r => {
                if (!r.ok) throw new Error('local geojson missing');
                return r.json();
            })
            .catch(() =>
                fetch('https://raw.githubusercontent.com/johan/world.geo.json/master/countries.geo.json')
                    .then(r => r.json())
            )
            .then(geo => {
                fixCrimea(geo);
                return geo;
            });
        return geoPromise;
    }

    function notifyResize(el) {
        const plugin = el.closest('.plugin');
        if (window.mosaicUtils && window.mosaicUtils.notifyContentChanged) {
            window.mosaicUtils.notifyContentChanged(plugin);
        } else if (window.mosaicUtils) {
            window.mosaicUtils.resizeAll();
        }
    }

    function setCounts(raw) {
        const out = {};
        let max = 0;
        Object.entries(raw || {}).forEach(([cc, n]) => {
            const key = ISO3[String(cc).toUpperCase()];
            if (!key || !(n > 0)) return;
            out[key] = (out[key] || 0) + n;
            if (out[key] > max) max = out[key];
        });
        countsByISO3 = out;
        maxCount = max;
    }

    function restyleGeoLayer() {
        if (!geoLayer) return;
        geoLayer.eachLayer(layer => {
            const f = layer.feature;
            if (!f) return;
            const n = countsByISO3[f.id] || 0;
            layer.setStyle({ fillOpacity: FILL_OPACITY[levelFor(n, maxCount)] });
            const name = (f.properties && f.properties.name) || f.id;
            const text = n > 0 ? `${name} · ${n} visitor${n === 1 ? '' : 's'}` : name;
            if (layer.getTooltip()) {
                layer.setTooltipContent(text);
            } else {
                layer.bindTooltip(text, { sticky: true, direction: 'top', className: 'visitors-map-tooltip' });
            }
        });
    }

    function scheduleEmptyRetry() {
        if (retryTimer || emptyRetries >= MAX_EMPTY_RETRIES) return;
        emptyRetries++;
        retryTimer = setTimeout(() => {
            retryTimer = null;
            refreshCounts();
        }, EMPTY_RETRY_DELAY);
    }

    function refreshCounts() {
        if (fetchInFlight || !mapEl || !document.body.contains(mapEl)) return;
        fetchInFlight = true;

        fetch('/api/visitors/regions')
            .then(r => {
                if (!r.ok) throw new Error('regions fetch failed: ' + r.status);
                return r.json();
            })
            .then(raw => {
                setCounts(raw);
                restyleGeoLayer();
                if (!Object.keys(countsByISO3).length) {
                    scheduleEmptyRetry();
                } else {
                    emptyRetries = 0;
                }
            })
            .catch(err => {
                console.debug('[Visitors] regions refresh failed:', err);
                scheduleEmptyRetry();
            })
            .finally(() => {
                fetchInFlight = false;
            });
    }

    function applyRegionsUpdate(countries) {
        if (!countries || !geoLayer) return;
        setCounts(countries);
        restyleGeoLayer();
    }

    function buildMap(el, initialCounts) {
        mapEl = el;
        geoLayer = null;
        setCounts(initialCounts);

        const accent = cssVar('--accent', '#4d9fff');
        const edge = 'rgba(255,255,255,0.07)';

        const map = L.map(el, {
            center: [20, 0],
            zoom: 1,
            minZoom: 1,
            maxZoom: 5,
            zoomControl: false,
            attributionControl: false,
            scrollWheelZoom: false,
            worldCopyJump: true,
            maxBounds: [[-56, -180], [78, 180]],
            maxBoundsViscosity: 1.0,
        });

        L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', {
            subdomains: 'abcd',
            maxZoom: 19
        }).addTo(map);

        loadGeo()
            .then(geo => {
                geoLayer = L.geoJSON(geo, {
                    style: f => {
                        const n = countsByISO3[f.id] || 0;
                        return {
                            color: edge,
                            weight: 0.5,
                            fillColor: accent,
                            fillOpacity: FILL_OPACITY[levelFor(n, maxCount)]
                        };
                    },
                    onEachFeature: (f, layer) => {
                        layer.on('mouseover', () => layer.setStyle({ weight: 1.2, color: accent }));
                        layer.on('mouseout', () => layer.setStyle({ weight: 0.5, color: edge }));
                    }
                }).addTo(map);

                restyleGeoLayer();

                setTimeout(() => {
                    map.invalidateSize();
                    try {
                        map.fitBounds([[-56, -168], [72, 190]], { padding: [0, 0] });
                    } catch (e) {}
                    notifyResize(el);
                }, 100);
            })
            .catch(err => console.warn('[Visitors] countries geojson failed:', err));

        refreshCounts();

        if (refreshTimer) clearInterval(refreshTimer);
        refreshTimer = setInterval(refreshCounts, REFRESH_INTERVAL);

        if (window.ResizeObserver) {
            new ResizeObserver(() => map.invalidateSize()).observe(el);
        }
    }

    function initVisitorsMap() {
        const el = document.querySelector('.visitors-map');
        if (!el || el.dataset.mapInit === '1') return;

        let counts;
        try {
            counts = JSON.parse(el.dataset.countries || '{}');
        } catch {
            counts = {};
        }

        el.dataset.mapInit = '1';
        emptyRetries = 0;
        if (retryTimer) {
            clearTimeout(retryTimer);
            retryTimer = null;
        }

        loadLeaflet(() => buildMap(el, counts));
    }

    function init() {
        initVisitorsMap();

        const mo = new MutationObserver(muts => {
            for (const m of muts) {
                for (const n of m.addedNodes) {
                    if (n.nodeType === 1 && (n.matches?.('.visitors-map, .visitors-section') || n.querySelector?.('.visitors-map'))) {
                        initVisitorsMap();
                        return;
                    }
                }
            }
        });
        mo.observe(document.body, { childList: true, subtree: true });

        document.addEventListener('visibilitychange', () => {
            if (!document.hidden) refreshCounts();
        });
    }

    window.applyVisitorsRegionsUpdate = applyRegionsUpdate;

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();