package datacleanup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"short-drama-cms/backend/internal/store"
)

const ConfirmationPhrase = "永久删除全部数据"

var (
	ErrInProgress       = errors.New("data reset is already in progress")
	ErrAIConfigModified = errors.New("AI configuration changed during data reset")
	safeContainerName   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

type BusinessStore interface {
	BusinessDataSummary(context.Context) (store.BusinessDataSummary, error)
	ResetBusinessData(context.Context) (store.BusinessDataSummary, error)
}

type Config struct {
	StorageDirectory     string
	ManagedEnvFile       string
	N8NContainer         string
	MediaWorkerContainer string
	PostgresContainer    string
	RedisContainer       string
	PostgresUser         string
	N8NDatabase          string
}

type StorageSummary struct {
	FileCount int   `json:"file_count"`
	TotalSize int64 `json:"total_size"`
}

type Preview struct {
	Business           store.BusinessDataSummary `json:"business"`
	Storage            StorageSummary            `json:"storage"`
	ConfirmationPhrase string                    `json:"confirmation_phrase"`
	Preserved          []string                  `json:"preserved"`
	AIConfigFileExists bool                      `json:"ai_config_file_exists"`
	Destructive        bool                      `json:"destructive"`
}

type Result struct {
	DeletedBusinessRows int64    `json:"deleted_business_rows"`
	DeletedStorageFiles int      `json:"deleted_storage_files"`
	DeletedStorageBytes int64    `json:"deleted_storage_bytes"`
	AIConfigPreserved   bool     `json:"ai_config_preserved"`
	RestartedServices   []string `json:"restarted_services"`
	CompletedAt         string   `json:"completed_at"`
}

type Manager struct {
	store BusinessStore
	cfg   Config
	run   func(context.Context, string, ...string) ([]byte, error)
	mu    sync.Mutex
	busy  bool
}

func New(database BusinessStore, cfg Config) *Manager {
	return &Manager{store: database, cfg: cfg, run: runCommand}
}

func (m *Manager) Preview(ctx context.Context) (Preview, error) {
	if m.store == nil {
		return Preview{}, errors.New("business store is unavailable")
	}
	business, err := m.store.BusinessDataSummary(ctx)
	if err != nil {
		return Preview{}, err
	}
	storage, err := scanStorage(m.cfg.StorageDirectory)
	if err != nil {
		return Preview{}, err
	}
	_, exists, err := fileFingerprint(m.cfg.ManagedEnvFile)
	if err != nil {
		return Preview{}, err
	}
	return Preview{
		Business:           business,
		Storage:            storage,
		ConfirmationPhrase: ConfirmationPhrase,
		Preserved: []string{
			"AI 配置与密钥",
			"n8n 工作流、凭证与登录账号",
			"数据库结构、迁移记录与系统字典",
		},
		AIConfigFileExists: exists,
		Destructive:        true,
	}, nil
}

func (m *Manager) Reset(ctx context.Context) (result Result, err error) {
	if !m.begin() {
		return Result{}, ErrInProgress
	}
	defer m.end()

	beforeHash, beforeExists, err := fileFingerprint(m.cfg.ManagedEnvFile)
	if err != nil {
		return Result{}, err
	}
	if err := validateConfig(m.cfg); err != nil {
		return Result{}, err
	}

	stopped := make([]string, 0, 2)
	for _, container := range []string{m.cfg.N8NContainer, m.cfg.MediaWorkerContainer} {
		running, inspectErr := m.containerRunning(ctx, container)
		if inspectErr != nil {
			return Result{}, inspectErr
		}
		if running {
			if _, stopErr := m.run(ctx, "docker", "stop", container); stopErr != nil {
				return Result{}, fmt.Errorf("stop %s: %w", container, stopErr)
			}
			stopped = append(stopped, container)
		}
	}
	defer func() {
		restartCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		for _, container := range stopped {
			if _, restartErr := m.run(restartCtx, "docker", "start", container); restartErr != nil && err == nil {
				err = fmt.Errorf("restart %s: %w", container, restartErr)
			}
		}
		result.RestartedServices = append(result.RestartedServices, stopped...)
	}()

	business, err := m.store.ResetBusinessData(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := m.clearN8NHistory(ctx); err != nil {
		return Result{}, err
	}
	if err := m.flushRedis(ctx); err != nil {
		return Result{}, err
	}
	storage, err := clearStorage(m.cfg.StorageDirectory)
	if err != nil {
		return Result{}, err
	}

	afterHash, afterExists, err := fileFingerprint(m.cfg.ManagedEnvFile)
	if err != nil {
		return Result{}, err
	}
	if beforeExists != afterExists || beforeHash != afterHash {
		return Result{}, ErrAIConfigModified
	}
	return Result{
		DeletedBusinessRows: business.RowCount,
		DeletedStorageFiles: storage.FileCount,
		DeletedStorageBytes: storage.TotalSize,
		AIConfigPreserved:   true,
		CompletedAt:         time.Now().Format(time.RFC3339),
	}, nil
}

func (m *Manager) begin() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy {
		return false
	}
	m.busy = true
	return true
}

func (m *Manager) end() {
	m.mu.Lock()
	m.busy = false
	m.mu.Unlock()
}

func (m *Manager) containerRunning(ctx context.Context, container string) (bool, error) {
	output, err := m.run(ctx, "docker", "inspect", "-f", "{{.State.Running}}", container)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", container, err)
	}
	value, err := strconv.ParseBool(strings.TrimSpace(string(output)))
	if err != nil {
		return false, fmt.Errorf("inspect %s returned invalid running state", container)
	}
	return value, nil
}

func (m *Manager) clearN8NHistory(ctx context.Context) error {
	sql := `WITH removed AS (DELETE FROM execution_entity RETURNING 1) SELECT count(*) FROM removed;
TRUNCATE TABLE insights_raw, insights_by_period, insights_metadata, workflow_statistics RESTART IDENTITY CASCADE;`
	if _, err := m.run(
		ctx,
		"docker", "exec", m.cfg.PostgresContainer,
		"psql", "-X", "-v", "ON_ERROR_STOP=1",
		"-U", m.cfg.PostgresUser,
		"-d", m.cfg.N8NDatabase,
		"-c", sql,
	); err != nil {
		return fmt.Errorf("clear n8n history: %w", err)
	}
	return nil
}

func (m *Manager) flushRedis(ctx context.Context) error {
	output, err := m.run(ctx, "docker", "exec", m.cfg.RedisContainer, "redis-cli", "FLUSHALL")
	if err != nil {
		return fmt.Errorf("flush redis: %w", err)
	}
	if strings.TrimSpace(string(output)) != "OK" {
		return fmt.Errorf("flush redis returned %q", strings.TrimSpace(string(output)))
	}
	return nil
}

func validateConfig(cfg Config) error {
	for label, value := range map[string]string{
		"n8n container": cfg.N8NContainer, "media worker container": cfg.MediaWorkerContainer,
		"postgres container": cfg.PostgresContainer, "redis container": cfg.RedisContainer,
	} {
		if !safeContainerName.MatchString(value) {
			return fmt.Errorf("%s has an unsafe name", label)
		}
	}
	if strings.TrimSpace(cfg.StorageDirectory) == "" {
		return errors.New("storage directory is not configured")
	}
	if strings.TrimSpace(cfg.PostgresUser) == "" {
		return errors.New("postgres user is not configured")
	}
	if strings.TrimSpace(cfg.N8NDatabase) == "" {
		return errors.New("n8n database is not configured")
	}
	return nil
}

func scanStorage(root string) (StorageSummary, error) {
	absolute, err := safeStorageRoot(root)
	if err != nil {
		return StorageSummary{}, err
	}
	summary := StorageSummary{}
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || preserveStorageFile(absolute, path, entry.Name()) {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		summary.FileCount++
		summary.TotalSize += info.Size()
		return nil
	})
	if err != nil {
		return StorageSummary{}, fmt.Errorf("scan storage: %w", err)
	}
	return summary, nil
}

func clearStorage(root string) (StorageSummary, error) {
	absolute, err := safeStorageRoot(root)
	if err != nil {
		return StorageSummary{}, err
	}
	summary := StorageSummary{}
	directories := make([]string, 0, 64)
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != absolute {
				directories = append(directories, path)
			}
			return nil
		}
		if preserveStorageFile(absolute, path, entry.Name()) {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return removeErr
		}
		summary.FileCount++
		summary.TotalSize += info.Size()
		return nil
	})
	if err != nil {
		return StorageSummary{}, fmt.Errorf("clear storage: %w", err)
	}
	for index := len(directories) - 1; index >= 0; index-- {
		_ = os.Remove(directories[index])
	}
	return summary, nil
}

func safeStorageRoot(root string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", fmt.Errorf("resolve storage directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("open storage directory: %w", err)
	}
	if !info.IsDir() || filepath.Base(absolute) != "storage" {
		return "", fmt.Errorf("unsafe storage directory %q", absolute)
	}
	return absolute, nil
}

func preserveStorageFile(root, path, name string) bool {
	return name == ".gitkeep" || path == filepath.Join(root, "healthz")
}

func fileFingerprint(path string) (string, bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read AI configuration fingerprint: %w", err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), true, nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
