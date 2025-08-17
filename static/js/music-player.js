(function() {
    'use strict';

    let currentTrack = null;
    let isPlaying = false;
    let audioElement = null;

    function initMusicPlayer() {
        audioElement = document.getElementById('music-audio');

        if (audioElement) {
            audioElement.addEventListener('play', onAudioPlay);
            audioElement.addEventListener('pause', onAudioPause);
            audioElement.addEventListener('ended', onAudioEnded);
            audioElement.addEventListener('error', onAudioError);
        }
    }

    async function playTrack(searchQuery) {
        try {
            showLoading('Searching for track...');

            const response = await fetch(`https://new.akarpov.ru/api/v1/music/song/?search=${encodeURIComponent(searchQuery)}`);

            if (!response.ok) {
                throw new Error('Search failed');
            }

            const data = await response.json();

            if (!data.results || data.results.length === 0) {
                showNotification('No tracks found for: ' + searchQuery, 'error');
                return;
            }

            const track = data.results[0];
            loadAndPlayTrack(track);

        } catch (error) {
            console.error('Error playing track:', error);
            showNotification('Failed to load track: ' + error.message, 'error');
            hideLoading();
        }
    }

    function loadAndPlayTrack(track) {
        currentTrack = track;

        if (!audioElement) {
            createAudioPlayer();
        }

        updateNowPlayingUI(track);

        audioElement.src = track.file;
        audioElement.load();

        showLoading('Loading track...');

        audioElement.addEventListener('canplay', function onCanPlay() {
            audioElement.removeEventListener('canplay', onCanPlay);
            hideLoading();
            audioElement.play().catch(error => {
                console.error('Auto-play failed:', error);
                showNotification('Click play button to start music', 'info');
            });
        }, { once: true });
    }

    function createAudioPlayer() {
        const lastfmSection = document.querySelector('.lastfm-section');
        if (!lastfmSection) return;

        const playerHTML = `
            <div class="audio-player" id="audio-player">
                <audio controls id="music-audio">
                    <source src="" type="audio/mpeg">
                    Your browser does not support the audio element.
                </audio>
                <div class="now-playing-info" id="now-playing-info">
                    <div class="track-info-mini">
                        <div class="track-name-mini"></div>
                        <div class="track-artist-mini"></div>
                    </div>
                    <div class="playback-controls">
                        <button class="btn btn-small" id="play-pause-btn" onclick="togglePlayPause()">Pause</button>
                        <button class="btn btn-small" id="stop-btn" onclick="stopPlayback()">Stop</button>
                    </div>
                </div>
            </div>
        `;

        lastfmSection.insertAdjacentHTML('beforeend', playerHTML);
        audioElement = document.getElementById('music-audio');
        initMusicPlayer();
    }

    function updateNowPlayingUI(track) {
        const player = document.getElementById('audio-player');
        if (!player) return;

        player.style.display = 'block';

        const trackNameMini = player.querySelector('.track-name-mini');
        const trackArtistMini = player.querySelector('.track-artist-mini');

        if (trackNameMini) {
            trackNameMini.textContent = track.name;
        }

        if (trackArtistMini) {
            const artists = track.authors?.map(author => author.name).join(', ') ||
                track.album?.authors?.map(author => author.name).join(', ') ||
                'Unknown Artist';
            trackArtistMini.textContent = artists;
        }

        showNotification(`Now playing: ${track.name}`, 'success');
    }

    function togglePlayPause() {
        if (!audioElement) return;

        if (audioElement.paused) {
            audioElement.play().catch(error => {
                console.error('Play failed:', error);
                showNotification('Failed to play track', 'error');
            });
        } else {
            audioElement.pause();
        }
    }

    function stopPlayback() {
        if (!audioElement) return;

        audioElement.pause();
        audioElement.currentTime = 0;

        const player = document.getElementById('audio-player');
        if (player) {
            player.style.display = 'none';
        }

        currentTrack = null;
        isPlaying = false;

        updatePlayPauseButton();
    }

    function onAudioPlay() {
        isPlaying = true;
        updatePlayPauseButton();
    }

    function onAudioPause() {
        isPlaying = false;
        updatePlayPauseButton();
    }

    function onAudioEnded() {
        isPlaying = false;
        updatePlayPauseButton();
        showNotification('Track finished', 'info');
    }

    function onAudioError(e) {
        console.error('Audio error:', e);
        hideLoading();
        showNotification('Audio playback error', 'error');

        const player = document.getElementById('audio-player');
        if (player) {
            player.style.display = 'none';
        }
    }

    function updatePlayPauseButton() {
        const playPauseBtn = document.getElementById('play-pause-btn');
        if (playPauseBtn) {
            playPauseBtn.textContent = isPlaying ? 'Pause' : 'Play';
        }
    }

    function showLoading(message) {
        let loader = document.getElementById('music-loader');
        if (!loader) {
            loader = document.createElement('div');
            loader.id = 'music-loader';
            loader.className = 'loading-indicator';
            loader.innerHTML = `
                <div class="loading"></div>
                <span class="loading-text">${message}</span>
            `;

            const lastfmSection = document.querySelector('.lastfm-section');
            if (lastfmSection) {
                lastfmSection.appendChild(loader);
            }
        } else {
            loader.querySelector('.loading-text').textContent = message;
            loader.style.display = 'flex';
        }
    }

    function hideLoading() {
        const loader = document.getElementById('music-loader');
        if (loader) {
            loader.style.display = 'none';
        }
    }

    function showNotification(message, type) {
        const notification = document.createElement('div');
        notification.className = `notification ${type}`;
        notification.textContent = message;

        Object.assign(notification.style, {
            position: 'fixed',
            top: '20px',
            right: '20px',
            padding: '8px 16px',
            borderRadius: '4px',
            color: 'white',
            fontWeight: '500',
            zIndex: '10000',
            transform: 'translateX(400px)',
            transition: 'transform 0.3s ease',
            maxWidth: '300px'
        });

        switch (type) {
            case 'success':
                notification.style.background = '#4CAF50';
                break;
            case 'error':
                notification.style.background = '#f44336';
                break;
            case 'info':
                notification.style.background = '#2196F3';
                break;
            default:
                notification.style.background = '#333';
        }

        document.body.appendChild(notification);

        setTimeout(() => {
            notification.style.transform = 'translateX(0)';
        }, 100);

        setTimeout(() => {
            notification.style.transform = 'translateX(400px)';
            setTimeout(() => {
                if (notification.parentNode) {
                    notification.parentNode.removeChild(notification);
                }
            }, 300);
        }, 3000);
    }

    function handleMusicUpdate(message) {
        if (message.type === 'music_play') {
            const trackInfo = message.data;
            console.log('Music played by another user:', trackInfo);

            if (!isPlaying) {
                showNotification(`♪ ${trackInfo.name} is now playing`, 'info');
            }
        }
    }

    window.playTrack = playTrack;
    window.togglePlayPause = togglePlayPause;
    window.stopPlayback = stopPlayback;

    window.musicPlayer = {
        playTrack,
        togglePlayPause,
        stopPlayback,
        handleMusicUpdate,
        getCurrentTrack: () => currentTrack,
        isPlaying: () => isPlaying
    };

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initMusicPlayer);
    } else {
        initMusicPlayer();
    }

})();