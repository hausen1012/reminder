// ai_service 负责 AI 助手配置、工具调用与待确认操作管理。
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/llm"
	"github.com/reminder/backend/internal/logger"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/textutil"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/gorm"
)

// AI 设置在 config 表中的键名。
const (
	KeyAIEnabled  = "ai_enabled"
	KeyAIProvider = "ai_provider"
	KeyAIBaseURL  = "ai_base_url"
	KeyAIModel    = "ai_model"
	KeyAIModels   = "ai_models"
	KeyAIAPIKey   = "ai_api_key_enc"
	// KeyAIPrompt 是用户自定义的追加提示词，与内置系统提示词拼接后一起发给模型。
	KeyAIPrompt = "ai_custom_prompt"
	// KeyAIMaxRetries 是接口调用失败后的最大重试次数。
	KeyAIMaxRetries = "ai_max_retries"
)

// chatMaxTokens 是单次回复的输出上限（厂商的 max_tokens），不可配置。
const chatMaxTokens = 4096

// AI 调用的默认与边界值。
const (
	// defaultMaxRetries 是接口失败后的默认重试次数。
	defaultMaxRetries = 10
	maxMaxRetries     = 100
)

// maxToolRounds 是单次对话里工具调用的最大轮数，防止模型陷入循环。
const maxToolRounds = 8

// maxHistoryTurns 是客户端回灌的历史消息条数上限。
const maxHistoryTurns = 30

// AIService 提供 AI 助手的设置、工具执行与草稿管理。
type AIService struct {
	DB          *gorm.DB
	Box         *secretbox.Box
	TZ          *timezone.Provider
	ReminderSvc *ReminderService
	ChannelSvc  *ChannelService
	// providerOverride 仅供测试注入 fake。
	providerOverride llm.Provider
}

// NewAIService 构造服务。
func NewAIService(db *gorm.DB, box *secretbox.Box, tz *timezone.Provider, reminderSvc *ReminderService, channelSvc *ChannelService) *AIService {
	if tz == nil {
		tz = timezone.New("")
	}
	return &AIService{DB: db, Box: box, TZ: tz, ReminderSvc: reminderSvc, ChannelSvc: channelSvc}
}

// AISettings 是对外的 AI 设置视图：密钥只回 "***" 表示"已配置"。
type AISettings struct {
	Enabled  bool     `json:"enabled"`
	Provider string   `json:"provider"` // anthropic | openai
	BaseURL  string   `json:"base_url"`
	Model    string   `json:"model"`
	Models   []string `json:"models"` // 备选模型列表
	// APIKey 恒为脱敏值：有密钥时是 "***"，否则为空串。
	APIKey string `json:"api_key"`
	// CustomPrompt 是用户自定义的追加提示词。
	CustomPrompt string `json:"custom_prompt"`
	// MaxRetries 是接口调用失败后的最大重试次数。
	MaxRetries int `json:"max_retries"`
}

// AISettingsInput 是保存设置的入参。
//
// api_key 传 "***" 表示保留原密钥，传空串表示清除。
type AISettingsInput struct {
	Enabled  bool     `json:"enabled"`
	Provider string   `json:"provider"`
	BaseURL  string   `json:"base_url"`
	Model    string   `json:"model"`
	Models   []string `json:"models"`
	APIKey   string   `json:"api_key"`
	// CustomPrompt 为空表示不使用自定义提示词（清空已保存的内容）。
	CustomPrompt string `json:"custom_prompt"`
	// MaxRetries 显式传 0 表示不重试，nil 使用默认值。
	MaxRetries *int `json:"max_retries"`
}

// GetSettings 读取设置（密钥脱敏）。
func (s *AIService) GetSettings() (*AISettings, error) {
	kv, err := s.configMap()
	if err != nil {
		return nil, err
	}
	out := &AISettings{
		Enabled:      kv[KeyAIEnabled] == "true",
		Provider:     kv[KeyAIProvider],
		BaseURL:      kv[KeyAIBaseURL],
		Model:        kv[KeyAIModel],
		CustomPrompt: kv[KeyAIPrompt],
		MaxRetries:   parseMaxRetries(kv[KeyAIMaxRetries]),
	}
	if out.Provider == "" {
		out.Provider = "anthropic"
	}
	if kv[KeyAIAPIKey] != "" {
		out.APIKey = sensitivePlaceholder
	}
	var models []string
	if rawModels, ok := kv[KeyAIModels]; ok {
		_ = json.Unmarshal([]byte(rawModels), &models)
	} else if out.Model != "" {
		models = []string{out.Model}
	}
	if models == nil {
		models = []string{}
	}
	out.Models = models
	return out, nil
}

// SaveSettings 保存设置。
func (s *AIService) SaveSettings(in AISettingsInput) (*AISettings, error) {
	provider := strings.TrimSpace(in.Provider)
	if provider == "" {
		provider = "anthropic"
	}
	if provider != "anthropic" && provider != "openai" {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed,
			"provider 仅支持 anthropic / openai").WithField("provider")
	}
	model := strings.TrimSpace(in.Model)
	if model == "" && in.Enabled {
		if provider == "anthropic" {
			model = "claude-opus-5-5"
		} else {
			model = "gpt-4o-mini"
		}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if baseURL != "" && !strings.HasPrefix(baseURL, "http") {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed,
			"base_url 必须以 http:// 或 https:// 开头").WithField("base_url")
	}
	customPrompt := strings.TrimSpace(in.CustomPrompt)
	if len([]rune(customPrompt)) > maxCustomPromptRunes {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed,
			fmt.Sprintf("自定义提示词过长（最多 %d 字）", maxCustomPromptRunes)).WithField("custom_prompt")
	}
	maxRetries, err := normalizeMaxRetries(in.MaxRetries)
	if err != nil {
		return nil, err
	}
	if in.Enabled && strings.TrimSpace(in.APIKey) == "" {
		// 允许使用已保存的密钥：只有既没传新密钥、库里也没有时才拒绝。
		kv, err := s.configMap()
		if err != nil {
			return nil, err
		}
		if kv[KeyAIAPIKey] == "" {
			return nil, middleware.NewAppError(middleware.CodeValidationFailed,
				"启用 AI 前请先填写 API Key").WithField("api_key")
		}
	}

	var cleanedModels []string
	seen := make(map[string]bool)
	for _, m := range in.Models {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			cleanedModels = append(cleanedModels, m)
		}
	}
	// 仅当调用方完全未传 in.Models (nil) 时才尝试把 model 加入列表；
	// 若调用方显式传了切片（哪怕为空 []），完全尊重入参，不强行塞入默认值
	if in.Models == nil && model != "" && !seen[model] {
		cleanedModels = append([]string{model}, cleanedModels...)
	}
	if cleanedModels == nil {
		cleanedModels = []string{}
	}
	modelsJSON, _ := json.Marshal(cleanedModels)

	updates := map[string]string{
		KeyAIEnabled:    boolString(in.Enabled),
		KeyAIProvider:   provider,
		KeyAIBaseURL:    baseURL,
		KeyAIModel:      model,
		KeyAIModels:     string(modelsJSON),
		KeyAIPrompt:     customPrompt,
		KeyAIMaxRetries: strconv.Itoa(maxRetries),
	}
	if in.APIKey == sensitivePlaceholder {
		// 占位符 = 保留原密钥，不写库
	} else if strings.TrimSpace(in.APIKey) == "" {
		updates[KeyAIAPIKey] = ""
	} else {
		enc, err := s.Box.EncryptString(strings.TrimSpace(in.APIKey))
		if err != nil {
			return nil, err
		}
		updates[KeyAIAPIKey] = enc
	}
	for k, v := range updates {
		if err := s.upsertConfig(k, v); err != nil {
			return nil, err
		}
	}
	slog.Info("AI 设置已更新", "enabled", in.Enabled, "provider", provider, "model", model,
		"max_retries", maxRetries)
	return s.GetSettings()
}

// TestConnection 用当前（或即将保存的）设置发一次最小请求，验证厂商与密钥可用。
//
// 优先使用入参里的 api_key；传占位符或空串时回落到库里已保存的密钥。
func (s *AIService) TestConnection(ctx context.Context, in AISettingsInput) (string, error) {
	cfg, err := s.resolveConfig(in)
	if err != nil {
		return "", err
	}
	p, err := newProvider(cfg)
	if err != nil {
		return "", middleware.NewAppError(middleware.CodeValidationFailed, err.Error())
	}
	reply, err := llm.ChatWithRetry(ctx, p, llm.Request{
		System:    "你是连通性测试助手。",
		Turns:     []llm.Turn{{Role: "user", Text: "请只回复：OK"}},
		MaxTokens: 256,
	}, cfg.MaxRetries)
	if err != nil {
		return "", middleware.NewAppError(middleware.CodeValidationFailed, "连接失败: "+err.Error())
	}
	return strings.TrimSpace(reply.Text), nil
}

// FetchModels 获取指定厂商或配置下的可用模型列表。
//
// 优先使用入参里的 provider、baseURL 与 api_key；传占位符或空串时回落到库中已保存的密钥。
func (s *AIService) FetchModels(ctx context.Context, in AISettingsInput) ([]string, error) {
	cfg, err := s.resolveConfig(in)
	if err != nil {
		return nil, err
	}
	switch cfg.Provider {
	case "anthropic":
		return s.fetchAnthropicModels(ctx, cfg)
	case "openai":
		return s.fetchOpenAIModels(ctx, cfg)
	default:
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, fmt.Sprintf("不支持的厂商协议: %s", cfg.Provider))
	}
}

// modelListEndpoint 描述一次模型列表请求的地址与鉴权头。
type modelListEndpoint struct {
	name    string
	url     string
	headers map[string]string
}

// listModels 请求模型列表接口并按 id 去重排序；parse 负责解析响应体并返回模型名列表。
func (s *AIService) listModels(ctx context.Context, ep modelListEndpoint, parse func([]byte) ([]string, error)) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range ep.headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, "请求"+ep.name+"模型列表失败: "+err.Error())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, "读取"+ep.name+"模型响应失败: "+err.Error())
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed,
			fmt.Sprintf("获取%s模型失败 (HTTP %d): %s", ep.name, resp.StatusCode, textutil.TruncateRunes(string(body), 500)))
	}

	ids, err := parse(body)
	if err != nil {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, "解析"+ep.name+"模型列表响应失败: "+err.Error())
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	models := make([]string, 0, len(set))
	for id := range set {
		models = append(models, id)
	}
	sort.Strings(models)
	return models, nil
}

func (s *AIService) fetchOpenAIModels(ctx context.Context, cfg *aiConfig) ([]string, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if !strings.HasSuffix(base, "/v1") && !strings.Contains(base, "/v1/") {
		base += "/v1"
	}
	headers := map[string]string{}
	if cfg.APIKey != "" {
		headers["Authorization"] = "Bearer " + cfg.APIKey
	}
	return s.listModels(ctx, modelListEndpoint{name: "OpenAI", url: base + "/models", headers: headers},
		func(body []byte) ([]string, error) {
			var parsed struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
				Models []struct {
					Name string `json:"name"`
					ID   string `json:"id"`
				} `json:"models"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(parsed.Data)+len(parsed.Models))
			for _, item := range parsed.Data {
				ids = append(ids, strings.TrimSpace(item.ID))
			}
			for _, item := range parsed.Models {
				id := strings.TrimSpace(item.ID)
				if id == "" {
					id = strings.TrimSpace(item.Name)
				}
				ids = append(ids, id)
			}
			return ids, nil
		})
}

func (s *AIService) fetchAnthropicModels(ctx context.Context, cfg *aiConfig) ([]string, error) {
	if cfg.APIKey == "" {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, "未配置 API Key")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	return s.listModels(ctx, modelListEndpoint{
		name: "Anthropic",
		url:  base + "/v1/models",
		headers: map[string]string{
			"x-api-key":         cfg.APIKey,
			"anthropic-version": "2023-06-01",
		},
	}, func(body []byte) ([]string, error) {
		var parsed struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(parsed.Data))
		for _, item := range parsed.Data {
			ids = append(ids, strings.TrimSpace(item.ID))
		}
		return ids, nil
	})
}

// ChatMessage 是客户端回传的一轮历史消息，只保留角色与文本。
type ChatMessage struct {
	Role    string `json:"role"` // user | assistant
	Content string `json:"content"`
}

// ChatInput 是一次对话的入参。
type ChatInput struct {
	Message string `json:"message"`
	// History 是客户端持有的此前对话（不含本次 message）。
	History []ChatMessage `json:"history,omitempty"`
	// Model 临时覆盖本次对话使用的模型，留空用设置值。
	Model string `json:"model,omitempty"`
}

// ChatResult 是一次对话的结果。
type ChatResult struct {
	Reply string `json:"reply"`
	// Actions 是本轮生成的待确认草稿。
	Actions []models.AIAction `json:"actions"`
	// Retries 是本次对话因临时性失败重试的总次数。
	Retries int `json:"retries"`
}

// 对话过程中的状态事件名（SSE 的 event 字段）。
const (
	ChatEventStatus = "status"
	ChatEventRetry  = "retry"
	ChatEventDone   = "done"
	ChatEventError  = "error"
)

// ChatStatus 是对话过程的进度快照。
type ChatStatus struct {
	Phase string `json:"phase"` // thinking | tools
	Text  string `json:"text"`
}

// ChatRetry 是一次即将开始的重试。
type ChatRetry struct {
	Attempt int    `json:"attempt"`
	Max     int    `json:"max"`
	DelayMs int64  `json:"delay_ms"`
	Reason  string `json:"reason"`
}

// ChatEventSink 接收对话过程中的状态事件，sink 为 nil 时忽略。
type ChatEventSink func(event string, data any)

// Chat 处理一次用户输入：拼历史 → 跑工具循环 → 返回回复与草稿。
func (s *AIService) Chat(ctx context.Context, in ChatInput) (*ChatResult, error) {
	return s.ChatStream(ctx, in, nil)
}

// ChatStream 与 Chat 相同，但会把过程中的状态事件推给 sink。
func (s *AIService) ChatStream(ctx context.Context, in ChatInput, sink ChatEventSink) (*ChatResult, error) {
	emit := func(event string, data any) {
		if sink != nil {
			sink(event, data)
		}
	}
	message := strings.TrimSpace(in.Message)
	if message == "" {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, "消息不能为空").WithField("message")
	}
	if len([]rune(message)) > 4000 {
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, "消息过长（最多 4000 字）").WithField("message")
	}

	// 本文件里的日志统一走 logger.FromContext：一条对话的多轮工具调用、
	// 每次重试都带同一个 request_id，能和访问日志串起来看。
	log := logger.FromContext(ctx)
	log.Info("AI 对话开始", "msg_len", len([]rune(message)),
		"history_turns", len(in.History), "requested_model", in.Model)

	cfg, err := s.resolveConfig(AISettingsInput{Model: strings.TrimSpace(in.Model)})
	if err != nil {
		log.Warn("AI 对话结束：读取设置失败", "error", err)
		return nil, err
	}
	if !cfg.Enabled {
		log.Warn("AI 对话结束：助手未启用")
		return nil, middleware.NewAppError(middleware.CodeValidationFailed, "AI 助手未启用，请先到设置页开启")
	}
	p := s.providerOverride
	if p == nil {
		created, err := newProvider(cfg)
		if err != nil {
			log.Warn("AI 对话结束：构造 Provider 失败",
				"provider", cfg.Provider, "base_url", cfg.BaseURL, "error", err)
			return nil, middleware.NewAppError(middleware.CodeValidationFailed, err.Error())
		}
		p = created
	}
	log = log.With("provider", cfg.Provider, "model", cfg.Model)

	// 提示词每轮请求只解析一次：库里的自定义内容在本次对话内保持一致
	system, err := s.SystemPrompt()
	if err != nil {
		return nil, err
	}

	turns := historyTurns(in.History)
	turns = append(turns, llm.Turn{Role: "user", Text: message})

	defs := s.toolDefinitions()
	tools := make([]llm.Tool, 0, len(defs))
	byName := make(map[string]toolDefinition, len(defs))
	for _, d := range defs {
		tools = append(tools, d.spec)
		byName[d.spec.Name] = d
	}

	var createdIDs []string
	var success bool
	defer func() {
		if !success && len(createdIDs) > 0 {
			// 对话异常结束，清理本轮草稿，避免留下孤儿待确认记录
			if delErr := s.DB.Where("id IN ?", createdIDs).Delete(&models.AIAction{}).Error; delErr != nil {
				logger.FromContext(ctx).Error("清理未完成对话的 AI 草稿失败", "error", delErr, "ids", createdIDs)
			}
		}
	}()

	var replyText string
	truncated := false
	retries := 0
	rounds := 0
	for round := 0; round < maxToolRounds; round++ {
		rounds = round + 1
		emit(ChatEventStatus, ChatStatus{Phase: "thinking", Text: "助手正在思考…"})
		reply, err := llm.ChatWithRetryNotify(ctx, p, llm.Request{
			System:    system,
			Turns:     turns,
			Tools:     tools,
			MaxTokens: chatMaxTokens,
		}, cfg.MaxRetries, func(attempt, max int, delay time.Duration, cause error) {
			retries++
			emit(ChatEventRetry, ChatRetry{
				Attempt: attempt,
				Max:     max,
				DelayMs: delay.Milliseconds(),
				Reason:  llm.RetryReason(cause),
			})
			emit(ChatEventStatus, ChatStatus{
				Phase: "thinking",
				Text:  fmt.Sprintf("第 %d/%d 次重试：%s", attempt, max, llm.RetryReason(cause)),
			})
		})
		if err != nil {
			log.Warn("AI 对话结束：调用失败", llm.ErrorAttrsWith(err,
				"round", rounds, "retries", retries, "tool_rounds_limit", maxToolRounds,
				"turns", len(turns), "tools", len(tools),
				"user_message", textutil.TruncateRunes(message, 500))...)
			return nil, middleware.NewAppError(middleware.CodeValidationFailed, "AI 调用失败: "+err.Error())
		}
		replyText = strings.TrimSpace(reply.Text)
		if len(reply.ToolCalls) == 0 {
			break
		}
		// 达到输出上限时结束工具循环，返回已生成的草稿。
		if reply.Truncated {
			truncated = true
			break
		}

		emit(ChatEventStatus, ChatStatus{
			Phase: "tools",
			Text:  fmt.Sprintf("正在执行 %d 个工具…", len(reply.ToolCalls)),
		})
		turns = append(turns, llm.Turn{
			Role:      "assistant",
			Text:      reply.Text,
			ToolCalls: reply.ToolCalls,
			Raw:       reply.Raw,
		})
		results := make([]llm.ToolResult, 0, len(reply.ToolCalls))
		toolNames := make([]string, 0, len(reply.ToolCalls))
		var toolErrs []any
		for _, call := range reply.ToolCalls {
			toolNames = append(toolNames, call.Name)
			def, ok := byName[call.Name]
			if !ok {
				results = append(results, llm.ToolResult{
					CallID: call.ID, Name: call.Name, Content: "未知工具: " + call.Name, IsError: true,
				})
				toolErrs = append(toolErrs, call.Name+": 未知工具")
				continue
			}
			out, runErr := def.run(ctx, call.Input)
			results = append(results, llm.ToolResult{
				CallID:  call.ID,
				Name:    call.Name,
				Content: out,
				IsError: runErr != nil,
			})
			if runErr != nil {
				results[len(results)-1].Content = runErr.Error()
				// 工具失败是「AI 回答了但不能用」的主要原因，必须能在日志里看到
				toolErrs = append(toolErrs, call.Name+": "+runErr.Error())
				continue
			}
			// 写工具会在结果里带上 draft_id，记下来用于本轮回传草稿
			if id := draftIDOf(out); id != "" {
				createdIDs = append(createdIDs, id)
			}
		}
		// 工具轮次明细属于 Debug：一次对话可能跑好几轮，每轮一条 Info 会刷满日志。
		log.Debug("AI 工具轮次完成",
			"round", rounds, "tools", toolNames, "errors", toolErrs,
			"drafts", len(createdIDs))
		turns = append(turns, llm.Turn{Role: "user", ToolResults: results})
	}
	if truncated {
		// 明确告诉用户「到此为止」的原因：工具参数被截断时继续执行会写坏数据。
		note := fmt.Sprintf("（回复达到单次输出上限 %d Token 已停止，请精简提问或分多次操作）", chatMaxTokens)
		if replyText == "" {
			replyText = note
		} else {
			replyText += "\n\n" + note
		}
	}
	if replyText == "" {
		replyText = "（助手没有返回文本，请重试或换个说法）"
	}

	actions, err := s.actionsByIDs(createdIDs)
	if err != nil {
		log.Warn("AI 对话结束：读取草稿失败", "drafts", createdIDs, "error", err)
		return nil, err
	}
	success = true
	// 每次对话一条汇总，排查「AI 没按要求干活」时先看这条。
	log.Info("AI 对话完成",
		"rounds", rounds, "retries", retries, "drafts", len(createdIDs),
		"truncated", truncated, "reply_len", len([]rune(replyText)),
		"user_message", textutil.TruncateRunes(message, 500))
	return &ChatResult{Reply: replyText, Actions: actions, Retries: retries}, nil
}

// ListActions 按状态列出草稿；status 为空时返回全部（最多 50 条，按时间倒序）。
func (s *AIService) ListActions(status string) ([]models.AIAction, error) {
	q := s.DB.Model(&models.AIAction{}).Order("created_at DESC").Limit(50)
	if status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	var rows []models.AIAction
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ActionRetention 是 AI 草稿的保留期，按创建时间计算。
const ActionRetention = 90 * 24 * time.Hour

// PurgeActions 删除创建时间早于保留期的草稿，返回删除行数；待确认的也一并清理。
func (s *AIService) PurgeActions(olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, nil
	}
	cutoff := models.TimeOf(time.Now().Add(-olderThan))
	res := s.DB.Where("created_at < ?", cutoff).Delete(&models.AIAction{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// ApplyAction 执行一条 pending 状态的待确认操作。
func (s *AIService) ApplyAction(id string) (*models.AIAction, error) {
	act, err := s.getAction(id)
	if err != nil {
		return nil, err
	}
	if act.Status != "pending" {
		return nil, middleware.NewAppError(middleware.CodeConflict, "该操作已处理，无法重复执行")
	}

	// 原子 CAS 抢占：先在库里将 pending 推进为 applied，
	// 防止快速连击或并发请求同时穿透并多次触发副作用（如创建重复提醒）。
	res := s.DB.Model(&models.AIAction{}).
		Where("id = ? AND status = ?", id, "pending").
		Update("status", "applied")
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, middleware.NewAppError(middleware.CodeConflict, "该操作已处理，无法重复执行")
	}
	act.Status = "applied"

	raw, err := s.Box.DecryptString(act.PayloadEnc)
	if err != nil {
		s.markAction(act, "failed", "草稿内容无法解密（ENCRYPTION_KEY 可能已更换）", "")
		return nil, middleware.NewAppError(middleware.CodeValidationFailed,
			"草稿内容无法解密（ENCRYPTION_KEY 可能已更换），请重新让 AI 生成")
	}

	var ref string
	switch act.Kind {
	case "create_channel":
		var in ChannelInput
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			return nil, s.failAction(act, err)
		}
		v, err := s.ChannelSvc.Create(in)
		if err != nil {
			return nil, s.failAction(act, err)
		}
		ref = fmt.Sprintf("channel:%d", v.ID)
	case "create_reminder":
		var in ReminderInput
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			return nil, s.failAction(act, err)
		}
		v, err := s.ReminderSvc.Create(in)
		if err != nil {
			return nil, s.failAction(act, err)
		}
		ref = fmt.Sprintf("reminder:%d", v.ID)
	case "update_reminder":
		var payload struct {
			ID    uint          `json:"id"`
			Input ReminderInput `json:"input"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, s.failAction(act, err)
		}
		v, err := s.ReminderSvc.Update(payload.ID, payload.Input)
		if err != nil {
			return nil, s.failAction(act, err)
		}
		ref = fmt.Sprintf("reminder:%d", v.ID)
	case "delete_reminder":
		var payload struct {
			ID uint `json:"id"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, s.failAction(act, err)
		}
		if err := s.ReminderSvc.Delete(payload.ID); err != nil {
			return nil, s.failAction(act, err)
		}
		ref = fmt.Sprintf("reminder:%d:deleted", payload.ID)
	default:
		return nil, s.failAction(act, fmt.Errorf("未知操作类型: %s", act.Kind))
	}

	s.markAction(act, "applied", "", ref)
	return act, nil
}

// RejectAction 拒绝一条待确认操作。
func (s *AIService) RejectAction(id string) (*models.AIAction, error) {
	act, err := s.getAction(id)
	if err != nil {
		return nil, err
	}
	if act.Status != "pending" {
		return nil, middleware.NewAppError(middleware.CodeConflict, "该操作已处理")
	}
	res := s.DB.Model(&models.AIAction{}).
		Where("id = ? AND status = ?", id, "pending").
		Update("status", "rejected")
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, middleware.NewAppError(middleware.CodeConflict, "该操作已处理")
	}
	act.Status = "rejected"
	return act, nil
}

// --- internal helpers ---

// getAction 按 id 取草稿，不存在时返回 404。
func (s *AIService) getAction(id string) (*models.AIAction, error) {
	if strings.TrimSpace(id) == "" {
		return nil, middleware.NewAppError(middleware.CodeNotFound, "操作不存在")
	}
	var act models.AIAction
	if err := s.DB.First(&act, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, middleware.NewAppError(middleware.CodeNotFound, "操作不存在")
		}
		return nil, err
	}
	return &act, nil
}

// actionsByIDs 按 id 顺序取回草稿；ids 为空时返回空切片（而不是 nil）。
func (s *AIService) actionsByIDs(ids []string) ([]models.AIAction, error) {
	out := make([]models.AIAction, 0, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []models.AIAction
	if err := s.DB.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	byID := make(map[string]models.AIAction, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// failAction 记录失败原因并返回原错误，便于 handler 直接 abortErr。
func (s *AIService) failAction(act *models.AIAction, err error) error {
	s.markAction(act, "failed", err.Error(), "")
	return err
}

// markAction 更新操作状态；写库失败只记日志，不覆盖调用方的业务错误。
func (s *AIService) markAction(act *models.AIAction, status, errMsg, ref string) {
	updates := map[string]any{"status": status}
	if errMsg != "" {
		updates["error"] = textutil.TruncateRunes(errMsg, 255)
	}
	if ref != "" {
		updates["result_ref"] = ref
	}
	if err := s.DB.Model(&models.AIAction{}).Where("id = ?", act.ID).Updates(updates).Error; err != nil {
		slog.Warn("回写 AI 操作状态失败", "id", act.ID, "status", status, "error", err)
	}
	act.Status = status
	if errMsg != "" {
		act.Error = textutil.TruncateRunes(errMsg, 255)
	}
	if ref != "" {
		act.ResultRef = ref
	}
}

// historyTurns 将客户端历史消息截取保留最近 maxHistoryTurns 条并过滤无效内容。
func historyTurns(history []ChatMessage) []llm.Turn {
	if len(history) > maxHistoryTurns {
		history = history[len(history)-maxHistoryTurns:]
	}
	out := make([]llm.Turn, 0, len(history))
	for _, m := range history {
		role := m.Role
		if role != "user" && role != "assistant" {
			continue
		}
		text := strings.TrimSpace(m.Content)
		if text == "" {
			continue
		}
		out = append(out, llm.Turn{Role: role, Text: text})
	}
	return out
}

// maxCustomPromptRunes 是自定义提示词的字数上限；提示词会随每轮工具调用重复发送。
const maxCustomPromptRunes = 4000

// SystemPrompt 返回当前生效的完整系统提示词（内置规则 + 用户追加内容）。
func (s *AIService) SystemPrompt() (string, error) {
	kv, err := s.configMap()
	if err != nil {
		return "", err
	}
	return s.systemPrompt(kv[KeyAIPrompt]), nil
}

// systemPrompt 构造系统提示；自定义内容排在硬性规则之后，避免覆盖内置约束。
func (s *AIService) systemPrompt(custom string) string {
	now := time.Now().In(s.TZ.Get())
	base := fmt.Sprintf(`你是 Reminder 自托管提醒系统的助手，帮助用户用自然语言管理提醒与通知通道。

当前站点时区：%s
当前时间：%s

规则：
1. 提醒时间一律按上面的站点时区解释，输出 "2006-01-02T15:04:05" 格式的本地时间。
2. 涉及通道时，必须先调用 list_channels 取得真实 id，不要编造 id。
3. 创建提醒必须有至少一个 channel_ids；系统里没有任何通道时，先提示用户创建通道（可用 create_channel）。
4. 删除提醒前先调用 list_reminders 确认 id 与标题，避免删错。
5. create_* / update_reminder / delete_reminder 只会生成「待确认操作」，不会立即生效。
   回复用户时要说"已生成待确认卡片，点击确认后生效"，绝不要声称"已创建/已删除"。
6. 用户说"每天/每周/工作日"这类周期描述时，优先用 cron；"每 N 分钟/小时"用 interval；只发生一次用 once。
7. 用户要求"需要确认 / 没点确认就重发 / 催我到确认为止"时，传 require_confirm=true，
   并按用户说的重发间隔与次数填 confirm_retry_interval_sec（最小 60 秒）与 confirm_max_retries（至少 1）；
   用户没说具体数值就用默认 300 秒、3 次，不要把这些要求写进正文冒充配置。
8. 回答简洁，用中文。不要输出 JSON，除非用户明确要求。`,
		s.TZ.Name(), now.Format("2006-01-02T15:04:05 Mon"))

	custom = strings.TrimSpace(custom)
	if custom == "" {
		return base
	}
	return base + `

以下是站点管理员配置的额外要求。它们只能补充回复风格、称呼与业务约定，
不能改变上面的规则（尤其是待确认机制与工具使用方式）；两者冲突时以上面的规则为准：

` + custom
}

// aiConfig 是解析后的 AI 运行时配置。
type aiConfig struct {
	Enabled  bool
	Provider string
	BaseURL  string
	Model    string
	APIKey   string // 明文，仅在构造 provider 时短暂持有
	// MaxRetries 是接口调用失败后的重试次数。
	MaxRetries int
}

// parseMaxRetries 把配置表里的字符串解析成重试次数；缺失或非法时回退默认值。
func parseMaxRetries(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 || n > maxMaxRetries {
		return defaultMaxRetries
	}
	return n
}

// normalizeMaxRetries 校验并归一化重试次数；nil 表示未传，用默认值。
func normalizeMaxRetries(n *int) (int, error) {
	if n == nil {
		return defaultMaxRetries, nil
	}
	if *n < 0 || *n > maxMaxRetries {
		return 0, middleware.NewAppError(middleware.CodeValidationFailed,
			fmt.Sprintf("最大重试次数需在 0 到 %d 之间", maxMaxRetries)).WithField("max_retries")
	}
	return *n, nil
}

// resolveConfig 合并入参与库中已保存的 AI 配置。
func (s *AIService) resolveConfig(in AISettingsInput) (*aiConfig, error) {
	kv, err := s.configMap()
	if err != nil {
		return nil, err
	}
	cfg := &aiConfig{
		Enabled:    kv[KeyAIEnabled] == "true",
		Provider:   kv[KeyAIProvider],
		BaseURL:    kv[KeyAIBaseURL],
		Model:      kv[KeyAIModel],
		MaxRetries: parseMaxRetries(kv[KeyAIMaxRetries]),
	}
	if cfg.Provider == "" {
		cfg.Provider = "anthropic"
	}
	if cfg.Model == "" {
		if cfg.Provider == "anthropic" {
			cfg.Model = "claude-opus-5-5"
		} else {
			cfg.Model = "gpt-4o-mini"
		}
	}
	if in.Provider != "" {
		cfg.Provider = in.Provider
	}
	if in.BaseURL != "" {
		cfg.BaseURL = strings.TrimRight(in.BaseURL, "/")
	}
	if in.Model != "" {
		cfg.Model = in.Model
	}
	if in.Enabled {
		cfg.Enabled = true
	}
	// 只有显式传入才覆盖，TestConnection / FetchModels 不带该字段
	if in.MaxRetries != nil {
		cfg.MaxRetries = *in.MaxRetries
	}

	// 密钥优先级：入参明文 > 库中密文
	switch {
	case in.APIKey != "" && in.APIKey != sensitivePlaceholder:
		cfg.APIKey = in.APIKey
	case kv[KeyAIAPIKey] != "":
		plain, err := s.Box.DecryptString(kv[KeyAIAPIKey])
		if err != nil {
			return nil, middleware.NewAppError(middleware.CodeValidationFailed,
				"AI API Key 无法解密（ENCRYPTION_KEY 可能已更换），请重新填写")
		}
		cfg.APIKey = plain
	}
	return cfg, nil
}

// newProvider 按配置构造 Provider。
func newProvider(cfg *aiConfig) (llm.Provider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" && cfg.Provider == "anthropic" {
		return nil, errors.New("未配置 API Key")
	}
	switch cfg.Provider {
	case "anthropic":
		return llm.NewAnthropicProvider(cfg.BaseURL, cfg.APIKey, cfg.Model), nil
	case "openai":
		// 自定义兼容厂商允许 API Key 为空
		return llm.NewOpenAIProvider(cfg.BaseURL, cfg.APIKey, cfg.Model), nil
	}
	return nil, fmt.Errorf("未知的 AI 厂商: %s", cfg.Provider)
}

// configMap 读取 ai_ 前缀的设置。
func (s *AIService) configMap() (map[string]string, error) {
	return (configStore{s.DB}).allWithPrefix("ai_")
}

// upsertConfig 写入单个配置项；空值表示删除。
func (s *AIService) upsertConfig(key, value string) error {
	return (configStore{s.DB}).set(key, value)
}

// boolString 把布尔转成配置表里的字符串形式。
func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
