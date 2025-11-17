package logger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	// Log는 전역 로거 인스턴스
	Log *zap.Logger
	// logConfig는 로거 설정을 저장 (재초기화용)
	logConfig *LogConfig
	// fileWriter는 현재 파일 writer
	fileWriter *lumberjack.Logger
	// ctx는 로거 컨텍스트
	ctx context.Context
	// cancel은 로거 취소 함수
	cancel context.CancelFunc
)

// LogConfig는 로거 설정
type LogConfig struct {
	Level      string
	Output     string
	FilePath   string
	MaxSize    int
	MaxBackups int
	MaxAge     int
}

// InitLogger는 zap 로거를 초기화합니다
func InitLogger(cfg LogConfig) error {
	// 설정 저장 (재초기화용)
	logConfig = &cfg

	// 컨텍스트 생성
	ctx, cancel = context.WithCancel(context.Background())

	// 로거 초기화
	if err := initLoggerCore(cfg); err != nil {
		return err
	}

	// 파일 출력이 활성화된 경우 매일 자정에 로그 파일 로테이션
	if cfg.Output == "file" || cfg.Output == "both" {
		go dailyRotation()
	}

	return nil
}

// initLoggerCore는 로거 코어를 초기화합니다
func initLoggerCore(cfg LogConfig) error {
	// 로그 레벨 파싱
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	// 인코더 설정
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	// 출력 설정
	var core zapcore.Core

	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
	fileEncoder := zapcore.NewJSONEncoder(encoderConfig)

	switch cfg.Output {
	case "console":
		core = zapcore.NewCore(
			consoleEncoder,
			zapcore.AddSync(os.Stdout),
			level,
		)
	case "file":
		// 날짜별 로그 파일 생성
		fileWriter = getFileWriter(cfg)
		core = zapcore.NewCore(
			fileEncoder,
			zapcore.AddSync(fileWriter),
			level,
		)
	case "both":
		// 날짜별 로그 파일 생성
		fileWriter = getFileWriter(cfg)
		core = zapcore.NewTee(
			zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), level),
			zapcore.NewCore(fileEncoder, zapcore.AddSync(fileWriter), level),
		)
	default:
		core = zapcore.NewCore(
			consoleEncoder,
			zapcore.AddSync(os.Stdout),
			level,
		)
	}

	// 로거 생성
	Log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return nil
}

// getFileWriter는 날짜별 로그 파일 writer를 생성합니다
func getFileWriter(cfg LogConfig) *lumberjack.Logger {
	// 로그 디렉토리 생성
	logDir := filepath.Dir(cfg.FilePath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
	} else {
		// 절대 경로 가져오기
		absLogDir, _ := filepath.Abs(logDir)
		fmt.Printf("Log directory created: %s\n", absLogDir)
	}

	// 날짜별 파일명 생성
	// 예: logs/media-server.log -> logs/media-server-2025-11-17.log
	dailyFilePath := getDailyFilePath(cfg.FilePath)

	// 절대 경로 가져오기
	absFilePath, err := filepath.Abs(dailyFilePath)
	if err != nil {
		absFilePath = dailyFilePath
	}

	// 로그 파일 경로 출력
	fmt.Printf("Log file path: %s\n", absFilePath)
	fmt.Printf("Log rotation settings: max_size=%dMB, max_backups=%d, max_age=%ddays\n",
		cfg.MaxSize, cfg.MaxBackups, cfg.MaxAge)

	// lumberjack 설정
	return &lumberjack.Logger{
		Filename:   dailyFilePath,
		MaxSize:    cfg.MaxSize,    // MB (하루 로그 용량 예상치)
		MaxBackups: cfg.MaxBackups, // 보관할 최대 파일 개수
		MaxAge:     cfg.MaxAge,     // 일 단위 (오래된 로그 자동 삭제)
		LocalTime:  true,           // 로컬 시간 사용
		Compress:   true,           // 압축 활성화 (구 로그 파일 압축)
	}
}

// getDailyFilePath는 날짜를 포함한 로그 파일 경로를 생성합니다
func getDailyFilePath(basePath string) string {
	// 확장자와 파일명 분리
	ext := filepath.Ext(basePath)
	nameWithoutExt := strings.TrimSuffix(basePath, ext)

	// 현재 날짜 (YYYY-MM-DD 형식)
	today := time.Now().Format("2006-01-02")

	// 날짜를 포함한 파일명 생성
	// 예: logs/media-server.log -> logs/media-server-2025-11-17.log
	return fmt.Sprintf("%s-%s%s", nameWithoutExt, today, ext)
}

// dailyRotation은 매일 자정에 로그 파일을 로테이션합니다
func dailyRotation() {
	for {
		// 다음 자정까지의 시간 계산
		now := time.Now()
		tomorrow := now.Add(24 * time.Hour)
		midnight := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, now.Location())
		duration := midnight.Sub(now)

		// 자정까지 대기
		select {
		case <-time.After(duration):
			// 자정이 되면 로거 재초기화 (새로운 날짜의 파일 생성)
			if logConfig != nil {
				// 기존 로거 동기화
				if Log != nil {
					_ = Log.Sync()
				}
				// 기존 파일 writer 닫기
				if fileWriter != nil {
					_ = fileWriter.Close()
				}
				// 로거 재초기화 (새 날짜 파일 생성)
				_ = initLoggerCore(*logConfig)
			}
		case <-ctx.Done():
			// 종료 시그널 받으면 고루틴 종료
			return
		}
	}
}

// Close는 로거를 종료하고 리소스를 정리합니다
func Close() {
	if cancel != nil {
		cancel()
	}
	if Log != nil {
		_ = Log.Sync()
	}
	if fileWriter != nil {
		_ = fileWriter.Close()
	}
}

// Sync는 로거 버퍼를 플러시합니다
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}

// Info는 info 레벨 로그를 출력합니다
func Info(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Info(msg, fields...)
	}
}

// Debug는 debug 레벨 로그를 출력합니다
func Debug(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Debug(msg, fields...)
	}
}

// Warn는 warn 레벨 로그를 출력합니다
func Warn(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Warn(msg, fields...)
	}
}

// Error는 error 레벨 로그를 출력합니다
func Error(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Error(msg, fields...)
	}
}

// Fatal는 fatal 레벨 로그를 출력하고 프로그램을 종료합니다
func Fatal(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Fatal(msg, fields...)
	}
}
