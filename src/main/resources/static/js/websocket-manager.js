/**
 * WebSocketManager - Singleton WebSocket Manager
 * Shares a single WebSocket connection per browser for multiple streams
 */

class WebSocketManager {
    constructor() {
        console.log('[WebSocketManager] Constructor called');

        if (WebSocketManager.instance) {
            console.log('[WebSocketManager] Returning existing singleton instance');
            return WebSocketManager.instance;
        }

        this.instanceId = Math.random().toString(36).substring(7);
        WebSocketManager.instance = this;

        this.ws = null;
        this.serverUrl = `ws://${window.location.host}/ws/signaling`;
        this.connected = false;
        this.reconnecting = false;
        this.reconnectDelay = 3000;
        this.reconnectTimer = null;

        // Stream-specific handlers
        this.streamHandlers = new Map(); // streamId -> handlers

        // Global handlers
        this.globalHandlers = {
            'open': [],
            'close': [],
            'error': []
        };

        console.log('[WebSocketManager] WebSocketManager singleton initialized');
        console.log('[WebSocketManager] Server URL:', this.serverUrl);
    }

    /**
     * Get singleton instance
     */
    static getInstance() {
        if (!WebSocketManager.instance) {
            WebSocketManager.instance = new WebSocketManager();
        }
        return WebSocketManager.instance;
    }

    /**
     * Connect WebSocket
     */
    async connect() {
        console.log('[WebSocketManager] connect() called');

        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.log('Already connected');
            setTimeout(() => this.emit('open'), 0);
            return Promise.resolve();
        }

        if (this.ws && this.ws.readyState === WebSocket.CONNECTING) {
            this.log('Connection in progress, waiting...');
            return this.waitForConnection();
        }

        return new Promise((resolve, reject) => {
            this.log('Connecting to WebSocket:', this.serverUrl);
            this.ws = new WebSocket(this.serverUrl);

            const timeout = setTimeout(() => {
                this.log('WebSocket connection timeout', null, 'error');
                reject(new Error('WebSocket connection timeout'));
            }, 10000);

            this.ws.onopen = () => {
                clearTimeout(timeout);
                this.connected = true;
                this.reconnecting = false;
                console.log('[WebSocketManager] WebSocket connected');
                this.log('WebSocket connected successfully');
                this.emit('open');
                resolve();
            };

            this.ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);
                    this.handleMessage(message);
                } catch (error) {
                    this.log('Failed to parse message:', error, 'error');
                }
            };

            this.ws.onerror = (error) => {
                clearTimeout(timeout);
                console.log('[WebSocketManager] WebSocket error:', error);
                this.log('WebSocket error:', error, 'error');
                this.emit('error', error);
                reject(error);
            };

            this.ws.onclose = () => {
                this.connected = false;
                console.log('[WebSocketManager] WebSocket closed');
                this.log('WebSocket closed');
                this.emit('close');

                // Auto reconnect if there are active stream handlers
                if (this.streamHandlers.size > 0 && !this.reconnecting) {
                    this.scheduleReconnect();
                }
            };
        });
    }

    /**
     * Wait for connection
     */
    waitForConnection() {
        return new Promise((resolve, reject) => {
            const checkInterval = setInterval(() => {
                if (this.ws.readyState === WebSocket.OPEN) {
                    clearInterval(checkInterval);
                    resolve();
                } else if (this.ws.readyState === WebSocket.CLOSED) {
                    clearInterval(checkInterval);
                    reject(new Error('WebSocket connection failed'));
                }
            }, 100);

            setTimeout(() => {
                clearInterval(checkInterval);
                reject(new Error('Connection wait timeout'));
            }, 10000);
        });
    }

    /**
     * Schedule reconnect
     */
    scheduleReconnect() {
        if (this.reconnecting) return;

        this.reconnecting = true;
        this.log(`Reconnecting in ${this.reconnectDelay}ms...`);

        this.reconnectTimer = setTimeout(() => {
            this.connect().catch(err => {
                this.log('Reconnection failed:', err, 'error');
            });
        }, this.reconnectDelay);
    }

    /**
     * Handle incoming message
     */
    handleMessage(message) {
        const { type, streamId, payload, sdp } = message;
        this.log(`Received message: ${type} for stream: ${streamId || 'N/A'}`);

        // Stream-specific handler
        if (streamId && this.streamHandlers.has(streamId)) {
            const handlers = this.streamHandlers.get(streamId);

            if (handlers[type]) {
                handlers[type].forEach(callback => {
                    try {
                        // For answer type, pass sdp directly
                        if (type === 'answer') {
                            callback(sdp || payload);
                        } else {
                            callback(payload);
                        }
                    } catch (error) {
                        this.log(`Handler error for ${type}:`, error, 'error');
                    }
                });
            } else {
                this.log(`No handler for ${type} on ${streamId}`, null, 'warn');
            }
        } else {
            this.log(`No handlers registered for stream: ${streamId}`, null, 'warn');
        }
    }

    /**
     * Register stream handlers
     */
    registerStream(streamId, handlers) {
        if (!this.streamHandlers.has(streamId)) {
            this.streamHandlers.set(streamId, {});
        }

        const streamHandler = this.streamHandlers.get(streamId);

        for (const [event, callback] of Object.entries(handlers)) {
            if (!streamHandler[event]) {
                streamHandler[event] = [];
            }
            streamHandler[event].push(callback);
        }

        this.log(`Stream handlers registered for: ${streamId} (events: ${Object.keys(handlers).join(', ')})`);
        this.log(`Total streams managed: ${this.streamHandlers.size}`);
    }

    /**
     * Unregister stream handlers
     */
    unregisterStream(streamId) {
        this.streamHandlers.delete(streamId);
        this.log(`Stream handlers unregistered: ${streamId}`);
        this.log(`Remaining streams: ${this.streamHandlers.size}`);

        // Close connection if all streams are removed
        if (this.streamHandlers.size === 0) {
            this.log('All streams disconnected, closing WebSocket');
            this.disconnect();
        }
    }

    /**
     * Register global event listener
     */
    on(event, callback) {
        if (!this.globalHandlers[event]) {
            throw new Error(`Unknown event: ${event}`);
        }
        this.globalHandlers[event].push(callback);
        return this;
    }

    /**
     * Emit global event
     */
    emit(event, data) {
        if (this.globalHandlers[event]) {
            this.globalHandlers[event].forEach(callback => {
                try {
                    callback(data);
                } catch (error) {
                    this.log('Event handler error:', error, 'error');
                }
            });
        }
    }

    /**
     * Send message
     */
    send(type, streamId, payload) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            this.log('Cannot send message - WebSocket not connected', 'error');
            throw new Error('WebSocket not connected');
        }

        // Construct message in the format expected by Kotlin backend
        const message = {
            type: type,
            streamId: streamId,
            ...payload
        };

        this.ws.send(JSON.stringify(message));
        this.log(`Message sent: ${type} for stream ${streamId}`);
    }

    /**
     * Disconnect
     */
    disconnect() {
        this.log('Disconnecting...');

        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }

        this.reconnecting = false;

        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }

        this.connected = false;
    }

    /**
     * Check connection status
     */
    isConnected() {
        return this.connected && this.ws && this.ws.readyState === WebSocket.OPEN;
    }

    /**
     * Log output
     */
    log(message, data, level = 'info') {
        const prefix = '[WebSocketManager]';

        if (level === 'error') {
            console.error(prefix, message, data || '');
        } else if (level === 'warn') {
            console.warn(prefix, message, data || '');
        } else {
            console.log(prefix, message, data || '');
        }
    }
}

// Export global singleton instance
if (typeof window !== 'undefined') {
    window.WebSocketManager = WebSocketManager;
}
