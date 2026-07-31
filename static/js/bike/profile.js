(function () {
    'use strict';

    const NS = (window.BikeMap = window.BikeMap || {});

    function parseTimestamp(c) {
        return (c.length > 3 && NS.num(c[3])) ? c[3] : null;
    }

    function parseElevation(c) {
        return (c.length > 2 && NS.num(c[2]) && c[2] !== 0) ? c[2] : null;
    }

    NS.buildProfile = function (ride) {
        const coords = (ride && ride.coordinates) || [];
        if (coords.length < 2) return null;

        const points = [];
        let dist = 0;
        let hasEle = false;
        let timeCount = 0;
        let minLat = Infinity;
        let maxLat = -Infinity;
        let minLng = Infinity;
        let maxLng = -Infinity;

        for (let i = 0; i < coords.length; i++) {
            const c = coords[i];
            if (!NS.validCoord(c)) continue;

            if (points.length > 0) {
                const prev = points[points.length - 1];
                dist += NS.getDistanceKm([prev.lat, prev.lng], [c[0], c[1]]);
            }

            const ele = parseElevation(c);
            const t = parseTimestamp(c);
            if (ele != null) hasEle = true;
            if (t != null) timeCount++;

            if (c[0] < minLat) minLat = c[0];
            if (c[0] > maxLat) maxLat = c[0];
            if (c[1] < minLng) minLng = c[1];
            if (c[1] > maxLng) maxLng = c[1];

            points.push({ lat: c[0], lng: c[1], ele: ele, t: t, dist: dist });
        }

        if (points.length < 2) return null;
        if (!NS.num(minLat) || !NS.num(maxLat) || !NS.num(minLng) || !NS.num(maxLng)) return null;

        const hasTime = timeCount >= 2 && NS.num(points[points.length - 1].t) && points[points.length - 1].t > 0;
        const rawSpeed = new Array(points.length).fill(null);

        if (hasTime) {
            for (let i = 1; i < points.length; i++) {
                const dt = (points[i].t != null && points[i - 1].t != null)
                    ? points[i].t - points[i - 1].t
                    : null;
                const dd = points[i].dist - points[i - 1].dist;
                if (dt != null && dt > 0) {
                    const v = dd / (dt / 3600);
                    rawSpeed[i] = v > NS.SPEED_NOISE_CAP ? null : v;
                }
            }
            rawSpeed[0] = rawSpeed[1];
        }

        const eleSeries = NS.smooth(NS.fillGaps(points.map(p => p.ele)), 1);
        const speedSeries = hasTime
            ? NS.smooth(NS.fillGaps(rawSpeed), 2)
            : new Array(points.length).fill(null);

        return {
            points: points,
            eleSeries: eleSeries,
            speedSeries: speedSeries,
            hasEle: hasEle,
            hasTime: hasTime,
            totalDist: dist,
            minLat: minLat,
            maxLat: maxLat,
            minLng: minLng,
            maxLng: maxLng
        };
    };

    NS.buildProfiles = function (rides) {
        return (rides || []).map(NS.buildProfile);
    };

    NS.hasAnyProfile = function () {
        return NS.state.profiles.some(function (p) {
            return !!p && p.points && p.points.length > 1;
        });
    };

    NS.firstValidLatLng = function () {
        const profiles = NS.state.profiles;
        for (let i = 0; i < profiles.length; i++) {
            const p = profiles[i];
            if (p && p.points && p.points.length) {
                return [p.points[0].lat, p.points[0].lng];
            }
        }
        return null;
    };

    NS.nearestIndexByDist = function (points, target) {
        let lo = 0;
        let hi = points.length - 1;
        while (lo < hi) {
            const mid = (lo + hi) >> 1;
            if (points[mid].dist < target) lo = mid + 1;
            else hi = mid;
        }
        if (lo > 0 && Math.abs(points[lo - 1].dist - target) < Math.abs(points[lo].dist - target)) return lo - 1;
        return lo;
    };

    NS.summaryText = function (idx) {
        const ride = NS.state.rides[idx];
        const prof = NS.state.profiles[idx];
        if (!ride || !prof) return '';

        const parts = [
            Number(ride.distance_km || prof.totalDist || 0).toFixed(1) + ' km',
            '↑' + Number(ride.elevation_gain_m || 0).toFixed(0) + ' m'
        ];

        if (ride.duration_minutes > 0) {
            const avg = Number(ride.distance_km || 0) / (Number(ride.duration_minutes) / 60);
            parts.push('avg ' + avg.toFixed(1) + ' km/h');
        } else if (!prof.hasTime) {
            parts.push('no timing data');
        }

        return parts.join(' · ');
    };

    NS.sparklineSVG = function (series, width, height) {
        if (!series || series.length < 2) return '';

        let min = Infinity;
        let max = -Infinity;
        for (const v of series) {
            if (v < min) min = v;
            if (v > max) max = v;
        }
        if (!NS.num(min) || !NS.num(max)) return '';

        const range = (max - min) || 1;
        const step = Math.max(1, Math.floor(series.length / 60));
        const pts = [];

        for (let i = 0; i < series.length; i += step) {
            const x = (i / (series.length - 1)) * width;
            const y = height - ((series[i] - min) / range) * (height - 2) - 1;
            pts.push(x.toFixed(1) + ',' + y.toFixed(1));
        }
        if (pts.length < 2) return '';

        const pathD = 'M ' + pts.join(' L ');
        const fillD = 'M 0,' + height + ' L ' + pts.join(' L ') + ' L ' + width + ',' + height + ' Z';

        return '<svg width="' + width + '" height="' + height + '" viewBox="0 0 ' + width + ' ' + height +
            '" preserveAspectRatio="none" aria-hidden="true">' +
            '<path d="' + fillD + '" fill="rgba(255,140,66,.18)"/>' +
            '<path d="' + pathD + '" fill="none" stroke="#ff8c42" stroke-width="1.5" stroke-linejoin="round"/></svg>';
    };
})();