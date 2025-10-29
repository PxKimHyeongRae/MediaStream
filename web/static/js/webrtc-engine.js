/**
 * WebRTCEngine - 재사용 가능한 WebRTC 클라이언트 라이브러리
 *
 * @example
 * const engine = new WebRTCEngine({
 *   serverUrl: 'ws://localhost:8080/ws',
 *   streamId: 'park_cctv_01',
 *   videoElement: document.getElementById('video1')
 * });
 *
 * engine.on('connected', () => console.log('Connected'));
 * engine.on('error', (err) => console.error(err));
 *
 * await engine.connect();
 */

class WebRTCEngine {
    constructor(config) {
        // 필수 파라미터 검증
        if (!config.videoElement) {
            throw new Error('videoElement is required');
        }
        if (!config.streamId) {
            throw new Error('streamId is required');
        }

        // 설정
        this.serverUrl = config.serverUrl || `ws://${window.location.host}/ws`;
        this.streamId = config.streamId;
        this.videoElement = config.videoElement;
        this.autoReconnect = config.autoReconnect !== undefined ? config.autoReconnect : true;
        this.reconnectDelay = config.reconnectDelay || 3000;

        // 상태
        this.pc = null;
        this.ws = null;
        this.connected = false;
        this.reconnecting = false;
        this.reconnectTimer = null;

        // 통계
        this.stats = {
            packetsReceived: 0,
            bytesReceived: 0,
            bitrate: 0
        };
        this.lastBytesReceived = 0;
        this.statsInterval = null;

        // 이벤트 핸들러
        this.eventHandlers = {
            'connected': [],
            'disconnected': [],
            'error': [],
            'stats': [],
            'statechange': []
        };

        // 비디오 엘리먼트 속성 설정
        this.videoElement.autoplay = true;
        this.videoElement.playsinline = true;
        this.videoElement.muted = true;

        this.log('WebRTCEngine initialized', { streamId: this.streamId });
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
                    console.error('[WebRTCEngine] Event handler error:', error);
                }
            });
        }
    }

    /**
     * 연결 시작
     */
    async connect() {
        try {
            this.log('Connecting...');
            this.emit('statechange', 'connecting');

            // WebSocket 연결
            await this.connectWebSocket();

            // PeerConnection 생성
            await this.createPeerConnection();

            // Offer 생성
            await this.createOffer();

        } catch (error) {
            this.log('Connection error:', error, 'error');
            this.emit('error', error);

            if (this.autoReconnect && !this.reconnecting) {
                this.scheduleReconnect();
            }
        }
    }

    /**
     * WebSocket 연결
     */
    connectWebSocket() {
        return new Promise((resolve, reject) => {
            this.log('Connecting to WebSocket:', this.serverUrl);
            this.ws = new WebSocket(this.serverUrl);

            const timeout = setTimeout(() => {
                reject(new Error('WebSocket connection timeout'));
            }, 10000);

            this.ws.onopen = () => {
                clearTimeout(timeout);
                this.log('WebSocket connected');
                resolve();
            };

            this.ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);
                    this.handleSignalingMessage(message);
                } catch (error) {
                    this.log('Failed to parse message:', error, 'error');
                }
            };

            this.ws.onerror = (error) => {
                clearTimeout(timeout);
                this.log('WebSocket error:', error, 'error');
                reject(error);
            };

            this.ws.onclose = () => {
                this.log('WebSocket closed');
                this.handleDisconnect();
            };
        });
    }

    /**
     * PeerConnection 생성
     */
    createPeerConnection() {
        const config = {
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ]
        };

        this.pc = new RTCPeerConnection(config);

        // 이벤트 핸들러
        this.pc.onicecandidate = (event) => {
            if (event.candidate) {
                this.log('ICE candidate:', event.candidate.candidate);
                this.sendMessage('ice', event.candidate);
            }
        };

        this.pc.oniceconnectionstatechange = () => {
            const state = this.pc.iceConnectionState;
            this.log('ICE connection state:', state);
            this.emit('statechange', state);

            if (state === 'connected') {
                this.connected = true;
                this.reconnecting = false;
                this.emit('connected');
                this.startStatsUpdate();
            } else if (state === 'failed' || state === 'closed') {
                this.handleDisconnect();
            }
        };

        this.pc.onconnectionstatechange = () => {
            this.log('Connection state:', this.pc.connectionState);
        };

        this.pc.ontrack = async (event) => {
            this.log('Received remote track:', event.track.kind);

            if (this.videoElement.srcObject !== event.streams[0]) {
                this.videoElement.srcObject = event.streams[0];

                try {
                    await this.videoElement.play();
                    this.log('Video playback started');
                } catch (error) {
                    this.log('Failed to start playback:', error, 'error');
                    this.emit('error', error);
                }
            }
        };

        // Transceiver 추가
        this.pc.addTransceiver('video', { direction: 'recvonly' });
        this.pc.addTransceiver('audio', { direction: 'recvonly' });

        this.log('PeerConnection created');
    }

    /**
     * Offer 생성 및 전송
     */
    async createOffer() {
        const offer = await this.pc.createOffer({
            offerToReceiveVideo: true,
            offerToReceiveAudio: true
        });

        await this.pc.setLocalDescription(offer);
        this.log('Local description set');

        // Offer 전송
        this.sendMessage('offer', {
            sdp: offer.sdp,
            streamId: this.streamId
        });
    }

    /**
     * 시그널링 메시지 처리
     */
    handleSignalingMessage(message) {
        this.log('Received message:', message.type);

        switch (message.type) {
            case 'answer':
                this.handleAnswer(message.payload);
                break;
            case 'ice':
                this.handleICE(message.payload);
                break;
            case 'error':
                this.log('Server error:', message.payload, 'error');
                this.emit('error', new Error(message.payload));
                break;
            default:
                this.log('Unknown message type:', message.type, 'warn');
        }
    }

    /**
     * Answer 처리
     */
    async handleAnswer(sdp) {
        try {
            const answer = new RTCSessionDescription({
                type: 'answer',
                sdp: sdp
            });

            await this.pc.setRemoteDescription(answer);
            this.log('Remote description set');
        } catch (error) {
            this.log('Failed to set remote description:', error, 'error');
            this.emit('error', error);
        }
    }

    /**
     * ICE Candidate 처리
     */
    async handleICE(candidate) {
        try {
            await this.pc.addIceCandidate(new RTCIceCandidate(candidate));
        } catch (error) {
            this.log('Failed to add ICE candidate:', error, 'error');
        }
    }

    /**
     * 메시지 전송
     */
    sendMessage(type, payload) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify({ type, payload }));
        } else {
            this.log('Cannot send message - WebSocket not connected', 'error');
        }
    }

    /**
     * 연결 해제 처리
     */
    handleDisconnect() {
        if (this.connected) {
            this.connected = false;
            this.emit('disconnected');
        }

        this.stopStatsUpdate();

        if (this.autoReconnect && !this.reconnecting) {
            this.scheduleReconnect();
        }
    }

    /**
     * 재연결 스케줄링
     */
    scheduleReconnect() {
        this.reconnecting = true;
        this.log(`Reconnecting in ${this.reconnectDelay}ms...`);

        this.reconnectTimer = setTimeout(() => {
            this.disconnect(false); // 완전 정리
            this.connect(); // 재연결
        }, this.reconnectDelay);
    }

    /**
     * 통계 업데이트 시작
     */
    startStatsUpdate() {
        this.statsInterval = setInterval(() => this.updateStats(), 1000);
    }

    /**
     * 통계 업데이트 중지
     */
    stopStatsUpdate() {
        if (this.statsInterval) {
            clearInterval(this.statsInterval);
            this.statsInterval = null;
        }
    }

    /**
     * 통계 업데이트
     */
    async updateStats() {
        if (!this.pc) return;

        try {
            const stats = await this.pc.getStats();
            let bytesReceived = 0;
            let packetsReceived = 0;

            stats.forEach(report => {
                if (report.type === 'inbound-rtp' && report.kind === 'video') {
                    bytesReceived = report.bytesReceived || 0;
                    packetsReceived = report.packetsReceived || 0;
                }
            });

            // 비트레이트 계산
            const bitrate = this.lastBytesReceived
                ? (bytesReceived - this.lastBytesReceived) * 8 / 1000
                : 0;

            this.stats = {
                packetsReceived,
                bytesReceived,
                bitrate
            };

            this.lastBytesReceived = bytesReceived;
            this.emit('stats', this.stats);

        } catch (error) {
            this.log('Failed to get stats:', error, 'error');
        }
    }

    /**
     * 통계 가져오기
     */
    getStats() {
        return this.stats;
    }

    /**
     * 연결 상태 확인
     */
    isConnected() {
        return this.connected;
    }

    /**
     * 연결 해제
     */
    disconnect(cleanup = true) {
        this.log('Disconnecting...');

        // 재연결 타이머 취소
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }

        // 자동 재연결 비활성화
        if (cleanup) {
            this.autoReconnect = false;
        }

        this.stopStatsUpdate();

        if (this.pc) {
            this.pc.close();
            this.pc = null;
        }

        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }

        if (this.videoElement.srcObject) {
            this.videoElement.srcObject.getTracks().forEach(track => track.stop());
            this.videoElement.srcObject = null;
        }

        this.connected = false;
        this.reconnecting = false;

        if (cleanup) {
            this.emit('disconnected');
        }
    }

    /**
     * 로그 출력
     */
    log(message, data, level = 'info') {
        const prefix = `[WebRTCEngine:${this.streamId}]`;

        if (level === 'error') {
            console.error(prefix, message, data || '');
        } else if (level === 'warn') {
            console.warn(prefix, message, data || '');
        } else {
            console.log(prefix, message, data || '');
        }
    }
}

// Export for use in other scripts
if (typeof module !== 'undefined' && module.exports) {
    module.exports = WebRTCEngine;
}
