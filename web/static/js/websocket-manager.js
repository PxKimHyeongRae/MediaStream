/**
 * WebSocketManager - 싱글톤 WebSocket 관리자
 * 브라우저당 하나의 WebSocket 연결을 공유하여 여러 스트림 관리
 */

class WebSocketManager {
    constructor() {
        console.log('[WebSocketManager] 🔍 Constructor called');
        console.log('[WebSocketManager] 🔍 Existing instance?', !!WebSocketManager.instance);

        if (WebSocketManager.instance) {
            console.log('[WebSocketManager] 🔄 Returning existing singleton instance');
            console.log('[WebSocketManager] 🔍 Instance ID:', WebSocketManager.instance.instanceId);
            return WebSocketManager.instance;
        }

        this.instanceId = Math.random().toString(36).substring(7);
        WebSocketManager.instance = this;

        this.ws = null;
        this.serverUrl = `ws://${window.location.host}/ws`;
        this.connected = false;
        this.reconnecting = false;
        this.reconnectDelay = 3000;
        this.reconnectTimer = null;

        // 스트림별 핸들러 관리
        this.streamHandlers = new Map(); // streamId -> handlers

        // 전역 핸들러
        this.globalHandlers = {
            'open': [],
            'close': [],
            'error': []
        };

        console.log('[WebSocketManager] 🚀 WebSocketManager singleton initialized');
        console.log('[WebSocketManager] 🔍 Instance ID:', this.instanceId);
        console.log('[WebSocketManager] 🔍 Server URL:', this.serverUrl);
    }

    /**
     * 싱글톤 인스턴스 가져오기
     */
    static getInstance() {
        if (!WebSocketManager.instance) {
            WebSocketManager.instance = new WebSocketManager();
        }
        return WebSocketManager.instance;
    }

    /**
     * WebSocket 연결
     */
    async connect() {
        console.log('[WebSocketManager] 🔍 connect() called');
        console.log('[WebSocketManager] 🔍 Current ws state:', this.ws?.readyState);
        console.log('[WebSocketManager] 🔍 WebSocket.OPEN =', WebSocket.OPEN);
        console.log('[WebSocketManager] 🔍 WebSocket.CONNECTING =', WebSocket.CONNECTING);

        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.log('Already connected');
            // 이미 연결된 상태면 즉시 open 이벤트 발생
            setTimeout(() => this.emit('open'), 0);
            return Promise.resolve();
        }

        if (this.ws && this.ws.readyState === WebSocket.CONNECTING) {
            this.log('Connection in progress, waiting...');
            return this.waitForConnection();
        }

        return new Promise((resolve, reject) => {
            this.log('Connecting to WebSocket:', this.serverUrl);
            console.log('[WebSocketManager] 🔍 Creating new WebSocket instance');
            this.ws = new WebSocket(this.serverUrl);
            console.log('[WebSocketManager] 🔍 WebSocket instance created:', this.ws);

            const timeout = setTimeout(() => {
                this.log('WebSocket connection timeout', null, 'error');
                reject(new Error('WebSocket connection timeout'));
            }, 10000);

            this.ws.onopen = () => {
                clearTimeout(timeout);
                this.connected = true;
                this.reconnecting = false;
                console.log('[WebSocketManager] ✅ WebSocket.onopen fired');
                this.log('✅ WebSocket connected successfully');
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
                console.log('[WebSocketManager] ❌ WebSocket.onerror fired:', error);
                this.log('WebSocket error:', error, 'error');
                this.emit('error', error);
                reject(error);
            };

            this.ws.onclose = () => {
                this.connected = false;
                console.log('[WebSocketManager] 🔌 WebSocket.onclose fired');
                this.log('WebSocket closed');
                this.emit('close');

                // 스트림 핸들러가 남아있으면 자동 재연결
                if (this.streamHandlers.size > 0 && !this.reconnecting) {
                    this.scheduleReconnect();
                }
            };
        });
    }

    /**
     * 연결 대기
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
     * 재연결 스케줄링
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
     * 메시지 처리
     */
    handleMessage(message) {
        const { type, streamId, payload } = message;
        this.log(`📨 Received message: ${type} for stream: ${streamId || 'N/A'}`);

        // 스트림별 핸들러 호출
        if (streamId && this.streamHandlers.has(streamId)) {
            const handlers = this.streamHandlers.get(streamId);

            if (handlers[type]) {
                this.log(`🎯 Calling ${handlers[type].length} handler(s) for ${type} on ${streamId}`);
                handlers[type].forEach(callback => {
                    try {
                        callback(payload);
                    } catch (error) {
                        this.log(`Handler error for ${type}:`, error, 'error');
                    }
                });
            } else {
                this.log(`⚠️ No handler for ${type} on ${streamId}`, null, 'warn');
            }
        } else {
            this.log(`⚠️ No handlers registered for stream: ${streamId}`, null, 'warn');
        }
    }

    /**
     * 스트림 핸들러 등록
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

        this.log(`✅ Stream handlers registered for: ${streamId} (events: ${Object.keys(handlers).join(', ')})`);
        this.log(`📊 Total streams managed: ${this.streamHandlers.size}`);
    }

    /**
     * 스트림 핸들러 해제
     */
    unregisterStream(streamId) {
        this.streamHandlers.delete(streamId);
        this.log(`❌ Stream handlers unregistered: ${streamId}`);
        this.log(`📊 Remaining streams: ${this.streamHandlers.size}`);

        // 모든 스트림이 제거되면 연결 종료
        if (this.streamHandlers.size === 0) {
            this.log('🔌 All streams disconnected, closing WebSocket');
            this.disconnect();
        }
    }

    /**
     * 전역 이벤트 리스너 등록
     */
    on(event, callback) {
        if (!this.globalHandlers[event]) {
            throw new Error(`Unknown event: ${event}`);
        }
        this.globalHandlers[event].push(callback);
        return this;
    }

    /**
     * 전역 이벤트 발생
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
     * 메시지 전송
     */
    send(type, streamId, payload) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            this.log('Cannot send message - WebSocket not connected', 'error');
            throw new Error('WebSocket not connected');
        }

        const message = { type, streamId, payload };
        this.ws.send(JSON.stringify(message));
        this.log(`📤 Message sent: ${type} for stream ${streamId}`);
    }

    /**
     * 연결 해제
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
     * 연결 상태 확인
     */
    isConnected() {
        return this.connected && this.ws && this.ws.readyState === WebSocket.OPEN;
    }

    /**
     * 로그 출력
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

// 전역 싱글톤 인스턴스 export
if (typeof window !== 'undefined') {
    window.WebSocketManager = WebSocketManager;
}

