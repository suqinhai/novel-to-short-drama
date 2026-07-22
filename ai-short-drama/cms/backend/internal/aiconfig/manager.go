package aiconfig

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrInvalidInput = errors.New("invalid AI configuration input")

type FieldSpec struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Category    string   `json:"category"`
	Kind        string   `json:"kind"`
	AllowEmpty  bool     `json:"allow_empty"`
	Options     []string `json:"options,omitempty"`
	Description string   `json:"description,omitempty"`
	Target      string   `json:"target,omitempty"`
}

type SecretSpec struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Kind        string `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`
	Target      string `json:"target,omitempty"`
}

var FieldSpecs = []FieldSpec{
	{Key: "AI_CONNECTION_MODE", Label: "AI 接入方案", Category: "接入方案", Kind: "select", Options: []string{"native", "custom", "gateway", "hybrid"}},
	{Key: "TEXT_API_SOURCE", Label: "文本接口来源", Category: "接入方案", Kind: "select", Options: []string{"native", "custom", "gateway"}},
	{Key: "IMAGE_API_SOURCE", Label: "图片接口来源", Category: "接入方案", Kind: "select", Options: []string{"native", "custom", "gateway"}},
	{Key: "VIDEO_API_SOURCE", Label: "视频接口来源", Category: "接入方案", Kind: "select", Options: []string{"native", "custom", "gateway"}},
	{Key: "TTS_API_SOURCE", Label: "语音接口来源", Category: "接入方案", Kind: "select", Options: []string{"native", "custom", "gateway"}},
	{Key: "MOCK_MODE", Label: "Mock 模式", Category: "运行模式", Kind: "boolean"},
	{Key: "LITELLM_BASE_URL", Label: "文本 API Base URL", Category: "运行模式", Kind: "url", Description: "兼容旧变量名；系统会追加 /v1/chat/completions。"},
	{Key: "TEXT_ANALYSIS_MODEL", Label: "小说分析模型", Category: "文本与质检模型", Kind: "text"},
	{Key: "STORY_BIBLE_MODEL", Label: "故事圣经模型", Category: "文本与质检模型", Kind: "text"},
	{Key: "EPISODE_PLANNING_MODEL", Label: "分集策划模型", Category: "文本与质检模型", Kind: "text"},
	{Key: "SCRIPT_WRITING_MODEL", Label: "剧本创作模型", Category: "文本与质检模型", Kind: "text"},
	{Key: "STORYBOARD_MODEL", Label: "分镜设计模型", Category: "文本与质检模型", Kind: "text"},
	{Key: "VISUAL_PROMPT_MODEL", Label: "视觉提示词模型", Category: "文本与质检模型", Kind: "text"},
	{Key: "QC_TEXT_MODEL", Label: "质检文本模型", Category: "文本与质检模型", Kind: "text"},
	{Key: "QC_VISION_MODEL", Label: "质检视觉模型", Category: "文本与质检模型", Kind: "text", AllowEmpty: true},
	{Key: "IMAGE_PROVIDER", Label: "图片供应商", Category: "图片生成", Kind: "select", Options: []string{"mock", "generic_openai_images", "generic_async_image"}},
	{Key: "IMAGE_MODEL", Label: "图片模型", Category: "图片生成", Kind: "text"},
	{Key: "IMAGE_API_BASE_URL", Label: "图片 API 地址", Category: "图片生成", Kind: "url"},
	{Key: "VIDEO_PROVIDER", Label: "视频供应商", Category: "视频生成", Kind: "select", Options: []string{"mock", "generic_sync_video", "generic_async_video"}},
	{Key: "VIDEO_MODEL", Label: "视频模型", Category: "视频生成", Kind: "suggest", Options: []string{"gemini-omni-flash-preview", "veo-3.1-generate-001", "veo-3.1-fast-generate-001", "mock-image-to-video"}, Description: "可选择 Gemini Omni、Veo 3.1 或 Veo 3.1 Fast，也可输入兼容接口支持的其他模型 ID。"},
	{Key: "VIDEO_API_BASE_URL", Label: "视频 API 地址", Category: "视频生成", Kind: "url"},
	{Key: "VIDEO_USE_GENERATED_AUDIO", Label: "保留模型生成的音频", Category: "视频生成", Kind: "boolean", Description: "关闭时移除模型原生音轨，继续使用系统自己的配音与混音。"},
	{Key: "VEO_PROJECT_ID", Label: "Google Cloud Project ID", Category: "Google 视频模型", Kind: "text", Description: "留空时从服务账号 JSON 自动读取。", AllowEmpty: true, Target: "video-adapter"},
	{Key: "VEO_LOCATION", Label: "Veo 区域", Category: "Google 视频模型", Kind: "text", Description: "Veo 3.1 默认使用 us-central1；Omni 固定使用 global。", Target: "video-adapter"},
	{Key: "VEO_GCS_OUTPUT_URI", Label: "Cloud Storage 输出目录", Category: "Google 视频模型", Kind: "gcs_uri", Description: "例如 gs://my-private-bucket/short-drama；Veo 和 Omni 共用此私有目录。", Target: "video-adapter"},
	{Key: "TTS_PROVIDER", Label: "语音供应商", Category: "语音合成", Kind: "select", Options: []string{"mock", "generic_sync_tts", "generic_async_tts"}},
	{Key: "TTS_MODEL", Label: "语音模型", Category: "语音合成", Kind: "text"},
	{Key: "TTS_API_BASE_URL", Label: "语音 API 地址", Category: "语音合成", Kind: "url"},
	{Key: "PUBLISH_PROVIDER", Label: "发布供应商", Category: "发布", Kind: "select", Options: []string{"manual_package", "generic_sync_publish", "generic_async_publish"}},
	{Key: "ALLOW_REAL_PUBLISH", Label: "允许真实发布", Category: "发布", Kind: "boolean"},
}

var SecretSpecs = []SecretSpec{
	{Key: "LITELLM_API_KEY", Label: "LiteLLM API Key"},
	{Key: "GLM_API_KEY", Label: "GLM API Key"},
	{Key: "IMAGE_API_KEY", Label: "图片 API Key"},
	{Key: "VIDEO_API_KEY", Label: "视频 API Key"},
	{Key: "VEO_SERVICE_ACCOUNT_JSON", Label: "Google 服务账号 JSON", Kind: "service_account_json", Description: "粘贴从 Google Cloud 下载的完整服务账号 JSON；系统只保存，不回显。", Target: "video-adapter"},
	{Key: "TTS_API_KEY", Label: "语音 API Key"},
	{Key: "PUBLISH_API_KEY", Label: "发布 API Key"},
}

type FieldState struct {
	FieldSpec
	CurrentValue       string `json:"current_value"`
	ManagedValue       string `json:"managed_value"`
	HasManagedOverride bool   `json:"has_managed_override"`
}

type SecretState struct {
	SecretSpec
	Configured                bool `json:"configured"`
	ManagedOverrideConfigured bool `json:"managed_override_configured"`
}

type Snapshot struct {
	Fields             []FieldState  `json:"fields"`
	Secrets            []SecretState `json:"secrets"`
	ContainerName      string        `json:"container_name"`
	ContainerStatus    string        `json:"container_status"`
	VideoAdapterName   string        `json:"video_adapter_name"`
	VideoAdapterStatus string        `json:"video_adapter_status"`
	Source             string        `json:"source"`
	ManagedFile        string        `json:"managed_file"`
	ManagedFileExists  bool          `json:"managed_file_exists"`
	PendingRestart     bool          `json:"pending_restart"`
	RestartCommand     string        `json:"restart_command"`
	SecretsExposed     bool          `json:"secrets_exposed"`
}

type SaveResult struct {
	SavedFieldCount  int
	SavedSecretCount int
}

type Manager struct {
	filePath              string
	containerName         string
	videoAdapterContainer string
	mu                    sync.Mutex
}

func New(filePath, containerName string, videoAdapterContainer ...string) *Manager {
	adapterName := "ai-short-drama-veo-adapter-1"
	if len(videoAdapterContainer) > 0 && strings.TrimSpace(videoAdapterContainer[0]) != "" {
		adapterName = videoAdapterContainer[0]
	}
	return &Manager{filePath: filePath, containerName: containerName, videoAdapterContainer: adapterName}
}

func (m *Manager) Snapshot(ctx context.Context) (Snapshot, error) {
	containerEnv, status, err := inspectContainerEnvironment(ctx, m.containerName)
	if err != nil {
		return Snapshot{}, err
	}
	managedEnv, exists, err := readEnvFile(m.filePath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read managed AI configuration: %w", err)
	}
	videoEnv, videoStatus, videoErr := inspectContainerEnvironment(ctx, m.videoAdapterContainer)
	if videoErr != nil {
		videoEnv = map[string]string{}
		videoStatus = "not-running"
	}

	snapshot := Snapshot{
		Fields: make([]FieldState, 0, len(FieldSpecs)), Secrets: make([]SecretState, 0, len(SecretSpecs)),
		ContainerName: m.containerName, ContainerStatus: status, Source: "n8n 与 Google 视频适配器容器环境",
		VideoAdapterName: m.videoAdapterContainer, VideoAdapterStatus: videoStatus,
		ManagedFile: "cms/config/cms-managed.env", ManagedFileExists: exists,
		RestartCommand: "$baseEnv = if (Test-Path .env) { '.env' } else { '.env.example' }; docker compose --profile veo --env-file $baseEnv --env-file cms/config/cms-managed.env up -d --build --force-recreate --no-deps n8n veo-adapter",
		SecretsExposed: false,
	}
	for _, spec := range FieldSpecs {
		managedValue, hasOverride := managedEnv[spec.Key]
		currentValue := containerEnv[spec.Key]
		if spec.Target == "video-adapter" {
			currentValue = videoEnv[spec.Key]
		}
		snapshot.Fields = append(snapshot.Fields, FieldState{
			FieldSpec: spec, CurrentValue: currentValue, ManagedValue: managedValue, HasManagedOverride: hasOverride,
		})
		if hasOverride && managedValue != currentValue {
			snapshot.PendingRestart = true
		}
	}
	for _, spec := range SecretSpecs {
		managedValue, hasOverride := managedEnv[spec.Key]
		currentValue := containerEnv[spec.Key]
		if spec.Target == "video-adapter" {
			currentValue = videoEnv[spec.Key]
		}
		snapshot.Secrets = append(snapshot.Secrets, SecretState{
			SecretSpec: spec, Configured: secretConfigured(currentValue),
			ManagedOverrideConfigured: hasOverride && secretConfigured(managedValue),
		})
		if hasOverride && managedValue != currentValue {
			snapshot.PendingRestart = true
		}
	}
	return snapshot, nil
}

func (m *Manager) Save(values, secrets map[string]string) (SaveResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	managed, _, err := readEnvFile(m.filePath)
	if err != nil {
		return SaveResult{}, fmt.Errorf("read managed AI configuration: %w", err)
	}
	fieldByKey := make(map[string]FieldSpec, len(FieldSpecs))
	for _, spec := range FieldSpecs {
		fieldByKey[spec.Key] = spec
	}
	secretKeys := make(map[string]SecretSpec, len(SecretSpecs))
	for _, spec := range SecretSpecs {
		secretKeys[spec.Key] = spec
	}

	result := SaveResult{}
	for key, value := range values {
		spec, allowed := fieldByKey[key]
		if !allowed || validateFieldValue(spec, value) != nil {
			return SaveResult{}, ErrInvalidInput
		}
		managed[key] = value
		result.SavedFieldCount++
	}
	for key, value := range secrets {
		spec, allowed := secretKeys[key]
		if !allowed || validateSecretValue(spec, value) != nil {
			return SaveResult{}, ErrInvalidInput
		}
		normalized, err := normalizeSecretValue(spec, value)
		if err != nil {
			return SaveResult{}, ErrInvalidInput
		}
		managed[key] = normalized
		result.SavedSecretCount++
	}
	if result.SavedFieldCount+result.SavedSecretCount == 0 {
		return SaveResult{}, ErrInvalidInput
	}
	if err := writeEnvFile(m.filePath, managed); err != nil {
		return SaveResult{}, fmt.Errorf("write managed AI configuration: %w", err)
	}
	return result, nil
}

func inspectContainerEnvironment(ctx context.Context, containerName string) (map[string]string, string, error) {
	inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(inspectCtx, "docker", "inspect", "--format", "{{json .Config.Env}}", containerName).Output()
	if err != nil {
		return nil, "unavailable", errors.New("n8n container environment is unavailable")
	}
	var entries []string
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, "unavailable", errors.New("n8n container environment is invalid")
	}
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		if key, value, ok := strings.Cut(entry, "="); ok {
			values[key] = value
		}
	}
	statusOutput, statusErr := exec.CommandContext(inspectCtx, "docker", "inspect", "--format", "{{.State.Status}}", containerName).Output()
	status := "available"
	if statusErr == nil && strings.TrimSpace(string(statusOutput)) != "" {
		status = strings.TrimSpace(string(statusOutput))
	}
	return values, status, nil
}

func validateFieldValue(spec FieldSpec, value string) error {
	if len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n") {
		return ErrInvalidInput
	}
	trimmed := strings.TrimSpace(value)
	switch spec.Kind {
	case "boolean":
		if trimmed != "true" && trimmed != "false" {
			return ErrInvalidInput
		}
	case "url":
		if trimmed == "" {
			return nil
		}
		parsed, err := url.ParseRequestURI(trimmed)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return ErrInvalidInput
		}
	case "select":
		valid := false
		for _, option := range spec.Options {
			if trimmed == option {
				valid = true
				break
			}
		}
		if !valid {
			return ErrInvalidInput
		}
	case "gcs_uri":
		if !strings.HasPrefix(trimmed, "gs://") {
			return ErrInvalidInput
		}
		withoutScheme := strings.TrimPrefix(trimmed, "gs://")
		bucket, _, _ := strings.Cut(withoutScheme, "/")
		if len(bucket) < 3 || len(bucket) > 222 || strings.ContainsAny(bucket, " \\?#") {
			return ErrInvalidInput
		}
	default:
		if (!spec.AllowEmpty && trimmed == "") || strings.Contains(value, "#") {
			return ErrInvalidInput
		}
	}
	return nil
}

func validateSecretValue(spec SecretSpec, value string) error {
	maxLength := 8192
	if spec.Kind == "service_account_json" {
		maxLength = 32768
	}
	if strings.TrimSpace(value) == "" || len(value) > maxLength || strings.ContainsRune(value, '\x00') {
		return ErrInvalidInput
	}
	if spec.Kind != "service_account_json" && strings.ContainsAny(value, "\r\n") {
		return ErrInvalidInput
	}
	_, err := normalizeSecretValue(spec, value)
	return err
}

func normalizeSecretValue(spec SecretSpec, value string) (string, error) {
	if spec.Kind != "service_account_json" {
		return value, nil
	}
	value = strings.TrimPrefix(strings.TrimSpace(value), "\ufeff")
	var credential struct {
		Type        string `json:"type"`
		ProjectID   string `json:"project_id"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal([]byte(value), &credential); err != nil || credential.Type != "service_account" || credential.ProjectID == "" || credential.ClientEmail == "" || credential.PrivateKey == "" {
		return "", ErrInvalidInput
	}
	var document any
	if err := json.Unmarshal([]byte(value), &document); err != nil {
		return "", ErrInvalidInput
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", ErrInvalidInput
	}
	return string(encoded), nil
}

func secretConfigured(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	for _, placeholder := range []string{"replace_me", "change_me", "replace_with", "your_api_key", "placeholder"} {
		if strings.Contains(value, placeholder) {
			return false
		}
	}
	return true
}

func readEnvFile(path string) (map[string]string, bool, error) {
	values := map[string]string{}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return values, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		if len(rawValue) >= 2 && strings.HasPrefix(rawValue, "\"") && strings.HasSuffix(rawValue, "\"") {
			if decoded, decodeErr := strconv.Unquote(rawValue); decodeErr == nil {
				rawValue = strings.ReplaceAll(decoded, "$$", "$")
			}
		}
		values[key] = rawValue
	}
	return values, true, scanner.Err()
}

func writeEnvFile(path string, values map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	allowedOrder := make([]string, 0, len(FieldSpecs)+len(SecretSpecs))
	for _, spec := range FieldSpecs {
		allowedOrder = append(allowedOrder, spec.Key)
	}
	for _, spec := range SecretSpecs {
		allowedOrder = append(allowedOrder, spec.Key)
	}
	var content strings.Builder
	content.WriteString("# Managed by the Short Drama CMS. Do not commit this file.\n")
	content.WriteString("# Recreate n8n and the Google video adapter with both .env files for changes to take effect.\n")
	for _, key := range allowedOrder {
		value, exists := values[key]
		if !exists {
			continue
		}
		content.WriteString(key)
		content.WriteByte('=')
		content.WriteString(encodeEnvValue(value))
		content.WriteByte('\n')
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(content.String()); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func encodeEnvValue(value string) string {
	return strconv.Quote(strings.ReplaceAll(value, "$", "$$"))
}
