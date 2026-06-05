(function() {
    'use strict';

    let customAudioElement = null;
    let isCustomPlaying = false;
    let currentLoadedTrack = null;
    let musicPlayerInitialized = false;
    let isLoadingTrack = false;

    function initCustomMusicPlayer() {
        if (musicPlayerInitialized) return;
        musicPlayerInitialized = true;
        customAudioElement = document.getElementById('lastfm-audio-element');
        if (!customAudioElement) return;

        customAudioElement.removeEventListener('loadedmetadata', updatePlayerDuration);
        customAudioElement.removeEventListener('timeupdate', updatePlayerProgress);
        customAudioElement.removeEventListener('play', onCustomPlay);
        customAudioElement.removeEventListener('pause', onCustomPause);
        customAudioElement.removeEventListener('ended', onCustomEnded);

        customAudioElement.addEventListener('loadedmetadata', updatePlayerDuration);
        customAudioElement.addEventListener('timeupdate', updatePlayerProgress);
        customAudioElement.addEventListener('play', onCustomPlay);
        customAudioElement.addEventListener('pause', onCustomPause);
        customAudioElement.addEventListener('ended', onCustomEnded);

        const progressBar = document.getElementById('player-progress-bar');
        const volumeSlider = document.getElementById('player-volume');

        if (progressBar && !progressBar.dataset.hasListener) {
            progressBar.dataset.hasListener = 'true';
            progressBar.addEventListener('click', seekToPosition);
        }

        if (volumeSlider && !volumeSlider.dataset.hasListener) {
            volumeSlider.dataset.hasListener = 'true';
            volumeSlider.addEventListener('input', (e) => {
                if (customAudioElement) {
                    customAudioElement.volume = e.target.value / 100;
                }
            });
            customAudioElement.volume = 0.8;
        }
    }

    function playCurrentLastFMTrack() {
        const currentTrackEl = document.querySelector('.current-track');
        if (!currentTrackEl) {
            console.error('No current track element found');
            return;
        }

        const artist = currentTrackEl.dataset.artist ||
            document.getElementById('lastfm-track-artist')?.textContent?.replace(/^by\s+/, '') || '';
        const track = currentTrackEl.dataset.track ||
            document.getElementById('lastfm-track-title')?.textContent || '';

        if (!artist || !track) {
            console.error('Could not get current track info');
            return;
        }

        const searchQuery = `${artist} ${track}`;
        console.log('Playing current track:', searchQuery);
        playTrack(searchQuery);
    }

    async function playTrack(searchInfo) {
        if (isLoadingTrack) return;
        isLoadingTrack = true;

        let searchQuery = "";
        let trackDetails = {};

        if (typeof searchInfo === 'string') {
            searchQuery = searchInfo;
        } else if (typeof searchInfo === 'object' && searchInfo !== null) {
            searchQuery = `${searchInfo.artist} - ${searchInfo.track}`;
            trackDetails = searchInfo;
        }

        console.log('Playing track query:', searchQuery);

        try {
            const url = new URL('https://new.akarpov.ru/api/v1/music/search/');
            url.searchParams.append('query', searchQuery);

            const response = await fetch(url);
            if (!response.ok) throw new Error('Search failed');
            const data = await response.json();

            if (!data.songs || data.songs.length === 0) {
                showNotification('No tracks found', 'error');
                return;
            }

            let track = data.songs[0];
            if (trackDetails.artist && trackDetails.track && data.songs.length > 1) {
                const targetArtist = trackDetails.artist.toLowerCase();
                const targetTrack = trackDetails.track.toLowerCase();

                const exactMatch = data.songs.find(s =>
                    s.name.toLowerCase() === targetTrack &&
                    s.authors.some(a => a.name.toLowerCase() === targetArtist)
                );

                if (exactMatch) {
                    track = exactMatch;
                }
            }

            let imageURL = track.image_cropped || '';
            if (imageURL && !imageURL.startsWith('http')) {
                imageURL = 'https://new.akarpov.ru' + imageURL;
            }
            if (!imageURL && track.album && track.album.image_cropped) {
                imageURL = track.album.image_cropped;
                if (!imageURL.startsWith('http')) {
                    imageURL = 'https://new.akarpov.ru' + imageURL;
                }
            }

            const normalizedTrack = {
                name: track.name,
                slug: track.slug,
                file: track.file,
                image_cropped: imageURL,
                length: track.length,
                album: track.album,
                authors: track.authors
            };

            loadAndPlayCustomTrack(normalizedTrack);
        } catch (error) {
            console.error('Error playing track:', error);
            showNotification('Failed to load track', 'error');
        } finally {
            isLoadingTrack = false;
        }
    }

    function setupMediaSession(track) {
        if (!('mediaSession' in navigator)) return;

        navigator.mediaSession.metadata = new MediaMetadata({
            title: track.name || 'Unknown',
            artist: track.authors?.map(a => a.name).join(', ') || 'Unknown Artist',
            album: track.album?.name || '',
            artwork: track.image_cropped
                ? [{src: track.image_cropped, sizes: '512x512', type: 'image/jpeg'}]
                : []
        });

        navigator.mediaSession.setActionHandler('play', () => customAudioElement?.play());
        navigator.mediaSession.setActionHandler('pause', () => customAudioElement?.pause());
        navigator.mediaSession.setActionHandler('stop', stopMusicPlayback);
        navigator.mediaSession.setActionHandler('seekbackward', (d) => {
            if (customAudioElement) customAudioElement.currentTime -= d.seekOffset || 10;
        });
        navigator.mediaSession.setActionHandler('seekforward', (d) => {
            if (customAudioElement) customAudioElement.currentTime += d.seekOffset || 10;
        });
        navigator.mediaSession.setActionHandler('seekto', (d) => {
            if (customAudioElement && d.seekTime != null) customAudioElement.currentTime = d.seekTime;
        });
    }

    function updateMediaSessionPosition() {
        if (!('mediaSession' in navigator) || !customAudioElement) return;
        if (!isFinite(customAudioElement.duration)) return;
        try {
            navigator.mediaSession.setPositionState({
                duration: customAudioElement.duration,
                playbackRate: customAudioElement.playbackRate,
                position: customAudioElement.currentTime
            });
        } catch {}
    }

    function loadAndPlayCustomTrack(track) {
        if (!customAudioElement) return;

        if (currentLoadedTrack && currentLoadedTrack.file === track.file) {
            if (customAudioElement.paused) {
                customAudioElement.play().catch(err => {
                    console.error('Resume failed:', err);
                    showNotification('Playback failed', 'error');
                });
            }
            return;
        }

        customAudioElement.pause();
        customAudioElement.currentTime = 0;

        currentLoadedTrack = track;
        setupMediaSession(track);

        const player = document.getElementById('custom-music-player');
        const artworkMini = document.getElementById('player-artwork-mini');
        const trackName = document.getElementById('player-track-name');
        const artistName = document.getElementById('player-artist-name');
        const progressFill = document.getElementById('player-progress-fill');

        if (player) player.style.display = 'flex';

        if (progressFill) progressFill.style.width = '0%';

        if (artworkMini) {
            artworkMini.src = track.image_cropped || track.album?.image_cropped || '';
        }

        if (trackName) trackName.textContent = track.name;
        if (artistName) {
            const artists = track.authors?.map(a => a.name).join(', ') || 'Unknown Artist';
            artistName.textContent = artists;
        }

        customAudioElement.src = track.file;
        customAudioElement.load();

        const handleCanPlay = () => {
            customAudioElement.removeEventListener('canplay', handleCanPlay);
            customAudioElement.removeEventListener('error', handleError);

            customAudioElement.play().catch(err => {
                console.error('Playback failed:', err);
                showNotification('Click play to start', 'info');
            });
        };

        const handleError = () => {
            customAudioElement.removeEventListener('canplay', handleCanPlay);
            customAudioElement.removeEventListener('error', handleError);
            showNotification('Failed to load track', 'error');
        };

        customAudioElement.addEventListener('canplay', handleCanPlay, {once: true});
        customAudioElement.addEventListener('error', handleError, {once: true});

        showNotification(`Loading: ${track.name}`, 'success');
    }

    function toggleMusicPlayPause() {
        if (!customAudioElement) return;

        if (customAudioElement.paused) {
            customAudioElement.play().catch(err => {
                console.error('Play failed:', err);
                showNotification('Playback failed', 'error');
            });
        } else {
            customAudioElement.pause();
        }
    }

    function stopMusicPlayback() {
        if (!customAudioElement) return;

        customAudioElement.pause();
        customAudioElement.currentTime = 0;

        const player = document.getElementById('custom-music-player');
        if (player) player.style.display = 'none';

        currentLoadedTrack = null;
        isCustomPlaying = false;
        updatePlayPauseIcon(false);
    }

    function updatePlayerProgress() {
        if (!customAudioElement || !currentLoadedTrack) return;

        const currentTime = customAudioElement.currentTime;
        const duration = customAudioElement.duration;

        if (!isFinite(currentTime) || !isFinite(duration) || duration === 0) return;

        updateMediaSessionPosition();

        const progressFill = document.getElementById('player-progress-fill');
        if (progressFill) {
            const percent = Math.min(100, (currentTime / duration) * 100);
            progressFill.style.width = percent + '%';
        }

        const currentTimeEl = document.getElementById('player-current-time');
        if (currentTimeEl) {
            currentTimeEl.textContent = formatTime(currentTime);
        }
    }

    function updatePlayerDuration() {
        if (!customAudioElement || !currentLoadedTrack) return;

        const duration = customAudioElement.duration;

        if (!isFinite(duration) || duration === 0) return;

        const durationEl = document.getElementById('player-duration');
        if (durationEl) {
            durationEl.textContent = formatTime(duration);
        }

        const currentTimeEl = document.getElementById('player-current-time');
        if (currentTimeEl && customAudioElement.currentTime === 0) {
            currentTimeEl.textContent = '0:00';
        }

        const progressFill = document.getElementById('player-progress-fill');
        if (progressFill && customAudioElement.currentTime === 0) {
            progressFill.style.width = '0%';
        }
    }

    function seekToPosition(e) {
        if (!customAudioElement) return;

        const bar = e.currentTarget;
        const rect = bar.getBoundingClientRect();
        const percent = (e.clientX - rect.left) / rect.width;
        customAudioElement.currentTime = percent * customAudioElement.duration;
    }

    function onCustomPlay() {
        isCustomPlaying = true;
        updatePlayPauseIcon(true);
        if ('mediaSession' in navigator) navigator.mediaSession.playbackState = 'playing';
    }

    function onCustomPause() {
        isCustomPlaying = false;
        updatePlayPauseIcon(false);
        if ('mediaSession' in navigator) navigator.mediaSession.playbackState = 'paused';
    }

    function onCustomEnded() {
        isCustomPlaying = false;
        updatePlayPauseIcon(false);

        const progressFill = document.getElementById('player-progress-fill');
        if (progressFill) {
            progressFill.style.width = '100%';
        }

        const currentTimeEl = document.getElementById('player-current-time');
        const durationEl = document.getElementById('player-duration');
        if (currentTimeEl && durationEl && customAudioElement) {
            currentTimeEl.textContent = durationEl.textContent;
        }

        showNotification('Track finished', 'info');

        setTimeout(() => {
            if (progressFill) {
                progressFill.style.width = '0%';
            }
            if (currentTimeEl) {
                currentTimeEl.textContent = '0:00';
            }
        }, 1000);
    }

    function updatePlayPauseIcon(playing) {
        const icon = document.getElementById('play-pause-icon');
        if (!icon) return;

        if (playing) {
            icon.innerHTML = '<path fill="currentColor" d="M6 4h4v16H6V4zm8 0h4v16h-4V4z"/>';
        } else {
            icon.innerHTML = '<path fill="currentColor" d="M8 5v14l11-7z"/>';
        }
    }

    function formatTime(seconds) {
        if (!isFinite(seconds)) return '0:00';
        const mins = Math.floor(seconds / 60);
        const secs = Math.floor(seconds % 60);
        return `${mins}:${secs.toString().padStart(2, '0')}`;
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
            borderRadius: '8px',
            color: 'white',
            fontWeight: '600',
            zIndex: '10000',
            transform: 'translateX(400px)',
            transition: 'transform 0.3s ease',
            maxWidth: '300px',
            boxShadow: '0 4px 16px rgba(0,0,0,.3)'
        });

        switch (type) {
            case 'success': notification.style.background = '#3ad38b'; break;
            case 'error': notification.style.background = '#ff6b6b'; break;
            case 'info': notification.style.background = '#7aa2ff'; break;
            default: notification.style.background = '#333';
        }

        document.body.appendChild(notification);
        setTimeout(() => notification.style.transform = 'translateX(0)', 100);

        setTimeout(() => {
            notification.style.transform = 'translateX(400px)';
            setTimeout(() => notification.remove(), 300);
        }, 3000);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initCustomMusicPlayer, { once: true });
    } else {
        initCustomMusicPlayer();
    }

    window.playTrack = playTrack;
    window.playCurrentLastFMTrack = playCurrentLastFMTrack;
    window.toggleMusicPlayPause = toggleMusicPlayPause;
    window.stopMusicPlayback = stopMusicPlayback;

    window.musicPlayer = {
        playTrack,
        playCurrentLastFMTrack,
        getCurrentTrack: () => currentLoadedTrack,
        isPlaying: () => isCustomPlaying,
        handleMusicUpdate: function(message) {
            if (message.type === 'music_play' && message.data) {
                let imageURL = message.data.image || '';
                if (imageURL && !imageURL.startsWith('http')) {
                    imageURL = 'https://new.akarpov.ru' + imageURL;
                }

                const track = {
                    name: message.data.name || 'Unknown Track',
                    file: message.data.file,
                    image_cropped: imageURL,
                    album: {
                        name: message.data.album || '',
                        image_cropped: imageURL
                    },
                    authors: message.data.artists ?
                        message.data.artists.map(name => ({name})) :
                        [{name: 'Unknown Artist'}],
                    length: message.data.length || 0
                };

                loadAndPlayCustomTrack(track);
            }
        }
    };
})();