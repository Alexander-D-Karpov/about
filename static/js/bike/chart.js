(function () {
    'use strict';

    const NS = (window.BikeMap = window.BikeMap || {});

    function buildGradientStops(prof, distMax) {
        const n = prof.points.length;
        const step = Math.max(1, Math.floor(n / 140));
        let out = '';
        for (let i = 0; i < n; i += step) {
            const off = (prof.points[i].dist / distMax) * 100;
            out += '<stop offset="' + off.toFixed(2) + '%" stop-color="' + NS.speedColor(prof.speedSeries[i]) + '"/>';
        }
        out += '<stop offset="100%" stop-color="' + NS.speedColor(prof.speedSeries[n - 1]) + '"/>';
        return out;
    }

    NS.renderChart = function () {
        const st = NS.state;
        if (!st.chartHost || !st.chart) return;

        const idx = st.chart.idx;
        const prof = st.profiles[idx];
        if (!prof || prof.points.length < 2) return;

        const width = Math.max(220, Math.floor(st.chartHost.clientWidth) || 320);
        const innerW = width - NS.PAD_L - NS.PAD_R;
        const innerH = NS.CHART_H - NS.PAD_T - NS.PAD_B;

        const pts = prof.points;
        const n = pts.length;
        const distMax = pts[n - 1].dist || 1;
        const series = prof.hasEle ? prof.eleSeries : prof.speedSeries;

        let minV = Infinity;
        let maxV = -Infinity;
        for (const v of series) {
            if (v < minV) minV = v;
            if (v > maxV) maxV = v;
        }
        if (!NS.num(minV) || !NS.num(maxV)) {
            minV = 0;
            maxV = 1;
        }
        if (maxV - minV < 1) maxV = minV + 1;

        const padV = (maxV - minV) * 0.15;
        const y0 = minV - padV;
        const y1 = maxV + padV;

        const X = d => NS.PAD_L + (d / distMax) * innerW;
        const Y = v => NS.PAD_T + innerH - ((v - y0) / (y1 - y0)) * innerH;

        let d = '';
        for (let i = 0; i < n; i++) {
            d += (i ? ' L ' : 'M ') + X(pts[i].dist).toFixed(1) + ',' + Y(series[i]).toFixed(1);
        }

        const baseY = (NS.PAD_T + innerH).toFixed(1);
        const area = d + ' L ' + X(distMax).toFixed(1) + ',' + baseY + ' L ' + X(0).toFixed(1) + ',' + baseY + ' Z';

        const useGradient = st.speedMode && prof.hasTime;
        const gradId = 'bike-grad-' + idx + '-' + (useGradient ? 's' : 'p');
        const paint = useGradient ? 'url(#' + gradId + ')' : NS.COLORS[idx % NS.COLORS.length];

        let grid = '';
        [minV, (minV + maxV) / 2, maxV].forEach(function (v) {
            const y = Y(v).toFixed(1);
            grid += '<line class="bike-profile-grid" x1="' + NS.PAD_L + '" y1="' + y + '" x2="' + (width - NS.PAD_R) + '" y2="' + y + '"/>';
            grid += '<text class="bike-profile-axis" x="' + (NS.PAD_L - 6) + '" y="' + (parseFloat(y) + 3.5).toFixed(1) +
                '" text-anchor="end">' + Math.round(v) + '</text>';
        });

        const xTicks = 4;
        let xAxis = '';
        for (let i = 0; i <= xTicks; i++) {
            const dv = (distMax / xTicks) * i;
            xAxis += '<text class="bike-profile-axis" x="' + X(dv).toFixed(1) + '" y="' + (NS.CHART_H - 5) +
                '" text-anchor="' + (i === 0 ? 'start' : (i === xTicks ? 'end' : 'middle')) + '">' +
                dv.toFixed(dv >= 10 ? 0 : 1) + '</text>';
        }

        const defs = useGradient
            ? '<defs><linearGradient id="' + gradId + '" x1="0" y1="0" x2="1" y2="0">' + buildGradientStops(prof, distMax) + '</linearGradient></defs>'
            : '';

        st.chartHost.innerHTML =
            '<svg class="bike-profile-svg" viewBox="0 0 ' + width + ' ' + NS.CHART_H + '" width="' + width +
            '" height="' + NS.CHART_H + '" preserveAspectRatio="none">' +
            defs +
            grid +
            '<path d="' + area + '" fill="' + paint + '" opacity="0.28"/>' +
            '<path d="' + d + '" fill="none" stroke="' + paint + '" stroke-width="1.8" stroke-linejoin="round"/>' +
            xAxis +
            '<line class="bike-profile-cursor" x1="0" y1="' + NS.PAD_T + '" x2="0" y2="' + baseY + '" style="display:none"/>' +
            '<circle class="bike-profile-dot" cx="0" cy="0" r="4.5" style="display:none"/>' +
            '</svg>';

        st.chart = {
            idx: idx,
            prof: prof,
            series: series,
            innerW: innerW,
            distMax: distMax,
            X: X,
            Y: Y,
            cursor: st.chartHost.querySelector('.bike-profile-cursor'),
            dot: st.chartHost.querySelector('.bike-profile-dot')
        };

        if (st.profileTitle) {
            const ride = st.rides[idx] || {};
            st.profileTitle.textContent = (ride.name || 'Ride') + (ride.date ? ' · ' + ride.date : '');
        }
        if (st.profileReadout) st.profileReadout.textContent = NS.summaryText(idx);
    };

    NS.setChartCursor = function (i) {
        const st = NS.state;
        const chart = st.chart;
        if (!chart || !chart.prof) return;

        const prof = chart.prof;
        const p = prof.points[i];
        if (!p) return;

        const x = chart.X(p.dist);
        const y = chart.Y(chart.series[i]);

        if (chart.cursor) {
            chart.cursor.setAttribute('x1', x.toFixed(1));
            chart.cursor.setAttribute('x2', x.toFixed(1));
            chart.cursor.style.display = '';
        }
        if (chart.dot) {
            chart.dot.setAttribute('cx', x.toFixed(1));
            chart.dot.setAttribute('cy', y.toFixed(1));
            chart.dot.setAttribute('fill', (prof.hasTime && st.speedMode)
                ? NS.speedColor(prof.speedSeries[i])
                : NS.COLORS[chart.idx % NS.COLORS.length]);
            chart.dot.style.display = '';
        }

        if (st.profileReadout) {
            const parts = ['Dist: ' + p.dist.toFixed(2) + ' km'];
            if (prof.hasEle) parts.push('Elev: ' + Math.round(prof.eleSeries[i]) + ' m');
            if (prof.hasTime) {
                parts.push('Speed: ' + prof.speedSeries[i].toFixed(1) + ' km/h');
                if (p.t != null) parts.push(NS.fmtClock(p.t));
            }
            st.profileReadout.textContent = parts.join(' · ');
        }

        if (NS.showHoverPoint) NS.showHoverPoint(chart.idx, i);
    };

    NS.clearChartCursor = function () {
        const st = NS.state;
        if (st.chart) {
            if (st.chart.cursor) st.chart.cursor.style.display = 'none';
            if (st.chart.dot) st.chart.dot.style.display = 'none';
            if (st.profileReadout && st.chart.prof) st.profileReadout.textContent = NS.summaryText(st.chart.idx);
        }
        if (NS.clearHoverPoint) NS.clearHoverPoint();
    };

    function onChartPointer(e) {
        const st = NS.state;
        if (!st.chart || !st.chart.prof || !st.chartHost) return;

        const rect = st.chartHost.getBoundingClientRect();
        const clientX = (e.touches && e.touches.length) ? e.touches[0].clientX : e.clientX;
        const frac = NS.clamp((clientX - rect.left - NS.PAD_L) / st.chart.innerW, 0, 1);
        NS.setChartCursor(NS.nearestIndexByDist(st.chart.prof.points, frac * st.chart.distMax));
    }

    NS.showProfile = function (idx) {
        const st = NS.state;
        if (!st.profileBox) return;

        const prof = st.profiles[idx];
        if (!prof || prof.points.length < 2) {
            NS.hideProfile();
            return;
        }

        const wasHidden = st.profileBox.hidden;
        st.profileBox.hidden = false;
        st.chart = { idx: idx };
        NS.renderChart();

        if (wasHidden) {
            NS.notifyLayout();
            NS.invalidateMapSize();
        }
    };

    NS.hideProfile = function () {
        const st = NS.state;
        if (!st.profileBox || st.profileBox.hidden) return;

        NS.clearChartCursor();
        st.profileBox.hidden = true;
        st.chart = null;
        NS.notifyLayout();
        NS.invalidateMapSize();
    };

    NS.bindProfileEvents = function () {
        const st = NS.state;
        st.section = document.getElementById('bike-plugin');
        st.profileBox = document.getElementById('bike-profile');
        st.profileTitle = document.getElementById('bike-profile-title');
        st.profileReadout = document.getElementById('bike-profile-readout');
        st.chartHost = document.getElementById('bike-profile-chart');
        if (!st.chartHost) return;

        st.chartHost.addEventListener('mousemove', onChartPointer);
        st.chartHost.addEventListener('mouseleave', NS.clearChartCursor);
        st.chartHost.addEventListener('touchstart', function (e) {
            onChartPointer(e);
            e.preventDefault();
        }, { passive: false });
        st.chartHost.addEventListener('touchmove', function (e) {
            onChartPointer(e);
            e.preventDefault();
        }, { passive: false });
        st.chartHost.addEventListener('touchend', NS.clearChartCursor);

        if (typeof ResizeObserver !== 'undefined') {
            let lastW = 0;
            st.chartRO = new ResizeObserver(function () {
                if (!NS.state.chart || !NS.state.chartHost) return;
                const w = Math.floor(NS.state.chartHost.clientWidth);
                if (Math.abs(w - lastW) < 4) return;
                lastW = w;
                NS.renderChart();
            });
            st.chartRO.observe(st.chartHost);
        }
    };
})();