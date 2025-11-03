package process

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Process는 실행 중인 외부 프로세스 정보
type Process struct {
	ID           string
	Cmd          *exec.Cmd
	Command      string
	Restart      bool
	CloseAfter   time.Duration
	cancelFunc   context.CancelFunc
	lastActivity time.Time
	mu           sync.RWMutex
}

// Manager는 runOnDemand 프로세스를 관리합니다
type Manager struct {
	processes map[string]*Process
	mu        sync.RWMutex
	logger    *zap.Logger
}

// NewManager는 새로운 프로세스 매니저를 생성합니다
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		processes: make(map[string]*Process),
		logger:    logger,
	}
}

// Start는 새로운 프로세스를 시작합니다
func (m *Manager) Start(id, command string, restart bool, closeAfter time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 이미 실행 중인 프로세스가 있으면 에러
	if proc, exists := m.processes[id]; exists {
		if proc.Cmd != nil && proc.Cmd.Process != nil {
			return fmt.Errorf("process %s is already running", id)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 프로세스 생성
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdout = nil
	cmd.Stderr = nil

	// 프로세스 시작
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start process: %w", err)
	}

	proc := &Process{
		ID:           id,
		Cmd:          cmd,
		Command:      command,
		Restart:      restart,
		CloseAfter:   closeAfter,
		cancelFunc:   cancel,
		lastActivity: time.Now(),
	}

	m.processes[id] = proc

	m.logger.Info("Process started",
		zap.String("id", id),
		zap.Int("pid", cmd.Process.Pid),
		zap.Bool("restart", restart),
		zap.Duration("closeAfter", closeAfter),
	)

	// 프로세스 감시 고루틴 시작
	go m.monitorProcess(proc)

	return nil
}

// Stop은 프로세스를 중지합니다
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	proc, exists := m.processes[id]
	if !exists {
		return fmt.Errorf("process %s not found", id)
	}

	// Context 취소로 프로세스 종료
	if proc.cancelFunc != nil {
		proc.cancelFunc()
	}

	// 프로세스가 정리될 때까지 잠시 대기
	time.Sleep(100 * time.Millisecond)

	delete(m.processes, id)

	m.logger.Info("Process stopped", zap.String("id", id))

	return nil
}

// UpdateActivity는 프로세스의 마지막 활동 시간을 갱신합니다
func (m *Manager) UpdateActivity(id string) {
	m.mu.RLock()
	proc, exists := m.processes[id]
	m.mu.RUnlock()

	if !exists {
		return
	}

	proc.mu.Lock()
	proc.lastActivity = time.Now()
	proc.mu.Unlock()
}

// IsRunning은 프로세스가 실행 중인지 확인합니다
func (m *Manager) IsRunning(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	proc, exists := m.processes[id]
	if !exists {
		return false
	}

	return proc.Cmd != nil && proc.Cmd.Process != nil
}

// monitorProcess는 프로세스를 감시하고 필요시 재시작/종료합니다
func (m *Manager) monitorProcess(proc *Process) {
	// 프로세스 종료 대기
	err := proc.Cmd.Wait()

	m.logger.Info("Process exited",
		zap.String("id", proc.ID),
		zap.Error(err),
	)

	// 재시작이 필요한 경우
	if proc.Restart && err != nil {
		m.logger.Info("Restarting process", zap.String("id", proc.ID))
		time.Sleep(2 * time.Second) // 재시작 전 잠시 대기

		// 재시작 시도
		if err := m.Start(proc.ID, proc.Command, proc.Restart, proc.CloseAfter); err != nil {
			m.logger.Error("Failed to restart process",
				zap.String("id", proc.ID),
				zap.Error(err),
			)
		}
		return
	}

	// 프로세스 목록에서 제거
	m.mu.Lock()
	delete(m.processes, proc.ID)
	m.mu.Unlock()
}

// StartInactivityMonitor는 비활동 시간을 체크하는 고루틴을 시작합니다
func (m *Manager) StartInactivityMonitor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkInactiveProcesses()
		}
	}
}

// checkInactiveProcesses는 비활동 프로세스를 확인하고 종료합니다
func (m *Manager) checkInactiveProcesses() {
	m.mu.RLock()
	var toStop []string

	for id, proc := range m.processes {
		if proc.CloseAfter <= 0 {
			continue // closeAfter가 설정되지 않은 경우 스킵
		}

		proc.mu.RLock()
		inactive := time.Since(proc.lastActivity)
		proc.mu.RUnlock()

		if inactive > proc.CloseAfter {
			toStop = append(toStop, id)
		}
	}
	m.mu.RUnlock()

	// 비활동 프로세스 종료
	for _, id := range toStop {
		m.logger.Info("Stopping inactive process",
			zap.String("id", id),
		)
		if err := m.Stop(id); err != nil {
			m.logger.Error("Failed to stop inactive process",
				zap.String("id", id),
				zap.Error(err),
			)
		}
	}
}

// StopAll은 모든 프로세스를 중지합니다
func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.processes))
	for id := range m.processes {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		if err := m.Stop(id); err != nil {
			m.logger.Error("Failed to stop process",
				zap.String("id", id),
				zap.Error(err),
			)
		}
	}
}
