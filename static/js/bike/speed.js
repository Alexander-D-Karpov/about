(function () {
    'use strict';

    const NS = (window.BikeMap = window.BikeMap || {});

    NS.speedWeight = function () {
        const w = NS.containerSize().w;
        if (!w) return 3.5;
        if (w < 480) return 5;
        if (w < 780) return 4.2;
        return 3.5;
    };

    function stampCells(cells, a, b, speed) {
        if (!NS.num(speed)) return;

        const dLat = b.lat - a.lat;
        const dLng = b.lng - a.lng;
        const span = Math.max(Math.abs(dLat), Math.abs(dLng) * 0.5);
        const steps = NS.clamp(Math.ceil(span / NS.CELL_DEG), 1, 64);

        for (let s = 0; s <= steps; s++) {
            const t = s / steps;
            const key = NS.cellKey(a.lat + dLat * t, a.lng + dLng * t);
            let e = cells.get(key);
            if (!e) {
                e = { sum: 0, n: 0 };
                cells.set(key, e);
            }
            e.sum += speed;
            e.n++;
        }
    }

    function sampleCells(cells, a, b) {
        const dLat = b.lat - a.lat;
        const dLng = b.lng - a.lng;
        const span = Math.max(Math.abs(dLat), Math.abs(dLng) * 0.5);
        const steps = NS.clamp(Math.ceil(span / NS.CELL_DEG), 1, 64);

        let sum = 0;
        let n = 0;
        for (let s = 0; s <= steps; s++) {
            const t = s / steps;
            const e = cells.get(NS.cellKey(a.lat + dLat * t, a.lng + dLng * t));
            if (e && e.n) {
                sum += e.sum / e.n;
                n++;
            }
        }
        return n ? sum / n : null;
    }

    NS.buildSpeedLayer = function () {
        const st = NS.state;
        if (!window.L) return;

        const weight = NS.speedWeight();
        const key = Array.from(st.hiddenRides).sort((a, b) => a - b).join(',');
        if (st.speedLayer && st.speedLayerKey === key && st.speedLayerWeight === weight) return;

        NS.removeLayerSafe(st.speedLayer);
        NS.removeLayerSafe(st.noSpeedLayer);

        st.speedLayer = L.layerGroup();
        st.noSpeedLayer = L.layerGroup();
        st.speedLayerKey = key;
        st.speedLayerWeight = weight;

        const cells = new Map();
        const timed = [];
        const untimed = [];

        st.rides.forEach(function (_, i) {
            if (st.hiddenRides.has(i)) return;
            const prof = st.profiles[i];
            if (!prof) return;

            if (!prof.hasTime) {
                untimed.push(i);
                return;
            }

            timed.push(i);
            const pts = prof.points;
            for (let k = 1; k < pts.length; k++) {
                stampCells(cells, pts[k - 1], pts[k], prof.speedSeries[k]);
            }
        });

        const runsByBucket = new Map();
        const drawn = new Set();

        timed.forEach(function (i) {
            const pts = st.profiles[i].points;
            let run = null;
            let runBucket = null;

            const closeRun = function () {
                if (run && run.length > 1) {
                    let arr = runsByBucket.get(runBucket);
                    if (!arr) {
                        arr = [];
                        runsByBucket.set(runBucket, arr);
                    }
                    arr.push(run);
                }
                run = null;
                runBucket = null;
            };

            for (let k = 1; k < pts.length; k++) {
                const a = pts[k - 1];
                const b = pts[k];
                const ka = NS.cellKey(a.lat, a.lng);
                const kb = NS.cellKey(b.lat, b.lng);
                const id = ka < kb ? ka + '>' + kb : kb + '>' + ka;

                if (drawn.has(id)) {
                    closeRun();
                    continue;
                }
                drawn.add(id);

                const avg = sampleCells(cells, a, b);
                if (avg == null) {
                    closeRun();
                    continue;
                }

                const bucket = NS.speedBucket(avg);
                if (run === null || bucket !== runBucket) {
                    closeRun();
                    run = [[a.lat, a.lng]];
                    runBucket = bucket;
                }
                run.push([b.lat, b.lng]);
            }

            closeRun();
        });

        runsByBucket.forEach(function (runs, bucket) {
            st.speedLayer.addLayer(L.polyline(runs, {
                color: NS.speedColor(bucket < 0 ? null : bucket * NS.SPEED_BUCKET),
                weight: weight,
                opacity: 0.95,
                lineCap: 'round',
                lineJoin: 'round',
                smoothFactor: 0,
                interactive: false
            }));
        });

        untimed.forEach(function (i) {
            st.noSpeedLayer.addLayer(L.polyline(st.profiles[i].points.map(p => [p.lat, p.lng]), {
                color: '#5b6472',
                weight: Math.max(2, weight - 2),
                opacity: 0.45,
                dashArray: '4 6',
                smoothFactor: 0,
                interactive: false
            }));
        });
    };

    NS.updateHalo = function () {
        const st = NS.state;

        NS.removeLayerSafe(st.halo);
        st.halo = null;

        if (!st.speedMode || !NS.mapReady() || !window.L) return;

        const idx = NS.activeRideIndex();
        if (idx == null || st.hiddenRides.has(idx)) return;

        const prof = st.profiles[idx];
        if (!prof) return;

        st.halo = L.polyline(prof.points.map(p => [p.lat, p.lng]), {
            color: '#ffffff',
            weight: NS.speedWeight() + 5,
            opacity: 0.2,
            lineCap: 'round',
            lineJoin: 'round',
            smoothFactor: 0,
            interactive: false
        });

        if (NS.addLayerSafe(st.halo) && st.halo.bringToBack) st.halo.bringToBack();
    };

    NS.buildLegend = function () {
        const bar = document.getElementById('bike-speed-legend-bar');
        if (!bar) return;
        const stops = NS.SPEED_STOPS.map(function (s) {
            return NS.rgbStr(s.c) + ' ' + ((s.v / NS.SPEED_MAX) * 100).toFixed(1) + '%';
        });
        bar.style.background = 'linear-gradient(90deg,' + stops.join(',') + ')';
    };
})();