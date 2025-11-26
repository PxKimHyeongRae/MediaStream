/**
 * WebRTCEngine - Reusable WebRTC client library
 * Shares a single WebSocket connection per browser for multiple streams
 *
 * @example
 * const engine = new WebRTCEngine({
 *   streamId: 'camera_01',
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
        // Required parameter validation
        if (!config.videoElement) {
            throw new Error('videoElement is required');
        }
        if (!config.streamId) {
            throw new Error('streamId is required');
        }

        // Configuration
        this.streamId = config.streamId;
        this.videoElement = config.videoElement;
        this.autoReconnect = config.autoReconnect !== undefined ? config.autoReconnect : true;
        this.reconnectDelay = config.reconnectDelay || 3000;

        // Use shared WebSocket manager
        console.log(`[WebRTCEngine:${this.streamId}] Getting WebSocketManager instance...`);
        this.wsManager = WebSocketManager.getInstance();

        // State
        this.pc = null;
        this.connected = false;
        this.reconnecting = false;
        this.reconnectTimer = null;

        // Stats
        this.stats = {
            packetsReceived: 0,
            bytesReceived: 0,
            bitrate: 0
        };
        this.lastBytesReceived = 0;
        this.statsInterval = null;

        // Event handlers
        this.eventHandlers = {
            'connected': [],
            'disconnected': [],
            'error': [],
            'stats': [],
            'statechange': []
        };

        // Video element properties
        this.videoElement.autoplay = true;
        this.videoElement.playsinline = true;
        this.videoElement.muted = true;

        this.log(`WebRTCEngine initialized for stream: ${this.streamId}`);
    }

    /**
     * Register event listener
     */
    on(event, callback) {
        if (!this.eventHandlers[event]) {
            throw new Error(`Unknown event: ${event}`);
        }
        this.eventHandlers[event].push(callback);
        return this;
    }

    /**
     * Emit event
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
     * Start connection
     */
    async connect() {
        try {
            this.log('Connecting...');
            this.emit('statechange', 'connecting');

            // Connect to shared WebSocket (reuses if already connected)
            await this.connectWebSocket();

            // Create PeerConnection
            await this.createPeerConnection();

            // Create Offer
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
     * Connect WebSocket (uses shared manager)
     */
    async connectWebSocket() {
        this.log('Setting up WebSocket handlers...');

        // Register stream-specific handlers
        this.wsManager.registerStream(this.streamId, {
            'answer': (sdp) => this.handleAnswer(sdp),
            'ice': (payload) => this.handleICE(payload),
            'error': (payload) => {
                this.log('Server error:', payload, 'error');
                this.emit('error', new Error(payload));
            }
        });

        // Connect WebSocket (reuses if already connected)
        if (!this.wsManager.isConnected()) {
            this.log('WebSocket not connected, initiating connection...');
            await this.wsManager.connect();
            this.log('WebSocket connection established (shared)');
        } else {
            this.log('Reusing existing WebSocket connection');
        }

        this.log('WebSocket ready for stream:', this.streamId);
    }

    /**
     * Create PeerConnection
     */
    createPeerConnection() {
        const config = {
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ]
        };

        this.pc = new RTCPeerConnection(config);

        // Event handlers
        this.pc.onicecandidate = (event) => {
            if (event.candidate) {
                this.log('ICE candidate:', event.candidate.candidate);
                this.sendMessage('candidate', { candidate: JSON.stringify(event.candidate) });
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

        // Add Transceiver
        this.pc.addTransceiver('video', { direction: 'recvonly' });

        this.log('PeerConnection created');
    }

    /**
     * Create and send Offer
     */
    async createOffer() {
        const offer = await this.pc.createOffer({
            offerToReceiveVideo: true,
            offerToReceiveAudio: false
        });

        await this.pc.setLocalDescription(offer);
        this.log('Local description set');

        // Send Offer (include streamId)
        this.sendMessage('offer', {
            sdp: offer.sdp,
            streamId: this.streamId
        });
    }

    /**
     * Send message
     */
    sendMessage(type, payload) {
        try {
            this.wsManager.send(type, this.streamId, payload);
        } catch (error) {
            this.log('Failed to send message:', error, 'error');
        }
    }

    /**
     * Handle Answer
     */
    async handleAnswer(sdp) {
        try {
            this.log('Received SDP Answer');
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
     * Handle ICE Candidate
     */
    async handleICE(candidate) {
        try {
            await this.pc.addIceCandidate(new RTCIceCandidate(candidate));
        } catch (error) {
            this.log('Failed to add ICE candidate:', error, 'error');
        }
    }

    /**
     * Handle disconnect
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
     * Schedule reconnect
     */
    scheduleReconnect() {
        this.reconnecting = true;
        this.log(`Reconnecting in ${this.reconnectDelay}ms...`);

        this.reconnectTimer = setTimeout(() => {
            this.disconnect(false); // Full cleanup
            this.connect(); // Reconnect
        }, this.reconnectDelay);
    }

    /**
     * Start stats update
     */
    startStatsUpdate() {
        this.statsInterval = setInterval(() => this.updateStats(), 1000);
    }

    /**
     * Stop stats update
     */
    stopStatsUpdate() {
        if (this.statsInterval) {
            clearInterval(this.statsInterval);
            this.statsInterval = null;
        }
    }

    /**
     * Update stats
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

            // Calculate bitrate
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
     * Get stats
     */
    getStats() {
        return this.stats;
    }

    /**
     * Check connection status
     */
    isConnected() {
        return this.connected;
    }

    /**
     * Disconnect
     */
    disconnect(cleanup = true) {
        this.log('Disconnecting...');

        // Cancel reconnect timer
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }

        // Disable auto reconnect
        if (cleanup) {
            this.autoReconnect = false;
        }

        this.stopStatsUpdate();

        if (this.pc) {
            this.pc.close();
            this.pc = null;
        }

        // Unregister WebSocket handlers (managed by shared manager)
        if (cleanup) {
            this.wsManager.unregisterStream(this.streamId);
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
     * Log output
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
