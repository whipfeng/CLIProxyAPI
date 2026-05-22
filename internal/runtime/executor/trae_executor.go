package executor

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/runtime/executor/helps"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

// TraeExecutor implements a native provider for Trae Gateway.
// It translates OpenAI and Anthropic requests into Trae Gateway's private protocol.
type TraeExecutor struct {
	cfg *config.Config
}

var (
	traeLogFile   *os.File
	traeLogMu     sync.Mutex
	traeLogPath   = `C:\Users\Docker\Desktop\Workspace\proxy-ai-model\logs\trae_debug.log`
)

func init() {
	os.MkdirAll(`C:\Users\Docker\Desktop\Workspace\proxy-ai-model\logs`, 0755)
	f, err := os.OpenFile(traeLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		traeLogFile = f
	}
}

func traeLogf(format string, args ...interface{}) {
	if traeLogFile == nil {
		return
	}
	traeLogMu.Lock()
	defer traeLogMu.Unlock()
	traeLogFile.WriteString(time.Now().Format("2006-01-02 15:04:05.000") + " ")
	fmt.Fprintf(traeLogFile, format, args...)
	traeLogFile.WriteString("\n")
	traeLogFile.Sync()
}

// NewTraeExecutor creates a new Trae executor.
func NewTraeExecutor(cfg *config.Config) *TraeExecutor {
	return &TraeExecutor{cfg: cfg}
}

// Identifier returns the provider key.
func (e *TraeExecutor) Identifier() string { return "trae" }

// HttpRequest implements cliproxyauth.ProviderExecutor.
func (e *TraeExecutor) HttpRequest(ctx context.Context, auth *cliproxyauth.Auth, req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("trae executor does not support direct HTTP proxying")
}

// Refresh implements cliproxyauth.ProviderExecutor.
func (e *TraeExecutor) Refresh(ctx context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	return auth, nil
}

// CountTokens implements cliproxyauth.ProviderExecutor.
func (e *TraeExecutor) CountTokens(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	total := estimateTokens(string(req.Payload))
	out, _ := json.Marshal(map[string]any{"input_tokens": total})
	return cliproxyexecutor.Response{Payload: out}, nil
}

// Execute handles non-streaming requests.
func (e *TraeExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (resp cliproxyexecutor.Response, err error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	reporter := helps.NewUsageReporter(ctx, e.Identifier(), baseModel, auth)
	defer reporter.TrackFailure(ctx, &err)

	gatewayHost, token := e.resolveCredentials(auth)
	if gatewayHost == "" {
		return resp, statusErr{code: http.StatusUnauthorized, msg: "missing trae gateway host"}
	}

	from := opts.SourceFormat
	isAnthropic := from == sdktranslator.FormatClaude
	traeLogf("[Execute] model=%s format=%s stream=false", req.Model, from)
	traeLogf("[Execute] client_payload=%s", string(req.Payload))

	var gatewayReq traeGatewayRequestBody
	if isAnthropic {
		gatewayReq, err = e.buildAnthropicGatewayRequest(req, opts, auth)
	} else {
		gatewayReq, err = e.buildOpenAIGatewayRequest(req, opts, auth)
	}
	if err != nil {
		return resp, err
	}
	gatewayReqBytes, _ := json.Marshal(gatewayReq)
	traeLogf("[Execute] gateway_request=%s", string(gatewayReqBytes))

	events, err := e.runTraeGateway(ctx, gatewayHost, token, gatewayReq, nil)
	if err != nil {
		return resp, err
	}

	var content strings.Builder
	var reasoningContent strings.Builder
	var tokenUsage *usageInfo
	var toolCalls []traeGatewayToolCall
	var gatewayFinishReason string
	for _, event := range events {
		if event.ReasoningContent != "" {
			reasoningContent.WriteString(event.ReasoningContent)
		}
		if event.Text != "" {
			content.WriteString(event.Text)
		}
		if event.Usage != nil {
			tokenUsage = event.Usage
		}
		if len(event.ToolCalls) > 0 {
			toolCalls = event.ToolCalls
		}
		if event.FinishReason != "" {
			gatewayFinishReason = event.FinishReason
		}
	}

	if content.Len() == 0 && reasoningContent.Len() == 0 && len(toolCalls) == 0 {
		return resp, statusErr{code: http.StatusBadGateway, msg: "empty trae gateway response"}
	}

	reporter.Publish(ctx, usage.Detail{InputTokens: int64(tokenUsage.PromptTokens), OutputTokens: int64(tokenUsage.CompletionTokens), TotalTokens: int64(tokenUsage.TotalTokens)})
	reporter.EnsurePublished(ctx)

	var finalText string
	if reasoningContent.Len() > 0 {
		finalText = reasoningContent.String() + content.String()
	} else {
		finalText = content.String()
	}

	now := time.Now().Unix()
	if isAnthropic {
		var anthropicContent []traeAnthropicContentBlock
		if finalText != "" {
			anthropicContent = append(anthropicContent, traeAnthropicContentBlock{Type: "text", Text: finalText})
		}
		for _, call := range toolCalls {
			fn := call.function()
			anthropicContent = append(anthropicContent, traeAnthropicContentBlock{
				Type:  "tool_use",
				ID:    call.ID,
				Name:  fn.Name,
				Input: json.RawMessage(fn.Arguments),
			})
		}
		stopReason := "end_turn"
		if gatewayFinishReason == "tool_calls" || len(toolCalls) > 0 {
			stopReason = "tool_use"
		}
		out, _ := json.Marshal(traeAnthropicResponse{
			ID:         fmt.Sprintf("msg_trae_%d", time.Now().UnixNano()),
			Type:       "message",
			Role:       "assistant",
			Model:      req.Model,
			Content:    anthropicContent,
			StopReason: stopReason,
			Usage:      traeAnthropicUsage{InputTokens: tokenUsage.PromptTokens, OutputTokens: tokenUsage.CompletionTokens},
		})
		return cliproxyexecutor.Response{Payload: out}, nil
	}

	openAIToolCalls := make([]traeOpenAIToolCall, len(toolCalls))
	for i, call := range toolCalls {
		fn := call.function()
		openAIToolCalls[i] = traeOpenAIToolCall{
			Index: i,
			ID:    call.ID,
			Type:  call.Type,
			Function: traeOpenAIToolCallFunc{
				Name:      fn.Name,
				Arguments: fn.Arguments,
			},
		}
	}
	message := traeOpenAIMessage{Role: "assistant", Content: finalText}
	if len(openAIToolCalls) > 0 {
		message.ToolCalls = openAIToolCalls
	}
	finishReason := "stop"
	if gatewayFinishReason == "tool_calls" || len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	out, _ := json.Marshal(traeOpenAIResponse{
		ID:      fmt.Sprintf("chatcmpl-trae-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: now,
		Model:   req.Model,
		Choices: []traeOpenAIChoice{{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		}},
		Usage: traeOpenAIUsage{PromptTokens: tokenUsage.PromptTokens, CompletionTokens: tokenUsage.CompletionTokens, TotalTokens: tokenUsage.TotalTokens},
	})
	return cliproxyexecutor.Response{Payload: out}, nil
}

// ExecuteStream handles streaming requests.
func (e *TraeExecutor) ExecuteStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	reporter := helps.NewUsageReporter(ctx, e.Identifier(), baseModel, auth)

	gatewayHost, token := e.resolveCredentials(auth)
	if gatewayHost == "" {
		return nil, statusErr{code: http.StatusUnauthorized, msg: "missing trae gateway host"}
	}

	from := opts.SourceFormat
	isAnthropic := from == sdktranslator.FormatClaude
	traeLogf("[ExecuteStream] model=%s format=%s stream=true", req.Model, from)
	traeLogf("[ExecuteStream] client_payload=%s", string(req.Payload))

	var gatewayReq traeGatewayRequestBody
	var err error
	if isAnthropic {
		gatewayReq, err = e.buildAnthropicGatewayRequest(req, opts, auth)
	} else {
		gatewayReq, err = e.buildOpenAIGatewayRequest(req, opts, auth)
	}
	if err != nil {
		return nil, err
	}
	gatewayReqBytes, _ := json.Marshal(gatewayReq)
	traeLogf("[ExecuteStream] gateway_request=%s", string(gatewayReqBytes))

	chunks := make(chan cliproxyexecutor.StreamChunk, 64)
	result := &cliproxyexecutor.StreamResult{Chunks: chunks}

	go func() {
		defer close(chunks)
		defer reporter.EnsurePublished(ctx)

		var tokenUsage *usageInfo
		id := fmt.Sprintf("chatcmpl-trae-%d", time.Now().UnixNano())
		if isAnthropic {
			id = fmt.Sprintf("msg_trae_%d", time.Now().UnixNano())
		}
		created := time.Now().Unix()

			var (
				anthropicStarted      bool
				textBlockStarted      bool
				textBlockStopped      bool
				nextBlockIndex        int
				hasToolCalls          bool
				gatewayFinishReason   string
				openAIToolCallSent    = make(map[int]bool)
				anthropicToolBlockIdx = make(map[int]int)
				anthropicToolStopped  = make(map[int]bool)
			)

		_, err := e.runTraeGateway(ctx, gatewayHost, token, gatewayReq, func(event gatewayStreamEvent) error {
				traeLogf("[GatewayEvent] text=%q reasoning=%q tool_calls=%d finish_reason=%q", event.Text, event.ReasoningContent, len(event.ToolCalls), event.FinishReason)
			if event.Usage != nil {
				tokenUsage = event.Usage
				return nil
			}
				if event.FinishReason != "" {
					gatewayFinishReason = event.FinishReason
				}
			if event.Text == "" && event.ReasoningContent == "" && len(event.ToolCalls) == 0 {
				return nil
			}

			if isAnthropic {
				if !anthropicStarted {
					anthropicStarted = true
					payload, _ := json.Marshal(map[string]any{
						"type":    "message_start",
						"message": map[string]any{"id": id, "type": "message", "role": "assistant", "model": req.Model, "content": []any{}, "usage": map[string]any{"input_tokens": 0, "output_tokens": 0}},
					})
					chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
				}

				if event.ReasoningContent != "" || event.Text != "" {
					if !textBlockStarted {
						textBlockStarted = true
						payload, _ := json.Marshal(map[string]any{
							"type":          "content_block_start",
							"index":         nextBlockIndex,
							"content_block": map[string]any{"type": "text", "text": ""},
						})
						chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
						nextBlockIndex++
					}
					payload, _ := json.Marshal(map[string]any{
						"type":  "content_block_delta",
						"index": nextBlockIndex - 1,
						"delta": map[string]any{"type": "text_delta", "text": event.ReasoningContent + event.Text},
					})
					chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
				}

				if len(event.ToolCalls) > 0 {
					hasToolCalls = true
					if textBlockStarted && !textBlockStopped {
						textBlockStopped = true
						payload, _ := json.Marshal(map[string]any{
							"type":  "content_block_stop",
							"index": nextBlockIndex - 1,
						})
						chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
					}
					for _, call := range event.ToolCalls {
						fn := call.function()
						blockIdx, seen := anthropicToolBlockIdx[call.Index]
						if !seen {
							blockIdx = nextBlockIndex
							anthropicToolBlockIdx[call.Index] = blockIdx
							nextBlockIndex++
							payload, _ := json.Marshal(map[string]any{
								"type":  "content_block_start",
								"index": blockIdx,
								"content_block": map[string]any{
									"type": "tool_use",
									"id":   call.ID,
									"name": fn.Name,
									"input": map[string]any{},
								},
							})
							chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
						}
						if fn.Arguments != "" {
							payload, _ := json.Marshal(map[string]any{
								"type":  "content_block_delta",
								"index": blockIdx,
								"delta": map[string]any{
									"type":         "input_json_delta",
									"partial_json": fn.Arguments,
								},
							})
							chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
						}
					}
				}
			} else {
				if event.ReasoningContent != "" || event.Text != "" {
					payload, _ := json.Marshal(traeOpenAIStreamChunk{
						ID:      id,
						Object:  "chat.completion.chunk",
						Created: created,
						Model:   req.Model,
						Choices: []traeOpenAIStreamChoice{{
							Index: 0,
							Delta: traeOpenAIStreamDelta{Content: event.ReasoningContent + event.Text},
						}},
					})
					chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
				}

					if len(event.ToolCalls) > 0 {
						hasToolCalls = true
						for _, call := range event.ToolCalls {
							fn := call.function()
							if !openAIToolCallSent[call.Index] {
								openAIToolCallSent[call.Index] = true
								payload, _ := json.Marshal(traeOpenAIStreamChunk{
									ID:      id,
									Object:  "chat.completion.chunk",
									Created: created,
									Model:   req.Model,
									Choices: []traeOpenAIStreamChoice{{
										Index: 0,
										Delta: traeOpenAIStreamDelta{
											ToolCalls: []traeOpenAIToolCall{{
												Index: call.Index,
												ID:    call.ID,
												Type:  call.Type,
												Function: traeOpenAIToolCallFunc{
													Name:      fn.Name,
													Arguments: fn.Arguments,
												},
											}},
										},
									}},
								})
								chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
							} else {
								if fn.Arguments != "" {
									payload, _ := json.Marshal(traeOpenAIStreamChunk{
										ID:      id,
										Object:  "chat.completion.chunk",
										Created: created,
										Model:   req.Model,
										Choices: []traeOpenAIStreamChoice{{
											Index: 0,
											Delta: traeOpenAIStreamDelta{
												ToolCalls: []traeOpenAIToolCall{{
													Index: call.Index,
													Function: traeOpenAIToolCallFunc{
														Arguments: fn.Arguments,
													},
												}},
											},
										}},
									})
									chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
								}
							}
						}
					}
			}
			return nil
		})

		if err != nil {
			chunks <- cliproxyexecutor.StreamChunk{Err: err}
			return
		}

		if tokenUsage != nil {
			reporter.Publish(ctx, usage.Detail{InputTokens: int64(tokenUsage.PromptTokens), OutputTokens: int64(tokenUsage.CompletionTokens), TotalTokens: int64(tokenUsage.TotalTokens)})
		}

		if isAnthropic {
			if anthropicStarted {
				if textBlockStarted && !textBlockStopped {
					payload, _ := json.Marshal(map[string]any{"type": "content_block_stop", "index": nextBlockIndex - 1})
					chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
				}
				for callIdx, blockIdx := range anthropicToolBlockIdx {
					if !anthropicToolStopped[callIdx] {
						anthropicToolStopped[callIdx] = true
						payload, _ := json.Marshal(map[string]any{"type": "content_block_stop", "index": blockIdx})
						chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
					}
				}
				stopReason := "end_turn"
				if gatewayFinishReason == "tool_calls" || hasToolCalls {
					stopReason = "tool_use"
				}
				payload, _ := json.Marshal(map[string]any{
					"type":         "message_delta",
					"delta":        map[string]any{"stop_reason": stopReason},
					"usage":        map[string]any{"output_tokens": tokenUsage.CompletionTokens},
				})
				chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
				payload, _ = json.Marshal(map[string]any{"type": "message_stop"})
				chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
			}
		} else {
			stop := "stop"
			if gatewayFinishReason == "tool_calls" || hasToolCalls {
				stop = "tool_calls"
			}
			payload, _ := json.Marshal(traeOpenAIStreamChunk{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   req.Model,
				Choices: []traeOpenAIStreamChoice{{
					Index:        0,
					Delta:        traeOpenAIStreamDelta{},
					FinishReason: &stop,
				}},
			})
			chunks <- cliproxyexecutor.StreamChunk{Payload: payload}
		}
	}()

	return result, nil
}

// resolveCredentials extracts Trae Gateway host and token from auth.
func (e *TraeExecutor) resolveCredentials(auth *cliproxyauth.Auth) (host, token string) {
	if auth == nil {
		return "", ""
	}
	if auth.Attributes != nil {
		host = strings.TrimSpace(auth.Attributes["base_url"])
		token = strings.TrimSpace(auth.Attributes["api_key"])
	}
	if host == "" {
		host = "https://console.enterprise.trae.cn"
	}
	return host, token
}

// resolveTraeKey finds the matching TraeKey from the executor config using auth credentials.
func (e *TraeExecutor) resolveTraeKey(auth *cliproxyauth.Auth) *config.TraeKey {
	if auth == nil || e.cfg == nil {
		return nil
	}
	var attrKey, attrBase string
	if auth.Attributes != nil {
		attrKey = strings.TrimSpace(auth.Attributes["api_key"])
		attrBase = strings.TrimSpace(auth.Attributes["base_url"])
	}
	for i := range e.cfg.TraeKey {
		entry := &e.cfg.TraeKey[i]
		cfgKey := strings.TrimSpace(entry.APIKey)
		cfgBase := strings.TrimSpace(entry.BaseURL)
		if attrKey != "" && attrBase != "" {
			if strings.EqualFold(cfgKey, attrKey) && strings.EqualFold(cfgBase, attrBase) {
				return entry
			}
			continue
		}
		if attrKey != "" && strings.EqualFold(cfgKey, attrKey) {
			if cfgBase == "" || strings.EqualFold(cfgBase, attrBase) {
				return entry
			}
		}
		if attrKey == "" && attrBase != "" && strings.EqualFold(cfgBase, attrBase) {
			return entry
		}
	}
	return nil
}

// resolveModelNames maps a client-facing model name to Trae Gateway config/model names.
// It first looks up the alias in the configured model list, then falls back to defaults.
func (e *TraeExecutor) resolveModelNames(requestModel string, traeKey *config.TraeKey) (configName, modelName string) {
	requestModel = strings.TrimSpace(requestModel)
	if requestModel == "" {
		return "deepseek-V4-Pro", "deepseek-V4-Pro__v2"
	}
	if traeKey != nil {
		for _, m := range traeKey.Models {
			alias := strings.TrimSpace(m.Name)
			if alias != "" && strings.EqualFold(alias, requestModel) {
				configName = strings.TrimSpace(m.ConfigName)
				modelName = strings.TrimSpace(m.ModelName)
				if configName == "" {
					configName = "deepseek-V4-Pro"
				}
				if modelName == "" {
					modelName = alias
				}
				return configName, modelName
			}
		}
	}
	return "deepseek-V4-Pro", requestModel
}

// buildOpenAIGatewayRequest translates an OpenAI-format request to Trae Gateway format.
func (e *TraeExecutor) buildOpenAIGatewayRequest(req cliproxyexecutor.Request, opts cliproxyexecutor.Options, auth *cliproxyauth.Auth) (traeGatewayRequestBody, error) {
	var body struct {
		Model    string            `json:"model"`
		Messages []json.RawMessage `json:"messages"`
		Stream   bool              `json:"stream"`
		Tools    json.RawMessage   `json:"tools,omitempty"`
	}
	if err := json.Unmarshal(req.Payload, &body); err != nil {
		return traeGatewayRequestBody{}, fmt.Errorf("invalid OpenAI request payload: %w", err)
	}

	traeKey := e.resolveTraeKey(auth)
	configName, modelName := e.resolveModelNames(body.Model, traeKey)

	messages := make([]traeGatewayMessage, 0, len(body.Messages))
	for _, rawMsg := range body.Messages {
		msg := parseOpenAIMessage(rawMsg)
		if msg.Role == "" {
			continue
		}
		// Skip messages with no content and no tool_calls
		if len(msg.Content) == 0 && len(msg.ToolCalls) == 0 && msg.ToolCallID == "" {
			continue
		}
		messages = append(messages, msg)
	}

	if len(messages) == 0 {
		return traeGatewayRequestBody{}, errors.New("message content is required")
	}

	conversationID := newTraeID()
	sessionID := newTraeID()
	return traeGatewayRequestBody{
		ConfigName:     configName,
		ConversationID: conversationID,
		SessionID:      sessionID,
		ModelName:      modelName,
		Messages:       messages,
		Tools:          normalizeOpenAITools(body.Tools),
	}, nil
}

// buildAnthropicGatewayRequest translates an Anthropic-format request to Trae Gateway format.
func (e *TraeExecutor) buildAnthropicGatewayRequest(req cliproxyexecutor.Request, opts cliproxyexecutor.Options, auth *cliproxyauth.Auth) (traeGatewayRequestBody, error) {
	var body struct {
		Model    string            `json:"model"`
		System   json.RawMessage   `json:"system"`
		Messages []json.RawMessage `json:"messages"`
		Tools    json.RawMessage   `json:"tools,omitempty"`
	}
	if err := json.Unmarshal(req.Payload, &body); err != nil {
		return traeGatewayRequestBody{}, fmt.Errorf("invalid Anthropic request payload: %w", err)
	}

	traeKey := e.resolveTraeKey(auth)
	configName, modelName := e.resolveModelNames(body.Model, traeKey)

	messages := make([]traeGatewayMessage, 0, len(body.Messages)+1)
	// Handle system field: may be a string or array of system blocks
	if len(body.System) > 0 && string(body.System) != "null" {
		var sysText string
		if err := json.Unmarshal(body.System, &sysText); err == nil && strings.TrimSpace(sysText) != "" {
			messages = append(messages, traeGatewayMessage{Role: "system", Content: []traeGatewayContentBlock{{Type: "text", Text: sysText}}})
		} else {
			var sysBlocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(body.System, &sysBlocks); err == nil {
				for _, b := range sysBlocks {
					if strings.TrimSpace(b.Text) != "" {
						messages = append(messages, traeGatewayMessage{Role: "system", Content: []traeGatewayContentBlock{{Type: "text", Text: b.Text}}})
					}
				}
			}
		}
	}
	for _, rawMsg := range body.Messages {
		msgs := parseAnthropicMessage(rawMsg)
		for _, msg := range msgs {
			if msg.Role == "" {
				continue
			}
			if len(msg.Content) == 0 && len(msg.ToolCalls) == 0 && msg.ToolCallID == "" {
				continue
			}
			messages = append(messages, msg)
		}
	}

	if len(messages) == 0 {
		return traeGatewayRequestBody{}, errors.New("message content is required")
	}

	conversationID := newTraeID()
	sessionID := newTraeID()
	return traeGatewayRequestBody{
		ConfigName:     configName,
		ConversationID: conversationID,
		SessionID:      sessionID,
		ModelName:      modelName,
		Messages:       messages,
		Tools:          convertAnthropicToolsToOpenAI(body.Tools),
	}, nil
}

// runTraeGateway sends a request to Trae Gateway and reads SSE events.
func (e *TraeExecutor) runTraeGateway(ctx context.Context, host, token string, body traeGatewayRequestBody, onEvent func(gatewayStreamEvent) error) ([]gatewayStreamEvent, error) {
	if token == "" {
		return nil, errors.New("gateway token is required")
	}
	if body.ConversationID == "" {
		body.ConversationID = newTraeID()
	}
	if body.SessionID == "" {
		body.SessionID = body.ConversationID
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(host, "/") + "/api/ide/v2/llm_raw_chat"
	traeLogf("[runTraeGateway] url=%s body=%s", url, string(encoded))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Cloud-IDE-JWT "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("X-App-Id", "7b3f9dc2-8a4e-5c6d-2f1b-9e4a3c5b7df0")
	httpReq.Header.Set("X-Ide-Function", "chat")
	httpReq.Header.Set("X-Ide-Version", "99.99.99")
	httpReq.Header.Set("X-Ide-Version-Code", "20260206")
	httpReq.Header.Set("X-Flow-Traceparent", generateTraceparent())
	// agent_loop_id must be a separate UUID from session_id/conversation_id,
	// matching real Trae capture behaviour.
	agentLoopID := newTraeID()
	extra, _ := json.Marshal(map[string]any{
		"agent_loop_id":         agentLoopID,
		"api_host":              host,
		"api_key":               token,
		"base_url":              host + "/trae-cli/api/v1/llm/proxy",
		"config_name":           body.ConfigName,
		"config_source":         1,
		"display_name":          body.ConfigName,
		"model_name":            body.ModelName,
		"real_api_key":          "",
		"real_base_url":         "",
		"session_id":            body.ConversationID,
		"user_prompt_submit_id": agentLoopID,
	})
	httpReq.Header.Set("Extra", string(extra))

	client := &http.Client{Timeout: 150 * time.Second}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trae gateway status %d", httpResp.StatusCode)
	}

	return readTraeGatewayEvents(ctx, httpResp.Body, onEvent)
}

// readTraeGatewayEvents reads SSE events from Trae Gateway response.
func readTraeGatewayEvents(ctx context.Context, body io.Reader, onEvent func(gatewayStreamEvent) error) ([]gatewayStreamEvent, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	lines := make(chan string, 100)
	doneScan := make(chan error, 1)

	go func() {
		defer close(lines)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case doneScan <- err:
			case <-ctx.Done():
			}
		}
	}()

	event := ""
	done := false
	events := []gatewayStreamEvent{}
	toolCalls := gatewayToolCallAccumulator{index: map[int]int{}}

	for line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		switch event {
		case "output":
			traeLogf("[GatewaySSE] event=output data=%s", data)
			var output traeGatewayOutputEvent
			if err := json.Unmarshal([]byte(data), &output); err != nil {
				return events, err
			}
			if output.ReasoningContent != "" {
				gatewayEvent := gatewayStreamEvent{ReasoningContent: output.ReasoningContent}
				events = append(events, gatewayEvent)
				if onEvent != nil {
					if err := onEvent(gatewayEvent); err != nil {
						return events, err
					}
				}
			}
			if output.Response != "" {
				gatewayEvent := gatewayStreamEvent{Text: output.Response}
				events = append(events, gatewayEvent)
				if onEvent != nil {
					if err := onEvent(gatewayEvent); err != nil {
						return events, err
					}
				}
			}
			toolCalls.add(output.ToolCalls)
			if onEvent != nil && len(output.ToolCalls) > 0 {
				if err := onEvent(gatewayStreamEvent{ToolCalls: output.ToolCalls}); err != nil {
					return events, err
				}
			}
		case "error":
			traeLogf("[GatewaySSE] event=error data=%s", data)
			var gatewayError traeGatewayErrorEvent
			if err := json.Unmarshal([]byte(data), &gatewayError); err != nil {
				return events, err
			}
			if gatewayError.Message != "" {
				return events, errors.New(gatewayError.Message)
			}
			return events, errors.New(gatewayError.Error)
		case "token_usage":
			traeLogf("[GatewaySSE] event=token_usage data=%s", data)
			var tu traeGatewayTokenUsage
			if err := json.Unmarshal([]byte(data), &tu); err != nil {
				return events, err
			}
			gatewayEvent := gatewayStreamEvent{Usage: &usageInfo{PromptTokens: tu.PromptTokens, CompletionTokens: tu.CompletionTokens, TotalTokens: tu.TotalTokens}}
			events = append(events, gatewayEvent)
			if onEvent != nil {
				if err := onEvent(gatewayEvent); err != nil {
					return events, err
				}
			}
		case "done":
			traeLogf("[GatewaySSE] event=done data=%s", data)
			done = true
			var doneEvent traeGatewayDoneEvent
			_ = json.Unmarshal([]byte(data), &doneEvent)
			// Non-streaming consumers need the full accumulated toolCalls
			events = append(events, gatewayStreamEvent{ToolCalls: toolCalls.calls, FinishReason: doneEvent.FinishReason})
			if onEvent != nil {
				// Streaming consumers already received incremental tool calls from output events
				if err := onEvent(gatewayStreamEvent{FinishReason: doneEvent.FinishReason}); err != nil {
					return events, err
				}
			}
			return events, nil
		}
	}

	select {
	case err := <-doneScan:
		return events, err
	default:
	}
	if !done {
		return events, io.ErrUnexpectedEOF
	}
	return events, nil
}

// --- Types ported from go-tools ---

type traeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type traeGatewayRequestBody struct {
	ConfigName     string               `json:"config_name"`
	ConversationID string               `json:"conversation_id"`
	ModelName      string               `json:"model_name"`
	Messages       []traeGatewayMessage `json:"messages"`
	SessionID      string               `json:"session_id"`
	Tools          json.RawMessage      `json:"tools,omitempty"`
	UserInput      string               `json:"user_input"`
}

// traeGatewayMessage mirrors the message object Trae Gateway expects.
// All fields are kept non-omitted because Gateway validates a uniform
// schema across every message in the array.
type traeGatewayMessage struct {
	Role       string                    `json:"role"`
	Content    []traeGatewayContentBlock `json:"content"`
	ToolCalls  []traeGatewayToolCall     `json:"tool_calls"`
	ToolCallID string                    `json:"tool_call_id"`
}

type traeGatewayContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type traeGatewayOutputEvent struct {
	Response         string                `json:"response"`
	ReasoningContent string                `json:"reasoning_content"`
	ToolCalls        []traeGatewayToolCall `json:"tool_calls"`
}

type traeGatewayErrorEvent struct {
	Code    int    `json:"code"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

type traeGatewayDoneEvent struct {
	FinishReason string `json:"finish_reason"`
}

type traeGatewayTokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// traeGatewayToolCall mirrors the Gateway wire format.
// The Gateway uses "function_call" (not "function") for both requests
// and responses.  The "function" field is kept for backward-compat
// parsing but is never serialised.
type traeGatewayToolCall struct {
	Index        int                     `json:"index"`
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Function     traeGatewayToolCallFunc `json:"-"`
	FunctionCall traeGatewayToolCallFunc `json:"function_call"`
}

type traeGatewayToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (c traeGatewayToolCall) function() traeGatewayToolCallFunc {
	if c.FunctionCall.Name != "" || c.FunctionCall.Arguments != "" {
		return c.FunctionCall
	}
	return c.Function
}

type gatewayStreamEvent struct {
	Text             string
	ReasoningContent string
	ToolCalls        []traeGatewayToolCall
	Usage            *usageInfo
	FinishReason     string
}

type gatewayToolCallAccumulator struct {
	calls []traeGatewayToolCall
	index map[int]int
}

func (a *gatewayToolCallAccumulator) add(calls []traeGatewayToolCall) {
	for _, call := range calls {
		function := call.function()
		if existingIndex, ok := a.index[call.Index]; ok {
			existing := &a.calls[existingIndex]
			if call.ID != "" {
				existing.ID = call.ID
			}
			if call.Type != "" {
				existing.Type = call.Type
			}
			if function.Name != "" {
				existing.FunctionCall.Name = function.Name
			}
			existing.FunctionCall.Arguments += function.Arguments
			continue
		}
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d", call.Index)
		}
		if call.Type == "" {
			call.Type = "function"
		}
		// Normalise: move any stray Function data into FunctionCall
		// so the wire format is consistent.
		if call.FunctionCall.Name == "" && call.FunctionCall.Arguments == "" {
			call.FunctionCall = call.Function
			call.Function = traeGatewayToolCallFunc{}
		}
		a.index[call.Index] = len(a.calls)
		a.calls = append(a.calls, call)
	}
}

type usageInfo struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// --- Response types ---

type traeOpenAIResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []traeOpenAIChoice `json:"choices"`
	Usage   traeOpenAIUsage  `json:"usage"`
}

type traeOpenAIChoice struct {
	Index        int              `json:"index"`
	Message      traeOpenAIMessage `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type traeOpenAIMessage struct {
	Role      string               `json:"role"`
	Content   string               `json:"content,omitempty"`
	ToolCalls []traeOpenAIToolCall `json:"tool_calls,omitempty"`
}

type traeOpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type traeOpenAIStreamChunk struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []traeOpenAIStreamChoice `json:"choices"`
}

type traeOpenAIStreamChoice struct {
	Index        int                `json:"index"`
	Delta        traeOpenAIStreamDelta `json:"delta"`
	FinishReason *string            `json:"finish_reason"`
}

type traeOpenAIStreamDelta struct {
	Role      string                   `json:"role,omitempty"`
	Content   string                   `json:"content,omitempty"`
	ToolCalls []traeOpenAIToolCall     `json:"tool_calls,omitempty"`
}

type traeOpenAIToolCall struct {
	Index    int                    `json:"index"`
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function traeOpenAIToolCallFunc `json:"function,omitempty"`
}

type traeOpenAIToolCallFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type traeAnthropicResponse struct {
	ID         string                     `json:"id"`
	Type       string                     `json:"type"`
	Role       string                     `json:"role"`
	Model      string                     `json:"model"`
	Content    []traeAnthropicContentBlock `json:"content"`
	StopReason string                     `json:"stop_reason"`
	Usage      traeAnthropicUsage         `json:"usage"`
}

type traeAnthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type traeAnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// --- Helpers ---

// convertAnthropicToolsToOpenAI converts Anthropic-format tools to OpenAI format
// expected by Trae Gateway. Anthropic uses "name" / "input_schema", while OpenAI
// uses "type" / "function" / "parameters".
func convertAnthropicToolsToOpenAI(tools json.RawMessage) json.RawMessage {
	if len(tools) == 0 || string(tools) == "null" {
		return nil
	}
	var anthropicTools []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	if err := json.Unmarshal(tools, &anthropicTools); err != nil {
		return normalizeOpenAITools(tools)
	}
	if len(anthropicTools) == 0 {
		return nil
	}
	// Sanity check: at least one tool must have a name, otherwise this is
	// probably already OpenAI format and we should not mangle it.
	hasName := false
	for _, t := range anthropicTools {
		if t.Name != "" {
			hasName = true
			break
		}
	}
	if !hasName {
		return normalizeOpenAITools(tools)
	}
	openAITools := make([]map[string]any, len(anthropicTools))
	for i, t := range anthropicTools {
		fn := map[string]any{
			"name":        t.Name,
			"description": t.Description,
		}
		if params := serializeToolParameters(t.InputSchema); params != nil {
			fn["parameters"] = params
		}
		openAITools[i] = map[string]any{
			"type":     "function",
			"function": fn,
		}
	}
	out, _ := json.Marshal(openAITools)
	return out
}

// serializeToolParameters ensures tool parameters are sent as a JSON string.
// Trae Gateway expects parameters to be a stringified JSON object, not a raw
// object. If the input is already a string we return it as-is; if it is a
// JSON object we marshal it to a string.
func serializeToolParameters(params json.RawMessage) json.RawMessage {
	if len(params) == 0 || string(params) == "null" {
		return nil
	}
	trimmed := bytes.TrimSpace(params)
	if len(trimmed) == 0 {
		return nil
	}
	// Already a JSON string (quoted)?
	if trimmed[0] == '"' {
		var str string
		if err := json.Unmarshal(params, &str); err == nil {
			return json.RawMessage(strconv.Quote(str))
		}
		return nil
	}
	// JSON object or array – marshal to string.
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return json.RawMessage(strconv.Quote(string(params)))
	}
	return nil
}

// normalizeOpenAITools walks an OpenAI-format tools array and ensures every
// tool's "parameters" field is a JSON string (stringifies object schemas).
func normalizeOpenAITools(tools json.RawMessage) json.RawMessage {
	if len(tools) == 0 || string(tools) == "null" {
		return nil
	}
	var toolList []map[string]json.RawMessage
	if err := json.Unmarshal(tools, &toolList); err != nil {
		return tools
	}
	for i, tool := range toolList {
		fn, ok := tool["function"]
		if !ok {
			continue
		}
		var fnObj map[string]json.RawMessage
		if err := json.Unmarshal(fn, &fnObj); err != nil {
			continue
		}
		if params, ok := fnObj["parameters"]; ok {
			normalized := serializeToolParameters(params)
			if normalized == nil {
				delete(fnObj, "parameters")
			} else {
				fnObj["parameters"] = normalized
			}
			fnBytes, _ := json.Marshal(fnObj)
			toolList[i]["function"] = json.RawMessage(fnBytes)
		}
	}
	out, _ := json.Marshal(toolList)
	return out
}

// parseOpenAIMessage parses an OpenAI-format message into Trae Gateway format.
// Preserves tool_calls on assistant messages and tool_call_id on tool messages.
func parseOpenAIMessage(raw json.RawMessage) traeGatewayMessage {
	var msg struct {
		Role       string                   `json:"role"`
		Content    json.RawMessage          `json:"content"`
		ToolCalls  []traeOpenAIToolCall     `json:"tool_calls,omitempty"`
		ToolCallID string                   `json:"tool_call_id,omitempty"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return traeGatewayMessage{}
	}

	var gatewayMsg traeGatewayMessage
	gatewayMsg.Role = msg.Role
	gatewayMsg.ToolCallID = msg.ToolCallID

	// Extract text content
	if len(msg.Content) > 0 && string(msg.Content) != "null" {
		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil {
			if strings.TrimSpace(text) != "" {
				gatewayMsg.Content = append(gatewayMsg.Content, traeGatewayContentBlock{Type: "text", Text: text})
			}
		} else {
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(msg.Content, &blocks); err == nil {
				for _, b := range blocks {
					if b.Type == "" || b.Type == "text" {
						if strings.TrimSpace(b.Text) != "" {
							gatewayMsg.Content = append(gatewayMsg.Content, traeGatewayContentBlock{Type: "text", Text: b.Text})
						}
					}
				}
			}
		}
	}

	// Convert tool_calls to gateway format (Gateway expects function_call field)
	if len(msg.ToolCalls) > 0 {
		gatewayMsg.ToolCalls = make([]traeGatewayToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			gatewayMsg.ToolCalls = append(gatewayMsg.ToolCalls, traeGatewayToolCall{
				Index:        tc.Index,
				ID:           tc.ID,
				Type:         tc.Type,
				FunctionCall: traeGatewayToolCallFunc{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
			})
		}
	}

	// Gateway rejects assistant messages with content: null when tool_calls are present.
	// Match real Trae client behaviour: always provide at least a single-space text block.
	if gatewayMsg.Role == "assistant" && len(gatewayMsg.ToolCalls) > 0 && len(gatewayMsg.Content) == 0 {
		gatewayMsg.Content = []traeGatewayContentBlock{{Type: "text", Text: " "}}
	}

	return gatewayMsg
}

// parseAnthropicMessage parses an Anthropic-format message into Trae Gateway format.
// Returns a slice because a single Anthropic message may contain multiple
// tool_result blocks, each of which must become its own gateway tool message.
func parseAnthropicMessage(raw json.RawMessage) []traeGatewayMessage {
	var msg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil
	}

	if len(msg.Content) == 0 || string(msg.Content) == "null" {
		return []traeGatewayMessage{{Role: msg.Role}}
	}

	// Content may be a string
	var textContent string
	if err := json.Unmarshal(msg.Content, &textContent); err == nil {
		if strings.TrimSpace(textContent) != "" {
			return []traeGatewayMessage{{
				Role:    msg.Role,
				Content: []traeGatewayContentBlock{{Type: "text", Text: textContent}},
			}}
		}
		return []traeGatewayMessage{{Role: msg.Role}}
	}

	// Array of content blocks
	var blocks []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		Content   json.RawMessage `json:"content"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return []traeGatewayMessage{{Role: msg.Role}}
	}

	// Build main message from text/tool_use blocks.
	// Each tool_result block becomes its own separate tool message.
	mainMsg := traeGatewayMessage{Role: msg.Role}
	var result []traeGatewayMessage

	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				mainMsg.Content = append(mainMsg.Content, traeGatewayContentBlock{Type: "text", Text: b.Text})
			}
		case "tool_use":
			args, _ := json.Marshal(b.Input)
			mainMsg.ToolCalls = append(mainMsg.ToolCalls, traeGatewayToolCall{
				ID:   b.ID,
				Type: "function",
				FunctionCall: traeGatewayToolCallFunc{
					Name:      b.Name,
					Arguments: string(args),
				},
			})
		case "tool_result":
			resultText := extractToolResultText(b.Content)
			if resultText == "" {
				resultText = strings.TrimSpace(b.Text)
			}
			toolMsg := traeGatewayMessage{
				Role:       "tool",
				ToolCallID: b.ToolUseID,
			}
			if resultText != "" {
				toolMsg.Content = []traeGatewayContentBlock{{Type: "text", Text: resultText}}
			}
			result = append(result, toolMsg)
		}
	}

	// Prepend main message if it has any content or tool_calls.
	if len(mainMsg.Content) > 0 || len(mainMsg.ToolCalls) > 0 {
		// Gateway rejects assistant messages with content: null when tool_calls are present.
		if mainMsg.Role == "assistant" && len(mainMsg.ToolCalls) > 0 && len(mainMsg.Content) == 0 {
			mainMsg.Content = []traeGatewayContentBlock{{Type: "text", Text: " "}}
		}
		result = append([]traeGatewayMessage{mainMsg}, result...)
	}

	if len(result) == 0 {
		return []traeGatewayMessage{{Role: msg.Role}}
	}
	return result
}

func extractToolResultText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var texts []string
		for _, b := range blocks {
			if b.Type == "" || b.Type == "text" {
				texts = append(texts, b.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

func extractMessageContent(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var texts []string
		for _, b := range blocks {
			if b.Type == "" || b.Type == "text" {
				texts = append(texts, b.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	cjk, ascii := 0, 0
	for _, r := range text {
		if r > 127 {
			cjk++
		} else {
			ascii++
		}
	}
	return cjk + ascii/4 + 1
}

func newTraeID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func generateTraceparent() string {
	var traceID [16]byte
	var parentID [8]byte
	rand.Read(traceID[:])
	rand.Read(parentID[:])
	return fmt.Sprintf("00-%x-%x-01", traceID[:], parentID[:])
}
