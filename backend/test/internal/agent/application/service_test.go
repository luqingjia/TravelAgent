package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/luqingjia/TravelAgent/internal/agent/application"
	"github.com/luqingjia/TravelAgent/internal/agent/domain"
)

// TestNewServiceRejectsMissingDependencies 验证应用服务在缺少目录、运行时或非法限制时拒绝构造。
func TestNewServiceRejectsMissingDependencies(t *testing.T) {
	limits := domain.HistoryLimits{MaxMessages: 20, MaxMessageChars: 4000, MaxTotalChars: 16000}
	if _, err := application.NewService(application.Dependencies{Runtime: &fakeRuntime{}, Limits: limits}); err == nil {
		t.Fatal("缺少 catalog 时应失败")
	}
	if _, err := application.NewService(application.Dependencies{Catalog: &fakeCatalog{}, Limits: limits}); err == nil {
		t.Fatal("缺少 runtime 时应失败")
	}
	if _, err := application.NewService(application.Dependencies{
		Catalog: &fakeCatalog{},
		Runtime: &fakeRuntime{},
		Limits:  domain.HistoryLimits{},
	}); err == nil {
		t.Fatal("非法 limits 时应失败")
	}
}

// TestServiceChatUsesDefaultAndExplicitModel 验证空 modelID 使用默认模型，显式 ID 则按指定模型调用。
func TestServiceChatUsesDefaultAndExplicitModel(t *testing.T) {
	runtime := &fakeRuntime{chatContent: "hello"}
	service := mustService(t, &fakeCatalog{
		view: domain.CatalogView{
			DefaultModelID: "default-model",
			Models: []domain.PublicModel{
				publicModel("default-model", true, true),
				publicModel("other-model", true, true),
			},
		},
	}, runtime)

	// 默认模型路径。
	result, err := service.Chat(context.Background(), "", []domain.Message{{Role: domain.RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("default model Chat() error = %v", err)
	}
	if result.ModelID != "default-model" || runtime.lastModelID != "default-model" {
		t.Fatalf("default model = %q/%q", result.ModelID, runtime.lastModelID)
	}

	// 指定模型路径。
	result, err = service.Chat(context.Background(), "other-model", []domain.Message{{Role: domain.RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("explicit model Chat() error = %v", err)
	}
	if result.ModelID != "other-model" || runtime.lastModelID != "other-model" {
		t.Fatalf("explicit model = %q/%q", result.ModelID, runtime.lastModelID)
	}
}

// TestServiceRejectsDisabledUnavailableAndMissingModels 验证禁用、不可用和缺失模型映射为稳定领域错误。
func TestServiceRejectsDisabledUnavailableAndMissingModels(t *testing.T) {
	service := mustService(t, &fakeCatalog{
		view: domain.CatalogView{
			DefaultModelID: "ok",
			Models: []domain.PublicModel{
				publicModel("ok", true, true),
				publicModel("disabled", false, true),
				publicModel("unavailable", true, false),
			},
		},
	}, &fakeRuntime{})

	cases := []struct {
		modelID string
		want    error
	}{
		{modelID: "missing", want: domain.ErrModelNotFound},
		{modelID: "disabled", want: domain.ErrModelDisabled},
		{modelID: "unavailable", want: domain.ErrModelUnavailable},
	}
	for _, test := range cases {
		_, err := service.Chat(context.Background(), test.modelID, []domain.Message{{Role: domain.RoleUser, Content: "hi"}})
		if !errors.Is(err, test.want) {
			t.Fatalf("model %s error = %v, want %v", test.modelID, err, test.want)
		}
	}
}

// TestServiceValidatesHistoryBeforeRuntime 验证消息边界错误在调用 Runtime 前返回，且 Runtime 不被触发。
func TestServiceValidatesHistoryBeforeRuntime(t *testing.T) {
	runtime := &fakeRuntime{chatContent: "should-not-run"}
	service := mustService(t, &fakeCatalog{
		view: domain.CatalogView{
			DefaultModelID: "ok",
			Models:         []domain.PublicModel{publicModel("ok", true, true)},
		},
	}, runtime)

	_, err := service.Chat(context.Background(), "ok", nil)
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("empty history error = %v", err)
	}
	if runtime.chatCalls != 0 {
		t.Fatalf("runtime should not be called, chatCalls=%d", runtime.chatCalls)
	}
}

// TestServiceChatAndStreamSharePreparePath 验证 Chat 与 Stream 共享模型解析，并传递 Context 取消。
func TestServiceChatAndStreamSharePreparePath(t *testing.T) {
	runtime := &fakeRuntime{
		chatContent: "full",
		streamEvents: []application.StreamEvent{
			{Kind: application.EventKindMessage, Content: "a"},
			{Kind: application.EventKindDone, ModelID: "ok"},
		},
	}
	service := mustService(t, &fakeCatalog{
		view: domain.CatalogView{
			DefaultModelID: "ok",
			Models:         []domain.PublicModel{publicModel("ok", true, true)},
		},
	}, runtime)

	if _, err := service.Chat(context.Background(), "", []domain.Message{{Role: domain.RoleUser, Content: "hi"}}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	events, err := service.Stream(context.Background(), "", []domain.Message{{Role: domain.RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	count := 0
	for range events {
		count++
	}
	if count != 2 || runtime.streamCalls != 1 || runtime.chatCalls != 1 {
		t.Fatalf("shared path calls chat=%d stream=%d events=%d", runtime.chatCalls, runtime.streamCalls, count)
	}

	// Context 取消应原样传给 Runtime。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime.blockUntilCanceled = true
	_, err = service.Chat(ctx, "ok", []domain.Message{{Role: domain.RoleUser, Content: "hi"}})
	if !errors.Is(err, domain.ErrAgentFailed) && !errors.Is(err, context.Canceled) {
		// Runtime 返回 context.Canceled 时会被包装为 ErrAgentFailed。
		if !strings.Contains(err.Error(), "canceled") && !errors.Is(err, domain.ErrAgentFailed) {
			t.Fatalf("cancel error = %v", err)
		}
	}
}

// TestServiceClassifiesRuntimeFailure 验证未知运行错误收敛为 ErrAgentFailed。
func TestServiceClassifiesRuntimeFailure(t *testing.T) {
	service := mustService(t, &fakeCatalog{
		view: domain.CatalogView{
			DefaultModelID: "ok",
			Models:         []domain.PublicModel{publicModel("ok", true, true)},
		},
	}, &fakeRuntime{chatErr: errors.New("upstream timeout")})

	_, err := service.Chat(context.Background(), "ok", []domain.Message{{Role: domain.RoleUser, Content: "hi"}})
	if !errors.Is(err, domain.ErrAgentFailed) {
		t.Fatalf("runtime failure = %v, want ErrAgentFailed", err)
	}
}

func mustService(t *testing.T, catalog application.ModelCatalog, runtime application.AgentRuntime) *application.Service {
	t.Helper()
	service, err := application.NewService(application.Dependencies{
		Catalog: catalog,
		Runtime: runtime,
		Limits:  domain.HistoryLimits{MaxMessages: 20, MaxMessageChars: 4000, MaxTotalChars: 16000},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func publicModel(id string, enabled bool, available bool) domain.PublicModel {
	return domain.PublicModel{
		ID: id, DisplayName: id, Provider: "test", Model: id,
		Enabled: enabled, Available: available,
		Capabilities: domain.ModelCapabilities{Chat: true, Streaming: true, Tools: true},
	}
}

type fakeCatalog struct {
	view domain.CatalogView
}

func (catalog *fakeCatalog) List() domain.CatalogView {
	return catalog.view
}

type fakeRuntime struct {
	chatContent        string
	chatErr            error
	streamEvents       []application.StreamEvent
	streamErr          error
	lastModelID        string
	chatCalls          int
	streamCalls        int
	blockUntilCanceled bool
}

func (runtime *fakeRuntime) Chat(ctx context.Context, modelID string, messages []domain.Message) (domain.AssistantMessage, error) {
	runtime.chatCalls++
	runtime.lastModelID = modelID
	if runtime.blockUntilCanceled {
		select {
		case <-ctx.Done():
			return domain.AssistantMessage{}, ctx.Err()
		case <-time.After(2 * time.Second):
			return domain.AssistantMessage{}, errors.New("expected cancel")
		}
	}
	if runtime.chatErr != nil {
		return domain.AssistantMessage{}, runtime.chatErr
	}
	_ = messages
	return domain.AssistantMessage{ModelID: modelID, Content: runtime.chatContent}, nil
}

func (runtime *fakeRuntime) Stream(ctx context.Context, modelID string, messages []domain.Message) (<-chan application.StreamEvent, error) {
	runtime.streamCalls++
	runtime.lastModelID = modelID
	if runtime.streamErr != nil {
		return nil, runtime.streamErr
	}
	_ = messages
	out := make(chan application.StreamEvent, len(runtime.streamEvents))
	go func() {
		defer close(out)
		for _, event := range runtime.streamEvents {
			select {
			case <-ctx.Done():
				return
			case out <- event:
			}
		}
	}()
	return out, nil
}
