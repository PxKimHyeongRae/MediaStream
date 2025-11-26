package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

// DB는 데이터베이스 연결을 관리합니다
type DB struct {
	conn   *sql.DB
	logger *zap.Logger
}

// New는 새로운 데이터베이스 연결을 생성합니다
func New(dbPath string, logger *zap.Logger) (*DB, error) {
	// 데이터베이스 디렉토리 생성
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// SQLite 연결 열기
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 연결 테스트
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{
		conn:   conn,
		logger: logger,
	}

	// 테이블 초기화
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	logger.Info("Database initialized successfully",
		zap.String("path", dbPath),
	)

	return db, nil
}

// migrate는 데이터베이스 스키마를 초기화합니다
func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS streams (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		source TEXT NOT NULL,
		source_on_demand BOOLEAN NOT NULL DEFAULT 1,
		rtsp_transport TEXT NOT NULL DEFAULT 'tcp',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_streams_name ON streams(name);
	CREATE INDEX IF NOT EXISTS idx_streams_created_at ON streams(created_at);
	`

	if _, err := db.conn.Exec(schema); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	db.logger.Info("Database schema migrated successfully")
	return nil
}

// Close는 데이터베이스 연결을 닫습니다
func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Conn은 기본 SQL 연결을 반환합니다
func (db *DB) Conn() *sql.DB {
	return db.conn
}
