package signaling

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Server는 WebSocket 기반 시그널링 서버입니다
type Server struct {
	logger   *zap.Logger
	upgrader websocket.Upgrader

	clients map[*Client]bool
	mutex   sync.RWMutex

	// 콜백
	onOffer func(offer string, streamID string, client *Client) (answer string, err error)
	onClose func(clientID string)
}

// Client는 WebSocket 클라이언트를 나타냅니다
type Client struct {
	id     string
	conn   *websocket.Conn
	send   chan []byte
	server *Server
	logger *zap.Logger
}

// Message는 시그널링 메시지를 나타냅니다
type Message struct {
	Type    string          `json:"type"`    // "offer", "answer", "ice"
	Payload json.RawMessage `json:"payload"` // SDP (string) or ICE candidate (object)
}

// OfferPayload는 Offer 페이로드를 나타냅니다
type OfferPayload struct {
	SDP      string `json:"sdp"`
	StreamID string `json:"streamId"`
}

// ServerConfig는 시그널링 서버 설정
type ServerConfig struct {
	Logger  *zap.Logger
	OnOffer func(offer string, streamID string, client *Client) (answer string, err error)
	OnClose func(clientID string)
}

// NewServer는 새로운 시그널링 서버를 생성합니다
func NewServer(config ServerConfig) *Server {
	return &Server{
		logger: config.Logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 개발 모드: 모든 origin 허용
			},
		},
		clients: make(map[*Client]bool),
		onOffer: config.OnOffer,
		onClose: config.OnClose,
	}
}

// HandleWebSocket은 WebSocket 연결을 처리합니다
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade connection",
			zap.Error(err),
		)
		return
	}

	clientID := generateClientID()
	client := &Client{
		id:     clientID,
		conn:   conn,
		send:   make(chan []byte, 256),
		server: s,
		logger: s.logger.With(zap.String("client_id", clientID)),
	}

	s.registerClient(client)

	// 읽기/쓰기 고루틴 시작
	go client.writePump()
	go client.readPump()

	client.logger.Info("WebSocket client connected",
		zap.String("remote_addr", r.RemoteAddr),
	)
}

// registerClient는 클라이언트를 등록합니다
func (s *Server) registerClient(client *Client) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.clients[client] = true

	s.logger.Info("Client registered",
		zap.String("client_id", client.id),
		zap.Int("total_clients", len(s.clients)),
	)
}

// unregisterClient는 클라이언트를 등록 해제합니다
func (s *Server) unregisterClient(client *Client) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.clients[client]; exists {
		delete(s.clients, client)
		close(client.send)

		s.logger.Info("Client unregistered",
			zap.String("client_id", client.id),
			zap.Int("total_clients", len(s.clients)),
		)

		if s.onClose != nil {
			s.onClose(client.id)
		}
	}
}

// readPump은 WebSocket에서 메시지를 읽습니다
func (c *Client) readPump() {
	defer func() {
		c.server.unregisterClient(c)
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("WebSocket error", zap.Error(err))
			}
			break
		}

		c.handleMessage(message)
	}
}

// writePump은 WebSocket으로 메시지를 씁니다
func (c *Client) writePump() {
	defer c.conn.Close()

	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			c.logger.Error("Failed to write message", zap.Error(err))
			break
		}
	}
}

// handleMessage는 클라이언트 메시지를 처리합니다
func (c *Client) handleMessage(data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		c.logger.Error("Failed to parse message", zap.Error(err))
		return
	}

	c.logger.Debug("Received message",
		zap.String("type", msg.Type),
	)

	switch msg.Type {
	case "offer":
		// Offer는 OfferPayload로 전달됨 (sdp + streamId)
		var offerPayload OfferPayload
		if err := json.Unmarshal(msg.Payload, &offerPayload); err != nil {
			c.logger.Error("Failed to parse offer payload", zap.Error(err))
			return
		}
		c.handleOffer(offerPayload.SDP, offerPayload.StreamID)
	case "ice":
		// ICE candidate는 object로 전달됨
		c.handleICE(msg.Payload)
	default:
		c.logger.Warn("Unknown message type", zap.String("type", msg.Type))
	}
}

// handleOffer는 Offer를 처리합니다
func (c *Client) handleOffer(offer string, streamID string) {
	if c.server.onOffer == nil {
		c.logger.Error("No offer handler configured")
		return
	}

	c.logger.Info("Processing offer",
		zap.String("stream_id", streamID),
	)

	answer, err := c.server.onOffer(offer, streamID, c)
	if err != nil {
		c.logger.Error("Failed to handle offer", zap.Error(err))
		c.SendError(err.Error())
		return
	}

	c.SendAnswer(answer)
}

// handleICE는 ICE candidate를 처리합니다
func (c *Client) handleICE(candidateData json.RawMessage) {
	// ICE candidate는 브라우저에서 object 형태로 전달됨
	// 예: {"candidate": "...", "sdpMLineIndex": 0, "sdpMid": "0"}
	// 현재는 로깅만 수행 (실제 처리는 브라우저가 trickle ICE로 자동 처리)
	c.logger.Debug("ICE candidate received", zap.ByteString("candidate", candidateData))
}

// SendAnswer는 Answer를 전송합니다
func (c *Client) SendAnswer(answer string) {
	// Answer는 이미 문자열이므로 직접 RawMessage로 변환
	// json.Marshal(string)을 사용하면 이중 인코딩됨: "sdp" → "\"sdp\""
	// 대신 string을 JSON으로 직접 인코딩
	answerJSON, err := json.Marshal(answer)
	if err != nil {
		c.logger.Error("Failed to marshal answer string", zap.Error(err))
		return
	}

	msg := Message{
		Type:    "answer",
		Payload: answerJSON, // 이미 JSON으로 인코딩된 string
	}

	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("Failed to marshal answer message", zap.Error(err))
		return
	}

	select {
	case c.send <- data:
		c.logger.Info("Answer sent")
	default:
		c.logger.Error("Send channel full, dropping answer")
	}
}

// SendError는 에러 메시지를 전송합니다
func (c *Client) SendError(errorMsg string) {
	// Error를 JSON string으로 마샬링
	errorJSON, err := json.Marshal(errorMsg)
	if err != nil {
		c.logger.Error("Failed to marshal error string", zap.Error(err))
		return
	}

	msg := Message{
		Type:    "error",
		Payload: errorJSON,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("Failed to marshal error message", zap.Error(err))
		return
	}

	select {
	case c.send <- data:
	default:
		c.logger.Error("Send channel full, dropping error")
	}
}

// GetID는 클라이언트 ID를 반환합니다
func (c *Client) GetID() string {
	return c.id
}

// generateClientID는 고유한 클라이언트 ID를 생성합니다
func generateClientID() string {
	// 간단한 구현: UUID 사용 권장
	return "client-" + randomString(8)
}

// randomString은 랜덤 문자열을 생성합니다
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	// 랜덤 시드 초기화 (Go 1.20+ 에서는 자동으로 초기화됨)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range b {
		b[i] = letters[rng.Intn(len(letters))]
	}
	return string(b)
}

// GetClientCount는 연결된 클라이언트 수를 반환합니다
func (s *Server) GetClientCount() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return len(s.clients)
}

// Close는 모든 클라이언트 연결을 종료합니다
func (s *Server) Close() {
	s.logger.Info("Closing signaling server")

	s.mutex.Lock()
	defer s.mutex.Unlock()

	for client := range s.clients {
		client.conn.Close()
		delete(s.clients, client)
	}
}
