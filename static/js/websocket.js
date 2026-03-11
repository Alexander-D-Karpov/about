(function() {
    'use strict';

    let ws = null;
    let reconnectAttempts = 0;
    const maxReconnectAttempts = 10;
    const baseReconnectDelay = 1000;
    let isConnected = false;
    let shouldReconnect = true;
    let reconnectTimeout = null;
    let connectionRetryCount = 0;
    let clientCountRequestTimeout = null;
    let imageLoadQueue = new Map();
    let wsInitialized = false;

    function connect() {
        if (wsInitialized && ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) {
            return;
        }

        if (reconnectAttempts >= maxReconnectAttempts) {
            console.log('Max reconnection attempts reached');
            updateConnectionStatus('failed');
            return;
        }

        wsInitialized = true;
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
                startLastFMCheck();

                if (reconnectTimeout) {
                    clearTimeout(reconnectTimeout);
                    reconnectTimeout = null;
                }

                sendMessage({ type: 'register', data: { page: window.location.pathname } });

                if (clientCountRequestTimeout) {
                    clearTimeout(clientCountRequestTimeout);
                }
                clientCountRequestTimeout = setTimeout(() => {
                    sendMessage({ type: 'get_client_count' });
                    clientCountRequestTimeout = null;
                }, 100);

                setTimeout(() => {
                    sendMessage({type: 'check_lastfm'});
                }, 500);
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
                stopLastFMCheck();
                if (clientCountRequestTimeout) {
                    clearTimeout(clientCountRequestTimeout);
                    clientCountRequestTimeout = null;
                }

                if (isGoodbye) {
                    return;
                }

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

    function updateHealthDisplay(data) {
        const section = document.querySelector('.health-section');
        if (!section) return;

        const updates = {
            'steps-today': data.steps_today?.toLocaleString(),
            'calories-today': Math.round(data.calories_today),
            'workout-minutes': data.workout_minutes,
            'sleep-hours': data.sleep_last_night?.toFixed(1) + 'h',
            'heart-rate': data.avg_heart_rate + ' bpm',
            'hydration': Math.round(data.hydration_today)
        };

        Object.entries(updates).forEach(([metric, value]) => {
            const el = section.querySelector(`[data-metric="${metric}"]`);
            if (el && value) {
                el.textContent = value;
                el.classList.add('updated');
                setTimeout(() => el.classList.remove('updated'), 500);
            }
        });
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

    let heartbeatInterval = null;
    let heartbeatTimeout = null;
    let missedHeartbeats = 0;
    const MAX_MISSED_HEARTBEATS = 3;

    function startHeartbeat() {
        stopHeartbeat();
        missedHeartbeats = 0;

        heartbeatInterval = setInterval(() => {
            if (ws && ws.readyState === WebSocket.OPEN) {
                try {
                    const pingMessage = JSON.stringify({ type: 'ping' });
                    ws.send(pingMessage);

                    if (heartbeatTimeout) clearTimeout(heartbeatTimeout);

                    heartbeatTimeout = setTimeout(() => {
                        missedHeartbeats++;
                        console.warn(`Missed heartbeat ${missedHeartbeats}/${MAX_MISSED_HEARTBEATS}`);

                        if (missedHeartbeats >= MAX_MISSED_HEARTBEATS) {
                            console.error('Too many missed heartbeats, reconnecting');
                            stopHeartbeat();
                            if (ws) ws.close();
                        }
                    }, 10000);
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
        if (heartbeatTimeout) {
            clearTimeout(heartbeatTimeout);
            heartbeatTimeout = null;
        }
        missedHeartbeats = 0;
    }

    function onHeartbeatResponse() {
        missedHeartbeats = 0;
        if (heartbeatTimeout) {
            clearTimeout(heartbeatTimeout);
            heartbeatTimeout = null;
        }
    }

    let lastFMCheckInterval = null;
    let lastFMCheckTimeout = null;
    let lastFMLastResponse = 0;

    function startLastFMCheck() {
        stopLastFMCheck();

        lastFMCheckInterval = setInterval(() => {
            const now = Date.now();

            if (lastFMLastResponse > 0 && (now - lastFMLastResponse) > 120000) {
                console.warn('LastFM updates stale, reconnecting WebSocket');
                if (shouldReconnect) {
                    ws?.close();
                    setTimeout(connect, 1000);
                }
                return;
            }

            if (ws && ws.readyState === WebSocket.OPEN && getClientCount() > 0) {
                try {
                    sendMessage({ type: 'check_lastfm' });

                    if (lastFMCheckTimeout) clearTimeout(lastFMCheckTimeout);

                    lastFMCheckTimeout = setTimeout(() => {
                        console.warn('LastFM check timeout, no response in 30s');
                    }, 30000);
                } catch (e) {
                    console.error('Failed to send LastFM check:', e);
                }
            }
        }, 30000);
    }

    function stopLastFMCheck() {
        if (lastFMCheckInterval) {
            clearInterval(lastFMCheckInterval);
            lastFMCheckInterval = null;
        }
        if (lastFMCheckTimeout) {
            clearTimeout(lastFMCheckTimeout);
            lastFMCheckTimeout = null;
        }
    }

    function onLastFMResponse() {
        lastFMLastResponse = Date.now();
        if (lastFMCheckTimeout) {
            clearTimeout(lastFMCheckTimeout);
            lastFMCheckTimeout = null;
        }
    }

    function getClientCount() {
        const clientsEl = document.getElementById('connected-clients');
        if (clientsEl) {
            return parseInt(clientsEl.textContent) || 0;
        }
        return 0;
    }


    function handleMessage(message) {
        try {
            switch (message.type) {
                case 'pong':
                    onHeartbeatResponse();
                    break;
                case 'client_count_update':
                    updateClientCount(message.data);
                    break;
                case 'lastfm_update':
                    onLastFMResponse();
                    updateLastFM(message.data);
                    break;
                case 'lastfm_realtime':
                    onLastFMResponse();
                    updateLastFM(message.data);
                    break;
                case 'beatleader_update':
                    updateBeatLeader(message.data);
                    break;
                case 'steam_update':
                case 'steam_games_update':
                    updateSteam(message.data);
                    break;
                case 'steam_status_update':
                    updateSteamStatus(message.data);
                    break;
                case 'visitors_update':
                    updateVisitors(message.data);
                    break;
                case 'webring_update':
                    updateWebring(message.data);
                    break;
                case 'services_summary_update':
                    updateServicesStatus(message.data);
                    break;
                case 'service_status_update':
                    updateSingleServiceStatus(message.data);
                    break;
                case 'music_play':
                    if (window.musicPlayer) {
                        window.musicPlayer.handleMusicUpdate(message);
                    }
                    break;
                case 'plugin_update':
                    handlePluginUpdate(message.data);
                    break;
                case 'plugin_rendered':
                    handlePluginRendered(message.data);
                    break;
                case 'health_update':
                    updateHealthDisplay(message.data);
                    break;
                case 'plugins_updated':
                    setTimeout(() => {
                        if (window.location.pathname !== '/admin') {
                            window.location.reload();
                        }
                    }, 1000);
                    break;
                case 'meme_update':
                    updateMemeGlobal(message.data);
                    break;
                case 'system_update':
                    updateSystemInfo(message.data);
                    break;
                case 'heartbeat':
                    onHeartbeatResponse();
                    break;
                default:
                    break;
            }
        } catch (e) {
            console.error('Error handling message:', message, e);
        }
    }

    function handlePluginRendered(data) {
        if (!data || !data.plugin || !data.rendered) return;

        if (data.plugin === 'meme') {
            return;
        }

        if (data.plugin === 'lastfm') {
            updateLastFMFromRendered(data.rendered);
            return;
        }

        let section = document.querySelector(`.${data.plugin}-section`);
        if (!section) {
            section = document.querySelector(`[data-plugin="${data.plugin}"]`);
        }

        if (section) {
            const prevHeight = section.offsetHeight;
            section.style.minHeight = prevHeight + 'px';

            section.outerHTML = data.rendered;

            const newSection = document.querySelector(`.${data.plugin}-section`) ||
                document.querySelector(`[data-plugin="${data.plugin}"]`);

            if (newSection) {
                if (typeof window.initTechFiltering === 'function' && data.plugin === 'techstack') {
                    window.initTechFiltering();
                } else if (typeof window.initCodeToggles === 'function' && data.plugin === 'code') {
                    window.initCodeToggles();
                }

                requestAnimationFrame(() => {
                    newSection.style.minHeight = '';
                    animateUpdate(newSection);

                    if (window.mosaicUtils && window.mosaicUtils.observe) {
                        window.mosaicUtils.observe(newSection);
                    }

                    if (window.mosaicUtils) {
                        window.mosaicUtils.resizeAll();
                    }
                });
            }
        }
    }

    function updateLastFMFromRendered(renderedHTML) {
        const section = document.querySelector('.lastfm-section');
        if (!section) return;

        const temp = document.createElement('div');
        temp.innerHTML = renderedHTML;
        const newSection = temp.querySelector('.lastfm-section');
        if (!newSection) return;

        const trackTitle = newSection.querySelector('.track-title, .track-name');
        const trackArtist = newSection.querySelector('.track-artist');
        const trackAlbum = newSection.querySelector('.track-album');
        const coverImg = newSection.querySelector('.track-cover-large img');
        const statusText = newSection.querySelector('.status-text');
        const scrobblesText = newSection.querySelector('.scrobbles-text');

        const currentTrackTitle = section.querySelector('.track-title, .track-name');
        const currentTrackArtist = section.querySelector('.track-artist');
        const currentTrackAlbum = section.querySelector('.track-album');
        const currentCoverImg = section.querySelector('.track-cover-large img');
        const currentStatusText = section.querySelector('.status-text');
        const currentScrobblesText = section.querySelector('.scrobbles-text');

        if (currentTrackTitle && trackTitle) {
            currentTrackTitle.textContent = trackTitle.textContent;
        }
        if (currentTrackArtist && trackArtist) {
            currentTrackArtist.textContent = trackArtist.textContent;
        }
        if (currentTrackAlbum && trackAlbum) {
            currentTrackAlbum.textContent = trackAlbum.textContent;
        }
        if (currentCoverImg && coverImg && coverImg.src) {
            loadImageSmoothly(currentCoverImg, coverImg.src);
        }
        if (currentStatusText && statusText) {
            currentStatusText.textContent = statusText.textContent;
            const statusContainer = currentStatusText.closest('.track-status');
            const newStatusContainer = statusText.closest('.track-status');
            if (statusContainer && newStatusContainer) {
                if (newStatusContainer.classList.contains('now-playing')) {
                    statusContainer.classList.add('now-playing');
                } else {
                    statusContainer.classList.remove('now-playing');
                }
            }
        }
        if (currentScrobblesText && scrobblesText) {
            currentScrobblesText.textContent = scrobblesText.textContent;
        }

        const newRecentList = newSection.querySelector('.recent-tracks-list');
        const currentRecentList = section.querySelector('.recent-tracks-list');
        if (newRecentList && currentRecentList) {
            const newItems = newRecentList.querySelectorAll('.recent-track-item');
            const currentItems = currentRecentList.querySelectorAll('.recent-track-item');

            let needsUpdate = newItems.length !== currentItems.length;
            if (!needsUpdate) {
                for (let i = 0; i < newItems.length; i++) {
                    const newName = newItems[i].querySelector('.recent-track-name')?.textContent || '';
                    const currentName = currentItems[i].querySelector('.recent-track-name')?.textContent || '';
                    if (newName !== currentName) {
                        needsUpdate = true;
                        break;
                    }
                }
            }

            if (needsUpdate) {
                currentRecentList.innerHTML = newRecentList.innerHTML;
                setupRecentTracksHandlers(section);
            }
        }
    }

    function initMemeAfterRender(section) {
        const btn = section.querySelector('.meme-refresh-btn');
        if (btn && !btn.dataset.listenerAttached) {
            btn.dataset.listenerAttached = '1';
            btn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                if (typeof window.refreshMeme === 'function') {
                    window.refreshMeme();
                }
            });
        }
    }

    let lastMemeId = null;
    let memeUpdateTimeout = null;
    let memeUpdatePending = false;

    function updateMemeGlobal(data) {
        const section = document.querySelector('.meme-section');
        if (!section || !data.meme) return;

        const meme = data.meme;
        const memeId = meme.image || meme.text || '';

        if (memeId === lastMemeId) {
            return;
        }

        if (memeUpdatePending) {
            return;
        }

        memeUpdatePending = true;

        if (memeUpdateTimeout) {
            clearTimeout(memeUpdateTimeout);
        }

        memeUpdateTimeout = setTimeout(() => {
            performMemeUpdate(section, meme, memeId);
            memeUpdatePending = false;
            memeUpdateTimeout = null;
        }, 100);
    }

    function performMemeUpdate(section, meme, memeId) {
        const memeContent = section.querySelector('.meme-content');
        if (!memeContent) return;

        const currentImg = memeContent.querySelector('img');
        if (currentImg && currentImg.src === meme.image) {
            lastMemeId = memeId;
            return;
        }

        const currentHeight = memeContent.offsetHeight;
        memeContent.style.minHeight = Math.max(currentHeight, 200) + 'px';

        const doSwap = (imgWidth, imgHeight) => {
            lastMemeId = memeId;

            let newContent = '';
            if (meme.type === 'image' || meme.type === 'gif') {
                newContent = `
            <div class="meme-${meme.type}">
                <img src="${meme.image}" alt="${meme.text || ''}" loading="eager">
            </div>
            ${meme.text ? `<p class="meme-caption">${meme.text}</p>` : ''}`;
            } else {
                newContent = `
            <div class="meme-text">
                <p class="meme-quote">${meme.text}</p>
                ${meme.source ? `<p class="meme-source">— ${meme.source}</p>` : ''}
            </div>`;
            }

            memeContent.innerHTML = newContent;

            const newImg = memeContent.querySelector('img');
            if (newImg) {
                newImg.onload = () => {
                    memeContent.style.minHeight = '';
                    if (window.mosaicUtils) window.mosaicUtils.resizeAll();
                };
                newImg.onerror = () => {
                    memeContent.style.minHeight = '';
                    if (window.mosaicUtils) window.mosaicUtils.resizeAll();
                };
            } else {
                requestAnimationFrame(() => {
                    memeContent.style.minHeight = '';
                    if (window.mosaicUtils) window.mosaicUtils.resizeAll();
                });
            }
        };

        if ((meme.type === 'image' || meme.type === 'gif') && meme.image) {
            const img = new Image();
            img.onload = () => doSwap(img.naturalWidth, img.naturalHeight);
            img.onerror = () => doSwap(0, 0);
            img.src = meme.image;
        } else {
            doSwap(0, 0);
        }
    }

    function updateClientCount(data) {
        const clientsEl = document.getElementById('connected-clients');
        if (clientsEl && data && typeof data.count === 'number') {
            const oldCount = parseInt(clientsEl.textContent) || 0;
            const newCount = data.count;

            if (oldCount !== newCount) {
                animateNumber(clientsEl, oldCount, newCount);
            }
        }

        const allClientEls = document.querySelectorAll('[data-client-count], .client-count');
        allClientEls.forEach(el => {
            if (data && typeof data.count === 'number') {
                const oldCount = parseInt(el.textContent) || 0;
                if (oldCount !== data.count) {
                    animateNumber(el, oldCount, data.count);
                }
            }
        });
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
                case 'goodbye':
                    statusIndicator.classList.add('status-offline');
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
                case 'goodbye':
                    statusText.textContent = 'Goodbye!';
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

    function preloadImage(src, onLoad, onError) {
        if (!src || imageLoadQueue.has(src)) return;

        const img = new Image();
        imageLoadQueue.set(src, img);

        img.onload = () => {
            imageLoadQueue.delete(src);
            if (onLoad) onLoad(img);
        };

        img.onerror = () => {
            imageLoadQueue.delete(src);
            console.warn('Failed to preload image:', src);
            if (onError) onError();
        };

        img.src = src;
    }

    function updateLastFM(data) {
        const section = document.querySelector('.lastfm-section');
        if (!section) return;

        const currentTrackEl = section.querySelector('.current-track');
        if (currentTrackEl) {
            currentTrackEl.dataset.artist = data.artist || '';
            currentTrackEl.dataset.track = data.name || '';
        }

        const trackName = section.querySelector('.track-title, .track-name, #lastfm-track-title');
        const trackArtist = section.querySelector('.track-artist, #lastfm-track-artist');
        const trackAlbum = section.querySelector('.track-album, #lastfm-track-album');
        const statusTextEl = section.querySelector('.status-text');
        const coverImg = section.querySelector('.track-cover-large img, .track-cover img, #lastfm-artwork');
        const lastfmLink = section.querySelector('#lastfm-link');

        if (trackName) trackName.textContent = data.name || 'Unknown Track';
        if (trackArtist) trackArtist.textContent = data.artist ? `by ${data.artist}` : 'Unknown Artist';

        if (trackAlbum) {
            if (data.album) {
                trackAlbum.textContent = `from ${data.album}`;
                trackAlbum.style.display = '';
            } else {
                trackAlbum.style.display = 'none';
            }
        }

        if (lastfmLink && data.url) lastfmLink.href = data.url;

        const isNowPlaying = data.isPlaying === true || data.isPlaying === 'true';

        if (statusTextEl) {
            // Prefer backend-provided statusText (it includes relative time when not playing)
            const finalStatus = data.statusText || (isNowPlaying ? 'Now Playing' : 'Last played');
            statusTextEl.textContent = finalStatus;

            const statusContainer = statusTextEl.closest('.track-status');
            if (statusContainer) {
                if (isNowPlaying) statusContainer.classList.add('now-playing');
                else statusContainer.classList.remove('now-playing');
            }

            const statusIndicator = section.querySelector('.status-indicator');
            if (statusIndicator) {
                statusIndicator.className = 'status-indicator';
                statusIndicator.classList.add(isNowPlaying ? 'status-online' : 'status-offline');
            }
        }

        if (coverImg && data.image) {
            loadImageSmoothly(coverImg, data.image);
        } else if (coverImg && !data.image) {
            coverImg.style.opacity = '0.3';
            coverImg.src = '/static/images/default-album.png';
            setTimeout(() => { coverImg.style.opacity = '1'; }, 150);
        }

        if (data.recentTracks && Array.isArray(data.recentTracks)) {
            updateRecentTracks(section, data.recentTracks);
        }

        // Ensure click handlers survive any DOM updates
        setupRecentTracksHandlers(section);
    }

    function updateRecentTracks(section, recentTracks) {
        const recentContainer = section.querySelector('.recent-tracks-list');
        if (!recentContainer) return;

        const existingTracks = Array.from(recentContainer.querySelectorAll('.recent-track-item')).map(item => {
            const nameEl = item.querySelector('.recent-track-name');
            const artistEl = item.querySelector('.recent-track-artist');
            return {
                name: nameEl ? nameEl.textContent.replace(' 🎵', '').trim() : '',
                artist: artistEl ? artistEl.textContent.trim() : ''
            };
        });

        const newTracks = recentTracks.slice(0, 5).map(track => ({
            name: (track.name || 'Unknown Track'),
            artist: (track.artist || 'Unknown Artist')
        }));

        const hasChanges = JSON.stringify(existingTracks) !== JSON.stringify(newTracks);
        if (!hasChanges) return;

        recentContainer.innerHTML = '';

        const tracksToShow = recentTracks.slice(0, 5);

        tracksToShow.forEach((track) => {
            const trackElement = document.createElement('div');
            trackElement.className = 'recent-track-item';

            const isCurrentlyPlaying = track.isPlaying === true || track.isPlaying === 'true';
            if (isCurrentlyPlaying) trackElement.classList.add('now-playing');

            const trackName = track.name || 'Unknown Track';
            const trackArtist = track.artist || 'Unknown Artist';
            const trackImage = track.image || '';
            const relativeTime = track.relativeTime || '';

            trackElement.innerHTML = `
            ${trackImage ? `<div class="recent-track-cover"><img src="${trackImage}" alt="${trackName}" loading="lazy"></div>` : ''}
            <div class="recent-track-info">
                <div class="recent-track-name">${trackName}${isCurrentlyPlaying ? ' 🎵' : ''}</div>
                <div class="recent-track-artist">${trackArtist}</div>
            </div>
            <div class="recent-track-time">${relativeTime}</div>
        `;

            recentContainer.appendChild(trackElement);
        });

        setupRecentTracksHandlers(section);
    }

    function loadImageSmoothly(imgElement, newSrc) {
        if (!imgElement || !newSrc) return;

        if (imgElement.src === newSrc) return;

        const fadeOut = () => {
            imgElement.style.transition = 'opacity 0.2s ease, filter 0.2s ease';
            imgElement.style.opacity = '0.4';
            imgElement.style.filter = 'blur(2px)';
        };

        const fadeIn = () => {
            imgElement.style.opacity = '1';
            imgElement.style.filter = 'blur(0px)';
            setTimeout(() => {
                imgElement.style.transition = '';
            }, 200);
        };

        preloadImage(newSrc,
            (loadedImg) => {
                imgElement.src = newSrc;
                fadeIn();
            },
            () => {
                imgElement.src = '/static/images/default-album.png';
                fadeIn();
            }
        );

        fadeOut();
    }

    function updateBeatLeader(data) {
        const section = document.querySelector('.beatleader-section');
        if (!section) return;

        const statItems = section.querySelectorAll('.stat-item');
        if (statItems.length >= 4) {
            const rankStat = statItems[0].querySelector('.stat-value');
            const countryRankStat = statItems[1].querySelector('.stat-value');
            const ppStat = statItems[2].querySelector('.stat-value');
            const cubesStat = statItems[3].querySelector('.stat-value');

            if (rankStat) {
                rankStat.textContent = '#' + data.rank;
                rankStat.dataset.rawValue = String(data.rank);
            }
            if (countryRankStat) {
                countryRankStat.textContent = '#' + data.countryRank;
                countryRankStat.dataset.rawValue = String(data.countryRank);
            }
            if (ppStat) {
                ppStat.textContent = Math.round(data.pp) + 'pp';
                ppStat.dataset.rawValue = String(Math.round(data.pp));
            }
            if (cubesStat && data.cubes) {
                cubesStat.textContent = data.formatted || formatNumberWithDecimals(data.cubes);
                cubesStat.dataset.cubes = String(data.cubes);
                const parent = cubesStat.parentElement;
                if (parent && data.cubes) {
                    parent.setAttribute('data-tooltip', 'Exact: ' + data.cubes.toLocaleString());
                }
            }
        }

        animateUpdate(section);
    }

    function formatNumberWithDecimals(n) {
        if (!n) return "0";
        if (n < 1000) return n.toString();
        if (n < 1000000) return (n / 1000).toFixed(1) + "K";
        return (n / 1000000).toFixed(2) + "M";
    }

    function updateSteam(data) {
        const section = document.querySelector('.steam-section');
        if (!section) return;

        console.debug('Steam games updated:', data.games);
        animateUpdate(section);
    }

    function updateSteamStatus(data) {
        const section = document.querySelector('.steam-section');
        if (!section) return;

        const currentGameDiv = section.querySelector('.current-game');
        const playerStatusDiv = section.querySelector('.player-status');

        if (data.isPlaying && data.currentGame) {
            if (!currentGameDiv) {
                const headerElement = section.querySelector('.plugin-header');
                if (headerElement) {
                    const gameHTML = `
                    <div class="current-game">
                        <div class="current-game-header">
                            <span class="status-indicator status-online"></span>
                            <span class="current-game-status">Currently Playing</span>
                        </div>
                        ${data.gameImage ? `
                        <div class="current-game-cover">
                            <img src="${data.gameImage}" alt="${data.currentGame}" loading="lazy" class="game-cover-image">
                        </div>
                        ` : ''}
                        <div class="current-game-info">
                            <div class="current-game-name">${data.currentGame}</div>
                            <div class="current-game-actions">
                                <a href="https://store.steampowered.com/${data.gameId ? 'app/' + data.gameId : 'search/?term=' + encodeURIComponent(data.currentGame)}" target="_blank" rel="noopener" class="btn btn-sm">
                                    <svg viewBox="0 0 24 24" width="14" height="14">
                                        <path fill="currentColor" d="M14 3v2h3.59l-9.83 9.83 1.41 1.41L19 6.41V10h2V3m-2 16H5V5h7V3H5c-1.11 0-2 .89-2 2v14c0 1.11.89 2 2 2h14c1.11 0 2-.89 2-2v-7h-2v7Z"/>
                                    </svg>
                                    View on Steam
                                </a>
                            </div>
                        </div>
                    </div>
                `;
                    headerElement.insertAdjacentHTML('afterend', gameHTML);
                }
            } else {
                const gameName = currentGameDiv.querySelector('.current-game-name');
                const gameLink = currentGameDiv.querySelector('.current-game-actions a');
                const gameCover = currentGameDiv.querySelector('.current-game-cover');

                if (gameName) gameName.textContent = data.currentGame;
                if (gameLink) {
                    gameLink.href = data.gameId
                        ? `https://store.steampowered.com/app/${data.gameId}`
                        : `https://store.steampowered.com/search/?term=${encodeURIComponent(data.currentGame)}`;
                }

                if (data.gameImage && !gameCover) {
                    const infoDiv = currentGameDiv.querySelector('.current-game-info');
                    if (infoDiv) {
                        const coverHTML = `
                        <div class="current-game-cover">
                            <img src="${data.gameImage}" alt="${data.currentGame}" loading="lazy" class="game-cover-image">
                        </div>
                    `;
                        infoDiv.insertAdjacentHTML('beforebegin', coverHTML);
                    }
                } else if (data.gameImage && gameCover) {
                    const img = gameCover.querySelector('img');
                    if (img) loadImageSmoothly(img, data.gameImage);
                }
            }

            if (playerStatusDiv) playerStatusDiv.style.display = 'none';
        } else {
            if (currentGameDiv) currentGameDiv.remove();

            if (!playerStatusDiv) {
                const headerElement = section.querySelector('.plugin-header');
                if (headerElement) {
                    const statusHTML = `
                    <div class="player-status">
                        <div class="status-info">
                            <span class="status-indicator status-${getPersonaStateClass(data.personaState)}"></span>
                            <span class="status-text">${getPersonaStateText(data.personaState)}</span>
                        </div>
                    </div>
                `;
                    headerElement.insertAdjacentHTML('afterend', statusHTML);
                }
            } else {
                const statusIndicator = playerStatusDiv.querySelector('.status-indicator');
                const statusText = playerStatusDiv.querySelector('.status-text');
                if (statusIndicator) {
                    statusIndicator.className = `status-indicator status-${getPersonaStateClass(data.personaState)}`;
                }
                if (statusText) {
                    statusText.textContent = getPersonaStateText(data.personaState);
                }
                playerStatusDiv.style.display = 'block';
            }
        }

        animateUpdate(section);
    }

    function getPersonaStateClass(state) {
        switch (state) {
            case 1: return 'online';
            case 2: case 3: case 4: case 5: case 6: return 'loading';
            default: return 'offline';
        }
    }

    function getPersonaStateText(state) {
        switch (state) {
            case 0: return 'Offline';
            case 1: return 'Online';
            case 2: return 'Busy';
            case 3: return 'Away';
            case 4: return 'Snooze';
            case 5: return 'Looking to trade';
            case 6: return 'Looking to play';
            default: return 'Unknown';
        }
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
        let updated = false;

        if (data.total !== undefined) {
            const totalElements = document.querySelectorAll('.total-visits, [data-stat="total"]');
            totalElements.forEach(el => {
                const newValue = data.total;
                el.dataset.rawValue = String(newValue);
                el.textContent = formatNumber(newValue);

                const container = el.closest('.visitor-stat');
                if (container) {
                    container.setAttribute('data-tooltip', `Exact: ${newValue.toLocaleString()}`);
                }
                updated = true;
            });
        }

        if (data.today !== undefined) {
            const todayElements = document.querySelectorAll('.today-visits, [data-stat="today"]');
            todayElements.forEach(el => {
                const newValue = data.today;
                el.dataset.rawValue = String(newValue);
                el.textContent = formatNumber(newValue);

                const container = el.closest('.visitor-stat');
                if (container) {
                    container.setAttribute('data-tooltip', `Exact: ${newValue.toLocaleString()}`);
                }
                updated = true;
            });
        }

        if (updated) {
            const visitorsSection = document.querySelector('.visitors-section');
            if (visitorsSection) {
                animateUpdate(visitorsSection);
            }
        }
    }

    function updateServicesStatus(data) {
        const section = document.querySelector('.services-section');
        if (!section) return;

        const onlineCount = section.querySelector('.online-count');
        const offlineCount = section.querySelector('.offline-count');
        const totalCount = section.querySelector('.total-count');

        if (onlineCount) onlineCount.textContent = data.online_count;
        if (offlineCount) offlineCount.textContent = data.offline_count;
        if (totalCount) totalCount.textContent = data.total_count;

        if (data.services) {
            Object.entries(data.services).forEach(([serviceName, serviceData]) => {
                const serviceItems = section.querySelectorAll('.service-item');
                serviceItems.forEach(item => {
                    const itemName = item.querySelector('.service-link, .service-title')?.textContent;
                    if (itemName === serviceName) {
                        item.setAttribute('data-status', serviceData.status);

                        const statusIndicator = item.querySelector('.status-indicator');
                        if (statusIndicator) {
                            statusIndicator.className = `status-indicator status-${serviceData.status}`;
                        }

                        const responseTime = item.querySelector('.service-response-time');
                        if (responseTime && serviceData.response_time) {
                            responseTime.textContent = `${serviceData.response_time}ms`;
                        }

                        const statusCode = item.querySelector('.service-status-code');
                        if (statusCode && serviceData.status_code) {
                            statusCode.textContent = serviceData.status_code;
                            statusCode.setAttribute('data-code', serviceData.status_code);
                        }
                    }
                });
            });
        }

        animateUpdate(section);
    }

    function updateSingleServiceStatus(data) {
        const section = document.querySelector('.services-section');
        if (!section) return;

        const serviceItems = section.querySelectorAll('.service-item');
        serviceItems.forEach(item => {
            const itemName = item.querySelector('.service-link, .service-title')?.textContent;
            if (itemName === data.name) {
                item.setAttribute('data-status', data.status);

                const statusIndicator = item.querySelector('.status-indicator');
                if (statusIndicator) {
                    statusIndicator.className = `status-indicator status-${data.status}`;
                    statusIndicator.title = `${data.status} (${data.response_time}ms)`;
                }

                const responseTime = item.querySelector('.service-response-time');
                if (responseTime && data.response_time) {
                    responseTime.textContent = `${data.response_time}ms`;
                }

                const statusCode = item.querySelector('.service-status-code');
                if (statusCode && data.status_code) {
                    statusCode.textContent = data.status_code;
                    statusCode.setAttribute('data-code', data.status_code);
                }

                animateUpdate(item);
            }
        });
    }

    function updateMeme(data) {
        const section = document.querySelector('.meme-section');
        if (!section || !data.meme) return;

        const memeContent = section.querySelector('.meme-content');
        if (!memeContent) return;

        const meme = data.meme;
        let newContent = '';

        if (meme.type === 'image' || meme.type === 'gif') {
            newContent = `
                <div class="meme-${meme.type}">
                    <img src="${meme.image}" alt="${meme.text}" loading="lazy">
                    ${meme.text ? `<p class="meme-caption">${meme.text}</p>` : ''}
                </div>
            `;
        } else {
            newContent = `
                <div class="meme-text">
                    <p class="meme-quote">${meme.text}</p>
                    ${meme.source ? `<p class="meme-source">— ${meme.source}</p>` : ''}
                </div>
            `;
        }

        memeContent.innerHTML = newContent;
        animateUpdate(section);
    }

    function updateSystemInfo(data) {
        if (data.uptime_text) {
            const uptimeElements = document.querySelectorAll('[data-uptime]:not([data-uptime="client"])');
            uptimeElements.forEach(el => {
                if (el.textContent !== data.uptime_text) {
                    el.textContent = data.uptime_text;
                    animateUpdate(el);
                }
            });
        }

        if (data.last_updated) {
            const lastUpdatedElements = document.querySelectorAll('#last-updated');
            lastUpdatedElements.forEach(el => {
                if (el.textContent !== data.last_updated) {
                    el.textContent = data.last_updated;
                }
            });
        }

        if (data.connected_clients !== undefined) {
            updateClientCount({ count: data.connected_clients });
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

    function animateNumber(element, oldValue, newValue) {
        if (!element) return;
        element.style.color = 'var(--accent)';
        element.style.transition = 'color 0.3s ease';
        element.textContent = newValue;
        setTimeout(() => {
            element.style.color = '';
        }, 300);
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

    let isGoodbye = false;

    function disconnect() {
        shouldReconnect = false;
        isGoodbye = true;
        stopHeartbeat();
        stopLastFMCheck();
        if (reconnectTimeout) {
            clearTimeout(reconnectTimeout);
            reconnectTimeout = null;
        }
        if (clientCountRequestTimeout) {
            clearTimeout(clientCountRequestTimeout);
            clientCountRequestTimeout = null;
        }
        updateConnectionStatus('goodbye');
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
            stopLastFMCheck();
        } else {
            if (isConnected) {
                startHeartbeat();
                startLastFMCheck();
                if (clientCountRequestTimeout) {
                    clearTimeout(clientCountRequestTimeout);
                }
                clientCountRequestTimeout = setTimeout(() => {
                    sendMessage({ type: 'get_client_count' });
                }, 100);
            } else if (shouldReconnect) {
                connect();
            }
        }
    });

    window.wsReconnect = connect;
    window.wsDisconnect = disconnect;
    window.wsStatus = () => isConnected;
    window.wsSend = sendMessage;

    let webringUpdateInterval = null;
    let webringInitialized = false;

    function initWebringUpdater() {
        if (webringInitialized) return;

        const webringSection = document.querySelector('.webring-section');
        if (!webringSection) return;

        const baseUrl = webringSection.dataset.baseUrl;
        const username = webringSection.dataset.username;

        if (!baseUrl || !username) return;

        webringInitialized = true;
        let isUpdating = false;
        let lastUpdateTime = 0;

        function updateWebringFromAPI() {
            const now = Date.now();

            if (isUpdating || (now - lastUpdateTime) < 5000) return;

            isUpdating = true;

            fetch(`${baseUrl}/${username}/data`)
                .then(response => {
                    if (!response.ok) throw new Error('Network response was not ok');
                    return response.json();
                })
                .then(data => {
                    if (!data.prev || !data.next) return;

                    const prevLink = webringSection.querySelector('.webring-prev');
                    const nextLink = webringSection.querySelector('.webring-next');

                    let updated = false;

                    if (prevLink && data.prev) {
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
                        animateUpdate(webringSection);
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

        setTimeout(updateWebringFromAPI, 2000);

        if (webringUpdateInterval) {
            clearInterval(webringUpdateInterval);
        }
        webringUpdateInterval = setInterval(updateWebringFromAPI, 60 * 60 * 1000);

        document.addEventListener('visibilitychange', () => {
            if (!document.hidden) {
                setTimeout(updateWebringFromAPI, 1000);
            }
        }, { once: false, passive: true });
    }
    let mainInitialized = false;
    function initialize() {
        if (mainInitialized) return;
        mainInitialized = true;
        connect();
        initWebringUpdater();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initialize, { once: true });
    } else {
        initialize();
    }
})();