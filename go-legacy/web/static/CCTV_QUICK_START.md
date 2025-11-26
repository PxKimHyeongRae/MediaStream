# CCTV 스트림 연결 빠른 시작 가이드

## 목차
1. [5분 안에 시작하기](#5분-안에-시작하기)
2. [단일 CCTV 표출](#단일-cctv-표출)
3. [다중 CCTV 그리드](#다중-cctv-그리드)
4. [대시보드 예제](#대시보드-예제)
5. [모바일 최적화](#모바일-최적화)

---

## 5분 안에 시작하기

### Step 1: HTML 파일 생성

`my-cctv.html` 파일을 생성하고 다음 코드를 추가하세요:

```html
<!DOCTYPE html>
<html lang="ko">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>내 CCTV</title>
    <style>
        body {
            margin: 0;
            padding: 20px;
            font-family: Arial, sans-serif;
            background: #1a1a1a;
        }
        
        #cctv {
            width: 100%;
            max-width: 1280px;
            height: auto;
            background: #000;
            border-radius: 8px;
        }
        
        #status {
            margin-top: 10px;
            padding: 10px;
            background: #2a2a2a;
            color: #fff;
            border-radius: 4px;
        }
    </style>
</head>
<body>
    <h1 style="color: #fff;">CCTV 모니터링</h1>
    
    <!-- 비디오 플레이어 -->
    <video id="cctv" autoplay playsinline muted></video>
    
    <!-- 상태 표시 -->
    <div id="status">연결 중...</div>

    <!-- 필수 스크립트 -->
    <script src="/static/js/websocket-manager.js"></script>
    <script src="/static/js/webrtc-engine.js"></script>
    
    <script>
        // WebRTC Engine 초기화
        const engine = new WebRTCEngine({
            streamId: 'camera1',  // 👈 여기에 실제 스트림 ID 입력
            videoElement: document.getElementById('cctv')
        });
        
        const statusDiv = document.getElementById('status');
        
        // 연결 성공
        engine.on('connected', () => {
            statusDiv.textContent = '✅ 연결됨';
            statusDiv.style.background = '#0a4d0a';
        });
        
        // 연결 끊김
        engine.on('disconnected', () => {
            statusDiv.textContent = '❌ 연결 끊김';
            statusDiv.style.background = '#4d0a0a';
        });
        
        // 에러 처리
        engine.on('error', (error) => {
            statusDiv.textContent = '⚠️ 에러: ' + error.message;
            statusDiv.style.background = '#4d2a0a';
        });
        
        // 연결 시작
        engine.connect();
    </script>
</body>
</html>
```

### Step 2: 서버에 스트림 추가

```bash
curl -X POST http://localhost:8080/api/v1/paths \
  -H "Content-Type: application/json" \
  -d '{
    "camera1": {
      "source": "rtsp://admin:password@192.168.1.100:554/stream",
      "sourceOnDemand": true,
      "rtspTransport": "tcp"
    }
  }'
```

### Step 3: 브라우저에서 열기

```
http://localhost:8080/static/my-cctv.html
```

완료! 🎉

---

## 단일 CCTV 표출

### 기본 예제

가장 간단한 형태의 CCTV 표출입니다.

```html
<!DOCTYPE html>
<html lang="ko">
<head>
    <meta charset="UTF-8">
    <title>CCTV 뷰어</title>
</head>
<body>
    <video id="cctv" autoplay playsinline muted style="width: 100%;"></video>

    <script src="/static/js/websocket-manager.js"></script>
    <script src="/static/js/webrtc-engine.js"></script>
    <script>
        const engine = new WebRTCEngine({
            streamId: 'front_door',
            videoElement: document.getElementById('cctv')
        });
        
        engine.connect();
    </script>
</body>
</html>
```

### 고급 예제 (컨트롤 포함)

재생/일시정지, 음소거 해제 등의 컨트롤이 포함된 예제입니다.

**파일**: `cctv-with-controls.html`

```html
<!DOCTYPE html>
<html lang="ko">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>CCTV with Controls</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: #0f0f0f;
            color: #fff;
            padding: 20px;
        }
        
        .container {
            max-width: 1280px;
            margin: 0 auto;
        }
        
        h1 {
            margin-bottom: 20px;
            font-size: 24px;
        }
        
        .player-wrapper {
            background: #000;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 4px 20px rgba(0,0,0,0.5);
        }
        
        video {
            width: 100%;
            height: auto;
            display: block;
        }
        
        .controls {
            background: #1a1a1a;
            padding: 15px;
            display: flex;
            gap: 10px;
            align-items: center;
            flex-wrap: wrap;
        }
        
        button {
            padding: 10px 20px;
            background: #2196F3;
            color: white;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 14px;
            transition: background 0.3s;
        }
        
        button:hover {
            background: #1976D2;
        }
        
        button:disabled {
            background: #555;
            cursor: not-allowed;
        }
        
        .status {
            padding: 8px 16px;
            background: #2a2a2a;
            border-radius: 4px;
            font-size: 14px;
        }
        
        .status.connected {
            background: #0a4d0a;
        }
        
        .status.disconnected {
            background: #4d0a0a;
        }
        
        .stats {
            margin-left: auto;
            font-size: 12px;
            color: #999;
        }
        
        .info-panel {
            margin-top: 20px;
            padding: 15px;
            background: #1a1a1a;
            border-radius: 8px;
        }
        
        .info-item {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            border-bottom: 1px solid #2a2a2a;
        }
        
        .info-item:last-child {
            border-bottom: none;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>📹 CCTV 모니터링</h1>
        
        <div class="player-wrapper">
            <video id="cctv" autoplay playsinline muted></video>
            
            <div class="controls">
                <button id="connectBtn">연결</button>
                <button id="disconnectBtn" disabled>연결 해제</button>
                <button id="muteBtn">음소거 해제</button>
                <button id="fullscreenBtn">전체화면</button>
                
                <div class="status" id="status">대기 중</div>
                <div class="stats" id="stats">0 kbps</div>
            </div>
        </div>
        
        <div class="info-panel">
            <div class="info-item">
                <span>스트림 ID:</span>
                <span id="streamId">-</span>
            </div>
            <div class="info-item">
                <span>연결 상태:</span>
                <span id="connState">-</span>
            </div>
            <div class="info-item">
                <span>수신 패킷:</span>
                <span id="packets">0</span>
            </div>
            <div class="info-item">
                <span>수신 데이터:</span>
                <span id="bytes">0 MB</span>
            </div>
        </div>
    </div>

    <script src="/static/js/websocket-manager.js"></script>
    <script src="/static/js/webrtc-engine.js"></script>
    <script>
        // URL에서 스트림 ID 가져오기 (예: ?stream=camera1)
        const urlParams = new URLSearchParams(window.location.search);
        const streamId = urlParams.get('stream') || 'camera1';
        
        document.getElementById('streamId').textContent = streamId;
        
        // 엘리먼트 참조
        const videoElement = document.getElementById('cctv');
        const connectBtn = document.getElementById('connectBtn');
        const disconnectBtn = document.getElementById('disconnectBtn');
        const muteBtn = document.getElementById('muteBtn');
        const fullscreenBtn = document.getElementById('fullscreenBtn');
        const statusDiv = document.getElementById('status');
        const statsDiv = document.getElementById('stats');
        
        let engine = null;
        
        // 연결 버튼
        connectBtn.addEventListener('click', async () => {
            if (!engine) {
                engine = new WebRTCEngine({
                    streamId: streamId,
                    videoElement: videoElement
                });
                
                setupEngineEvents();
            }
            
            statusDiv.textContent = '연결 중...';
            statusDiv.className = 'status';
            connectBtn.disabled = true;
            
            await engine.connect();
        });
        
        // 연결 해제 버튼
        disconnectBtn.addEventListener('click', () => {
            if (engine) {
                engine.disconnect();
            }
        });
        
        // 음소거 토글
        muteBtn.addEventListener('click', () => {
            videoElement.muted = !videoElement.muted;
            muteBtn.textContent = videoElement.muted ? '음소거 해제' : '음소거';
        });
        
        // 전체화면
        fullscreenBtn.addEventListener('click', () => {
            if (videoElement.requestFullscreen) {
                videoElement.requestFullscreen();
            } else if (videoElement.webkitRequestFullscreen) {
                videoElement.webkitRequestFullscreen();
            }
        });
        
        // 엔진 이벤트 설정
        function setupEngineEvents() {
            engine.on('connected', () => {
                statusDiv.textContent = '✅ 연결됨';
                statusDiv.className = 'status connected';
                connectBtn.disabled = true;
                disconnectBtn.disabled = false;
            });
            
            engine.on('disconnected', () => {
                statusDiv.textContent = '❌ 연결 끊김';
                statusDiv.className = 'status disconnected';
                connectBtn.disabled = false;
                disconnectBtn.disabled = true;
            });
            
            engine.on('error', (error) => {
                statusDiv.textContent = '⚠️ 에러: ' + error.message;
                statusDiv.className = 'status disconnected';
                console.error('Engine error:', error);
            });
            
            engine.on('stats', (stats) => {
                statsDiv.textContent = `${stats.bitrate.toFixed(1)} kbps`;
                document.getElementById('packets').textContent = 
                    stats.packetsReceived.toLocaleString();
                document.getElementById('bytes').textContent = 
                    (stats.bytesReceived / 1024 / 1024).toFixed(2) + ' MB';
            });
            
            engine.on('statechange', (state) => {
                document.getElementById('connState').textContent = state;
            });
        }
    </script>
</body>
</html>
```

---

## 다중 CCTV 그리드

여러 CCTV를 동시에 표출하는 그리드 레이아웃입니다.

**파일**: `cctv-grid.html` (실제 구현 파일은 별도로 생성됩니다)

```html
<!DOCTYPE html>
<html lang="ko">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>CCTV 그리드</title>
    <style>
        body {
            margin: 0;
            padding: 20px;
            background: #0f0f0f;
            font-family: Arial, sans-serif;
        }
        
        h1 {
            color: #fff;
            margin-bottom: 20px;
        }
        
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        
        .grid-item {
            background: #1a1a1a;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 4px 10px rgba(0,0,0,0.5);
        }
        
        .grid-item video {
            width: 100%;
            height: auto;
            background: #000;
            display: block;
        }
        
        .grid-item-header {
            padding: 10px 15px;
            background: #2a2a2a;
            color: #fff;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        
        .camera-name {
            font-weight: bold;
        }
        
        .status-indicator {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            background: #666;
        }
        
        .status-indicator.connected {
            background: #4CAF50;
            box-shadow: 0 0 10px #4CAF50;
        }
        
        @media (max-width: 768px) {
            .grid {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <h1>📹 CCTV 모니터링 그리드</h1>
    
    <div class="grid" id="cctv-grid">
        <!-- JavaScript로 동적 생성 -->
    </div>

    <script src="/static/js/websocket-manager.js"></script>
    <script src="/static/js/webrtc-engine.js"></script>
    <script>
        // CCTV 설정
        const cameras = [
            { id: 'front_door', name: '정문' },
            { id: 'back_door', name: '후문' },
            { id: 'parking_lot', name: '주차장' },
            { id: 'lobby', name: '로비' }
        ];
        
        const engines = new Map();
        const grid = document.getElementById('cctv-grid');
        
        // 각 카메라 그리드 아이템 생성
        cameras.forEach(camera => {
            // 그리드 아이템 생성
            const item = document.createElement('div');
            item.className = 'grid-item';
            item.innerHTML = `
                <div class="grid-item-header">
                    <span class="camera-name">${camera.name}</span>
                    <div class="status-indicator" id="status-${camera.id}"></div>
                </div>
                <video id="video-${camera.id}" autoplay playsinline muted></video>
            `;
            grid.appendChild(item);
            
            // WebRTC Engine 생성
            const engine = new WebRTCEngine({
                streamId: camera.id,
                videoElement: document.getElementById(`video-${camera.id}`)
            });
            
            const statusIndicator = document.getElementById(`status-${camera.id}`);
            
            // 이벤트 핸들러
            engine.on('connected', () => {
                statusIndicator.classList.add('connected');
            });
            
            engine.on('disconnected', () => {
                statusIndicator.classList.remove('connected');
            });
            
            engine.on('error', (error) => {
                console.error(`${camera.name} 에러:`, error);
            });
            
            // 연결
            engine.connect();
            engines.set(camera.id, engine);
        });
        
        // 페이지 언로드 시 정리
        window.addEventListener('beforeunload', () => {
            engines.forEach(engine => engine.disconnect());
        });
    </script>
</body>
</html>
```

---

## 대시보드 예제

상세한 통계와 컨트롤이 포함된 대시보드입니다.

```javascript
// 대시보드 구성 예제 (간단버전)
const dashboard = {
    cameras: [],
    
    init() {
        this.loadCameras();
        this.setupEventListeners();
    },
    
    async loadCameras() {
        // 서버에서 카메라 목록 가져오기
        const response = await fetch('/api/v1/paths');
        const data = await response.json();
        
        Object.keys(data.paths).forEach(streamId => {
            this.addCamera(streamId, data.paths[streamId]);
        });
    },
    
    addCamera(streamId, config) {
        const engine = new WebRTCEngine({
            streamId: streamId,
            videoElement: this.createVideoElement(streamId)
        });
        
        engine.on('stats', (stats) => {
            this.updateStats(streamId, stats);
        });
        
        engine.connect();
        this.cameras.push({ streamId, engine });
    },
    
    createVideoElement(streamId) {
        const video = document.createElement('video');
        video.id = streamId;
        video.autoplay = true;
        video.playsinline = true;
        video.muted = true;
        document.getElementById('video-container').appendChild(video);
        return video;
    },
    
    updateStats(streamId, stats) {
        // UI 업데이트
        console.log(`${streamId}:`, stats);
    }
};

// 초기화
dashboard.init();
```

---

## 모바일 최적화

모바일 기기에서 최적화된 CCTV 뷰어입니다.

```html
<!DOCTYPE html>
<html lang="ko">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <title>Mobile CCTV</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
            -webkit-tap-highlight-color: transparent;
        }
        
        body {
            background: #000;
            overflow: hidden;
            position: fixed;
            width: 100%;
            height: 100%;
        }
        
        video {
            width: 100vw;
            height: 100vh;
            object-fit: contain;
        }
        
        .overlay {
            position: fixed;
            bottom: 20px;
            left: 50%;
            transform: translateX(-50%);
            padding: 10px 20px;
            background: rgba(0,0,0,0.7);
            color: white;
            border-radius: 20px;
            font-size: 14px;
            backdrop-filter: blur(10px);
        }
    </style>
</head>
<body>
    <video id="cctv" autoplay playsinline muted></video>
    <div class="overlay" id="status">연결 중...</div>

    <script src="/static/js/websocket-manager.js"></script>
    <script src="/static/js/webrtc-engine.js"></script>
    <script>
        const streamId = new URLSearchParams(location.search).get('stream') || 'camera1';
        
        const engine = new WebRTCEngine({
            streamId: streamId,
            videoElement: document.getElementById('cctv')
        });
        
        const statusDiv = document.getElementById('status');
        
        engine.on('connected', () => {
            statusDiv.textContent = '✅ ' + streamId;
            setTimeout(() => statusDiv.style.opacity = '0', 3000);
        });
        
        engine.on('stats', (stats) => {
            if (statusDiv.style.opacity === '0') return;
            statusDiv.textContent = `${stats.bitrate.toFixed(0)} kbps`;
        });
        
        engine.connect();
        
        // 화면 터치로 상태 표시 토글
        document.body.addEventListener('click', () => {
            statusDiv.style.opacity = statusDiv.style.opacity === '0' ? '1' : '0';
        });
    </script>
</body>
</html>
```

---

## 트러블슈팅

### 문제: 비디오가 재생되지 않음

**해결**:
```javascript
// 브라우저 자동재생 정책으로 인해 muted 필수
videoElement.muted = true;

// 또는 사용자 제스처 후 재생
button.onclick = async () => {
    await videoElement.play();
};
```

### 문제: 여러 스트림 연결 시 느려짐

**해결**: Intersection Observer로 보이는 스트림만 연결
```javascript
const observer = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
        const engine = engines.get(entry.target.id);
        if (entry.isIntersecting) {
            engine.connect();
        } else {
            engine.disconnect(false);
        }
    });
});
```

### 문제: 모바일에서 전체화면 안됨

**해결**:
```javascript
// iOS Safari용
if (video.webkitEnterFullscreen) {
    video.webkitEnterFullscreen();
}
```

---

## 다음 단계

- [WebRTC Engine 전체 가이드](./WEBRTC_ENGINE_GUIDE.md)
- [API 레퍼런스](../docs/API_REFERENCE.md)
- [서버 설정 가이드](../docs/API_QUICKSTART.md)

## 샘플 파일 위치

모든 예제 파일은 `web/static/examples/` 폴더에서 확인할 수 있습니다.

