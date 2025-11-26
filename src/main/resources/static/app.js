// WebSocket 연결
let ws = null;
let pc = null; // RTCPeerConnection
let currentStreamId = null;
let statsInterval = null;

// DOM 요소
const statusDiv = document.getElementById('status');
const connectionIndicator = document.getElementById('connectionIndicator');
const logDiv = document.getElementById('log');
const video = document.getElementById('video');
const videoPlaceholder = document.getElementById('videoPlaceholder');
const streamSelect = document.getElementById('streamSelect');
const streamStats = document.getElementById('streamStats');
const autoPlayCheckbox = document.getElementById('autoPlay');

// 로그 함수
function log(message, type = 'info') {
    const timestamp = new Date().toLocaleTimeString();
    const entry = document.createElement('div');
    entry.className = `log-entry ${type}`;
    entry.innerHTML = `<span class="timestamp">[${timestamp}]</span> ${message}`;
    logDiv.appendChild(entry);
    logDiv.scrollTop = logDiv.scrollHeight;
    console.log(`[${type.toUpperCase()}]`, message);
}

// 상태 업데이트
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

// WebSocket 연결
function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/signaling`;

    log(`WebSocket 연결 시도: ${wsUrl}`);
    updateStatus('WebSocket 연결 중...', 'info');

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
        updateStatus('WebSocket 연결됨', 'success');
        log('WebSocket 연결 성공', 'success');
    };

    ws.onmessage = async (event) => {
        try {
            const msg = JSON.parse(event.data);
            log(`수신: ${msg.type}`, 'info');

            switch (msg.type) {
                case 'welcome':
                    log(`서버: ${msg.message}`, 'success');
                    break;

                case 'answer':
                    if (pc && msg.sdp) {
                        // RTCPeerConnection이 have-local-offer 상태일 때만 answer 설정 가능
                        if (pc.signalingState === 'have-local-offer') {
                            log('SDP Answer 수신, 설정 중...', 'info');
                            await pc.setRemoteDescription(new RTCSessionDescription({
                                type: 'answer',
                                sdp: msg.sdp
                            }));
                            log('SDP Answer 설정 완료', 'success');
                        } else {
                            log(`SDP Answer 무시 (상태: ${pc.signalingState})`, 'warning');
                        }
                    }
                    break;

                case 'candidate_ack':
                    log('ICE Candidate 확인됨', 'info');
                    break;

                case 'error':
                    updateStatus(`에러: ${msg.message}`, 'error');
                    break;

                default:
                    log(`알 수 없는 메시지: ${msg.type}`, 'warning');
            }
        } catch (e) {
            log(`메시지 처리 에러: ${e.message}`, 'error');
        }
    };

    ws.onerror = (error) => {
        updateStatus('WebSocket 에러', 'error');
        log(`WebSocket 에러`, 'error');
    };

    ws.onclose = () => {
        updateStatus('WebSocket 연결 끊김', 'warning');
        log('WebSocket 연결 종료', 'warning');
        // 3초 후 재연결 시도
        setTimeout(connectWebSocket, 3000);
    };
}

// WebSocket 메시지 전송
function sendMessage(msg) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(msg));
        log(`전송: ${msg.type}`, 'info');
    } else {
        log('WebSocket이 연결되지 않았습니다', 'error');
    }
}

// 스트림 목록 새로고침
async function refreshStreams() {
    try {
        const response = await fetch('/api/v1/streams');
        const data = await response.json();

        log(`활성 스트림: ${data.totalStreams}개`, 'info');

        // select 옵션 업데이트
        const currentValue = streamSelect.value;
        streamSelect.innerHTML = '<option value="">-- 스트림 선택 --</option>';

        if (data.rtspClients && data.rtspClients.length > 0) {
            data.rtspClients.forEach(streamId => {
                const option = document.createElement('option');
                option.value = streamId;
                option.textContent = `${streamId} (RTSP 연결됨)`;
                streamSelect.appendChild(option);
            });

            // 이전 선택값 복원 또는 첫 번째 스트림 자동 선택
            if (currentValue && data.rtspClients.includes(currentValue)) {
                streamSelect.value = currentValue;
            } else if (data.rtspClients.length > 0 && !currentStreamId) {
                streamSelect.value = data.rtspClients[0];
                onStreamSelect();
            }
        }

        // 스트림 목록만 있고 RTSP 클라이언트가 없는 경우
        if (data.streams && data.streams.length > 0) {
            data.streams.forEach(streamId => {
                if (!data.rtspClients || !data.rtspClients.includes(streamId)) {
                    const option = document.createElement('option');
                    option.value = streamId;
                    option.textContent = `${streamId} (대기 중)`;
                    streamSelect.appendChild(option);
                }
            });
        }

    } catch (e) {
        log(`스트림 목록 조회 에러: ${e.message}`, 'error');
    }
}

// 스트림 선택 시
function onStreamSelect() {
    const streamId = streamSelect.value;

    if (streamId) {
        currentStreamId = streamId;
        document.getElementById('streamId').value = streamId;
        log(`스트림 선택: ${streamId}`, 'info');

        // 통계 표시
        streamStats.classList.add('visible');
        updateStreamStats(streamId);

        // 자동 재생
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

// 선택된 스트림 재생
function playSelectedStream() {
    const streamId = streamSelect.value;
    if (!streamId) {
        updateStatus('스트림을 선택하세요', 'warning');
        return;
    }

    currentStreamId = streamId;
    startWebRTC();
}

// 스트림 통계 업데이트
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
        // 무시
    }
}

// RTSP 스트림 시작
async function startRTSP() {
    const streamId = document.getElementById('streamId').value.trim();
    const rtspUrl = document.getElementById('rtspUrl').value.trim();

    if (!streamId || !rtspUrl) {
        updateStatus('스트림 ID와 RTSP URL을 입력하세요', 'warning');
        return;
    }

    try {
        log(`RTSP 스트림 시작: ${streamId}`, 'info');

        const response = await fetch(`/api/v1/streams/${streamId}/start`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url: rtspUrl })
        });

        const result = await response.json();

        if (response.ok) {
            updateStatus(`RTSP 스트림 시작됨: ${streamId}`, 'success');
            currentStreamId = streamId;
            await refreshStreams();

            // select에서 해당 스트림 선택
            streamSelect.value = streamId;
            onStreamSelect();
        } else {
            updateStatus(`RTSP 시작 실패: ${result.message}`, 'error');
        }
    } catch (e) {
        updateStatus(`RTSP 시작 에러: ${e.message}`, 'error');
    }
}

// RTSP 스트림 중지
async function stopRTSP() {
    const streamId = document.getElementById('streamId').value.trim() || currentStreamId;

    if (!streamId) {
        updateStatus('스트림 ID를 입력하세요', 'warning');
        return;
    }

    try {
        log(`RTSP 스트림 중지: ${streamId}`, 'info');

        const response = await fetch(`/api/v1/streams/${streamId}/stop`, {
            method: 'POST'
        });

        const result = await response.json();

        if (response.ok) {
            updateStatus(`RTSP 스트림 중지됨: ${streamId}`, 'success');
            await refreshStreams();
        } else {
            updateStatus(`RTSP 중지 실패: ${result.message}`, 'error');
        }
    } catch (e) {
        updateStatus(`RTSP 중지 에러: ${e.message}`, 'error');
    }
}

// 연결 중 상태 플래그
let isConnecting = false;

// WebRTC 연결 시작
async function startWebRTC() {
    if (!currentStreamId) {
        updateStatus('먼저 스트림을 선택하세요', 'warning');
        return;
    }

    // 이미 연결 중이면 무시 (중복 offer 방지)
    if (isConnecting) {
        log('이미 연결 중입니다. 중복 요청 무시.', 'warning');
        return;
    }

    if (pc) {
        log('기존 WebRTC 연결 종료 중...', 'info');
        stopWebRTC();
    }

    isConnecting = true;

    try {
        log(`WebRTC 연결 시작: ${currentStreamId}`, 'info');
        updateStatus('WebRTC 연결 중...', 'info');

        // RTCPeerConnection 생성
        pc = new RTCPeerConnection({
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ]
        });

        // ICE Candidate 처리
        pc.onicecandidate = (event) => {
            if (event.candidate) {
                log(`ICE Candidate 생성`, 'info');
                sendMessage({
                    type: 'candidate',
                    streamId: currentStreamId,
                    candidate: JSON.stringify(event.candidate)
                });
            }
        };

        // 트랙 수신
        pc.ontrack = (event) => {
            log(`비디오 트랙 수신됨`, 'success');
            video.srcObject = event.streams[0];
            videoPlaceholder.style.display = 'none';
            video.style.display = 'block';
            updateStatus(`재생 중: ${currentStreamId}`, 'success');
        };

        // 연결 상태 모니터링
        pc.onconnectionstatechange = () => {
            log(`WebRTC 상태: ${pc.connectionState}`, 'info');
            if (pc.connectionState === 'failed' || pc.connectionState === 'disconnected') {
                isConnecting = false;
                updateStatus('WebRTC 연결 실패', 'error');
            } else if (pc.connectionState === 'connected') {
                isConnecting = false;
                updateStatus(`연결됨: ${currentStreamId}`, 'success');
            }
        };

        // Transceiver 추가 (recvonly)
        pc.addTransceiver('video', { direction: 'recvonly' });

        // SDP Offer 생성
        log('SDP Offer 생성 중...', 'info');
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);

        log('SDP Offer 전송 중...', 'info');
        sendMessage({
            type: 'offer',
            streamId: currentStreamId,
            sdp: offer.sdp
        });

    } catch (e) {
        isConnecting = false;
        updateStatus(`WebRTC 에러: ${e.message}`, 'error');
        log(`WebRTC 에러: ${e.stack}`, 'error');
    }
}

// WebRTC 연결 종료
function stopWebRTC() {
    isConnecting = false;

    if (pc) {
        pc.close();
        pc = null;
        log('WebRTC 연결 종료', 'info');
    }

    video.srcObject = null;
    video.style.display = 'none';
    videoPlaceholder.style.display = 'flex';

    updateStatus('재생 중지됨', 'warning');
}

// 페이지 로드 시 초기화
window.addEventListener('load', () => {
    log('Media Server Client 시작', 'success');
    connectWebSocket();
    refreshStreams();

    // 3초마다 스트림 목록 및 통계 자동 갱신
    setInterval(() => {
        refreshStreams();
        if (currentStreamId) {
            updateStreamStats(currentStreamId);
        }
    }, 3000);
});

// 페이지 언로드 시 정리
window.addEventListener('beforeunload', () => {
    stopWebRTC();
    if (ws) {
        ws.close();
    }
    if (statsInterval) {
        clearInterval(statsInterval);
    }
});
