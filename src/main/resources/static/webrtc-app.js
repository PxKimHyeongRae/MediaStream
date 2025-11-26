// WebSocket connection
let ws = null;
let pc = null; // RTCPeerConnection
let currentStreamId = null;
let statsInterval = null;

// DOM elements
const statusDiv = document.getElementById('status');
const connectionIndicator = document.getElementById('connectionIndicator');
const logDiv = document.getElementById('log');
const video = document.getElementById('video');
const videoPlaceholder = document.getElementById('videoPlaceholder');
const streamSelect = document.getElementById('streamSelect');
const streamStats = document.getElementById('streamStats');
const autoPlayCheckbox = document.getElementById('autoPlay');

// Log function
function log(message, type = 'info') {
    const timestamp = new Date().toLocaleTimeString();
    const entry = document.createElement('div');
    entry.className = `log-entry ${type}`;
    entry.innerHTML = `<span class="timestamp">[${timestamp}]</span> ${message}`;
    logDiv.appendChild(entry);
    logDiv.scrollTop = logDiv.scrollHeight;
    console.log(`[${type.toUpperCase()}]`, message);
}

// Status update
function updateStatus(message, type = 'info') {
    statusDiv.innerHTML = `<span id="connectionIndicator" class="connection-indicator ${getIndicatorClass(type)}"></span>${message}`;
    statusDiv.className = `status ${type}`;
}

function getIndicatorClass(type) {
    switch(type) {
        case 'success': return 'connected';
        case 'info': return 'connecting';
        default: return 'disconnected';
    }
}

// WebSocket connection
function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/signaling`;

    log(`WebSocket connecting: ${wsUrl}`);
    updateStatus('WebSocket connecting...', 'info');

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
        updateStatus('WebSocket connected', 'success');
        log('WebSocket connected', 'success');
    };

    ws.onmessage = async (event) => {
        try {
            const msg = JSON.parse(event.data);
            log(`Received: ${msg.type}`, 'info');

            switch (msg.type) {
                case 'welcome':
                    log(`Server: ${msg.message}`, 'success');
                    break;

                case 'answer':
                    if (pc && msg.sdp) {
                        // Can only set answer when RTCPeerConnection is in have-local-offer state
                        if (pc.signalingState === 'have-local-offer') {
                            log('SDP Answer received, setting...', 'info');
                            await pc.setRemoteDescription(new RTCSessionDescription({
                                type: 'answer',
                                sdp: msg.sdp
                            }));
                            log('SDP Answer set', 'success');
                        } else {
                            log(`SDP Answer ignored (state: ${pc.signalingState})`, 'warning');
                        }
                    }
                    break;

                case 'candidate_ack':
                    log('ICE Candidate acknowledged', 'info');
                    break;

                case 'error':
                    updateStatus(`Error: ${msg.message}`, 'error');
                    break;

                default:
                    log(`Unknown message: ${msg.type}`, 'warning');
            }
        } catch (e) {
            log(`Message processing error: ${e.message}`, 'error');
        }
    };

    ws.onerror = (error) => {
        updateStatus('WebSocket error', 'error');
        log(`WebSocket error`, 'error');
    };

    ws.onclose = () => {
        updateStatus('WebSocket disconnected', 'warning');
        log('WebSocket closed', 'warning');
        // Reconnect after 3 seconds
        setTimeout(connectWebSocket, 3000);
    };
}

// Send WebSocket message
function sendMessage(msg) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(msg));
        log(`Sent: ${msg.type}`, 'info');
    } else {
        log('WebSocket not connected', 'error');
    }
}

// Refresh stream list
async function refreshStreams() {
    try {
        const response = await fetch('/api/v1/streams');
        const data = await response.json();

        log(`Active streams: ${data.totalStreams}`, 'info');

        // Update select options
        const currentValue = streamSelect.value;
        streamSelect.innerHTML = '<option value="">-- Select Stream --</option>';

        if (data.rtspClients && data.rtspClients.length > 0) {
            data.rtspClients.forEach(streamId => {
                const option = document.createElement('option');
                option.value = streamId;
                option.textContent = `${streamId} (RTSP connected)`;
                streamSelect.appendChild(option);
            });

            // Restore previous selection or auto-select first stream
            if (currentValue && data.rtspClients.includes(currentValue)) {
                streamSelect.value = currentValue;
            } else if (data.rtspClients.length > 0 && !currentStreamId) {
                streamSelect.value = data.rtspClients[0];
                onStreamSelect();
            }
        }

        // Streams without RTSP clients
        if (data.streams && data.streams.length > 0) {
            data.streams.forEach(streamId => {
                if (!data.rtspClients || !data.rtspClients.includes(streamId)) {
                    const option = document.createElement('option');
                    option.value = streamId;
                    option.textContent = `${streamId} (waiting)`;
                    streamSelect.appendChild(option);
                }
            });
        }

    } catch (e) {
        log(`Stream list error: ${e.message}`, 'error');
    }
}

// On stream select
function onStreamSelect() {
    const streamId = streamSelect.value;

    if (streamId) {
        currentStreamId = streamId;
        document.getElementById('streamId').value = streamId;
        log(`Stream selected: ${streamId}`, 'info');

        // Show stats
        streamStats.classList.add('visible');
        updateStreamStats(streamId);

        // Auto play
        if (autoPlayCheckbox.checked) {
            playSelectedStream();
        }
    } else {
        currentStreamId = null;
        streamStats.classList.remove('visible');
        if (statsInterval) {
            clearInterval(statsInterval);
            statsInterval = null;
        }
    }
}

// Play selected stream
function playSelectedStream() {
    const streamId = streamSelect.value;
    if (!streamId) {
        updateStatus('Please select a stream', 'warning');
        return;
    }

    currentStreamId = streamId;
    startWebRTC();
}

// Update stream stats
async function updateStreamStats(streamId) {
    try {
        const response = await fetch(`/api/v1/streams/${streamId}`);
        if (!response.ok) return;

        const data = await response.json();

        document.getElementById('statPackets').textContent =
            data.stats.packetsPublished.toLocaleString();
        document.getElementById('statBitrate').textContent =
            data.stats.avgBitrateFormatted || '0 Mbps';
        document.getElementById('statUptime').textContent =
            Math.round(data.stats.uptimeSeconds) + 's';
        document.getElementById('statSubscribers').textContent =
            data.subscriberCount;

    } catch (e) {
        // Ignore
    }
}

// Start RTSP stream
async function startRTSP() {
    const streamId = document.getElementById('streamId').value.trim();
    const rtspUrl = document.getElementById('rtspUrl').value.trim();

    if (!streamId || !rtspUrl) {
        updateStatus('Please enter Stream ID and RTSP URL', 'warning');
        return;
    }

    try {
        log(`Starting RTSP stream: ${streamId}`, 'info');

        const response = await fetch(`/api/v1/streams/${streamId}/start`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url: rtspUrl })
        });

        const result = await response.json();

        if (response.ok) {
            updateStatus(`RTSP stream started: ${streamId}`, 'success');
            currentStreamId = streamId;
            await refreshStreams();

            // Select the stream
            streamSelect.value = streamId;
            onStreamSelect();
        } else {
            updateStatus(`RTSP start failed: ${result.message}`, 'error');
        }
    } catch (e) {
        updateStatus(`RTSP start error: ${e.message}`, 'error');
    }
}

// Stop RTSP stream
async function stopRTSP() {
    const streamId = document.getElementById('streamId').value.trim() || currentStreamId;

    if (!streamId) {
        updateStatus('Please enter Stream ID', 'warning');
        return;
    }

    try {
        log(`Stopping RTSP stream: ${streamId}`, 'info');

        const response = await fetch(`/api/v1/streams/${streamId}/stop`, {
            method: 'POST'
        });

        const result = await response.json();

        if (response.ok) {
            updateStatus(`RTSP stream stopped: ${streamId}`, 'success');
            await refreshStreams();
        } else {
            updateStatus(`RTSP stop failed: ${result.message}`, 'error');
        }
    } catch (e) {
        updateStatus(`RTSP stop error: ${e.message}`, 'error');
    }
}

// Connection in progress flag
let isConnecting = false;

// Start WebRTC connection
async function startWebRTC() {
    if (!currentStreamId) {
        updateStatus('Please select a stream first', 'warning');
        return;
    }

    // Ignore if already connecting (prevent duplicate offers)
    if (isConnecting) {
        log('Already connecting. Ignoring duplicate request.', 'warning');
        return;
    }

    if (pc) {
        log('Closing existing WebRTC connection...', 'info');
        stopWebRTC();
    }

    isConnecting = true;

    try {
        log(`Starting WebRTC connection: ${currentStreamId}`, 'info');
        updateStatus('WebRTC connecting...', 'info');

        // Create RTCPeerConnection
        pc = new RTCPeerConnection({
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ]
        });

        // ICE Candidate handling
        pc.onicecandidate = (event) => {
            if (event.candidate) {
                log(`ICE Candidate generated`, 'info');
                sendMessage({
                    type: 'candidate',
                    streamId: currentStreamId,
                    candidate: JSON.stringify(event.candidate)
                });
            }
        };

        // Track received
        pc.ontrack = (event) => {
            log(`Video track received`, 'success');
            video.srcObject = event.streams[0];
            videoPlaceholder.style.display = 'none';
            video.style.display = 'block';
            updateStatus(`Playing: ${currentStreamId}`, 'success');
        };

        // Connection state monitoring
        pc.onconnectionstatechange = () => {
            log(`WebRTC state: ${pc.connectionState}`, 'info');
            if (pc.connectionState === 'failed' || pc.connectionState === 'disconnected') {
                isConnecting = false;
                updateStatus('WebRTC connection failed', 'error');
            } else if (pc.connectionState === 'connected') {
                isConnecting = false;
                updateStatus(`Connected: ${currentStreamId}`, 'success');
            }
        };

        // Add Transceiver (recvonly)
        pc.addTransceiver('video', { direction: 'recvonly' });

        // Create SDP Offer
        log('Creating SDP Offer...', 'info');
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);

        log('Sending SDP Offer...', 'info');
        sendMessage({
            type: 'offer',
            streamId: currentStreamId,
            sdp: offer.sdp
        });

    } catch (e) {
        isConnecting = false;
        updateStatus(`WebRTC error: ${e.message}`, 'error');
        log(`WebRTC error: ${e.stack}`, 'error');
    }
}

// Stop WebRTC connection
function stopWebRTC() {
    isConnecting = false;

    if (pc) {
        pc.close();
        pc = null;
        log('WebRTC connection closed', 'info');
    }

    video.srcObject = null;
    video.style.display = 'none';
    videoPlaceholder.style.display = 'flex';

    updateStatus('Playback stopped', 'warning');
}

// Initialize on page load
window.addEventListener('load', () => {
    log('Media Server Client started', 'success');
    connectWebSocket();
    refreshStreams();

    // Auto refresh stream list and stats every 3 seconds
    setInterval(() => {
        refreshStreams();
        if (currentStreamId) {
            updateStreamStats(currentStreamId);
        }
    }, 3000);
});

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
    stopWebRTC();
    if (ws) {
        ws.close();
    }
    if (statsInterval) {
        clearInterval(statsInterval);
    }
});
