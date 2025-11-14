# CCTV WebRTC 스트리밍 예제

이 폴더에는 WebRTC Engine을 사용한 CCTV 스트리밍 예제 파일들이 포함되어 있습니다.

## 📚 문서

### 개발자 가이드
- **[WEBRTC_ENGINE_GUIDE.md](./WEBRTC_ENGINE_GUIDE.md)** - WebRTC Engine 완전 가이드
  - API 레퍼런스
  - 이벤트 설명
  - React/Vue 컴포넌트 예제
  - 문제 해결 가이드

- **[CCTV_QUICK_START.md](./CCTV_QUICK_START.md)** - 빠른 시작 가이드
  - 5분 안에 시작하기
  - 단계별 튜토리얼
  - 실제 코드 예제

## 🎬 실행 가능한 예제 파일

### 1. 단일 CCTV 뷰어
**파일**: `cctv-viewer.html`

단일 CCTV 스트림을 보여주는 기본 뷰어입니다.

**사용 방법**:
```
http://localhost:8080/static/cctv-viewer.html?stream=camera1
```

**기능**:
- ✅ 실시간 스트림 재생
- ✅ 연결/연결 해제 컨트롤
- ✅ 음소거 토글
- ✅ 전체화면 지원
- ✅ 실시간 통계 표시 (비트레이트, 패킷, 데이터)
- ✅ 상태 표시 (연결 중, 연결됨, 연결 끊김)

**URL 파라미터**:
- `stream` - 스트림 ID (기본값: camera1)

---

### 2. 다중 CCTV 그리드
**파일**: `cctv-grid.html`

여러 CCTV를 동시에 보여주는 그리드 레이아웃입니다.

**사용 방법**:
```
http://localhost:8080/static/cctv-grid.html
```

**기능**:
- ✅ 여러 스트림 동시 재생
- ✅ 그리드 레이아웃 변경 (1x1, 2x2, 3x3)
- ✅ 모두 연결/해제 버튼
- ✅ 각 스트림별 상태 표시
- ✅ 각 스트림별 비트레이트 표시
- ✅ 비디오 클릭으로 전체화면 모달
- ✅ 연결된 스트림 수 카운터

**커스터마이징**:
파일을 열어서 `cameras` 배열을 수정하세요:
```javascript
const cameras = [
    { id: 'camera1', name: '정문' },
    { id: 'camera2', name: '후문' },
    { id: 'parking', name: '주차장' },
    { id: 'lobby', name: '로비' }
];
```

---

### 3. 모바일 CCTV 뷰어
**파일**: `cctv-mobile.html`

모바일 기기에 최적화된 풀스크린 뷰어입니다.

**사용 방법**:
```
http://localhost:8080/static/cctv-mobile.html?stream=camera1
```

**기능**:
- ✅ 모바일 최적화 UI
- ✅ 풀스크린 재생
- ✅ 터치 제스처
  - 단일 탭: 오버레이/컨트롤 표시/숨김
  - 더블 탭: 전체화면 토글
- ✅ 자동 재연결
- ✅ 절전 모드 방지 (지원 시)
- ✅ 화면 회전 지원
- ✅ 에러 처리 및 재시도

**URL 파라미터**:
- `stream` - 스트림 ID (기본값: camera1)

---

## 🚀 빠른 시작

### 1단계: 서버에 스트림 추가

```bash
# PowerShell
$body = @{
    camera1 = @{
        source = "rtsp://admin:password@192.168.1.100:554/stream"
        sourceOnDemand = $true
        rtspTransport = "tcp"
    }
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/v1/paths" `
  -Method Post `
  -ContentType "application/json" `
  -Body $body
```

```bash
# cURL (Linux/macOS)
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

### 2단계: 브라우저에서 열기

```
http://localhost:8080/static/cctv-viewer.html?stream=camera1
```

완료! 🎉

---

## 🛠️ 프로젝트에 통합하기

### HTML 프로젝트

```html
<!DOCTYPE html>
<html>
<head>
    <title>My CCTV</title>
</head>
<body>
    <video id="cctv" autoplay playsinline muted></video>

    <!-- 필수 스크립트 -->
    <script src="/static/js/websocket-manager.js"></script>
    <script src="/static/js/webrtc-engine.js"></script>
    
    <script>
        const engine = new WebRTCEngine({
            streamId: 'camera1',
            videoElement: document.getElementById('cctv')
        });
        
        engine.on('connected', () => console.log('Connected!'));
        engine.connect();
    </script>
</body>
</html>
```

### React 프로젝트

```jsx
import { useEffect, useRef } from 'react';

function CCTVPlayer({ streamId }) {
    const videoRef = useRef(null);
    const engineRef = useRef(null);

    useEffect(() => {
        // WebSocketManager와 WebRTCEngine을 public/에 복사 후
        const engine = new window.WebRTCEngine({
            streamId: streamId,
            videoElement: videoRef.current
        });

        engine.on('connected', () => console.log('Connected'));
        engine.connect();

        engineRef.current = engine;

        return () => engine.disconnect();
    }, [streamId]);

    return <video ref={videoRef} autoPlay playsInline muted />;
}
```

### Vue 프로젝트

```vue
<template>
  <video ref="videoElement" autoplay playsinline muted></video>
</template>

<script>
export default {
  props: ['streamId'],
  data() {
    return { engine: null };
  },
  mounted() {
    this.engine = new WebRTCEngine({
      streamId: this.streamId,
      videoElement: this.$refs.videoElement
    });
    this.engine.connect();
  },
  beforeUnmount() {
    this.engine?.disconnect();
  }
};
</script>
```

---

## 📋 필수 요구사항

### 브라우저
- Chrome 60+
- Firefox 55+
- Safari 11+
- Edge 79+

### 서버
- Media Stream Server 실행 중
- 스트림이 서버에 등록되어 있어야 함

### 네트워크
- WebSocket 연결 가능 (포트 8080)
- WebRTC 연결 가능 (UDP)

---

## 🔧 커스터마이징

### 스트림 ID 변경

**cctv-viewer.html**:
```
?stream=your_stream_id
```

**cctv-grid.html**:
```javascript
const cameras = [
    { id: 'your_stream_1', name: 'Camera 1' },
    { id: 'your_stream_2', name: 'Camera 2' }
];
```

### 스타일 변경

각 HTML 파일의 `<style>` 섹션을 수정하세요.

### 자동 연결

**cctv-grid.html**에서 주석 해제:
```javascript
// 페이지 로드 시 자동 연결
window.addEventListener('load', () => {
    document.getElementById('connectAllBtn').click();
});
```

---

## 🐛 문제 해결

### 비디오가 재생되지 않음

**원인**: 브라우저 자동재생 정책

**해결**:
```javascript
// video 태그에 muted 속성 필수
videoElement.muted = true;
```

### 스트림을 찾을 수 없음

**확인**:
```bash
# 등록된 스트림 확인
curl http://localhost:8080/api/v1/paths
```

### WebSocket 연결 실패

**확인**:
- 서버가 실행 중인지 확인
- 브라우저 콘솔에서 에러 확인
- 포트 8080이 열려있는지 확인

### 모바일에서 작동하지 않음

**확인**:
- HTTPS 연결 필요 (일부 기능)
- 브라우저가 WebRTC를 지원하는지 확인
- 네트워크 연결 상태 확인

---

## 📱 테스트된 환경

### 데스크톱
- ✅ Windows 10/11 - Chrome, Edge, Firefox
- ✅ macOS - Chrome, Safari, Firefox
- ✅ Linux - Chrome, Firefox

### 모바일
- ✅ iOS 13+ - Safari
- ✅ Android 8+ - Chrome

---

## 📖 추가 리소스

- [WebRTC Engine 완전 가이드](./WEBRTC_ENGINE_GUIDE.md)
- [CCTV 빠른 시작](./CCTV_QUICK_START.md)
- [API 레퍼런스](../docs/API_REFERENCE.md)
- [서버 API 가이드](../docs/API_QUICKSTART.md)

---

## 💡 팁

### 성능 최적화
- 동시에 많은 스트림을 연결하지 마세요 (권장: 최대 4개)
- 보이지 않는 비디오는 연결 해제하세요
- Intersection Observer를 사용하세요

### 보안
- RTSP 인증 정보를 클라이언트에 노출하지 마세요
- 서버 측에서 스트림을 관리하세요
- 가능하면 HTTPS를 사용하세요

### 디버깅
- 브라우저 개발자 도구의 콘솔을 확인하세요
- Network 탭에서 WebSocket 연결을 확인하세요
- WebRTC Internals (chrome://webrtc-internals)를 사용하세요

---

## 📧 지원

문제가 발생하면 GitHub Issues에 보고해주세요.

---

**Happy Streaming! 🎥**

