(function () {
    'use strict';

    let currentBPM = 0;
    let heartbeatInterval = null;
    let healthSection = null;

    function init() {
        healthSection = document.querySelector('[data-plugin="health"]');
        if (!healthSection) return;

        const heartbeatEl = healthSection.querySelector('[data-heartbeat]');
        if (heartbeatEl) {
            currentBPM = parseInt(heartbeatEl.dataset.bpm) || 0;
            if (currentBPM > 0) {
                startHeartbeat(currentBPM);
            }
        }

        if (window.aboutWS) {
            window.aboutWS.subscribe('heartrate_update', handleHeartRateUpdate);
            window.aboutWS.subscribe('health_update', handleHealthUpdate);
        }
    }

    function handleHeartRateUpdate(data) {
        if (!data || !data.bpm) return;

        currentBPM = data.bpm;
        updateHeartRateDisplay(currentBPM);
        startHeartbeat(currentBPM);
    }

    function handleHealthUpdate(data) {
        if (!healthSection) return;

        const updates = {
            'steps-today': data.steps_today,
            'calories-today': data.calories_today,
            'workout-minutes': data.workout_minutes_week ? `${data.workout_minutes_week} min` : null,
            'sleep-hours': data.sleep_last_night ? `${data.sleep_last_night.toFixed(1)}h` : null,
            'heart-rate': data.current_heart_rate ? `${data.current_heart_rate} bpm` : null,
            'resting-hr': data.resting_heart_rate ? `${data.resting_heart_rate} bpm resting` : null,
            'hydration': data.hydration_today
        };

        Object.entries(updates).forEach(([metric, value]) => {
            if (value === null || value === undefined) return;

            const el = healthSection.querySelector(`[data-metric="${metric}"]`);
            if (el) {
                const formattedValue = typeof value === 'number' ? formatNumber(value) : value;
                if (el.textContent !== formattedValue) {
                    el.textContent = formattedValue;
                    el.classList.add('updated');
                    setTimeout(() => el.classList.remove('updated'), 500);
                }
            }
        });

        if (data.current_heart_rate && data.current_heart_rate !== currentBPM) {
            currentBPM = data.current_heart_rate;
            startHeartbeat(currentBPM);
        }

        const updatedEl = healthSection.querySelector('[data-health-updated]');
        if (updatedEl) {
            updatedEl.textContent = 'just now';
        }
    }

    function updateHeartRateDisplay(bpm) {
        if (!healthSection) return;

        const hrElement = healthSection.querySelector('[data-metric="heart-rate"]');
        if (hrElement) {
            hrElement.textContent = `${bpm} bpm`;
            hrElement.classList.add('updated');
            setTimeout(() => hrElement.classList.remove('updated'), 500);
        }

        const heartbeatEl = healthSection.querySelector('[data-heartbeat]');
        if (heartbeatEl) {
            heartbeatEl.dataset.bpm = bpm;
        }
    }

    function startHeartbeat(bpm) {
        if (heartbeatInterval) {
            clearInterval(heartbeatInterval);
        }

        if (bpm <= 0 || bpm > 250) return;

        const heartbeatElement = healthSection?.querySelector('[data-heartbeat]');
        if (!heartbeatElement) return;

        const intervalMs = (60 / bpm) * 1000;

        heartbeatElement.style.setProperty('--heartbeat-duration', `${intervalMs}ms`);

        function beat() {
            heartbeatElement.classList.remove('beating');
            requestAnimationFrame(() => {
                requestAnimationFrame(() => {
                    heartbeatElement.classList.add('beating');
                });
            });
        }

        beat();
        heartbeatInterval = setInterval(beat, intervalMs);
    }

    function formatNumber(num) {
        if (typeof num !== 'number') return num;
        if (num >= 1000) {
            return num.toLocaleString();
        }
        return num.toString();
    }

    function cleanup() {
        if (heartbeatInterval) {
            clearInterval(heartbeatInterval);
            heartbeatInterval = null;
        }

        if (window.aboutWS) {
            window.aboutWS.unsubscribe('heartrate_update', handleHeartRateUpdate);
            window.aboutWS.unsubscribe('health_update', handleHealthUpdate);
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    window.addEventListener('beforeunload', cleanup);

    window.healthPlugin = {
        getCurrentBPM: () => currentBPM,
        startHeartbeat,
        cleanup
    };
})();