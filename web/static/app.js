// WebRTC Player Application

class WebRTCPlayer {
    constructor() {
        this.pc = null;
        this.ws = null;
        this.videoElement = document.getElementById('videoPlayer');
        this.connectBtn = document.getElementById('connectBtn');
        this.disconnectBtn = document.getElementById('disconnectBtn');
        this.statusElement = document.getElementById('connectionStatus');

        this.setupEventListeners();
        this.log('Application initialized');
    }

    setupEventListeners() {
        this.connectBtn.addEventListener('click', () => this.connect());
        this.disconnectBtn.addEventListener('click', () => this.disconnect());

        document.getElementById('clearLogsBtn').addEventListener('click', () => {
            document.getElementById('logContainer').innerHTML = '';
        });
    }

    async connect() {
        try {
            this.log('Connecting to signaling server...');
            this.updateStatus('connecting');

            // WebSocket 연결
            const wsUrl = `ws://${window.location.host}/ws`;
            this.ws = new WebSocket(wsUrl);

            this.ws.onopen = () => {
                this.log('WebSocket connected');
                this.createPeerConnection();
            };

            this.ws.onmessage = (event) => {
                console.log('[WS] Raw message received:', event.data);
                try {
                    const message = JSON.parse(event.data);
                    console.log('[WS] Parsed message:', message);
                    this.handleSignalingMessage(message);
                } catch (error) {
                    console.error('[WS] Failed to parse message:', error);
                    console.error('[WS] Raw data was:', event.data);
                }
            };

            this.ws.onerror = (error) => {
                this.log(`WebSocket error: ${error}`, 'error');
                this.updateStatus('error');
            };

            this.ws.onclose = () => {
                this.log('WebSocket closed');
                this.updateStatus('disconnected');
            };

        } catch (error) {
            this.log(`Connection error: ${error.message}`, 'error');
            this.updateStatus('error');
        }
    }

    createPeerConnection() {
        this.log('Creating peer connection...');

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
                console.log('[ICE] New candidate:', event.candidate);
                console.log('[ICE] Candidate string:', event.candidate.candidate);
                this.log(`ICE candidate: ${event.candidate.candidate}`);
                this.sendMessage('ice', event.candidate);
            } else {
                console.log('[ICE] All candidates gathered');
            }
        };

        this.pc.oniceconnectionstatechange = () => {
            this.log(`ICE connection state: ${this.pc.iceConnectionState}`);
            this.updateStat('statIceState', this.pc.iceConnectionState);

            if (this.pc.iceConnectionState === 'connected') {
                this.updateStatus('connected');
            } else if (this.pc.iceConnectionState === 'failed') {
                this.updateStatus('error');
            }
        };

        this.pc.onconnectionstatechange = () => {
            this.log(`Connection state: ${this.pc.connectionState}`);
            this.updateStat('statConnectionState', this.pc.connectionState);
        };

        this.pc.onsignalingstatechange = () => {
            this.log(`Signaling state: ${this.pc.signalingState}`);
            this.updateStat('statSignalingState', this.pc.signalingState);
        };

        this.pc.ontrack = (event) => {
            this.log('Received remote track');
            if (this.videoElement.srcObject !== event.streams[0]) {
                this.videoElement.srcObject = event.streams[0];
                this.log('Video stream attached');
            }
        };

        // Transceiver 추가 (recvonly)
        this.pc.addTransceiver('video', { direction: 'recvonly' });
        this.pc.addTransceiver('audio', { direction: 'recvonly' });

        // Offer 생성
        this.createOffer();
    }

    async createOffer() {
        try {
            console.log('[OFFER] Creating offer...');
            this.log('Creating offer...');

            const offer = await this.pc.createOffer({
                offerToReceiveVideo: true,
                offerToReceiveAudio: true
            });
            console.log('[OFFER] Offer created, type:', offer.type);
            console.log('[OFFER] SDP length:', offer.sdp.length);

            await this.pc.setLocalDescription(offer);
            console.log('[OFFER] Local description set');
            console.log('[OFFER] Signaling state:', this.pc.signalingState);
            this.log('Local description set');

            // Offer 전송
            console.log('[OFFER] Sending offer to server...');
            this.sendMessage('offer', offer.sdp);

        } catch (error) {
            console.error('[OFFER] Error:', error);
            this.log(`Failed to create offer: ${error.message}`, 'error');
        }
    }

    handleSignalingMessage(message) {
        console.log('[SIG] Handling message type:', message.type);
        this.log(`Received message: ${message.type}`);

        switch (message.type) {
            case 'answer':
                console.log('[SIG] Answer payload type:', typeof message.payload);
                console.log('[SIG] Answer payload:', message.payload);
                this.handleAnswer(message.payload);
                break;
            case 'ice':
                console.log('[SIG] ICE payload:', message.payload);
                this.handleICE(message.payload);
                break;
            case 'error':
                console.error('[SIG] Server error:', message.payload);
                this.log(`Server error: ${message.payload}`, 'error');
                break;
            default:
                console.warn('[SIG] Unknown message type:', message.type);
                this.log(`Unknown message type: ${message.type}`, 'warn');
        }
    }

    async handleAnswer(sdp) {
        try {
            console.log('[ANSWER] Received SDP type:', typeof sdp);
            console.log('[ANSWER] SDP length:', sdp ? sdp.length : 'null');
            console.log('[ANSWER] SDP preview (first 200 chars):', sdp ? sdp.substring(0, 200) : 'null');
            this.log('Setting remote description...');

            const answer = new RTCSessionDescription({
                type: 'answer',
                sdp: sdp
            });
            console.log('[ANSWER] Created RTCSessionDescription:', answer);

            console.log('[ANSWER] Current signaling state:', this.pc.signalingState);
            await this.pc.setRemoteDescription(answer);
            console.log('[ANSWER] Remote description set successfully');
            console.log('[ANSWER] New signaling state:', this.pc.signalingState);
            this.log('Remote description set');

        } catch (error) {
            console.error('[ANSWER] Error details:', error);
            console.error('[ANSWER] Error name:', error.name);
            console.error('[ANSWER] Error message:', error.message);
            console.error('[ANSWER] Error stack:', error.stack);
            this.log(`Failed to set remote description: ${error.message}`, 'error');
        }
    }

    handleICE(candidate) {
        try {
            this.pc.addIceCandidate(new RTCIceCandidate(candidate));
        } catch (error) {
            this.log(`Failed to add ICE candidate: ${error.message}`, 'error');
        }
    }

    sendMessage(type, payload) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            const message = { type, payload };
            console.log('[WS] Sending message:', message);
            const jsonString = JSON.stringify(message);
            console.log('[WS] JSON string to send:', jsonString);
            this.ws.send(jsonString);
        } else {
            console.error('[WS] Cannot send message - WebSocket not connected');
            this.log('WebSocket not connected', 'error');
        }
    }

    disconnect() {
        this.log('Disconnecting...');

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

        this.updateStatus('disconnected');
        this.connectBtn.disabled = false;
        this.disconnectBtn.disabled = true;
    }

    updateStatus(status) {
        const statusDot = this.statusElement.querySelector('.status-dot');
        const statusText = this.statusElement.querySelector('.status-text');

        statusDot.className = 'status-dot';

        switch (status) {
            case 'connecting':
                statusDot.classList.add('status-connecting');
                statusText.textContent = 'Connecting...';
                this.connectBtn.disabled = true;
                this.disconnectBtn.disabled = false;
                break;
            case 'connected':
                statusDot.classList.add('status-connected');
                statusText.textContent = 'Connected';
                this.connectBtn.disabled = true;
                this.disconnectBtn.disabled = false;
                this.startStatsUpdate();
                break;
            case 'disconnected':
                statusDot.classList.add('status-disconnected');
                statusText.textContent = 'Disconnected';
                this.connectBtn.disabled = false;
                this.disconnectBtn.disabled = true;
                this.stopStatsUpdate();
                break;
            case 'error':
                statusDot.classList.add('status-error');
                statusText.textContent = 'Error';
                this.connectBtn.disabled = false;
                this.disconnectBtn.disabled = true;
                break;
        }
    }

    startStatsUpdate() {
        this.statsInterval = setInterval(() => this.updateStats(), 1000);
    }

    stopStatsUpdate() {
        if (this.statsInterval) {
            clearInterval(this.statsInterval);
            this.statsInterval = null;
        }
    }

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

            this.updateStat('statBytes', this.formatBytes(bytesReceived));
            this.updateStat('statPackets', packetsReceived.toLocaleString());

            // 비트레이트 계산 (간단한 구현)
            if (this.lastBytesReceived) {
                const bitrate = (bytesReceived - this.lastBytesReceived) * 8 / 1000; // kbps
                this.updateStat('statBitrate', `${bitrate.toFixed(2)} kbps`);
            }
            this.lastBytesReceived = bytesReceived;

        } catch (error) {
            console.error('Failed to get stats:', error);
        }
    }

    updateStat(elementId, value) {
        const element = document.getElementById(elementId);
        if (element) {
            element.textContent = value;
        }
    }

    formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    log(message, level = 'info') {
        const logContainer = document.getElementById('logContainer');
        const timestamp = new Date().toLocaleTimeString();
        const logEntry = document.createElement('div');
        logEntry.className = `log-entry log-${level}`;
        logEntry.textContent = `[${timestamp}] ${message}`;
        logContainer.appendChild(logEntry);
        logContainer.scrollTop = logContainer.scrollHeight;

        console.log(`[${level.toUpperCase()}] ${message}`);
    }
}

// 애플리케이션 시작
window.addEventListener('DOMContentLoaded', () => {
    new WebRTCPlayer();
});
