# WebRTC Engine - 프론트엔드 개발자 가이드

## 개요

`WebRTCEngine`은 Media Stream Server와 WebRTC 기반 실시간 CCTV 스트림을 연결하기 위한 재사용 가능한 JavaScript 라이브러리입니다.

## 주요 특징

- ✅ **간단한 API**: 몇 줄의 코드로 CCTV 스트림 연결
- ✅ **자동 재연결**: 네트워크 끊김 시 자동으로 재연결
- ✅ **다중 스트림 지원**: 하나의 페이지에서 여러 CCTV 동시 표출
- ✅ **공유 WebSocket**: 브라우저당 하나의 WebSocket 연결로 효율적인 리소스 사용
- ✅ **이벤트 기반**: 연결 상태 변화를 이벤트로 알림
- ✅ **통계 제공**: 실시간 비트레이트, 패킷 수신 정보

## 빠른 시작

### 1. HTML에 스크립트 포함

```html
<!-- WebSocket Manager (필수) -->
<script src="/static/js/websocket-manager.js"></script>
<!-- WebRTC Engine -->
<script src="/static/js/webrtc-engine.js"></script>
```

### 2. 비디오 엘리먼트 생성

```html
<video id="cctv1" autoplay playsinline muted></video>
```

### 3. 스트림 연결

```javascript
// WebRTC Engine 인스턴스 생성
const engine = new WebRTCEngine({
    streamId: 'camera1',              // 서버에 등록된 스트림 ID
    videoElement: document.getElementById('cctv1'),
    autoReconnect: true,              // 자동 재연결 활성화 (기본값: true)
    reconnectDelay: 3000              // 재연결 대기 시간 (ms)
});

// 이벤트 리스너 등록
engine.on('connected', () => {
    console.log('CCTV 연결됨');
});

engine.on('error', (error) => {
    console.error('에러 발생:', error);
});

// 연결 시작
await engine.connect();
```

## 생성자 옵션

```javascript
new WebRTCEngine({
    streamId: string,           // 필수: 스트림 ID
    videoElement: HTMLVideoElement,  // 필수: video 태그
    autoReconnect: boolean,     // 선택: 자동 재연결 (기본값: true)
    reconnectDelay: number      // 선택: 재연결 대기 시간 ms (기본값: 3000)
})
```

### 옵션 설명

| 옵션 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `streamId` | string | ✅ | - | 서버에 등록된 스트림 식별자 |
| `videoElement` | HTMLVideoElement | ✅ | - | 비디오를 표시할 video 태그 |
| `autoReconnect` | boolean | ❌ | true | 연결 끊김 시 자동 재연결 여부 |
| `reconnectDelay` | number | ❌ | 3000 | 재연결 대기 시간 (밀리초) |

## 메서드

### connect()

스트림 연결을 시작합니다.

```javascript
await engine.connect();
```

**반환값**: `Promise<void>`

### disconnect(cleanup = true)

스트림 연결을 해제합니다.

```javascript
engine.disconnect();  // 완전 종료
engine.disconnect(false);  // 재연결 가능하도록 임시 해제
```

**매개변수**:
- `cleanup` (boolean): 완전히 정리할지 여부 (기본값: true)

### isConnected()

현재 연결 상태를 확인합니다.

```javascript
if (engine.isConnected()) {
    console.log('연결됨');
}
```

**반환값**: `boolean`

### getStats()

현재 스트림 통계를 가져옵니다.

```javascript
const stats = engine.getStats();
console.log('비트레이트:', stats.bitrate, 'kbps');
console.log('수신 패킷:', stats.packetsReceived);
console.log('수신 바이트:', stats.bytesReceived);
```

**반환값**: 
```javascript
{
    packetsReceived: number,  // 수신한 패킷 수
    bytesReceived: number,    // 수신한 바이트 수
    bitrate: number           // 현재 비트레이트 (kbps)
}
```

### on(event, callback)

이벤트 리스너를 등록합니다.

```javascript
engine.on('connected', () => {
    console.log('연결됨');
});
```

**매개변수**:
- `event` (string): 이벤트 이름
- `callback` (Function): 콜백 함수

**반환값**: `WebRTCEngine` (체이닝 가능)

## 이벤트

### connected

스트림 연결이 완료되었을 때 발생합니다.

```javascript
engine.on('connected', () => {
    console.log('스트림 연결됨');
    // 연결 성공 UI 업데이트
});
```

### disconnected

스트림 연결이 끊겼을 때 발생합니다.

```javascript
engine.on('disconnected', () => {
    console.log('스트림 연결 끊김');
    // 연결 끊김 UI 업데이트
});
```

### error

에러가 발생했을 때 발생합니다.

```javascript
engine.on('error', (error) => {
    console.error('에러:', error);
    // 에러 메시지 표시
});
```

### stats

통계가 업데이트될 때 발생합니다 (1초마다).

```javascript
engine.on('stats', (stats) => {
    document.getElementById('bitrate').textContent = 
        `${stats.bitrate.toFixed(0)} kbps`;
});
```

**콜백 매개변수**:
```javascript
{
    packetsReceived: number,
    bytesReceived: number,
    bitrate: number
}
```

### statechange

연결 상태가 변경될 때 발생합니다.

```javascript
engine.on('statechange', (state) => {
    console.log('상태 변경:', state);
    // 'connecting', 'connected', 'disconnected', 'failed', 'closed' 등
});
```

**가능한 상태값**:
- `connecting`: 연결 중
- `connected`: 연결됨
- `checking`: ICE 체크 중
- `completed`: ICE 완료
- `failed`: 연결 실패
- `disconnected`: 연결 끊김
- `closed`: 종료됨

## 사용 예제

### 기본 사용

```javascript
// HTML
<video id="cctv" autoplay playsinline muted></video>
<div id="status">연결 중...</div>

// JavaScript
const engine = new WebRTCEngine({
    streamId: 'front_door',
    videoElement: document.getElementById('cctv')
});

const statusDiv = document.getElementById('status');

engine.on('connected', () => {
    statusDiv.textContent = '연결됨';
    statusDiv.style.color = 'green';
});

engine.on('disconnected', () => {
    statusDiv.textContent = '연결 끊김';
    statusDiv.style.color = 'red';
});

engine.on('error', (error) => {
    statusDiv.textContent = '에러: ' + error.message;
    statusDiv.style.color = 'red';
});

await engine.connect();
```

### 다중 스트림

```javascript
// HTML
<div class="cctv-grid">
    <video id="cctv1" autoplay playsinline muted></video>
    <video id="cctv2" autoplay playsinline muted></video>
    <video id="cctv3" autoplay playsinline muted></video>
    <video id="cctv4" autoplay playsinline muted></video>
</div>

// JavaScript
const cameras = [
    { id: 'cctv1', streamId: 'front_door' },
    { id: 'cctv2', streamId: 'parking_lot' },
    { id: 'cctv3', streamId: 'back_door' },
    { id: 'cctv4', streamId: 'lobby' }
];

const engines = cameras.map(camera => {
    const engine = new WebRTCEngine({
        streamId: camera.streamId,
        videoElement: document.getElementById(camera.id)
    });
    
    engine.on('connected', () => {
        console.log(`${camera.streamId} 연결됨`);
    });
    
    return engine;
});

// 모든 스트림 동시 연결
await Promise.all(engines.map(engine => engine.connect()));
```

### 통계 표시

```javascript
// HTML
<video id="cctv" autoplay playsinline muted></video>
<div id="stats">
    <div>비트레이트: <span id="bitrate">0</span> kbps</div>
    <div>패킷: <span id="packets">0</span></div>
    <div>데이터: <span id="bytes">0</span> bytes</div>
</div>

// JavaScript
const engine = new WebRTCEngine({
    streamId: 'camera1',
    videoElement: document.getElementById('cctv')
});

engine.on('stats', (stats) => {
    document.getElementById('bitrate').textContent = 
        stats.bitrate.toFixed(1);
    document.getElementById('packets').textContent = 
        stats.packetsReceived.toLocaleString();
    document.getElementById('bytes').textContent = 
        stats.bytesReceived.toLocaleString();
});

await engine.connect();
```

### 연결/해제 버튼

```javascript
// HTML
<video id="cctv" autoplay playsinline muted></video>
<button id="connectBtn">연결</button>
<button id="disconnectBtn" disabled>연결 해제</button>

// JavaScript
let engine = null;

document.getElementById('connectBtn').addEventListener('click', async () => {
    if (!engine) {
        engine = new WebRTCEngine({
            streamId: 'camera1',
            videoElement: document.getElementById('cctv')
        });
        
        engine.on('connected', () => {
            document.getElementById('connectBtn').disabled = true;
            document.getElementById('disconnectBtn').disabled = false;
        });
        
        engine.on('disconnected', () => {
            document.getElementById('connectBtn').disabled = false;
            document.getElementById('disconnectBtn').disabled = true;
        });
    }
    
    await engine.connect();
});

document.getElementById('disconnectBtn').addEventListener('click', () => {
    if (engine) {
        engine.disconnect();
    }
});
```

### React 컴포넌트

```jsx
import { useEffect, useRef, useState } from 'react';

function CCTVPlayer({ streamId }) {
    const videoRef = useRef(null);
    const engineRef = useRef(null);
    const [status, setStatus] = useState('connecting');
    const [bitrate, setBitrate] = useState(0);

    useEffect(() => {
        if (!videoRef.current) return;

        // WebRTC Engine 생성
        const engine = new WebRTCEngine({
            streamId: streamId,
            videoElement: videoRef.current
        });

        // 이벤트 리스너
        engine.on('connected', () => setStatus('connected'));
        engine.on('disconnected', () => setStatus('disconnected'));
        engine.on('error', (error) => {
            console.error(error);
            setStatus('error');
        });
        engine.on('stats', (stats) => setBitrate(stats.bitrate));

        // 연결
        engine.connect();

        engineRef.current = engine;

        // 정리
        return () => {
            engine.disconnect();
        };
    }, [streamId]);

    return (
        <div className="cctv-player">
            <video ref={videoRef} autoPlay playsInline muted />
            <div className="status">
                상태: {status} | {bitrate.toFixed(1)} kbps
            </div>
        </div>
    );
}

export default CCTVPlayer;
```

### Vue 컴포넌트

```vue
<template>
  <div class="cctv-player">
    <video ref="videoElement" autoplay playsinline muted></video>
    <div class="status">
      상태: {{ status }} | {{ bitrate.toFixed(1) }} kbps
    </div>
  </div>
</template>

<script>
export default {
  name: 'CCTVPlayer',
  props: {
    streamId: {
      type: String,
      required: true
    }
  },
  data() {
    return {
      engine: null,
      status: 'connecting',
      bitrate: 0
    };
  },
  mounted() {
    this.engine = new WebRTCEngine({
      streamId: this.streamId,
      videoElement: this.$refs.videoElement
    });

    this.engine.on('connected', () => {
      this.status = 'connected';
    });

    this.engine.on('disconnected', () => {
      this.status = 'disconnected';
    });

    this.engine.on('error', (error) => {
      console.error(error);
      this.status = 'error';
    });

    this.engine.on('stats', (stats) => {
      this.bitrate = stats.bitrate;
    });

    this.engine.connect();
  },
  beforeUnmount() {
    if (this.engine) {
      this.engine.disconnect();
    }
  }
};
</script>

<style scoped>
.cctv-player video {
  width: 100%;
  height: auto;
  background: #000;
}
</style>
```

## 문제 해결

### 비디오가 재생되지 않음

**원인**: 브라우저의 자동재생 정책

**해결**:
```javascript
// muted 속성 필수
videoElement.muted = true;
videoElement.autoplay = true;
videoElement.playsInline = true;

// 또는 사용자 제스처 후 재생
button.addEventListener('click', async () => {
    await engine.connect();
    await videoElement.play();
});
```

### WebSocket 연결 실패

**원인**: CORS 또는 서버 URL 문제

**해결**:
```javascript
// WebSocketManager의 서버 URL 확인
console.log(engine.wsManager.serverUrl);

// 수동으로 설정 (필요시)
engine.wsManager.serverUrl = 'ws://192.168.1.100:8080/ws';
```

### 스트림이 자주 끊김

**원인**: 네트워크 불안정 또는 서버 문제

**해결**:
```javascript
// 재연결 딜레이 늘리기
const engine = new WebRTCEngine({
    streamId: 'camera1',
    videoElement: videoElement,
    reconnectDelay: 5000  // 5초로 증가
});

// 연결 상태 모니터링
engine.on('statechange', (state) => {
    console.log('Connection state:', state);
});
```

## 브라우저 호환성

- ✅ Chrome 60+
- ✅ Firefox 55+
- ✅ Safari 11+
- ✅ Edge 79+
- ✅ Opera 47+

## 성능 최적화

### 1. 동시 스트림 수 제한

```javascript
// 한 번에 최대 4개의 스트림만 연결
const MAX_STREAMS = 4;
let activeEngines = [];

async function connectStream(streamId, videoElement) {
    if (activeEngines.length >= MAX_STREAMS) {
        // 가장 오래된 스트림 해제
        activeEngines[0].disconnect();
        activeEngines.shift();
    }
    
    const engine = new WebRTCEngine({ streamId, videoElement });
    await engine.connect();
    activeEngines.push(engine);
}
```

### 2. Intersection Observer 사용

```javascript
// 화면에 보이는 비디오만 연결
const observer = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
        const engine = videoEngines.get(entry.target.id);
        
        if (entry.isIntersecting) {
            engine.connect();
        } else {
            engine.disconnect(false);
        }
    });
});

videoElements.forEach(video => {
    observer.observe(video);
});
```

## API 참조

전체 서버 API 문서는 [API Reference](../docs/API_REFERENCE.md)를 참고하세요.

## 라이센스

이 라이브러리는 Media Stream Server 프로젝트의 일부입니다.

