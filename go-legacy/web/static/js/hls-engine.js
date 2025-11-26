/**
 * HLSEngine - 재사용 가능한 HLS 클라이언트 라이브러리
 * hls.js를 사용하여 HLS 스트림 재생
 *
 * @example
 * const engine = new HLSEngine({
 *   streamId: 'park_cctv_01',
 *   videoElement: document.getElementById('video1')
 * });
 *
 * engine.on('loaded', () => console.log('HLS loaded'));
 * engine.on('error', (err) => console.error(err));
 *
 * await engine.load();
 */

class HLSEngine {
    constructor(config) {
        // 필수 파라미터 검증
        if (!config.videoElement) {
            throw new Error('videoElement is required');
        }
        if (!config.streamId) {
            throw new Error('streamId is required');
        }

        // 설정
        this.streamId = config.streamId;
        this.videoElement = config.videoElement;
        this.serverUrl = config.serverUrl || window.location.origin;
        this.autoReconnect = config.autoReconnect !== undefined ? config.autoReconnect : true;
        this.reconnectDelay = config.reconnectDelay || 3000;

        // HLS.js 인스턴스
        this.hls = null;

        // 상태
        this.loaded = false;
        this.reconnecting = false;
        this.reconnectTimer = null;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;  // 최대 재연결 시도 횟수

        // 통계
        this.stats = {
            bytesLoaded: 0,
            bitrate: 0,
            bufferLength: 0,
            droppedFrames: 0
        };
        this.lastBytesLoaded = 0;
        this.statsInterval = null;

        // 이벤트 핸들러
        this.eventHandlers = {
            'loaded': [],
            'playing': [],
            'error': [],
            'stats': [],
            'buffering': [],
            'quality': []
        };

        // 비디오 엘리먼트 속성 설정
        this.videoElement.autoplay = true;
        this.videoElement.playsinline = true;
        this.videoElement.muted = true;

        this.log(`🎬 HLSEngine initialized for stream: ${this.streamId}`);
    }

    /**
     * 이벤트 리스너 등록
     */
    on(event, callback) {
        if (!this.eventHandlers[event]) {
            throw new Error(`Unknown event: ${event}`);
        }
        this.eventHandlers[event].push(callback);
        return this;
    }

    /**
     * 이벤트 발생
     */
    emit(event, data) {
        if (this.eventHandlers[event]) {
            this.eventHandlers[event].forEach(callback => {
                try {
                    callback(data);
                } catch (error) {
                    console.error('[HLSEngine] Event handler error:', error);
                }
            });
        }
    }

    /**
     * HLS 스트림 로드
     */
    async load() {
        if (this.loaded) {
            this.log('⚠️ Already loaded');
            return;
        }

        try {
            this.log(`📡 Loading HLS stream: ${this.streamId}`);

            // hls.js 지원 확인
            if (!this.checkHLSSupport()) {
                throw new Error('HLS is not supported in this browser');
            }

            // On-demand 스트림 시작 (필요한 경우)
            await this.startOnDemandStream();

            // HLS 플레이리스트 URL 구성
            const playlistUrl = `${this.serverUrl}/hls/${this.streamId}/index.m3u8`;
            this.log(`📋 Playlist URL: ${playlistUrl}`);

            // hls.js 우선 사용 (더 안정적)
            if (Hls.isSupported()) {
                this.log('📦 Using hls.js');
                this.loadHlsJs(playlistUrl);
            } else if (this.videoElement.canPlayType('application/vnd.apple.mpegurl')) {
                // Safari에서만 Native HLS 사용
                this.log('🍎 Using native HLS support (Safari)');
                this.loadNativeHLS(playlistUrl);
            } else {
                throw new Error('HLS is not supported');
            }

        } catch (error) {
            this.handleError('Failed to load HLS stream', error);
        }
    }

    /**
     * On-demand 스트림 시작
     */
    async startOnDemandStream() {
        try {
            // 스트림 정보 가져오기
            this.log(`🔍 Checking if stream is on-demand: ${this.streamId}`);

            const response = await fetch(`${this.serverUrl}/v3/config/paths/list`);
            if (!response.ok) {
                this.log(`⚠️ Failed to get stream list, skipping on-demand check`);
                return;
            }

            const data = await response.json();
            const streamInfo = data.items?.find(s => s.name === this.streamId);

            if (!streamInfo) {
                this.log(`⚠️ Stream not found in list, skipping on-demand check`);
                return;
            }

            // On-demand 스트림인 경우 시작
            if (streamInfo.conf?.sourceOnDemand) {
                this.log(`🚀 Starting on-demand stream: ${this.streamId}`);

                const startResponse = await fetch(`${this.serverUrl}/api/v1/streams/${this.streamId}/start`, {
                    method: 'POST'
                });

                if (startResponse.ok) {
                    this.log(`✅ On-demand stream started, waiting 2 seconds for muxer initialization...`);
                    // HLS muxer가 시작될 때까지 대기
                    await new Promise(resolve => setTimeout(resolve, 2000));
                } else if (startResponse.status === 409) {
                    // 이미 실행 중 (정상)
                    this.log(`ℹ️ Stream already running`);
                    await new Promise(resolve => setTimeout(resolve, 500));
                } else {
                    this.log(`⚠️ Failed to start on-demand stream: ${startResponse.status}`);
                }
            } else {
                this.log(`ℹ️ Stream is always-on, no need to start`);
            }
        } catch (error) {
            this.log(`⚠️ Error checking/starting on-demand stream: ${error.message}`);
            // 에러가 발생해도 계속 진행 (HLS 로드 시도)
        }
    }

    /**
     * Native HLS 로드 (Safari)
     */
    loadNativeHLS(playlistUrl) {
        this.videoElement.src = playlistUrl;

        this.videoElement.addEventListener('loadedmetadata', () => {
            this.log('✅ HLS metadata loaded (native)');
            this.loaded = true;
            this.reconnectAttempts = 0;  // 성공 시 재연결 카운터 리셋
            this.emit('loaded');
            this.startStatsCollection();
        });

        this.videoElement.addEventListener('playing', () => {
            this.log('▶️ HLS playing (native)');
            this.emit('playing');
        });

        this.videoElement.addEventListener('error', (e) => {
            this.handleError('Native HLS error', e);
        });

        this.videoElement.addEventListener('waiting', () => {
            this.emit('buffering', true);
        });

        this.videoElement.addEventListener('canplay', () => {
            this.emit('buffering', false);
        });
    }

    /**
     * hls.js 로드
     */
    loadHlsJs(playlistUrl) {
        this.hls = new Hls({
            debug: false,
            enableWorker: true,
            lowLatencyMode: true,
            backBufferLength: 90
        });

        // HLS 이벤트 핸들러
        this.hls.on(Hls.Events.MANIFEST_PARSED, () => {
            this.log('✅ HLS manifest parsed');
            this.loaded = true;
            this.reconnectAttempts = 0;  // 성공 시 재연결 카운터 리셋
            this.emit('loaded');
            this.startStatsCollection();

            // 자동 재생
            this.videoElement.play().catch(err => {
                this.log('⚠️ Autoplay prevented, user interaction required');
            });
        });

        this.hls.on(Hls.Events.LEVEL_LOADED, (event, data) => {
            this.emit('quality', {
                level: data.level,
                details: data.details
            });
        });

        this.hls.on(Hls.Events.FRAG_LOADED, (event, data) => {
            this.stats.bytesLoaded += data.frag.stats.total;
        });

        this.hls.on(Hls.Events.ERROR, (event, data) => {
            this.handleHlsError(data);
        });

        // 비디오 이벤트
        this.videoElement.addEventListener('playing', () => {
            this.log('▶️ HLS playing');
            this.emit('playing');
        });

        this.videoElement.addEventListener('waiting', () => {
            this.emit('buffering', true);
        });

        this.videoElement.addEventListener('canplay', () => {
            this.emit('buffering', false);
        });

        // 플레이리스트 로드
        this.hls.loadSource(playlistUrl);
        this.hls.attachMedia(this.videoElement);
    }

    /**
     * hls.js 에러 핸들링
     */
    handleHlsError(data) {
        if (data.fatal) {
            switch (data.type) {
                case Hls.ErrorTypes.NETWORK_ERROR:
                    this.log('💥 Fatal network error, attempting recovery...');
                    // startLoad로 한번 복구 시도, 실패하면 hls.js가 다시 에러 발생시킴
                    this.hls.startLoad();
                    break;

                case Hls.ErrorTypes.MEDIA_ERROR:
                    this.log('💥 Fatal media error, attempting recovery...');
                    this.hls.recoverMediaError();
                    break;

                default:
                    this.log(`💥 Unrecoverable HLS error: ${data.type}`);
                    this.handleError('Fatal HLS error', data);
                    this.destroy();

                    // 복구 불가능한 에러만 재연결 시도
                    if (this.autoReconnect && !this.reconnecting) {
                        this.scheduleReconnect();
                    }
                    break;
            }
        } else {
            this.log(`⚠️ Non-fatal HLS error: ${data.details}`);
        }
    }

    /**
     * 재연결 스케줄
     */
    scheduleReconnect() {
        if (this.reconnecting) return;

        // 최대 재연결 시도 횟수 확인
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            this.log(`❌ Max reconnect attempts (${this.maxReconnectAttempts}) reached. Giving up.`);
            this.emit('error', {
                message: 'Max reconnect attempts reached',
                attempts: this.reconnectAttempts
            });
            return;
        }

        this.reconnecting = true;
        this.reconnectAttempts++;
        this.log(`🔄 Reconnecting in ${this.reconnectDelay}ms... (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);

        this.reconnectTimer = setTimeout(() => {
            this.reconnecting = false;
            this.log('🔄 Attempting to reconnect...');
            this.destroy();
            this.load();
        }, this.reconnectDelay);
    }

    /**
     * 통계 수집 시작
     */
    startStatsCollection() {
        if (this.statsInterval) {
            clearInterval(this.statsInterval);
        }

        this.statsInterval = setInterval(() => {
            this.updateStats();
        }, 1000);
    }

    /**
     * 통계 업데이트
     */
    updateStats() {
        if (!this.loaded) return;

        // 비트레이트 계산
        const bytesDelta = this.stats.bytesLoaded - this.lastBytesLoaded;
        this.stats.bitrate = (bytesDelta * 8) / 1000; // kbps
        this.lastBytesLoaded = this.stats.bytesLoaded;

        // 버퍼 길이
        if (this.videoElement.buffered.length > 0) {
            const currentTime = this.videoElement.currentTime;
            const bufferedEnd = this.videoElement.buffered.end(this.videoElement.buffered.length - 1);
            this.stats.bufferLength = bufferedEnd - currentTime;
        }

        // Dropped frames (WebKit only)
        if (this.videoElement.webkitDecodedFrameCount !== undefined) {
            const decodedFrames = this.videoElement.webkitDecodedFrameCount || 0;
            const droppedFrames = this.videoElement.webkitDroppedFrameCount || 0;
            this.stats.droppedFrames = droppedFrames;
        }

        this.emit('stats', { ...this.stats });
    }

    /**
     * HLS 지원 확인
     */
    checkHLSSupport() {
        // Native HLS (Safari) 또는 hls.js 지원 확인
        const nativeSupport = this.videoElement.canPlayType('application/vnd.apple.mpegurl');
        const hlsJsSupport = typeof Hls !== 'undefined' && Hls.isSupported();

        if (!nativeSupport && !hlsJsSupport) {
            this.log('❌ HLS is not supported in this browser');
            return false;
        }

        return true;
    }

    /**
     * 에러 핸들링
     */
    handleError(message, error) {
        this.log(`❌ ${message}:`, error);
        this.emit('error', { message, error });
    }

    /**
     * 정리
     */
    destroy() {
        this.log('🧹 Destroying HLS engine');

        // 통계 수집 중지
        if (this.statsInterval) {
            clearInterval(this.statsInterval);
            this.statsInterval = null;
        }

        // 재연결 타이머 취소
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }

        // hls.js 정리
        if (this.hls) {
            this.hls.destroy();
            this.hls = null;
        }

        // 비디오 엘리먼트 정리
        this.videoElement.src = '';
        this.videoElement.load();

        this.loaded = false;
        this.reconnecting = false;
    }

    /**
     * 로깅
     */
    log(...args) {
        console.log(`[HLSEngine:${this.streamId}]`, ...args);
    }

    /**
     * 재생/일시정지
     */
    async play() {
        try {
            await this.videoElement.play();
        } catch (error) {
            this.handleError('Failed to play', error);
        }
    }

    pause() {
        this.videoElement.pause();
    }

    /**
     * 볼륨 제어
     */
    setVolume(volume) {
        this.videoElement.volume = Math.max(0, Math.min(1, volume));
        this.videoElement.muted = volume === 0;
    }

    mute() {
        this.videoElement.muted = true;
    }

    unmute() {
        this.videoElement.muted = false;
    }

    /**
     * 통계 가져오기
     */
    getStats() {
        return { ...this.stats };
    }

    /**
     * 연결 상태 확인
     */
    isLoaded() {
        return this.loaded;
    }
}
