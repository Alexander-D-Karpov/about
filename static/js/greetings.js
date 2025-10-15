(function() {
    'use strict';

    const styles = {
        title: 'color: #7aa2ff; font-size: 24px; font-weight: bold; text-shadow: 2px 2px 4px rgba(0,0,0,0.5);',
        subtitle: 'color: #b692ff; font-size: 14px; font-weight: 600;',
        info: 'color: #8a94a2; font-size: 12px;',
        accent: 'color: #3ad38b; font-weight: bold;',
        warning: 'color: #ffd166; font-weight: bold;',
        code: 'background: #0b0f14; color: #7aa2ff; padding: 2px 6px; border-radius: 4px; font-family: monospace;'
    };

    function formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    function formatTime(ms) {
        if (ms < 1000) return Math.round(ms) + 'ms';
        return (ms / 1000).toFixed(2) + 's';
    }

    function getPerformanceMetrics() {
        const metrics = {};

        if (window.performance && window.performance.timing) {
            const timing = window.performance.timing;
            const nav = window.performance.navigation;

            if (timing.loadEventEnd > 0) {
                metrics.loadTime = timing.loadEventEnd - timing.navigationStart;
            }
            if (timing.domContentLoadedEventEnd > 0) {
                metrics.domContentLoaded = timing.domContentLoadedEventEnd - timing.navigationStart;
            }
            if (timing.responseStart > 0) {
                metrics.firstByte = timing.responseStart - timing.navigationStart;
            }
            if (timing.domInteractive > 0) {
                metrics.domInteractive = timing.domInteractive - timing.navigationStart;
            }

            if (nav) {
                metrics.navigationType = nav.type === 0 ? 'navigate' :
                    nav.type === 1 ? 'reload' :
                        nav.type === 2 ? 'back/forward' : 'unknown';
            }
        }

        if (window.performance && window.performance.getEntriesByType) {
            const entries = window.performance.getEntriesByType('resource');
            metrics.resourceCount = entries.length;
            metrics.totalSize = entries.reduce((sum, entry) => sum + (entry.transferSize || 0), 0);

            const paintEntries = window.performance.getEntriesByType('paint');
            paintEntries.forEach(entry => {
                if (entry.name === 'first-contentful-paint') {
                    metrics.fcp = entry.startTime;
                }
            });
        }

        if (window.performance && window.performance.memory) {
            metrics.jsHeapSize = window.performance.memory.usedJSHeapSize;
            metrics.jsHeapLimit = window.performance.memory.jsHeapSizeLimit;
        }

        metrics.domNodes = document.querySelectorAll('*').length;
        metrics.plugins = document.querySelectorAll('.plugin').length;

        return metrics;
    }

    console.log('%c Hello there', styles.title);
    console.log('%c Welcome to my site!', styles.subtitle);

    console.log('%c Source:', styles.accent, 'https://github.com/Alexander-D-Karpov/about');

    const metrics = getPerformanceMetrics();

    console.log('%c System Info:', styles.warning);
    console.log('%c  • Plugins:', styles.info, metrics.plugins);
    console.log('%c  • DOM nodes:', styles.info, metrics.domNodes.toLocaleString());
    console.log('%c  • Grid columns:', styles.info, getComputedStyle(document.querySelector('.mosaic') || document.body).gridTemplateColumns);
    console.log('%c  • Theme:', styles.info, 'Dark mode');

    console.log('%c Performance Metrics:', styles.warning);
    if (metrics.loadTime) {
        console.log('%c  • Page load:', styles.info, formatTime(metrics.loadTime));
    }
    if (metrics.domContentLoaded) {
        console.log('%c  • DOM ready:', styles.info, formatTime(metrics.domContentLoaded));
    }
    if (metrics.domInteractive) {
        console.log('%c  • DOM interactive:', styles.info, formatTime(metrics.domInteractive));
    }
    if (metrics.firstByte) {
        console.log('%c  • Time to first byte:', styles.info, formatTime(metrics.firstByte));
    }
    if (metrics.fcp) {
        console.log('%c  • First contentful paint:', styles.info, formatTime(metrics.fcp));
    }
    if (metrics.totalSize) {
        console.log('%c  • Total transferred:', styles.info, formatBytes(metrics.totalSize));
    }
    if (metrics.resourceCount) {
        console.log('%c  • HTTP requests:', styles.info, metrics.resourceCount);
    }
    if (metrics.jsHeapSize) {
        console.log('%c  • JS heap size:', styles.info, formatBytes(metrics.jsHeapSize) + ' / ' + formatBytes(metrics.jsHeapLimit));
    }
    if (metrics.navigationType) {
        console.log('%c  • Navigation type:', styles.info, metrics.navigationType);
    }

    console.log('%c Try this:', styles.warning);
    console.log('%c  mosaicUtils.expand(document.querySelector(\'.plugin\'))', styles.code, '- Expand a plugin');
    console.log('%c  playTrack(\'your favorite song\')', styles.code, '- Search and play music');
    console.log('%c  mosaicUtils.resizeAll()', styles.code, '- Recalculate layout');

    console.log('%c Looking for a developer?', styles.accent);
    console.log('%c  Telegram: @sanspie', styles.info);
    console.log('%c  GitHub: Alexander-D-Karpov', styles.info);
    console.log('%c  Email: sanspie@akarpov.ru', styles.info);

    window.siteMetrics = {
        getPerformance: getPerformanceMetrics,
        refresh: function() {
            const fresh = getPerformanceMetrics();
            console.log('%c 📊 Current Metrics:', styles.warning);
            console.table(fresh);
        },
        compare: function() {
            const fresh = getPerformanceMetrics();
            console.log('%c Metrics Comparison:', styles.warning);
            if (metrics.loadTime && fresh.loadTime) {
                console.log('%c  Initial load:', styles.info, formatTime(metrics.loadTime));
            }
            console.log('%c  Current uptime:', styles.info, formatTime(performance.now()));
            console.log('%c  DOM nodes change:', styles.info,
                (fresh.domNodes - metrics.domNodes > 0 ? '+' : '') + (fresh.domNodes - metrics.domNodes));
        }
    };

    if (document.readyState === 'complete') {
        setTimeout(() => {
            const updated = getPerformanceMetrics();
            if (updated.loadTime && !metrics.loadTime) {
                console.log('%c Load Complete:', styles.accent, formatTime(updated.loadTime));
            }
        }, 100);
    } else {
        window.addEventListener('load', () => {
            setTimeout(() => {
                const updated = getPerformanceMetrics();
                if (updated.loadTime) {
                    console.log('%c Load Complete:', styles.accent, formatTime(updated.loadTime));
                }
            }, 100);
        });
    }
})();