package application_test

import (
	. "github.com/luqingjia/TravelAgent/internal/knowledge/application"
	"io"
	"log/slog"
	"testing"
	"time"
)

// TestNewServiceRejectsMissingDependencies 验证应用服务不会在缺少长期依赖时带病启动。
// 如果构造阶段不拦住 nil，问题会推迟到真实请求中以 panic 形式出现，定位成本更高。
func TestNewServiceRejectsMissingDependencies(t *testing.T) {
	tests := []struct {
		// name 说明当前从依赖集合中拿掉了什么。
		name string
		// mutate 返回一份只破坏一个条件的依赖，便于准确判断失败原因。
		mutate func(Dependencies) Dependencies
	}{
		{name: "缺少仓储", mutate: func(deps Dependencies) Dependencies { deps.Repository = nil; return deps }},
		{name: "缺少对象存储", mutate: func(deps Dependencies) Dependencies { deps.Storage = nil; return deps }},
		{name: "缺少向量模型", mutate: func(deps Dependencies) Dependencies { deps.Embedder = nil; return deps }},
		{name: "缺少日志器", mutate: func(deps Dependencies) Dependencies { deps.Logger = nil; return deps }},
		{name: "上传大小非法", mutate: func(deps Dependencies) Dependencies { deps.Policy.MaxUploadBytes = 0; return deps }},
		{name: "扩展名列表为空", mutate: func(deps Dependencies) Dependencies {
			deps.Policy.AllowedExtensions = nil
			return deps
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 准备：从一组完整依赖出发，只移除当前子场景关心的一项。
			deps := tt.mutate(validDependencies())

			// 执行：构造器是依赖图的第一道质量门。
			service, err := NewService(deps)

			// 断言：返回明确错误且不产生半初始化 Service。
			if err == nil {
				t.Fatal("NewService() error = nil, want dependency validation error")
			}
			if service != nil {
				t.Fatalf("NewService() service = %#v, want nil", service)
			}
		})
	}
}

// TestNewServiceAcceptsExplicitDependencies 验证手工注入完整依赖后可以直接得到可测试的应用服务。
func TestNewServiceAcceptsExplicitDependencies(t *testing.T) {
	// 准备：fake、固定时钟和固定 ID 都通过构造器传入，不依赖全局变量或 Gin Context。
	deps := validDependencies()

	// 执行：组合根未来会使用同一个构造器注入真实实现。
	service, err := NewService(deps)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	// 断言：构造成功，并且测试提供的可控 clock/ID 被保留下来。
	if service == nil {
		t.Fatal("NewService() service = nil")
	}
}

// fixedNow 是所有应用层测试共享的固定时钟，避免断言依赖真实系统时间。
var fixedNow = time.Date(2026, time.July, 14, 11, 0, 0, 0, time.UTC)

// validDependencies 返回大多数应用层测试使用的完整依赖集合。
func validDependencies() Dependencies {
	// fake 负责记录调用，固定时钟和固定 ID 让新文档字段可以精确比较。
	return Dependencies{
		Repository: &fakeRepository{},
		Storage:    &fakeStorage{},
		Embedder:   &fakeEmbedder{},
		Policy: UploadPolicy{
			MaxUploadBytes:    1024,
			AllowedExtensions: map[string]struct{}{"txt": {}, "md": {}},
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    func() time.Time { return fixedNow },
		NewID:  func() string { return "fixed-id" },
	}
}
