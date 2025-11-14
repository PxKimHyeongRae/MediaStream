# WebRTC CCTV 스트리밍 - 프론트엔드 리소스

## 📚 문서

### 1. [WebRTC Engine 개발자 가이드](./WEBRTC_ENGINE_GUIDE.md)
WebRTC Engine API의 완전한 참조 문서입니다.

**포함 내용**:
- 생성자 옵션
- 메서드 레퍼런스
- 이벤트 설명
- React/Vue 컴포넌트 예제
- 문제 해결 가이드
- 성능 최적화 팁

---

### 2. [CCTV 빠른 시작 가이드](./CCTV_QUICK_START.md)
5분 안에 CCTV 스트리밍을 시작할 수 있는 가이드입니다.

**포함 내용**:
- 단계별 튜토리얼
- 실제 동작하는 코드 예제
- 단일/다중 CCTV 표출 방법
- 모바일 최적화 가이드
- 트러블슈팅

---

## 🎬 실행 가능한 예제

### [예제 파일 모음](./examples/)

1. **cctv-viewer.html** - 단일 CCTV 뷰어
   - URL: `http://localhost:8080/static/cctv-viewer.html?stream=camera1`

2. **cctv-grid.html** - 다중 CCTV 그리드
   - URL: `http://localhost:8080/static/cctv-grid.html`

3. **cctv-mobile.html** - 모바일 최적화 뷰어
   - URL: `http://localhost:8080/static/cctv-mobile.html?stream=camera1`

자세한 내용은 [examples/README.md](./examples/README.md)를 참조하세요.

---

## 🚀 빠른 시작

### 1. 스트림 추가

```bash
curl -X POST http://localhost:8080/api/v1/paths \
  -H "Content-Type: application/json" \
  -d '{
    "camera1": {
      "source": "rtsp://admin:pass@192.168.1.100:554/stream",
      "sourceOnDemand": true,
      "rtspTransport": "tcp"
    }
  }'
```

### 2. 브라우저에서 열기

```
http://localhost:8080/static/cctv-viewer.html?stream=camera1
```

---

## 🔗 관련 문서

- [서버 API 레퍼런스](../../docs/API_REFERENCE.md)
- [서버 API 빠른 시작](../../docs/API_QUICKSTART.md)
- [프로젝트 상태](../../docs/PROJECT_STATUS.md)

---

## 📝 기본 사용 예제

```html
<!DOCTYPE html>
<html>
<head>
    <title>My CCTV</title>
</head>
<body>
    <video id="cctv" autoplay playsinline muted style="width: 100%;"></video>

    <script src="/static/js/websocket-manager.js"></script>
    <script src="/static/js/webrtc-engine.js"></script>
    <script>
        const engine = new WebRTCEngine({
            streamId: 'camera1',
            videoElement: document.getElementById('cctv')
        });
        
        engine.on('connected', () => console.log('✅ 연결됨'));
        engine.on('error', (err) => console.error('❌ 에러:', err));
        
        engine.connect();
    </script>
</body>
</html>
```

---

**Happy Streaming! 🎥**

