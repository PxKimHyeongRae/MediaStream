# RTSP to WebRTC Streaming System

> **Living Skill Document**: This skill should be updated (CRUD) as the project evolves, similar to CLAUDE.md.

## 📋 Skill Overview

This skill provides comprehensive knowledge for building a production-ready RTSP to WebRTC media streaming server with multi-camera support, inspired by mediaMTX architecture.

**Use this skill when:**
- Building IP camera streaming systems for web browsers
- Converting RTSP streams to WebRTC for low-latency viewing
- Implementing multi-camera dashboard applications
- Creating on-demand video streaming services
- Needing mediaMTX-compatible configuration systems

**Technology Stack:**
- Backend: Go 1.23+ (pion/webrtc v4, bluenviron/gortsplib v4)
- Frontend: HTML5, JavaScript (WebRTC API)
- Configuration: YAML-based (mediaMTX-style)

---

## 🏗️ Architecture Pattern

### System Flow
```
[IP Camera (RTSP H.265/H.264)]
    ↓ TCP/RTSP
[RTSP Client (gortsplib v4)]
    ↓ RTP Packets (OnPacketRTPAny)
[Stream Manager (Pub/Sub)]
    ↓ Subscribe
[WebRTC Peer (pion v4)]
    ├─ Dynamic Codec Selection
    └─ ICE Connection
    ↓ WebRTC/SRTP
[Web Browser] ✅ Real-time Video
```

### Core Components

1. **RTSP Client** (`internal/rtsp/client.go`)
   - Use gortsplib v4's `OnPacketRTPAny()` callback (NOT deprecated OnPacketRTP)
   - TCP transport for reliability
   - Automatic reconnection logic

2. **Stream Manager** (`internal/core/stream_manager.go`)
   - Pub/Sub pattern for 1:N distribution
   - Thread-safe with sync.RWMutex
   - Automatic subscriber cleanup

3. **WebRTC Peer** (`internal/webrtc/peer.go`)
   - Dynamic codec selection (parse Offer SDP)
   - ICE candidate gathering with `GatheringCompletePromise`
   - Cleanup callbacks with sync.Once

4. **API Server** (`internal/api/server.go`)
   - Stream management endpoints
   - On-demand stream control
   - Health check and metrics

5. **WebRTC Engine Library** (`web/static/js/webrtc-engine.js`)
   - Reusable JavaScript class
   - Event-based API
   - Auto-reconnection

---

## 🔑 Critical Implementation Patterns

### 1. Dynamic Codec Selection

**Problem**: Browsers have different codec support (Chrome/Edge: H.265, Firefox: H.264 only)

**Solution**: Parse client Offer SDP to detect supported codecs

```go
func (p *Peer) selectVideoCodec(offerSDP string) string {
    offerUpper := strings.ToUpper(offerSDP)

    if strings.Contains(offerUpper, "H265") || strings.Contains(offerUpper, "HEVC") {
        return "H265"
    }
    if strings.Contains(offerUpper, "H264") || strings.Contains(offerUpper, "AVC") {
        return "H264"
    }

    return "H265" // default
}
```

**Why**: Avoids expensive transcoding while ensuring browser compatibility.

---

### 2. RTP Packet Reception (gortsplib v4)

**Problem**: v4 deprecated OnPacketRTP, manual read loops don't work

**Solution**: Use OnPacketRTPAny callback

```go
// Setup callback for each media track
medi.OnPacketRTPAny(func(medi *media.Media, forma format.Format, pkt *rtp.Packet) {
    if c.onPacket != nil {
        c.onPacket(pkt)
    }
})

// Start playback (callback receives packets automatically)
err := c.rtspClient.Play(nil)
```

**Why**: Automatic packet delivery without manual read loops.

---

### 3. ICE Connection Handling

**Problem**: Answer SDP sent too early → no ICE candidates → connection fails

**Solution**: Wait for ICE gathering completion

```go
// Wait for all ICE candidates to be gathered
<-webrtc.GatheringCompletePromise(pc)

// Now create Answer with all candidates
answer, err := pc.CreateAnswer(nil)
pc.SetLocalDescription(answer)
```

**Why**: Ensures Answer SDP contains server's ICE candidates.

---

### 4. Subscriber Cleanup Pattern

**Problem**: Peer closes but remains in stream's subscriber list → errors

**Solution**: Callback chain for automatic cleanup

```go
// Peer creation with OnClose callback
peer := NewPeer(PeerConfig{
    OnClose: func(peerID string) {
        // Use goroutine to avoid deadlock
        go func() {
            if m.onPeerClosed != nil {
                m.onPeerClosed(peerID)
            }
            m.RemovePeer(peerID)
        }()
    },
})

// Peer.Close() with sync.Once protection
func (p *Peer) Close() {
    p.closeOnce.Do(func() {
        // Cleanup logic
        if p.onClose != nil {
            p.onClose(p.id)
        }
    })
}
```

**Why**: Prevents "send to closed channel" errors and memory leaks.

---

### 5. mediaMTX-Style Configuration

**Pattern**: YAML paths section for easy stream management

```yaml
# config.yaml
paths:
  camera_01:
    source: rtsp://admin:password@192.168.1.100:554/stream
    sourceOnDemand: no   # Start immediately
    rtspTransport: tcp
  camera_02:
    source: rtsp://user:pass%21@192.168.1.101:554/stream
    sourceOnDemand: yes  # Start on client request
    rtspTransport: tcp
```

**Implementation**:
```go
type PathConfig struct {
    Source         string `yaml:"source"`
    SourceOnDemand bool   `yaml:"sourceOnDemand"`
    RTSPTransport  string `yaml:"rtspTransport"`
}

type Config struct {
    Paths map[string]PathConfig `yaml:"paths"`
}
```

**Lifecycle**:
1. Server startup: Create all Stream objects
2. sourceOnDemand=no: Create RTSP client immediately
3. sourceOnDemand=yes: Create RTSP client on API request

**Why**: No code changes needed to add/remove cameras.

---

### 6. On-Demand Stream Management

**Pattern**: Separate Stream object and RTSP client lifecycles

```go
func (app *Application) loadStreamsFromConfig(config *core.Config) error {
    for streamID, pathConfig := range config.Paths {
        // Always create Stream object
        stream, err := app.streamManager.CreateStream(streamID, streamID)

        if !pathConfig.SourceOnDemand {
            // Create RTSP client immediately
            app.startRTSPClient(streamID, pathConfig)
        }
        // Otherwise, wait for API request
    }
}

func (app *Application) startOnDemandStream(streamID string) error {
    // Check if RTSP client already running
    if _, exists := app.rtspClients[streamID]; exists {
        return nil
    }

    // Get existing Stream object
    stream, _ := app.streamManager.GetStream(streamID)

    // Create RTSP client only
    pathConfig := app.config.Paths[streamID]
    return app.startRTSPClient(streamID, pathConfig)
}
```

**API**:
- `POST /api/v1/streams/:id/start` - Start on-demand stream
- `DELETE /api/v1/streams/:id` - Stop stream
- `GET /api/v1/streams` - List all streams with status

---

### 7. Reusable WebRTC Engine (Frontend)

**Pattern**: Event-driven JavaScript class for easy integration

```javascript
// web/static/js/webrtc-engine.js
class WebRTCEngine {
    constructor(config) {
        this.streamId = config.streamId;
        this.videoElement = config.videoElement;
        this.autoReconnect = config.autoReconnect ?? true;

        this.eventHandlers = {
            'connected': [],
            'disconnected': [],
            'error': [],
            'stats': []
        };
    }

    on(event, callback) {
        this.eventHandlers[event].push(callback);
        return this;
    }

    async connect() {
        await this.connectWebSocket();
        await this.createPeerConnection();
        await this.createOffer();
    }
}
```

**Usage**:
```javascript
// Single stream
const engine = new WebRTCEngine({
    streamId: 'camera_01',
    videoElement: document.getElementById('video1')
});

engine.on('connected', () => console.log('Connected!'));
engine.on('stats', (stats) => updateUI(stats));
await engine.connect();

// Multi-camera dashboard
const engines = {};
for (const streamId of cameraIds) {
    engines[streamId] = new WebRTCEngine({
        streamId: streamId,
        videoElement: document.getElementById(`video-${streamId}`)
    });
    await engines[streamId].connect();
}
```

**Why**: Clean separation, multiple instances, reusable across projects.

---

## 🐛 Common Issues & Solutions

### Issue 1: RTP Packets Not Received

**Symptom**: RTSP connects but no video in browser

**Cause**: Using deprecated OnPacketRTP or manual read loop

**Solution**: Use OnPacketRTPAny callback

```go
// ❌ Wrong (deprecated)
c.rtspClient.OnPacketRTP(...)

// ❌ Wrong (doesn't work in v4)
go func() {
    for {
        pkt, err := c.rtspClient.ReadPacketRTPOrRTCP()
    }
}()

// ✅ Correct
medi.OnPacketRTPAny(func(medi *media.Media, forma format.Format, pkt *rtp.Packet) {
    c.onPacket(pkt)
})
```

---

### Issue 2: ICE Connection Failed

**Symptom**: Browser console shows "ICE connection state: failed"

**Cause**: Answer SDP missing ICE candidates

**Solution**: Wait for gathering completion

```go
// ❌ Wrong (too early)
answer, _ := pc.CreateAnswer(nil)
pc.SetLocalDescription(answer)

// ✅ Correct (wait for ICE gathering)
<-webrtc.GatheringCompletePromise(pc)
answer, _ := pc.CreateAnswer(nil)
pc.SetLocalDescription(answer)
```

---

### Issue 3: RTSP 401 Unauthorized

**Symptom**: Log shows "bad status code: 401"

**Cause**: Special characters in password not URL-encoded

**Solution**: URL-encode credentials in config

```yaml
# ❌ Wrong
source: rtsp://admin:password!@#@192.168.1.100:554/stream

# ✅ Correct
source: rtsp://admin:password%21%40%23@192.168.1.100:554/stream
```

Encoding: `!` → `%21`, `@` → `%40`, `#` → `%23`

---

### Issue 4: "Stream Already Exists" Error

**Symptom**: Starting on-demand stream fails with duplicate error

**Cause**: Trying to create Stream object twice

**Solution**: Separate Stream and RTSP client creation

```go
// ❌ Wrong
func startOnDemandStream(streamID string) {
    app.addStream(streamID, source)  // Creates Stream + RTSP client
}

// ✅ Correct
func startOnDemandStream(streamID string) {
    stream, _ := app.streamManager.GetStream(streamID)  // Stream already exists
    app.startRTSPClient(streamID, pathConfig)            // Create RTSP client only
}
```

---

### Issue 5: Mutex Deadlock on Peer Close

**Symptom**: Second test hangs indefinitely after first succeeds

**Cause**: OnClose callback called synchronously while holding mutex

**Solution**: Use goroutine for callbacks

```go
// ❌ Wrong (deadlock)
peer.OnClose = func() {
    m.RemovePeer(peerID)  // Tries to acquire same mutex
}

// ✅ Correct (async)
peer.OnClose = func() {
    go func() {
        m.RemovePeer(peerID)
    }()
}
```

Also protect Close() with sync.Once:
```go
func (p *Peer) Close() {
    p.closeOnce.Do(func() {
        // Cleanup only once
    })
}
```

---

## 📦 Project Structure Template

```
project/
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── rtsp/
│   │   └── client.go           # RTSP client (gortsplib v4)
│   ├── core/
│   │   ├── stream.go           # Stream (Pub/Sub)
│   │   ├── stream_manager.go  # Stream registry
│   │   └── config.go           # Configuration structures
│   ├── webrtc/
│   │   ├── peer.go             # WebRTC peer connection
│   │   └── manager.go          # Peer management
│   ├── signaling/
│   │   └── server.go           # WebSocket signaling
│   └── api/
│       └── server.go           # HTTP API (Gin)
├── web/
│   └── static/
│       ├── dashboard.html      # Multi-camera view
│       ├── viewer.html         # Single stream view
│       └── js/
│           └── webrtc-engine.js  # Reusable library
├── configs/
│   └── config.yaml             # mediaMTX-style configuration
├── test/
│   └── e2e/
│       └── stream_test.go      # E2E tests
├── CLAUDE.md                   # Living documentation
├── README.md
└── go.mod
```

---

## 🚀 Quick Start Template

### 1. Configuration Setup

```yaml
# configs/config.yaml
server:
  http_port: 8080
  ws_port: 8081

paths:
  camera_01:
    source: rtsp://camera-url
    sourceOnDemand: no
    rtspTransport: tcp

webrtc:
  ice_servers:
    - urls:
      - "stun:stun.l.google.com:19302"
```

### 2. Main Server Setup

```go
// cmd/server/main.go
func main() {
    // Load config
    config := loadConfig("configs/config.yaml")

    // Create components
    streamManager := core.NewStreamManager()
    webrtcManager := webrtc.NewManager()
    signalingServer := signaling.NewServer(webrtcManager)
    apiServer := api.NewServer(streamManager, signalingServer)

    // Load streams from config
    loadStreamsFromConfig(config, streamManager)

    // Start servers
    go apiServer.Start(fmt.Sprintf(":%d", config.Server.HTTPPort))

    // Wait for shutdown
    <-ctx.Done()
}
```

### 3. Frontend Integration

```html
<!-- Single Camera View -->
<video id="video1" autoplay playsinline muted></video>
<script src="/static/js/webrtc-engine.js"></script>
<script>
const engine = new WebRTCEngine({
    streamId: 'camera_01',
    videoElement: document.getElementById('video1')
});

engine.on('connected', () => console.log('Streaming!'));
engine.on('error', (err) => console.error(err));
await engine.connect();
</script>
```

### 4. Multi-Camera Dashboard

```javascript
// Auto-connect all cameras
async function init() {
    const response = await fetch('/api/v1/streams');
    const { streams } = await response.json();

    // Wait for DOM ready
    setTimeout(() => {
        streams.forEach(stream => connectCamera(stream.id));
    }, 1000);
}

async function connectCamera(streamId) {
    // Start on-demand stream if needed
    const streamInfo = streams.find(s => s.id === streamId);
    if (streamInfo.onDemand && streamInfo.status === 'stopped') {
        await fetch(`/api/v1/streams/${streamId}/start`, { method: 'POST' });
        await sleep(1500);
    }

    // Connect WebRTC
    const engine = new WebRTCEngine({
        streamId: streamId,
        videoElement: document.getElementById(`video-${streamId}`)
    });

    engine.on('connected', () => updateStatus(streamId, 'connected'));
    await engine.connect();
}
```

---

## 🧪 Testing Strategy

### E2E Test Pattern

```go
// test/e2e/stream_test.go
func TestVideoStreaming(t *testing.T) {
    // Setup
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Create WebRTC client
    pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})

    // Create offer
    offer, _ := pc.CreateOffer(nil)
    pc.SetLocalDescription(offer)

    // Send to server via WebSocket
    ws := connectWebSocket("ws://localhost:8080/ws")
    sendOffer(ws, offer.SDP)

    // Receive answer
    answer := receiveAnswer(ws)
    pc.SetRemoteDescription(answer)

    // Wait for packets
    packetsReceived := 0
    pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
        for {
            _, _, err := track.ReadRTP()
            if err != nil {
                break
            }
            packetsReceived++
        }
    })

    <-ctx.Done()
    assert.Greater(t, packetsReceived, 100, "Should receive video packets")
}
```

---

## 📊 Performance Considerations

### Metrics to Monitor
- Active streams count
- Connected peers count
- RTP packets per second
- WebRTC bitrate
- Memory usage
- Goroutine count

### Optimization Tips
1. **Buffer Sizes**: Tune video/audio buffer sizes based on network
2. **Goroutine Pools**: Limit concurrent goroutines
3. **GC Tuning**: Adjust GOGC for media server workloads
4. **Connection Limits**: Set max peers per stream
5. **On-Demand Streams**: Use sourceOnDemand to save resources

---

## 🔐 Security Best Practices

1. **RTSP Credentials**: Always URL-encode, never commit to git
2. **WebSocket Origin**: Validate Origin header in production
3. **HTTPS/WSS**: Use TLS in production
4. **Authentication**: Add JWT-based auth for API endpoints
5. **Rate Limiting**: Prevent connection spam
6. **Input Validation**: Sanitize stream IDs and parameters

---

## 📚 References

### Libraries
- [pion/webrtc](https://github.com/pion/webrtc) - Pure Go WebRTC implementation
- [bluenviron/gortsplib](https://github.com/bluenviron/gortsplib) - RTSP client/server library
- [bluenviron/mediamtx](https://github.com/bluenviron/mediamtx) - Reference architecture

### Protocols
- [WebRTC Specification](https://webrtc.org/)
- [RTSP RFC 2326](https://tools.ietf.org/html/rfc2326)
- [RTP RFC 3550](https://tools.ietf.org/html/rfc3550)

---

## 🔄 Skill Maintenance Log

### Version 1.0 (2025-10-29)
- Initial skill creation
- Based on cctv3 project v0.2.0
- Includes 4 CCTV production deployment experience
- mediaMTX-style configuration pattern
- Reusable WebRTC engine library

### Future Updates
- Add PTZ camera control patterns
- Add recording/playback functionality
- Add TURN server configuration
- Add Prometheus metrics integration
- Add Docker deployment patterns

---

**Last Updated**: 2025-10-29
**Based on Project**: cctv3 v0.2.0
**Status**: Production-ready ✅
