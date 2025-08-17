(function() {
    'use strict';

    let ws = null;
    let reconnectAttempts = 0;
    const maxReconnectAttempts = 10;
    const baseReconnectDelay = 1000;
    let isConnected = false;
    let shouldReconnect = true;
    let heartbeatInterval = null;
    let reconnectTimeout = null;
    let connectionRetryCount = 0;

    function connect() {
        if (ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) {
            return;
        }

        if (reconnectAttempts >= maxReconnectAttempts) {
            console.log('Max reconnection attempts reached');
            updateConnectionStatus('failed');
            return;
        }

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsURL = protocol + '//' + window.location.host + '/ws';

        try {
            updateConnectionStatus('connecting');

            if (ws) {
                ws.onopen = null;
                ws.onmessage = null;
                ws.onclose = null;
                ws.onerror = null;
                ws.close();
                ws = null;
            }

            ws = new WebSocket(wsURL);
            ws.binaryType = 'arraybuffer';

            const connectionTimeout = setTimeout(() => {
                if (ws && ws.readyState === WebSocket.CONNECTING) {
                    console.log('Connection timeout, closing WebSocket');
                    ws.close();
                }
            }, 10000);

            ws.onopen = function() {
                console.log('WebSocket connected successfully');
                clearTimeout(connectionTimeout);
                reconnectAttempts = 0;
                connectionRetryCount = 0;
                isConnected = true;
                shouldReconnect = true;
                updateConnectionStatus('connected');
                startHeartbeat();

                if (reconnectTimeout) {
                    clearTimeout(reconnectTimeout);
                    reconnectTimeout = null;
                }

                sendMessage({ type: 'register', data: { page: window.location.pathname } });
            };

            ws.onmessage = function(event) {
                try {
                    let messageData = event.data;

                    if (messageData instanceof ArrayBuffer) {
                        messageData = new TextDecoder().decode(messageData);
                    }

                    const messages = messageData.toString().split('\n');
                    messages.forEach(messageStr => {
                        messageStr = messageStr.trim();
                        if (messageStr) {
                            try {
                                const message = JSON.parse(messageStr);
                                handleMessage(message);
                            } catch (parseErr) {
                                console.debug('Failed to parse message:', messageStr, parseErr);
                            }
                        }
                    });
                } catch (e) {
                    console.error('Error processing WebSocket message:', e);
                }
            };

            ws.onclose = function(event) {
                clearTimeout(connectionTimeout);
                console.log('WebSocket disconnected, code:', event.code, 'reason:', event.reason || 'none', 'clean:', event.wasClean);
                isConnected = false;
                stopHeartbeat();

                if (event.code === 1000 || event.code === 1001) {
                    shouldReconnect = false;
                    updateConnectionStatus('disconnected');
                    return;
                }

                if (shouldReconnect) {
                    updateConnectionStatus('disconnected');
                    attemptReconnect();
                } else {
                    updateConnectionStatus('disconnected');
                }
            };

            ws.onerror = function(error) {
                clearTimeout(connectionTimeout);
                console.error('WebSocket error:', error);
                updateConnectionStatus('error');

                if (ws) {
                    ws.close();
                }
            };

        } catch (e) {
            console.error('Failed to create WebSocket connection:', e);
            updateConnectionStatus('error');
            attemptReconnect();
        }
    }

    function sendMessage(message) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            try {
                ws.send(JSON.stringify(message));
            } catch (e) {
                console.error('Failed to send message:', e);
            }
        }
    }

    function attemptReconnect() {
        if (!shouldReconnect || reconnectTimeout) return;

        reconnectAttempts++;
        connectionRetryCount++;

        const delay = Math.min(baseReconnectDelay * Math.pow(2, Math.min(reconnectAttempts - 1, 5)), 30000);
        const jitter = Math.random() * 1000;
        const finalDelay = delay + jitter;

        console.log(`Reconnecting in ${Math.round(finalDelay)}ms (attempt ${reconnectAttempts}/${maxReconnectAttempts})`);
        updateConnectionStatus('connecting');

        reconnectTimeout = setTimeout(() => {
            reconnectTimeout = null;
            if (shouldReconnect) {
                connect();
            }
        }, finalDelay);
    }

    function startHeartbeat() {
        stopHeartbeat();
        heartbeatInterval = setInterval(() => {
            if (ws && ws.readyState === WebSocket.OPEN) {
                try {
                    const pingMessage = JSON.stringify({ type: 'ping' });
                    ws.send(pingMessage);
                } catch (e) {
                    console.error('Failed to send ping:', e);
                }
            } else {
                stopHeartbeat();
            }
        }, 25000);
    }

    function stopHeartbeat() {
        if (heartbeatInterval) {
            clearInterval(heartbeatInterval);
            heartbeatInterval = null;
        }
    }

    function handleMessage(message) {
        try {
            switch (message.type) {
                case 'pong':
                    break;
                case 'lastfm_update':
                    updateLastFM(message.data);
                    break;
                case 'lastfm_realtime':
                    updateLastFMRealtime(message.data);
                    break;
                case 'beatleader_update':
                    updateBeatLeader(message.data);
                    break;
                case 'steam_update':
                    updateSteam(message.data);
                    break;
                case 'visitors_update':
                    updateVisitors(message.data);
                    break;
                case 'webring_update':
                    updateWebring(message.data);
                    break;
                case 'music_play':
                    if (window.musicPlayer) {
                        window.musicPlayer.handleMusicUpdate(message);
                    }
                    break;
                case 'plugin_update':
                    handlePluginUpdate(message.data);
                    break;
                case 'plugins_updated':
                    setTimeout(() => {
                        if (window.location.pathname !== '/admin') {
                            window.location.reload();
                        }
                    }, 1000);
                    break;
                default:
                    console.debug('Unknown message type:', message.type);
            }
        } catch (e) {
            console.error('Error handling message:', message, e);
        }
    }

    function updateConnectionStatus(status) {
        const statusIndicator = document.getElementById('connection-status');
        const statusText = document.getElementById('status-text');

        if (statusIndicator) {
            statusIndicator.className = 'status-indicator';
            switch (status) {
                case 'connected':
                    statusIndicator.classList.add('status-online');
                    break;
                case 'connecting':
                    statusIndicator.classList.add('status-loading');
                    break;
                case 'disconnected':
                case 'error':
                case 'failed':
                    statusIndicator.classList.add('status-offline');
                    break;
            }
        }

        if (statusText) {
            switch (status) {
                case 'connected':
                    statusText.textContent = 'Connected';
                    break;
                case 'connecting':
                    statusText.textContent = connectionRetryCount > 0 ? `Reconnecting... (${connectionRetryCount})` : 'Connecting...';
                    break;
                case 'disconnected':
                    statusText.textContent = 'Disconnected';
                    break;
                case 'error':
                    statusText.textContent = 'Connection Error';
                    break;
                case 'failed':
                    statusText.textContent = 'Connection Failed';
                    break;
            }
        }
    }

    function updateLastFM(data) {
        const section = document.querySelector('.lastfm-section');
        if (!section) return;

        const trackName = section.querySelector('.lastfm-track, .track-name, .track-title');
        const trackArtist = section.querySelector('.lastfm-artist, .track-artist');
        const trackAlbum = section.querySelector('.lastfm-album, .track-album');
        const statusText = section.querySelector('.status-text');
        const coverImg = section.querySelector('.lastfm-cover img, .track-cover img, .track-cover-large img');

        if (trackName) trackName.textContent = data.name;
        if (trackArtist) trackArtist.textContent = `by ${data.artist}`;
        if (trackAlbum && data.album) trackAlbum.textContent = `from ${data.album}`;

        if (statusText) {
            statusText.textContent = data.isPlaying ? 'Now Playing' : 'Last played';
        }

        if (coverImg && data.image) {
            coverImg.src = data.image;
        }

        const statusIndicator = section.querySelector('.status-indicator');
        if (statusIndicator) {
            statusIndicator.className = 'status-indicator';
            if (data.isPlaying) {
                statusIndicator.classList.add('status-online');
            } else {
                statusIndicator.classList.add('status-offline');
            }
        }

        if (data.recentTracks && data.recentTracks.length > 0) {
            const recentContainer = section.querySelector('.recent-tracks-list');
            if (recentContainer) {
                recentContainer.innerHTML = '';
                data.recentTracks.forEach(track => {
                    const trackElement = document.createElement('div');
                    trackElement.className = 'recent-track-item';
                    trackElement.innerHTML = `
                        ${track.image ? `<div class="recent-track-cover"><img src="${track.image}" alt="${track.name}" loading="lazy"></div>` : ''}
                        <div class="recent-track-info">
                            <div class="recent-track-name">${track.name}</div>
                            <div class="recent-track-artist">${track.artist}</div>
                            <div class="recent-track-time">${track.relativeTime}</div>
                        </div>
                    `;
                    recentContainer.appendChild(trackElement);
                });
            }
        }

        animateUpdate(section);
    }

    function updateLastFMRealtime(data) {
        updateLastFM(data);
    }

    function updateBeatLeader(data) {
        const section = document.querySelector('.beatleader-section');
        if (!section) return;

        const statValues = section.querySelectorAll('.stat-value');
        if (statValues.length >= 4) {
            statValues[0].textContent = '#' + data.rank;
            statValues[1].textContent = '#' + data.countryRank;
            statValues[2].textContent = Math.round(data.pp) + 'pp';
            statValues[3].textContent = data.accuracy.toFixed(1) + '%';
        }

        animateUpdate(section);
    }

    function updateSteam(data) {
        const section = document.querySelector('.steam-section');
        if (!section) return;

        console.debug('Steam games updated:', data.games);
        animateUpdate(section);
    }

    function updateWebring(data) {
        const section = document.querySelector('.webring-section');
        if (!section) return;

        const prevLink = section.querySelector('.webring-prev');
        const nextLink = section.querySelector('.webring-next');

        let updated = false;

        if (prevLink && data.prev) {
            const currentHref = prevLink.href;
            if (currentHref !== data.prev.url) {
                prevLink.href = data.prev.url;
                updated = true;
            }

            const prevImg = prevLink.querySelector('img');
            const prevText = prevLink.querySelector('.webring-text');

            if (prevImg && data.prev.favicon && prevImg.src !== data.prev.favicon) {
                prevImg.src = data.prev.favicon;
                updated = true;
            }

            if (prevText) {
                const newText = `← ${data.prev.name}`;
                if (prevText.textContent !== newText) {
                    prevText.textContent = newText;
                    updated = true;
                }
            }
        }

        if (nextLink && data.next) {
            const currentHref = nextLink.href;
            if (currentHref !== data.next.url) {
                nextLink.href = data.next.url;
                updated = true;
            }

            const nextImg = nextLink.querySelector('img');
            const nextText = nextLink.querySelector('.webring-text');

            if (nextImg && data.next.favicon && nextImg.src !== data.next.favicon) {
                nextImg.src = data.next.favicon;
                updated = true;
            }

            if (nextText) {
                const newText = `${data.next.name} →`;
                if (nextText.textContent !== newText) {
                    nextText.textContent = newText;
                    updated = true;
                }
            }
        }

        if (updated) {
            animateUpdate(section);
            console.debug('Webring updated via WebSocket');
        }
    }

    function updateVisitors(data) {
        const visitorsInfo = document.querySelector('.visitors-info');
        if (visitorsInfo && data.total && data.today) {
            const newText = `Total visits: ${formatNumber(data.total)} • Today: ${formatNumber(data.today)}`;
            visitorsInfo.textContent = newText;
        }
    }

    function handlePluginUpdate(data) {
        if (data.action === 'settings_changed') {
            console.debug('Plugin settings updated:', data.plugin);

            if (window.location.pathname !== '/admin') {
                setTimeout(() => {
                    window.location.reload();
                }, 2000);
            }
        }
    }

    function animateUpdate(element) {
        if (!element) return;

        element.style.transform = 'scale(1.01)';
        element.style.transition = 'transform 0.15s ease';

        setTimeout(() => {
            element.style.transform = '';
        }, 150);
    }

    function formatNumber(n) {
        if (n < 1000) {
            return n.toString();
        } else if (n < 1000000) {
            return (n / 1000).toFixed(1) + 'K';
        } else {
            return (n / 1000000).toFixed(1) + 'M';
        }
    }

    function disconnect() {
        shouldReconnect = false;
        stopHeartbeat();

        if (reconnectTimeout) {
            clearTimeout(reconnectTimeout);
            reconnectTimeout = null;
        }

        if (ws) {
            ws.close(1000, 'Client disconnect');
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', connect);
    } else {
        connect();
    }

    window.addEventListener('beforeunload', disconnect);
    window.addEventListener('pagehide', disconnect);

    document.addEventListener('visibilitychange', function() {
        if (document.hidden) {
            stopHeartbeat();
        } else {
            if (isConnected) {
                startHeartbeat();
            } else if (shouldReconnect) {
                connect();
            }
        }
    });

    window.wsReconnect = connect;
    window.wsDisconnect = disconnect;
    window.wsStatus = () => isConnected;
    window.wsSend = sendMessage;

    // Client-side webring updater (independent of WebSocket)
    function initWebringUpdater() {
        const webringSection = document.querySelector('.webring-section');
        if (!webringSection) return;

        const baseUrl = webringSection.dataset.baseUrl;
        const username = webringSection.dataset.username;

        if (!baseUrl || !username) return;

        let isUpdating = false;
        let lastUpdateTime = 0;

        function updateWebringFromAPI() {
            const now = Date.now();

            // Prevent multiple simultaneous updates
            if (isUpdating || (now - lastUpdateTime) < 5000) return;

            isUpdating = true;

            fetch(`${baseUrl}/${username}/data`)
                .then(response => {
                    if (!response.ok) throw new Error('Network response was not ok');
                    return response.json();
                })
                .then(data => {
                    // Only update if we have valid data
                    if (!data.prev || !data.next) return;

                    const prevLink = webringSection.querySelector('.webring-prev');
                    const nextLink = webringSection.querySelector('.webring-next');

                    let updated = false;

                    if (prevLink && data.prev) {
                        // Only update if the content actually changed
                        const currentHref = prevLink.href;
                        const newHref = data.prev.url;

                        if (currentHref !== newHref) {
                            prevLink.href = newHref;
                            updated = true;
                        }

                        const prevImg = prevLink.querySelector('img');
                        const prevText = prevLink.querySelector('.webring-text');

                        if (prevImg && data.prev.favicon) {
                            const newSrc = `${baseUrl}/media/${data.prev.favicon}`;
                            if (prevImg.src !== newSrc) {
                                prevImg.src = newSrc;
                                updated = true;
                            }
                        }

                        if (prevText) {
                            const newText = `← ${data.prev.name}`;
                            if (prevText.textContent !== newText) {
                                prevText.textContent = newText;
                                updated = true;
                            }
                        }
                    }

                    if (nextLink && data.next) {
                        // Only update if the content actually changed
                        const currentHref = nextLink.href;
                        const newHref = data.next.url;

                        if (currentHref !== newHref) {
                            nextLink.href = newHref;
                            updated = true;
                        }

                        const nextImg = nextLink.querySelector('img');
                        const nextText = nextLink.querySelector('.webring-text');

                        if (nextImg && data.next.favicon) {
                            const newSrc = `${baseUrl}/media/${data.next.favicon}`;
                            if (nextImg.src !== newSrc) {
                                nextImg.src = newSrc;
                                updated = true;
                            }
                        }

                        if (nextText) {
                            const newText = `${data.next.name} →`;
                            if (nextText.textContent !== newText) {
                                nextText.textContent = newText;
                                updated = true;
                            }
                        }
                    }

                    if (updated) {
                        console.debug('Webring updated from API');
                        // Add a subtle visual feedback
                        webringSection.style.opacity = '0.8';
                        setTimeout(() => {
                            webringSection.style.opacity = '';
                        }, 200);
                    }

                    lastUpdateTime = now;
                })
                .catch(error => {
                    console.debug('Webring API update failed:', error);
                })
                .finally(() => {
                    isUpdating = false;
                });
        }

        // Update after a delay to avoid conflicts with initial render
        setTimeout(updateWebringFromAPI, 2000);

        // Update every hour
        setInterval(updateWebringFromAPI, 60 * 60 * 1000);

        // Also update when the page becomes visible again
        document.addEventListener('visibilitychange', () => {
            if (!document.hidden) {
                setTimeout(updateWebringFromAPI, 1000);
            }
        });
    }

    // Initialize webring updater when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initWebringUpdater);
    } else {
        initWebringUpdater();
    }

})();