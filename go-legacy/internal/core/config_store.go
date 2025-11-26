package core

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
)

// ConfigStore는 동적 경로 설정을 관리합니다
type ConfigStore struct {
	paths      map[string]PathConfig
	mu         sync.RWMutex
	filePath   string
	logger     *zap.Logger
	yamlPaths  map[string]PathConfig // YAML에서 로드된 원본 paths (읽기 전용)
}

// NewConfigStore는 새로운 ConfigStore를 생성합니다
func NewConfigStore(yamlPaths map[string]PathConfig, runtimeFilePath string, logger *zap.Logger) *ConfigStore {
	store := &ConfigStore{
		paths:     make(map[string]PathConfig),
		yamlPaths: make(map[string]PathConfig),
		filePath:  runtimeFilePath,
		logger:    logger,
	}

	// YAML paths 복사 (읽기 전용)
	for id, config := range yamlPaths {
		store.yamlPaths[id] = config
		store.paths[id] = config
	}

	// 런타임 설정 로드 시도
	if err := store.LoadFromFile(); err != nil {
		logger.Warn("Failed to load runtime config, using YAML config only", zap.Error(err))
	}

	return store
}

// AddPath는 새로운 경로를 추가합니다
func (cs *ConfigStore) AddPath(id string, config PathConfig) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// 이미 존재하는 경로인지 확인
	if _, exists := cs.paths[id]; exists {
		return fmt.Errorf("path %s already exists", id)
	}

	// 유효성 검증
	if err := cs.validatePathConfig(id, config); err != nil {
		return fmt.Errorf("invalid path config: %w", err)
	}

	cs.paths[id] = config

	// 파일에 저장
	if err := cs.saveToFileUnsafe(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	cs.logger.Info("Path added", zap.String("id", id))
	return nil
}

// GetPath는 특정 경로 설정을 가져옵니다
func (cs *ConfigStore) GetPath(id string) (PathConfig, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	config, exists := cs.paths[id]
	return config, exists
}

// GetAllPaths는 모든 경로 설정을 가져옵니다
func (cs *ConfigStore) GetAllPaths() map[string]PathConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// 복사본 반환
	result := make(map[string]PathConfig, len(cs.paths))
	for id, config := range cs.paths {
		result[id] = config
	}
	return result
}

// UpdatePath는 경로 설정을 업데이트합니다
func (cs *ConfigStore) UpdatePath(id string, config PathConfig) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// 존재하는 경로인지 확인
	if _, exists := cs.paths[id]; !exists {
		return fmt.Errorf("path %s not found", id)
	}

	// YAML에서 로드된 경로는 수정 불가
	if _, isYamlPath := cs.yamlPaths[id]; isYamlPath {
		return fmt.Errorf("cannot update YAML-defined path %s, use API-only paths", id)
	}

	// 유효성 검증
	if err := cs.validatePathConfig(id, config); err != nil {
		return fmt.Errorf("invalid path config: %w", err)
	}

	cs.paths[id] = config

	// 파일에 저장
	if err := cs.saveToFileUnsafe(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	cs.logger.Info("Path updated", zap.String("id", id))
	return nil
}

// DeletePath는 경로를 삭제합니다
func (cs *ConfigStore) DeletePath(id string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// 존재하는 경로인지 확인
	if _, exists := cs.paths[id]; !exists {
		return fmt.Errorf("path %s not found", id)
	}

	// YAML에서 로드된 경로는 삭제 불가
	if _, isYamlPath := cs.yamlPaths[id]; isYamlPath {
		return fmt.Errorf("cannot delete YAML-defined path %s", id)
	}

	delete(cs.paths, id)

	// 파일에 저장
	if err := cs.saveToFileUnsafe(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	cs.logger.Info("Path deleted", zap.String("id", id))
	return nil
}

// LoadFromFile은 런타임 설정 파일을 로드합니다
func (cs *ConfigStore) LoadFromFile() error {
	data, err := os.ReadFile(cs.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 파일이 없으면 정상 (처음 실행)
			return nil
		}
		return fmt.Errorf("failed to read runtime config: %w", err)
	}

	var runtimePaths map[string]PathConfig
	if err := json.Unmarshal(data, &runtimePaths); err != nil {
		return fmt.Errorf("failed to parse runtime config: %w", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	// 런타임 설정을 paths에 추가 (YAML 설정은 덮어쓰지 않음)
	for id, config := range runtimePaths {
		if _, isYamlPath := cs.yamlPaths[id]; !isYamlPath {
			cs.paths[id] = config
		}
	}

	cs.logger.Info("Runtime config loaded", zap.Int("count", len(runtimePaths)))
	return nil
}

// SaveToFile은 런타임 설정을 파일에 저장합니다
func (cs *ConfigStore) SaveToFile() error {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.saveToFileUnsafe()
}

// saveToFileUnsafe는 mutex 없이 파일에 저장합니다 (내부용)
func (cs *ConfigStore) saveToFileUnsafe() error {
	// API로 추가된 경로만 저장 (YAML 경로 제외)
	runtimePaths := make(map[string]PathConfig)
	for id, config := range cs.paths {
		if _, isYamlPath := cs.yamlPaths[id]; !isYamlPath {
			runtimePaths[id] = config
		}
	}

	data, err := json.MarshalIndent(runtimePaths, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal runtime config: %w", err)
	}

	if err := os.WriteFile(cs.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write runtime config: %w", err)
	}

	return nil
}

// validatePathConfig는 경로 설정의 유효성을 검증합니다
func (cs *ConfigStore) validatePathConfig(id string, config PathConfig) error {
	if id == "" {
		return fmt.Errorf("path ID cannot be empty")
	}

	// Source와 RunOnDemand 중 하나는 반드시 있어야 함
	if config.Source == "" && config.RunOnDemand == "" {
		return fmt.Errorf("either source or runOnDemand must be specified")
	}

	// Source와 RunOnDemand가 동시에 있으면 에러
	if config.Source != "" && config.RunOnDemand != "" {
		return fmt.Errorf("source and runOnDemand cannot be used together")
	}

	return nil
}

// IsYAMLPath는 해당 경로가 YAML에서 정의된 것인지 확인합니다
func (cs *ConfigStore) IsYAMLPath(id string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	_, isYaml := cs.yamlPaths[id]
	return isYaml
}
