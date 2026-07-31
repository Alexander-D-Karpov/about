(function () {
    'use strict';

    const NS = (window.BikeMap = window.BikeMap || {});

    NS.rideShareKey = function (idx) {
        const ride = NS.state.rides[idx];
        if (!ride) return String(idx);
        const date = NS.slugify(ride.date || 'unknown-date');
        const name = NS.slugify(ride.name || 'ride');
        const dist = Number(ride.distance_km || 0).toFixed(1).replace('.', '-');
        return date + '-' + name + '-' + dist + 'km';
    };

    NS.rideIndexFromURL = function () {
        const url = new URL(window.location.href);
        const raw = url.searchParams.get('ride');
        if (!raw) return null;

        if (/^\d+$/.test(raw)) {
            const numeric = parseInt(raw, 10);
            return numeric >= 0 && numeric < NS.state.rides.length ? numeric : null;
        }

        const idx = NS.state.rides.findIndex(function (_, i) {
            return NS.rideShareKey(i) === raw;
        });
        return idx >= 0 ? idx : null;
    };

    NS.updateRideURL = function () {
        const url = new URL(window.location.href);
        if (NS.state.selectedRideIndex == null) url.searchParams.delete('ride');
        else url.searchParams.set('ride', NS.rideShareKey(NS.state.selectedRideIndex));
        history.replaceState({}, '', url.toString());
    };

    function makeControlButton(html, title) {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'map-control-btn';
        b.title = title;
        b.setAttribute('aria-label', title);
        b.innerHTML = html;
        return b;
    }

    NS.syncFullscreenState = function (target) {
        const btn = NS.state.fullscreenBtn;
        if (!btn) return;
        btn.setAttribute('aria-pressed', document.fullscreenElement === target ? 'true' : 'false');
    };

    NS.buildMapControls = function (fullscreenTarget) {
        const host = document.querySelector('#bike-map-container .map-controls');
        if (!host) return;

        const st = NS.state;
        host.innerHTML = '';

        const fitBtn = makeControlButton(NS.ICONS.fit, 'Fit all rides');
        fitBtn.addEventListener('click', function () {
            NS.fitAllRides(true);
        });
        host.appendChild(fitBtn);

        st.speedBtn = makeControlButton(NS.ICONS.speed, 'Speed map (all rides merged)');
        st.speedBtn.setAttribute('aria-pressed', 'false');
        st.speedBtn.addEventListener('click', function () {
            if (!NS.state.speedRideCount) {
                NS.showMapToast('None of these GPX files contain timestamps, so speed cannot be computed');
                return;
            }
            NS.state.speedMode = !NS.state.speedMode;
            NS.applyMapMode();
            if (NS.state.chart) NS.renderChart();
            if (NS.state.speedMode && NS.state.noSpeedRideCount) {
                NS.showMapToast(NS.state.noSpeedRideCount + ' of ' + NS.state.rides.length +
                    ' rides have no timestamps — shown as grey dashes');
            }
        });
        if (!st.speedRideCount) st.speedBtn.classList.add('is-unavailable');
        host.appendChild(st.speedBtn);

        st.fullscreenBtn = makeControlButton(NS.ICONS.fullscreen, 'Toggle fullscreen');
        st.fullscreenBtn.setAttribute('aria-pressed', 'false');
        st.fullscreenBtn.addEventListener('click', async function () {
            try {
                if (document.fullscreenElement === fullscreenTarget) await document.exitFullscreen();
                else if (fullscreenTarget.requestFullscreen) await fullscreenTarget.requestFullscreen();
            } catch (err) {
                console.debug('[bike] fullscreen toggle failed:', err);
            } finally {
                NS.syncFullscreenState(fullscreenTarget);
                NS.invalidateMapSize();
            }
        });
        host.appendChild(st.fullscreenBtn);

        document.addEventListener('fullscreenchange', function () {
            NS.syncFullscreenState(fullscreenTarget);
            setTimeout(function () {
                if (!NS.state.map) return;
                try {
                    NS.state.map.invalidateSize({ pan: false, animate: false });
                } catch (e) {}
                if (NS.state.speedMode) {
                    NS.state.speedLayerKey = null;
                    NS.applyMapMode();
                }
                if (NS.state.autoView && NS.state.selectedRideIndex == null) NS.fitAllRides(false);
            }, 120);
        });
    };

    NS.setupMapResizeHandling = function (targetEl) {
        if (!targetEl) return function () {};

        const refresh = function () {
            NS.invalidateMapSize();
            if (NS.state.speedMode && NS.speedWeight() !== NS.state.speedLayerWeight) {
                NS.state.speedLayerKey = null;
                NS.applyMapMode();
            }
            if (NS.state.chart) NS.renderChart();
        };

        const onWindowResize = function () { refresh(); };
        const onOrientationChange = function () { refresh(); };
        const onVisibilityChange = function () { if (!document.hidden) refresh(); };

        window.addEventListener('resize', onWindowResize);
        window.addEventListener('orientationchange', onOrientationChange);
        document.addEventListener('visibilitychange', onVisibilityChange);

        let resizeObserver = null;
        if (typeof ResizeObserver !== 'undefined') {
            resizeObserver = new ResizeObserver(function () { refresh(); });
            resizeObserver.observe(targetEl);
        }

        refresh();

        return function () {
            window.removeEventListener('resize', onWindowResize);
            window.removeEventListener('orientationchange', onOrientationChange);
            document.removeEventListener('visibilitychange', onVisibilityChange);
            if (resizeObserver) resizeObserver.disconnect();
        };
    };

    NS.setupRideListScrolling = function () {
        const list = document.querySelector('.bike-rides-list');
        if (!list) return;
        list.style.overflowY = 'auto';
        list.style.overflowX = 'hidden';
        list.style.height = '280px';
        list.style.maxHeight = '280px';
        list.style.scrollBehavior = 'smooth';
        list.style.webkitOverflowScrolling = 'touch';
    };

    NS.decorateRideList = function () {
        document.querySelectorAll('.bike-ride-item').forEach(function (item) {
            if (item.dataset.bikeDecorated === '1') return;
            item.dataset.bikeDecorated = '1';

            const idx = parseInt(item.dataset.ride, 10);
            const prof = NS.state.profiles[idx];

            const toggleBtn = document.createElement('button');
            toggleBtn.className = 'bike-toggle-btn';
            toggleBtn.type = 'button';
            toggleBtn.title = 'Toggle visibility';
            toggleBtn.setAttribute('aria-pressed', 'false');
            toggleBtn.setAttribute('aria-label', 'Toggle ride visibility');
            toggleBtn.innerHTML = NS.ICONS.eye;
            toggleBtn.addEventListener('click', function (e) {
                e.stopPropagation();
                NS.toggleRideVisibility(idx);
            });

            const chips = item.querySelector('.bike-ride-chips');

            if (prof && !prof.hasTime) {
                item.dataset.noSpeed = '1';
                item.title = 'This GPX has no timestamps — no speed data';
                if (chips) {
                    const chip = document.createElement('span');
                    chip.className = 'bike-chip bike-nospeed-chip';
                    chip.textContent = 'no speed';
                    chips.appendChild(chip);
                }
            }

            if (chips) chips.after(toggleBtn);
            else item.appendChild(toggleBtn);

            if (prof && prof.hasEle) {
                const info = item.querySelector('.bike-ride-info');
                if (info) {
                    const sparkline = document.createElement('div');
                    sparkline.className = 'bike-ride-sparkline';
                    sparkline.innerHTML = NS.sparklineSVG(prof.eleSeries, 120, 24);
                    info.appendChild(sparkline);
                }
            }

            if (!prof) {
                item.classList.add('ride-hidden');
                item.title = 'This ride has no usable track data';
            }

            item.addEventListener('mouseenter', function () {
                if (!NS.state.hiddenRides.has(idx)) NS.highlightRide(idx);
            });
            item.addEventListener('mouseleave', function () {
                NS.resetHighlight();
            });
            item.addEventListener('click', function (e) {
                if (e.target.closest('.bike-toggle-btn')) return;
                NS.selectRide(idx, {
                    toggle: true,
                    fit: true,
                    openPopup: true,
                    scrollIntoView: true,
                    updateURL: true
                });
                NS.invalidateMapSize();
            });
        });
    };
})();