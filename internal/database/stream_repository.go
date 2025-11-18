package database

import (
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Stream은 스트림 정보를 나타냅니다
type Stream struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Source         string    `json:"source"`
	SourceOnDemand bool      `json:"source_on_demand"`
	RTSPTransport  string    `json:"rtsp_transport"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// StreamRepository는 스트림 데이터 액세스 레이어입니다
type StreamRepository struct {
	db     *DB
	logger *zap.Logger
}

// NewStreamRepository는 새로운 StreamRepository를 생성합니다
func NewStreamRepository(db *DB, logger *zap.Logger) *StreamRepository {
	return &StreamRepository{
		db:     db,
		logger: logger,
	}
}

// Create는 새로운 스트림을 생성합니다
func (r *StreamRepository) Create(stream *Stream) error {
	query := `
		INSERT INTO streams (id, name, source, source_on_demand, rtsp_transport, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	stream.CreatedAt = now
	stream.UpdatedAt = now

	_, err := r.db.Conn().Exec(
		query,
		stream.ID,
		stream.Name,
		stream.Source,
		stream.SourceOnDemand,
		stream.RTSPTransport,
		stream.CreatedAt,
		stream.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	r.logger.Info("Stream created",
		zap.String("id", stream.ID),
		zap.String("name", stream.Name),
	)

	return nil
}

// Get은 ID로 스트림을 조회합니다
func (r *StreamRepository) Get(id string) (*Stream, error) {
	query := `
		SELECT id, name, source, source_on_demand, rtsp_transport, created_at, updated_at
		FROM streams
		WHERE id = ?
	`

	stream := &Stream{}
	err := r.db.Conn().QueryRow(query, id).Scan(
		&stream.ID,
		&stream.Name,
		&stream.Source,
		&stream.SourceOnDemand,
		&stream.RTSPTransport,
		&stream.CreatedAt,
		&stream.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("stream not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	return stream, nil
}

// List는 모든 스트림을 조회합니다
func (r *StreamRepository) List() ([]*Stream, error) {
	query := `
		SELECT id, name, source, source_on_demand, rtsp_transport, created_at, updated_at
		FROM streams
		ORDER BY created_at DESC
	`

	rows, err := r.db.Conn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query streams: %w", err)
	}
	defer rows.Close()

	var streams []*Stream
	for rows.Next() {
		stream := &Stream{}
		err := rows.Scan(
			&stream.ID,
			&stream.Name,
			&stream.Source,
			&stream.SourceOnDemand,
			&stream.RTSPTransport,
			&stream.CreatedAt,
			&stream.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan stream: %w", err)
		}
		streams = append(streams, stream)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating streams: %w", err)
	}

	return streams, nil
}

// Update는 스트림을 업데이트합니다
func (r *StreamRepository) Update(stream *Stream) error {
	query := `
		UPDATE streams
		SET name = ?, source = ?, source_on_demand = ?, rtsp_transport = ?, updated_at = ?
		WHERE id = ?
	`

	stream.UpdatedAt = time.Now()

	result, err := r.db.Conn().Exec(
		query,
		stream.Name,
		stream.Source,
		stream.SourceOnDemand,
		stream.RTSPTransport,
		stream.UpdatedAt,
		stream.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update stream: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("stream not found: %s", stream.ID)
	}

	r.logger.Info("Stream updated",
		zap.String("id", stream.ID),
		zap.String("name", stream.Name),
	)

	return nil
}

// Delete는 스트림을 삭제합니다
func (r *StreamRepository) Delete(id string) error {
	query := `DELETE FROM streams WHERE id = ?`

	result, err := r.db.Conn().Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete stream: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("stream not found: %s", id)
	}

	r.logger.Info("Stream deleted",
		zap.String("id", id),
	)

	return nil
}

// Exists는 스트림이 존재하는지 확인합니다
func (r *StreamRepository) Exists(id string) (bool, error) {
	query := `SELECT COUNT(*) FROM streams WHERE id = ?`

	var count int
	err := r.db.Conn().QueryRow(query, id).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check stream existence: %w", err)
	}

	return count > 0, nil
}

// Count는 스트림 개수를 반환합니다
func (r *StreamRepository) Count() (int, error) {
	query := `SELECT COUNT(*) FROM streams`

	var count int
	err := r.db.Conn().QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count streams: %w", err)
	}

	return count, nil
}
