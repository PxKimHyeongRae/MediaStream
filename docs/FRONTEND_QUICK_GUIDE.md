# 프론트엔드 Quick Guide - WebRTC & HLS 스트리밍

> 웹 브라우저에서 CCTV 스트림을 WebRTC와 HLS로 재생하는 방법을 안내합니다.

## 📋 목차

1. [시작하기](#시작하기)
2. [WebRTC 스트리밍](#webrtc-스트리밍)
3. [HLS 스트리밍](#hls-스트리밍)
4. [듀얼 플레이어 (WebRTC + HLS)](#듀얼-플레이어-webrtc--hls)
5. [트러블슈팅](#트러블슈팅)

---

## 시작하기

### 프로토콜 비교

| 특징 | WebRTC | HLS |
|-----|--------|-----|
| **지연시간** | 매우 낮음 (~500ms) | 중간 (~3-6초) |
| **브라우저 지원** | Chrome, Edge, Firefox | 모든 브라우저 |
| **코덱 지원** | H.264, H.265 (브라우저별 상이) | H.264만 (MPEG-TS variant) |
| **네트워크 효율** | UDP 기반, 적응형 | TCP 기반, 안정적 |
| **사용 사례** | 실시간 모니터링 | 녹화 재생, 안정성 우선 |

### 서버 URL

```
WebRTC: ws://[SERVER_IP]:8107/ws
HLS: http://[SERVER_IP]:8107/hls/[STREAM_ID]/index.m3u8
```

---

## WebRTC 스트리밍

### 기본 HTML 구조

```html
<!DOCTYPE html>
<html>
<head>
    <title>WebRTC Viewer</title>
</head>
<body>
    <video id="videoPlayer" autoplay playsinline muted></video>

    <!-- WebRTC Engine 라이브러리 -->
    <script src="/static/js/websocket-manager.js"></script>
    <script src="/static/js/webrtc-engine.js"></script>
</body>
</html>
```

### JavaScript 사용법

#### 1. 기본 연결

```javascript
// WebRTC 엔진 생성
const engine = new WebRTCEngine({
    streamId: 'CCTV-TEST',                    // 스트림 ID
    videoElement: document.getElementById('videoPlayer'),  // video 엘리먼트
    autoReconnect: true                        // 자동 재연결 활성화
});

// 연결
await engine.connect();
```

#### 2. 이벤트 핸들러

```javascript
// 연결 성공
engine.on('connected', () => {
    console.log('WebRTC 연결 성공!');
});

// 연결 해제
engine.on('disconnected', () => {
    console.log('WebRTC 연결 끊김');
});

// 에러 발생
engine.on('error', (error) => {
    console.error('WebRTC 에러:', error);
});

// 상태 변경
engine.on('statechange', (state) => {
    console.log('상태:', state); // connecting, connected, disconnected
});

// 통계 정보 (1초마다)
engine.on('stats', (stats) => {
    console.log('비트레이트:', stats.bitrate, 'kbps');
    console.log('패킷 수신:', stats.packetsReceived);
    console.log('패킷 손실:', stats.packetsLost);
});
```

#### 3. 연결 제어

```javascript
// 연결
await engine.connect();

// 연결 해제
engine.disconnect();

// 연결 상태 확인
if (engine.isConnected()) {
    console.log('연결됨');
}

// 통계 가져오기
const stats = engine.getStats();
console.log(stats.bitrate, stats.packetsReceived);
```

#### 4. 전체 예시

```javascript
const videoElement = document.getElementById('videoPlayer');

// WebRTC 엔진 생성
const engine = new WebRTCEngine({
    streamId: 'CCTV-TEST',
    videoElement: videoElement,
    autoReconnect: true
});

// 이벤트 등록
engine.on('connected', () => {
    console.log('✅ 연결됨');
    document.getElementById('status').textContent = '재생 중';
});

engine.on('stats', (stats) => {
    document.getElementById('bitrate').textContent = stats.bitrate.toFixed(1) + ' kbps';
    document.getElementById('packets').textContent = stats.packetsReceived;
});

engine.on('error', (error) => {
    console.error('❌ 에러:', error);
    document.getElementById('status').textContent = '에러: ' + error.message;
});

// 연결 시작
async function start() {
    try {
        await engine.connect();
    } catch (error) {
        console.error('연결 실패:', error);
    }
}

// 연결 중지
function stop() {
    engine.disconnect();
}

start();
```

---

## HLS 스트리밍

### 기본 HTML 구조

```html
<!DOCTYPE html>
<html>
<head>
    <title>HLS Viewer</title>
</head>
<body>
    <video id="videoPlayer" controls autoplay muted></video>

    <!-- HLS.js 라이브러리 (CDN) -->
    <script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
</body>
</html>
```

### JavaScript 사용법

#### 1. 기본 연결

```javascript
const videoElement = document.getElementById('videoPlayer');
const streamId = 'CCTV-TEST';
const hlsUrl = `/hls/${streamId}/index.m3u8`;

if (Hls.isSupported()) {
    // HLS.js 사용 (Chrome, Firefox, Edge 등)
    const hls = new Hls({
        debug: false,
        enableWorker: true,
        lowLatencyMode: true,
        backBufferLength: 90
    });

    hls.loadSource(hlsUrl);
    hls.attachMedia(videoElement);

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
        console.log('✅ HLS 로드 완료');
        videoElement.play();
    });

} else if (videoElement.canPlayType('application/vnd.apple.mpegurl')) {
    // Safari 네이티브 HLS 지원
    videoElement.src = hlsUrl;
    videoElement.addEventListener('loadedmetadata', () => {
        videoElement.play();
    });
}
```

#### 2. 이벤트 핸들러

```javascript
const hls = new Hls();

// Manifest 로드 완료
hls.on(Hls.Events.MANIFEST_PARSED, () => {
    console.log('Manifest 파싱 완료');
    videoElement.play();
});

// 레벨 변경 (품질 변경)
hls.on(Hls.Events.LEVEL_LOADED, (event, data) => {
    console.log('현재 레벨:', data.level);
});

// Fragment 로드 완료
hls.on(Hls.Events.FRAG_LOADED, (event, data) => {
    console.log('세그먼트 로드:', data.frag.sn);
});

// 에러 처리
hls.on(Hls.Events.ERROR, (event, data) => {
    console.error('HLS 에러:', data);

    if (data.fatal) {
        switch(data.type) {
            case Hls.ErrorTypes.NETWORK_ERROR:
                console.error('네트워크 에러 - 재시도 중...');
                hls.startLoad();
                break;

            case Hls.ErrorTypes.MEDIA_ERROR:
                console.error('미디어 에러 - 복구 시도 중...');
                hls.recoverMediaError();
                break;

            default:
                console.error('치명적 에러 - 중지됨');
                hls.destroy();
                break;
        }
    }
});
```

#### 3. 통계 정보

```javascript
// 버퍼 길이 (초)
setInterval(() => {
    if (videoElement.buffered.length > 0) {
        const bufferLength = videoElement.buffered.end(0) - videoElement.currentTime;
        console.log('버퍼:', bufferLength.toFixed(2), '초');
    }
}, 1000);

// 재생 시간
console.log('현재 시간:', videoElement.currentTime);
console.log('전체 길이:', videoElement.duration);

// 품질 레벨
console.log('사용 가능한 레벨:', hls.levels);
console.log('현재 레벨:', hls.currentLevel);

// 수동 품질 선택
hls.currentLevel = 0; // 첫 번째 레벨로 변경
hls.currentLevel = -1; // 자동 선택
```

#### 4. 전체 예시

```javascript
const videoElement = document.getElementById('videoPlayer');
const streamId = 'CCTV-TEST';
const hlsUrl = `/hls/${streamId}/index.m3u8`;

if (Hls.isSupported()) {
    const hls = new Hls({
        debug: false,
        enableWorker: true,
        lowLatencyMode: true,
        backBufferLength: 90
    });

    hls.loadSource(hlsUrl);
    hls.attachMedia(videoElement);

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
        console.log('✅ HLS 준비 완료');
        videoElement.play();
        document.getElementById('status').textContent = '재생 중';
    });

    hls.on(Hls.Events.ERROR, (event, data) => {
        console.error('❌ HLS 에러:', data);

        if (data.fatal) {
            switch(data.type) {
                case Hls.ErrorTypes.NETWORK_ERROR:
                    document.getElementById('status').textContent = '네트워크 에러 - 재시도 중...';
                    hls.startLoad();
                    break;

                case Hls.ErrorTypes.MEDIA_ERROR:
                    document.getElementById('status').textContent = '미디어 에러 - 복구 중...';
                    hls.recoverMediaError();
                    break;

                default:
                    document.getElementById('status').textContent = '치명적 에러';
                    hls.destroy();
                    break;
            }
        }
    });

    // 버퍼 상태 표시
    setInterval(() => {
        if (videoElement.buffered.length > 0) {
            const buffer = videoElement.buffered.end(0) - videoElement.currentTime;
            document.getElementById('buffer').textContent = buffer.toFixed(1) + 's';
        }
    }, 1000);

    // 정리 함수
    window.stopHLS = function() {
        hls.destroy();
        videoElement.src = '';
    };
}
```

---

## 듀얼 플레이어 (WebRTC + HLS)

### HTML 구조

```html
<!DOCTYPE html>
<html>
<head>
    <title>Dual Player - WebRTC + HLS</title>
    <style>
        .dual-container {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 10px;
        }
        .player-section {
            position: relative;
        }
        .player-label {
            position: absolute;
            top: 10px;
            left: 10px;
            background: rgba(0, 0, 0, 0.7);
            color: white;
            padding: 5px 10px;
            border-radius: 4px;
            z-index: 10;
        }
        video {
            width: 100%;
            height: auto;
        }
    </style>
</head>
<body>
    <div class="dual-container">
        <!-- WebRTC Player -->
        <div class="player-section">
            <div class="player-label">WebRTC (Low Latency)</div>
            <video id="webrtc-video" autoplay playsinline muted></video>
            <div id="webrtc-stats"></div>
        </div>

        <!-- HLS Player -->
        <div class="player-section">
            <div class="player-label">HLS (Stable)</div>
            <video id="hls-video" controls autoplay muted></video>
            <div id="hls-stats"></div>
        </div>
    </div>

    <!-- 라이브러리 -->
    <script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
    <script src="/static/js/websocket-manager.js"></script>
    <script src="/static/js/webrtc-engine.js"></script>
</body>
</html>
```

### JavaScript 전체 예시

```javascript
const streamId = 'CCTV-TEST';

// WebRTC 초기화
const webrtcVideo = document.getElementById('webrtc-video');
const webrtcEngine = new WebRTCEngine({
    streamId: streamId,
    videoElement: webrtcVideo,
    autoReconnect: true
});

webrtcEngine.on('connected', () => {
    console.log('✅ WebRTC 연결됨');
});

webrtcEngine.on('stats', (stats) => {
    document.getElementById('webrtc-stats').innerHTML = `
        <div>비트레이트: ${stats.bitrate.toFixed(1)} kbps</div>
        <div>패킷: ${stats.packetsReceived}</div>
    `;
});

webrtcEngine.connect();

// HLS 초기화
const hlsVideo = document.getElementById('hls-video');
const hlsUrl = `/hls/${streamId}/index.m3u8`;

if (Hls.isSupported()) {
    const hls = new Hls({
        debug: false,
        enableWorker: true,
        lowLatencyMode: true
    });

    hls.loadSource(hlsUrl);
    hls.attachMedia(hlsVideo);

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
        console.log('✅ HLS 로드 완료');
        hlsVideo.play();
    });

    hls.on(Hls.Events.ERROR, (event, data) => {
        console.error('❌ HLS 에러:', data);
        if (data.fatal) {
            switch(data.type) {
                case Hls.ErrorTypes.NETWORK_ERROR:
                    hls.startLoad();
                    break;
                case Hls.ErrorTypes.MEDIA_ERROR:
                    hls.recoverMediaError();
                    break;
                default:
                    hls.destroy();
                    break;
            }
        }
    });

    // 버퍼 상태 표시
    setInterval(() => {
        if (hlsVideo.buffered.length > 0) {
            const buffer = hlsVideo.buffered.end(0) - hlsVideo.currentTime;
            document.getElementById('hls-stats').innerHTML = `
                <div>버퍼: ${buffer.toFixed(1)}s</div>
            `;
        }
    }, 1000);
}
```

---

## 트러블슈팅

### WebRTC 문제

#### 1. "ICE connection failed"

**원인**: ICE 연결 실패 (방화벽, NAT 문제)

**해결**:
```javascript
// STUN 서버 설정 확인
// 서버측 config.yaml에서 ice_servers 확인

// 브라우저 콘솔에서 ICE 상태 확인
engine.on('statechange', (state) => {
    console.log('ICE State:', state);
});
```

#### 2. "No video playing"

**원인**: 코덱 미지원 (H.265를 Firefox에서 재생)

**해결**:
```javascript
// 스트림 코덱 확인
fetch(`/api/v1/streams/${streamId}`)
    .then(res => res.json())
    .then(data => {
        console.log('Codec:', data.codec);
        // Firefox는 H.264만 지원
    });
```

#### 3. "WebSocket connection failed"

**원인**: 서버 연결 실패, 잘못된 URL

**해결**:
```javascript
// URL 확인
console.log('Server URL:', engine.serverUrl);

// 수동으로 서버 URL 지정
const engine = new WebRTCEngine({
    streamId: 'CCTV-TEST',
    videoElement: videoElement,
    serverUrl: 'ws://192.168.10.181:8107/ws' // 명시적 지정
});
```

### HLS 문제

#### 1. "404 Not Found - playlist.m3u8"

**원인**: 스트림이 시작되지 않음

**해결**:
```javascript
// 스트림 시작 확인
fetch(`/api/v1/streams/${streamId}/start`, { method: 'POST' })
    .then(() => {
        // 1초 대기 후 HLS 로드
        setTimeout(() => {
            hls.loadSource(hlsUrl);
        }, 1000);
    });
```

#### 2. "H.265 not supported"

**원인**: MPEG-TS variant는 H.264만 지원

**해결**:
```
- H.265 스트림은 HLS로 재생 불가 (현재 구성)
- WebRTC로 재생하거나 fMP4 variant 사용 필요
```

#### 3. "Buffer stalls frequently"

**원인**: 네트워크 불안정, 세그먼트 크기 문제

**해결**:
```javascript
// HLS 설정 조정
const hls = new Hls({
    maxBufferLength: 30,        // 최대 버퍼 (초)
    maxMaxBufferLength: 600,    // 최대 최대 버퍼 (초)
    maxBufferSize: 60 * 1000 * 1000, // 60MB
    maxBufferHole: 0.5          // 버퍼 홀 허용 (초)
});
```

### 일반 문제

#### 1. "CORS Error"

**원인**: Cross-Origin 요청 차단

**해결**:
```
- 서버에서 CORS 헤더 설정 (이미 설정됨)
- 같은 도메인/포트에서 접속
```

#### 2. "Autoplay blocked"

**원인**: 브라우저 자동재생 정책

**해결**:
```javascript
// muted 속성 추가
<video autoplay muted playsinline></video>

// 또는 사용자 인터랙션 후 재생
button.addEventListener('click', () => {
    videoElement.play();
});
```

---

## 참고 자료

### 내부 문서
- [API.md](./API.md) - 전체 API 문서
- [HLS_IMPLEMENTATION.md](./HLS_IMPLEMENTATION.md) - HLS 구현 상세

### 예제 페이지
- `/static/viewer.html` - WebRTC 단일 스트림 뷰어
- `/static/hls-viewer.html` - HLS 단일 스트림 뷰어
- `/static/dashboard.html` - WebRTC + HLS 듀얼 뷰어

### 외부 문서
- [WebRTC API - MDN](https://developer.mozilla.org/en-US/docs/Web/API/WebRTC_API)
- [HLS.js Documentation](https://github.com/video-dev/hls.js/)
- [HTML5 Video - MDN](https://developer.mozilla.org/en-US/docs/Web/HTML/Element/video)

---

**마지막 업데이트**: 2025-11-18
**버전**: v0.2.1
