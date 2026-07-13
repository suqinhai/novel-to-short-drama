package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ServiceSpec struct {
	Name           string
	ComposeService string
	ContainerName  string
}

type ServiceCheck struct {
	Name            string `json:"name"`
	ComposeService  string `json:"compose_service"`
	ContainerName   string `json:"container_name"`
	Status          string `json:"status"`
	ContainerStatus string `json:"container_status"`
	Health          string `json:"health"`
	Message         string `json:"message"`
	Suggestion      string `json:"suggestion,omitempty"`
	DurationMS      int64  `json:"duration_ms"`
}

type WorkflowRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type WorkflowActivationCheck struct {
	Status        string        `json:"status"`
	ExpectedCount int           `json:"expected_count"`
	ImportedCount int           `json:"imported_count"`
	ActiveCount   int           `json:"active_count"`
	Inactive      []WorkflowRef `json:"inactive"`
	Missing       []WorkflowRef `json:"missing"`
	Message       string        `json:"message"`
	Suggestion    string        `json:"suggestion,omitempty"`
}

type CredentialCheck struct {
	Status     string `json:"status"`
	Key        string `json:"key"`
	Exists     bool   `json:"exists"`
	Configured bool   `json:"configured"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type UnsupportedNode struct {
	WorkflowID   string `json:"workflow_id"`
	WorkflowName string `json:"workflow_name"`
	File         string `json:"file"`
	NodeName     string `json:"node_name"`
	NodeType     string `json:"node_type"`
	Disabled     bool   `json:"disabled"`
}

type UnsupportedNodeCheck struct {
	Status     string            `json:"status"`
	Count      int               `json:"count"`
	Nodes      []UnsupportedNode `json:"nodes"`
	Message    string            `json:"message"`
	Suggestion string            `json:"suggestion,omitempty"`
}

type Result struct {
	Services           []ServiceCheck          `json:"services"`
	WorkflowActivation WorkflowActivationCheck `json:"workflow_activation"`
	PostgresCredential CredentialCheck         `json:"postgres_credential"`
	ExecuteCommand     UnsupportedNodeCheck    `json:"execute_command"`
}

type Runner struct {
	services          []ServiceSpec
	n8nContainer      string
	postgresContainer string
	workflowDirectory string
}

func New(n8nContainer, postgresContainer, mediaContainer, mediaWorkerContainer, liteLLMContainer, workflowDirectory string) *Runner {
	return &Runner{
		services: []ServiceSpec{
			{Name: "n8n", ComposeService: "n8n", ContainerName: n8nContainer},
			{Name: "postgres", ComposeService: "postgres", ContainerName: postgresContainer},
			{Name: "media", ComposeService: "media", ContainerName: mediaContainer},
			{Name: "media-worker", ComposeService: "media-worker", ContainerName: mediaWorkerContainer},
			{Name: "litellm", ComposeService: "litellm", ContainerName: liteLLMContainer},
		},
		n8nContainer: n8nContainer, postgresContainer: postgresContainer, workflowDirectory: workflowDirectory,
	}
}

func (r *Runner) Run(ctx context.Context) Result {
	result := Result{Services: make([]ServiceCheck, 0, len(r.services))}
	for _, service := range r.services {
		result.Services = append(result.Services, inspectService(ctx, service))
	}
	expected, unsupportedNodes, scanErr := scanWorkflowFiles(r.workflowDirectory)
	result.ExecuteCommand = evaluateUnsupportedNodes(unsupportedNodes, scanErr)
	result.WorkflowActivation = r.workflowActivation(ctx, expected, scanErr)
	result.PostgresCredential = r.postgresCredential(ctx)
	return result
}

type dockerState struct {
	Status  string `json:"Status"`
	Running bool   `json:"Running"`
	Health  *struct {
		Status string `json:"Status"`
	} `json:"Health"`
}

func inspectService(ctx context.Context, spec ServiceSpec) ServiceCheck {
	started := time.Now()
	check := ServiceCheck{Name: spec.Name, ComposeService: spec.ComposeService, ContainerName: spec.ContainerName}
	commandCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "docker", "inspect", "--format", "{{json .State}}", spec.ContainerName).Output()
	check.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		check.Status, check.ContainerStatus, check.Health = "unhealthy", "missing", "missing"
		check.Message = "未找到容器或 Docker 无法访问。"
		check.Suggestion = fmt.Sprintf("在项目根目录使用实际 env 文件启动 %s：docker compose --env-file .env.example up -d %s；然后确认 health=healthy。", spec.Name, spec.ComposeService)
		return check
	}
	var state dockerState
	if json.Unmarshal(output, &state) != nil {
		check.Status, check.ContainerStatus, check.Health = "unhealthy", "unknown", "unknown"
		check.Message = "Docker 返回了无法识别的容器状态。"
		check.Suggestion = fmt.Sprintf("运行 docker inspect %s 和 docker compose logs --tail=200 %s 排查。", spec.ContainerName, spec.ComposeService)
		return check
	}
	check.ContainerStatus = state.Status
	if state.Health != nil {
		check.Health = state.Health.Status
	} else {
		check.Health = "not_configured"
	}
	switch {
	case state.Running && check.Health == "healthy":
		check.Status = "healthy"
		check.Message = "容器运行中，Docker health check 正常。"
	case state.Running && check.Health == "starting":
		check.Status = "degraded"
		check.Message = "容器正在运行，但 health check 尚未就绪。"
		check.Suggestion = fmt.Sprintf("等待片刻后重新诊断；若持续 starting，请查看 docker compose logs --tail=200 %s。", spec.ComposeService)
	case state.Running && check.Health == "not_configured":
		check.Status = "degraded"
		check.Message = "容器正在运行，但没有配置 Docker health check。"
		check.Suggestion = fmt.Sprintf("为 %s 增加 healthcheck，或手工确认服务端口与依赖可用。", spec.ComposeService)
	default:
		check.Status = "unhealthy"
		check.Message = fmt.Sprintf("容器状态为 %s，health=%s。", state.Status, check.Health)
		check.Suggestion = fmt.Sprintf("先查看 docker compose logs --tail=200 %s，修复后重新创建该服务。", spec.ComposeService)
	}
	return check
}

type workflowFile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Nodes []struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Disabled bool   `json:"disabled"`
	} `json:"nodes"`
}

func scanWorkflowFiles(directory string) ([]WorkflowRef, []UnsupportedNode, error) {
	files, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil || len(files) == 0 {
		return nil, nil, errors.New("workflow files were not found")
	}
	sort.Strings(files)
	expected := make([]WorkflowRef, 0, len(files))
	unsupported := make([]UnsupportedNode, 0)
	for _, path := range files {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, nil, readErr
		}
		var workflow workflowFile
		if json.Unmarshal(content, &workflow) != nil || workflow.ID == "" {
			return nil, nil, fmt.Errorf("invalid workflow file: %s", filepath.Base(path))
		}
		expected = append(expected, WorkflowRef{ID: workflow.ID, Name: workflow.Name})
		for _, node := range workflow.Nodes {
			if node.Type == "n8n-nodes-base.executeCommand" {
				unsupported = append(unsupported, UnsupportedNode{
					WorkflowID: workflow.ID, WorkflowName: workflow.Name, File: filepath.Base(path),
					NodeName: node.Name, NodeType: node.Type, Disabled: node.Disabled,
				})
			}
		}
	}
	return expected, unsupported, nil
}

func evaluateUnsupportedNodes(nodes []UnsupportedNode, scanErr error) UnsupportedNodeCheck {
	check := UnsupportedNodeCheck{Nodes: nodes, Count: len(nodes)}
	if scanErr != nil {
		check.Status = "unhealthy"
		check.Message = "无法扫描 workflow 文件中的节点类型。"
		check.Suggestion = "确认 CMS_WORKFLOW_DIR 指向可读的 workflows 目录，并修复无效 JSON。"
		return check
	}
	if len(nodes) == 0 {
		check.Status = "healthy"
		check.Message = "未发现 executeCommand 节点。"
		return check
	}
	check.Status = "degraded"
	check.Message = fmt.Sprintf("发现 %d 个当前策略不支持的 executeCommand 节点。", len(nodes))
	check.Suggestion = "将命令执行迁移到隔离的 media-worker，并由 n8n 通过 HTTP Request 调用；迁移完成前避免在受限环境运行相关工作流。"
	return check
}

func (r *Runner) workflowActivation(ctx context.Context, expected []WorkflowRef, scanErr error) WorkflowActivationCheck {
	check := WorkflowActivationCheck{ExpectedCount: len(expected), Inactive: make([]WorkflowRef, 0), Missing: make([]WorkflowRef, 0)}
	if scanErr != nil {
		check.Status = "unhealthy"
		check.Message = "无法确定预期 workflow 列表。"
		check.Suggestion = "确认 workflows 目录存在且所有 JSON 可以解析。"
		return check
	}
	postgresEnv, err := inspectContainerEnv(ctx, r.postgresContainer)
	if err != nil {
		check.Status = "unhealthy"
		check.Message = "无法读取 n8n 数据库连接上下文。"
		check.Suggestion = "确认 postgres 容器运行，并允许 CMS 执行只读 docker inspect/exec。"
		return check
	}
	user := defaultValue(postgresEnv["POSTGRES_USER"], "n8n")
	database := defaultValue(postgresEnv["POSTGRES_DB"], "n8n")
	query := "SELECT id,name,active FROM workflow_entity ORDER BY name;"
	commandCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "docker", "exec", r.postgresContainer,
		"psql", "-U", user, "-d", database, "-At", "-F", "\t", "-c", query).Output()
	if err != nil {
		check.Status = "unhealthy"
		check.Message = "无法从 n8n 数据库读取 workflow active 状态。"
		check.Suggestion = "确认 n8n workflow_entity 表可读，并检查 postgres 容器日志。"
		return check
	}
	databaseWorkflows := make(map[string]WorkflowRef)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		item := WorkflowRef{ID: parts[0], Name: parts[1], Active: parts[2] == "t" || parts[2] == "true"}
		databaseWorkflows[item.ID] = item
		check.ImportedCount++
		if item.Active {
			check.ActiveCount++
		} else {
			check.Inactive = append(check.Inactive, item)
		}
	}
	for _, item := range expected {
		if _, exists := databaseWorkflows[item.ID]; !exists {
			check.Missing = append(check.Missing, item)
		}
	}
	if check.ExpectedCount > 0 && len(check.Inactive) == 0 && len(check.Missing) == 0 {
		check.Status = "healthy"
		check.Message = fmt.Sprintf("%d 个 workflow 均已导入并处于 active 状态。", check.ActiveCount)
		return check
	}
	check.Status = "unhealthy"
	check.Message = fmt.Sprintf("发现 %d 个未启用、%d 个未导入的 workflow。", len(check.Inactive), len(check.Missing))
	check.Suggestion = "在 n8n 管理界面逐个启用 inactive workflow；缺失项应从 workflows 目录重新导入并核对固定 workflow ID。"
	return check
}

func (r *Runner) postgresCredential(ctx context.Context) CredentialCheck {
	check := CredentialCheck{Key: "POSTGRES_CREDENTIAL_ID"}
	values, err := inspectContainerEnv(ctx, r.n8nContainer)
	if err != nil {
		check.Status = "unhealthy"
		check.Message = "无法读取 n8n 容器环境。"
		check.Suggestion = "确认 n8n 容器运行，并允许 CMS 执行只读 docker inspect。"
		return check
	}
	value, exists := values[check.Key]
	check.Exists = exists
	check.Configured = exists && configuredValue(value)
	if check.Configured {
		check.Status = "healthy"
		check.Message = "n8n 容器已配置 Postgres Credential ID。"
		return check
	}
	check.Status = "unhealthy"
	if check.Exists {
		check.Message = "n8n 容器存在 POSTGRES_CREDENTIAL_ID，但值为空或仍是占位符。"
	} else {
		check.Message = "n8n 容器缺少 POSTGRES_CREDENTIAL_ID。"
	}
	check.Suggestion = "先在 n8n 中创建并验证 Postgres Credential，再把其 ID 写入 POSTGRES_CREDENTIAL_ID，随后强制重建 n8n 容器。"
	return check
}

func inspectContainerEnv(ctx context.Context, containerName string) (map[string]string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "docker", "inspect", "--format", "{{json .Config.Env}}", containerName).Output()
	if err != nil {
		return nil, errors.New("container environment unavailable")
	}
	var entries []string
	if json.Unmarshal(output, &entries) != nil {
		return nil, errors.New("container environment invalid")
	}
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		if key, value, ok := strings.Cut(entry, "="); ok {
			values[key] = value
		}
	}
	return values, nil
}

func defaultValue(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func configuredValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	for _, placeholder := range []string{"replace_me", "change_me", "replace_with", "placeholder"} {
		if strings.Contains(normalized, placeholder) {
			return false
		}
	}
	return true
}
