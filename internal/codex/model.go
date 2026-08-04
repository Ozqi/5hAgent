// model.go - Codex Responses SSE 到 Eino ToolCallingChatModel 的薄适配。
package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Model 使用 ChatGPT OAuth 调用 Codex Responses backend。
type Model struct {
	store      *Store
	name       string
	tools      []*schema.ToolInfo
	toRemote   map[string]string
	fromRemote map[string]string
	client     *http.Client
}

var _ model.ToolCallingChatModel = (*Model)(nil)

// NewModel 创建一个 Codex 模型适配器。
func NewModel(name string) (*Model, error) {
	store, err := DefaultStore()
	if err != nil {
		return nil, err
	}
	return &Model{store: store, name: name, client: store.client}, nil
}

// WithTools 返回绑定工具后的副本。
func (m *Model) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	clone := *m
	clone.tools = append([]*schema.ToolInfo(nil), tools...)
	clone.toRemote = make(map[string]string, len(tools))
	clone.fromRemote = make(map[string]string, len(tools))
	for _, tool := range tools {
		alias := strings.ReplaceAll(tool.Name, ".", "__")
		if previous, exists := clone.fromRemote[alias]; exists && previous != tool.Name {
			return nil, fmt.Errorf("Codex tool alias collision: %s and %s", previous, tool.Name)
		}
		clone.toRemote[tool.Name] = alias
		clone.fromRemote[alias] = tool.Name
	}
	return &clone, nil
}

// Generate 聚合流式结果，供上下文压缩等非流式调用使用。
func (m *Model) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	reader, err := m.Stream(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var chunks []*schema.Message
	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return schema.ConcatMessages(chunks)
}

// Stream 请求 Codex backend，并将 SSE 事件转换成 Eino message chunks。
func (m *Model) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	requestBody, err := m.buildRequest(input, opts...)
	if err != nil {
		return nil, err
	}
	reader, writer := schema.Pipe[*schema.Message](16)
	go func() {
		defer writer.Close()
		if err := m.stream(ctx, requestBody, writer); err != nil {
			writer.Send(nil, err)
		}
	}()
	return reader, nil
}

func (m *Model) buildRequest(messages []*schema.Message, opts ...model.Option) (map[string]any, error) {
	options := model.GetCommonOptions(&model.Options{Tools: m.tools}, opts...)
	modelName := m.name
	if options.Model != nil && *options.Model != "" {
		modelName = *options.Model
	}
	var instructions []string
	var input []any
	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == schema.System {
			instructions = append(instructions, message.Content)
			continue
		}
		if raw, ok := message.Extra["codex_output"].([]any); ok && len(raw) > 0 {
			input = append(input, raw...)
			continue
		}
		switch message.Role {
		case schema.User:
			input = append(input, responseMessage("user", message.Content))
		case schema.Assistant:
			if message.Content != "" {
				input = append(input, responseMessage("assistant", message.Content))
			}
			for _, call := range message.ToolCalls {
				name := m.toRemote[call.Function.Name]
				if name == "" {
					name = strings.ReplaceAll(call.Function.Name, ".", "__")
				}
				input = append(input, map[string]any{"type": "function_call", "call_id": call.ID, "name": name, "arguments": call.Function.Arguments})
			}
		case schema.Tool:
			input = append(input, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": message.Content})
		}
	}
	tools, err := m.responseTools(options.Tools)
	if err != nil {
		return nil, err
	}
	choice := "auto"
	if options.ToolChoice != nil {
		switch *options.ToolChoice {
		case schema.ToolChoiceForbidden:
			choice = "none"
		case schema.ToolChoiceForced:
			choice = "required"
		}
	}
	return map[string]any{
		"model": modelName, "instructions": strings.Join(instructions, "\n\n"), "input": input,
		"tools": tools, "tool_choice": choice, "parallel_tool_calls": true,
		"reasoning": map[string]any{"effort": "medium", "summary": "auto"},
		"include":   []string{"reasoning.encrypted_content"}, "store": false, "stream": true,
	}, nil
}

func responseMessage(role string, text string) map[string]any {
	contentType := "input_text"
	if role == "assistant" {
		contentType = "output_text"
	}
	return map[string]any{"type": "message", "role": role, "content": []any{map[string]any{"type": contentType, "text": text}}}
}

func (m *Model) responseTools(tools []*schema.ToolInfo) ([]any, error) {
	result := make([]any, 0, len(tools))
	for _, tool := range tools {
		parameters := map[string]any{"type": "object", "properties": map[string]any{}}
		if tool.ParamsOneOf != nil {
			value, err := tool.ParamsOneOf.ToJSONSchema()
			if err != nil {
				return nil, err
			}
			data, _ := json.Marshal(value)
			if err := json.Unmarshal(data, &parameters); err != nil {
				return nil, err
			}
		}
		name := m.toRemote[tool.Name]
		if name == "" {
			name = strings.ReplaceAll(tool.Name, ".", "__")
		}
		result = append(result, map[string]any{"type": "function", "name": name, "description": tool.Desc, "parameters": parameters, "strict": false})
	}
	return result, nil
}

func (m *Model) stream(ctx context.Context, body map[string]any, writer *schema.StreamWriter[*schema.Message]) error {
	return m.streamWithRetry(ctx, body, writer, true)
}

func (m *Model) streamWithRetry(ctx context.Context, body map[string]any, writer *schema.StreamWriter[*schema.Message], retry bool) error {
	accessToken, accountID, err := m.store.Token(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, codexBaseURL+"/responses", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	if accountID != "" {
		request.Header.Set("ChatGPT-Account-ID", accountID)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Originator", "5hagent")
	response, err := m.client.Do(request)
	if err != nil {
		return fmt.Errorf("call Codex: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		if response.StatusCode == http.StatusUnauthorized && retry {
			_ = m.store.ForceRefresh(ctx)
			return m.streamWithRetry(ctx, body, writer, false)
		}
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Codex returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	return m.readSSE(response.Body, writer)
}

func (m *Model) readSSE(body io.Reader, writer *schema.StreamWriter[*schema.Message]) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var data strings.Builder
	var output []any
	var calls []*schema.Message
	completed := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		if line != "" || data.Len() == 0 {
			continue
		}
		raw := data.String()
		data.Reset()
		if raw == "[DONE]" {
			continue
		}
		var event struct {
			Type        string          `json:"type"`
			Delta       string          `json:"delta"`
			OutputIndex int             `json:"output_index"`
			Item        json.RawMessage `json:"item"`
			Response    json.RawMessage `json:"response"`
			Error       json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return fmt.Errorf("decode Codex event: %w", err)
		}
		switch event.Type {
		case "response.output_text.delta":
			writer.Send(&schema.Message{Role: schema.Assistant, Content: event.Delta}, nil)
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			writer.Send(&schema.Message{Role: schema.Assistant, ReasoningContent: event.Delta}, nil)
		case "response.output_item.done":
			var item struct {
				Type      string `json:"type"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if json.Unmarshal(event.Item, &item) == nil {
				var value any
				if json.Unmarshal(event.Item, &value) == nil {
					output = append(output, value)
				}
				if item.Type == "function_call" {
					index := event.OutputIndex
					name := m.fromRemote[item.Name]
					if name == "" {
						name = strings.ReplaceAll(item.Name, "__", ".")
					}
					calls = append(calls, &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{Index: &index, ID: item.CallID, Type: "function", Function: schema.FunctionCall{Name: name, Arguments: item.Arguments}}}})
				}
			}
		case "response.completed":
			for _, call := range calls {
				writer.Send(call, nil)
			}
			meta := responseMeta(event.Response)
			writer.Send(&schema.Message{Role: schema.Assistant, ResponseMeta: meta, Extra: map[string]any{"codex_output": output}}, nil)
			completed = true
			return nil
		case "response.incomplete":
			return fmt.Errorf("Codex response incomplete: %s", raw)
		case "response.failed", "error":
			return fmt.Errorf("Codex response failed: %s", raw)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !completed {
		return fmt.Errorf("Codex stream ended before completion")
	}
	return nil
}

func responseMeta(raw json.RawMessage) *schema.ResponseMeta {
	var response struct {
		Status string `json:"status"`
		Usage  struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(raw, &response)
	reason := "stop"
	if response.Status == "incomplete" {
		reason = "length"
	}
	return &schema.ResponseMeta{FinishReason: reason, Usage: &schema.TokenUsage{PromptTokens: response.Usage.InputTokens, CompletionTokens: response.Usage.OutputTokens, TotalTokens: response.Usage.TotalTokens}}
}
