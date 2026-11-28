package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cwmerch-api/internal/models"
)

func cwcloudConn(url string) models.AIConnection {
	return models.AIConnection{Format: models.AIFormatCWCloud, BaseURL: url, Model: "claude-sonnet", Token: "tok", MaxTokens: 500}
}

func TestCWCloudEndpointTolerance(t *testing.T) {
	cases := map[string]string{
		"https://api.cwcloud.tech/v1/ai/prompt": "https://api.cwcloud.tech/v1/ai/prompt",
		"https://api.cwcloud.tech/v1":           "https://api.cwcloud.tech/v1/ai/prompt",
		"https://api.cwcloud.tech":              "https://api.cwcloud.tech/v1/ai/prompt",
		"https://api.cwcloud.tech/":             "https://api.cwcloud.tech/v1/ai/prompt",
	}
	for in, want := range cases {
		if got := cwcloudEndpoint(in); got != want {
			t.Errorf("cwcloudEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCWCloudHappyStringAndRequestShape(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != "tok" {
			t.Errorf("missing X-Auth-Token, got %q", r.Header.Get("X-Auth-Token"))
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusCreated) // 201
		_, _ = w.Write([]byte(`{"status":"ok","response":"Hello there","model":"claude-sonnet-5-5","usage":{"prompt":8,"completion":10,"total":18,"cached":2}}`))
	}))
	defer srv.Close()

	resp, err := New().askCWCloud(context.Background(), cwcloudConn(srv.URL), "claude-sonnet", Request{
		System:   "be brief",
		Messages: []Turn{{Role: models.AIChatRoleUser, Text: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Message != "Hello there" {
		t.Errorf("message = %q", resp.Message)
	}
	if resp.Model != "claude-sonnet-5-5" {
		t.Errorf("model = %q (want CWCloud's actual model)", resp.Model)
	}
	if resp.Tokens.Input != 8 || resp.Tokens.Output != 10 || resp.Tokens.Cached != 2 {
		t.Errorf("tokens = %+v (want 8/10/2)", resp.Tokens)
	}
	if gotBody["adapter"] != "claude-sonnet" {
		t.Errorf("adapter = %v (want the model)", gotBody["adapter"])
	}
	if gotBody["enable_history"] != false {
		t.Errorf("enable_history = %v (want false)", gotBody["enable_history"])
	}
	if gotBody["system"] != "be brief" {
		t.Errorf("system = %v (want the dedicated system field)", gotBody["system"])
	}
	tools, present := gotBody["tools"].([]any)
	if !present || len(tools) != 0 {
		t.Errorf("tools = %#v (want [] present even with no tool: absent makes CWCloud run its own assistant)", gotBody["tools"])
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages = %v (want the user turn only, no system turn)", gotBody["messages"])
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "user" || first["message"] != "hi" {
		t.Errorf("first message = %v", first)
	}
	if _, hasContent := first["content"]; hasContent {
		t.Errorf("turn used `content`, must be `message`")
	}
}

func TestCWCloudToolsTranscriptAndToolCalls(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"ok","response":"","finish_reason":"tool_calls",` +
			`"tool_calls":[{"id":"c9","name":"cwmerch_api","args":{"method":"GET","path":"/v1/admin/orders"}},{"name":"cwmerch_api","args":{}}],` +
			`"model":"m","usage":{"prompt":1,"completion":2,"total":3,"cached":0}}`))
	}))
	defer srv.Close()

	resp, err := New().askCWCloud(context.Background(), cwcloudConn(srv.URL), "m", Request{
		System: "sys",
		Tools: []Tool{{Name: "cwmerch_api", Description: "call the API",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}}},
		Messages: []Turn{
			{Role: models.AIChatRoleUser, Text: "orders?"},
			{Role: models.AIChatRoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "cwmerch_api", Args: json.RawMessage(`{"k":"v"}`)}}},
			{Role: RoleTool, ToolCallID: "c1", ToolName: "cwmerch_api", Text: "result"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	tools, _ := gotBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", gotBody["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "cwmerch_api" || tool["description"] != "call the API" {
		t.Errorf("tool = %v", tool)
	}
	if params, _ := tool["parameters"].(map[string]any); params["type"] != "object" {
		t.Errorf("parameters = %v (want the JSON schema object)", tool["parameters"])
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages = %v", gotBody["messages"])
	}
	assistant := msgs[1].(map[string]any)
	calls, _ := assistant["tool_calls"].([]any)
	if assistant["role"] != "assistant" || len(calls) != 1 {
		t.Fatalf("assistant turn = %v", assistant)
	}
	call := calls[0].(map[string]any)
	if call["id"] != "c1" || call["name"] != "cwmerch_api" {
		t.Errorf("tool call = %v", call)
	}
	if args, _ := call["args"].(map[string]any); args["k"] != "v" {
		t.Errorf("args = %#v (want a JSON object)", call["args"])
	}
	toolTurn := msgs[2].(map[string]any)
	if toolTurn["role"] != "tool" || toolTurn["tool_call_id"] != "c1" || toolTurn["tool_name"] != "cwmerch_api" || toolTurn["message"] != "result" {
		t.Errorf("tool turn = %v", toolTurn)
	}

	if len(resp.ToolCalls) != 2 {
		t.Fatalf("tool calls = %+v (an empty response with tool_calls is valid)", resp.ToolCalls)
	}
	if resp.ToolCalls[0].ID != "c9" || resp.ToolCalls[0].Name != "cwmerch_api" ||
		!strings.Contains(string(resp.ToolCalls[0].Args), "/v1/admin/orders") {
		t.Errorf("first call = %+v", resp.ToolCalls[0])
	}
	if resp.ToolCalls[1].ID == "" || string(resp.ToolCalls[1].Args) != "{}" {
		t.Errorf("second call = %+v (want a generated id and {} args)", resp.ToolCalls[1])
	}
	if resp.Tokens.Input != 1 || resp.Tokens.Output != 2 {
		t.Errorf("tokens = %+v", resp.Tokens)
	}
}

func TestCWCloudEmptyAnswerIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"ok","response":"","finish_reason":"stop"}`))
	}))
	defer srv.Close()
	if _, err := New().askCWCloud(context.Background(), cwcloudConn(srv.URL), "m", Request{Messages: []Turn{{Role: "user", Text: "x"}}}); err == nil {
		t.Fatal("want an error for an empty answer with no tool calls")
	}
}

func TestCWCloudResponseArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"ok","response":["line one","line two"],"usage":{"prompt":1,"completion":1}}`))
	}))
	defer srv.Close()
	resp, err := New().askCWCloud(context.Background(), cwcloudConn(srv.URL), "m", Request{Messages: []Turn{{Role: "user", Text: "x"}}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Message != "line one\nline two" {
		t.Errorf("array join = %q", resp.Message)
	}
}

func TestCWCloudKoOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // 2xx but ko
		_, _ = w.Write([]byte(`{"status":"ko","error":"rate limited","i18n_code":"x","http_code":429}`))
	}))
	defer srv.Close()
	_, err := New().askCWCloud(context.Background(), cwcloudConn(srv.URL), "m", Request{Messages: []Turn{{Role: "user", Text: "x"}}})
	var refused ErrRefused
	if !errors.As(err, &refused) {
		t.Fatalf("want ErrRefused, got %v", err)
	}
	if !strings.Contains(refused.Detail, "rate limited") {
		t.Errorf("detail = %q", refused.Detail)
	}
}

func TestCWCloud424UpstreamUnwrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusFailedDependency) // 424
		_, _ = w.Write([]byte(`{"status":"ko","error":"model failed","upstream":{"http_code":401,"source":"anthropic","adapter":"claude-sonnet","body":"Incorrect API key"}}`))
	}))
	defer srv.Close()
	_, err := New().askCWCloud(context.Background(), cwcloudConn(srv.URL), "m", Request{Messages: []Turn{{Role: "user", Text: "x"}}})
	var refused ErrRefused
	if !errors.As(err, &refused) {
		t.Fatalf("want ErrRefused, got %v", err)
	}
	if !strings.Contains(refused.Detail, "Incorrect API key") || !strings.Contains(refused.Detail, "401") {
		t.Errorf("424 detail did not unwrap upstream: %q", refused.Detail)
	}
}
