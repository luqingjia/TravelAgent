// Package application 编排知识文档的上传、处理、查询和删除用例。
//
// 这个包可以依赖 domain，但不能导入 Gin、sqlx、AWS SDK 或具体数据库实现。
// 所有外部能力都通过 ports.go 中由使用方定义的小接口传入。
package application

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"
)

// UploadPolicy 是应用层执行上传校验所需的稳定策略。
// 配置包负责从环境变量读取原始值，应用层只接收已经明确的字节上限和扩展名集合。
type UploadPolicy struct {
	MaxUploadBytes    int64
	AllowedExtensions map[string]struct{}
}

// AllowsExtension 判断去掉点号后的扩展名是否在允许集合内。
func (p UploadPolicy) AllowsExtension(extension string) bool {
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
	_, allowed := p.AllowedExtensions[normalized]
	return allowed
}

// Dependencies 列出 Service 构造时需要的全部长期依赖。
// 组合根用真实实现填充它，测试用 fake 和固定函数填充它；Gin Context 不参与依赖保存。
type Dependencies struct {
	Repository DocumentRepository
	Storage    ObjectStorage
	Embedder   Embedder
	Policy     UploadPolicy
	Logger     *slog.Logger
	Now        func() time.Time
	NewID      func() string
}

// Service 是知识文档应用用例的入口。
// 它只保存接口和纯函数，因此构造完成后可以被 HTTP Handler 复用，也可以在单元测试中直接调用。
type Service struct {
	repo     DocumentRepository
	storage  ObjectStorage
	embedder Embedder
	policy   UploadPolicy
	logger   *slog.Logger
	now      func() time.Time
	newID    func() string
}

// NewService 校验并保存应用服务需要的依赖。
// 构造阶段失败比处理请求时 panic 更安全，也能让启动日志明确指出究竟缺少哪一项。
func NewService(deps Dependencies) (*Service, error) {
	// 接口可能装着“有类型的 nil 指针”，此时 interface 本身不等于 nil。
	// isNilDependency 同时识别普通 nil 和这种 Go 初学者很容易踩到的情况。
	if isNilDependency(deps.Repository) {
		return nil, fmt.Errorf("repository is required")
	}
	if isNilDependency(deps.Storage) {
		return nil, fmt.Errorf("object storage is required")
	}
	if isNilDependency(deps.Embedder) {
		return nil, fmt.Errorf("embedder is required")
	}
	if deps.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	policy, err := normalizeUploadPolicy(deps.Policy)
	if err != nil {
		return nil, err
	}

	// 时钟和 ID 生成器也是依赖。生产环境可使用默认实现，测试传入固定函数后就不会依赖真实时间和随机数。
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	newID := deps.NewID
	if newID == nil {
		newID = defaultNewID
	}

	return &Service{
		repo:     deps.Repository,
		storage:  deps.Storage,
		embedder: deps.Embedder,
		policy:   policy,
		logger:   deps.Logger,
		now:      now,
		newID:    newID,
	}, nil
}

// normalizeUploadPolicy 校验上传策略，并把扩展名统一成小写、无点号的形式。
func normalizeUploadPolicy(policy UploadPolicy) (UploadPolicy, error) {
	if policy.MaxUploadBytes <= 0 {
		return UploadPolicy{}, fmt.Errorf("upload max bytes must be positive")
	}
	if len(policy.AllowedExtensions) == 0 {
		return UploadPolicy{}, fmt.Errorf("at least one upload extension is required")
	}

	normalized := make(map[string]struct{}, len(policy.AllowedExtensions))
	for extension := range policy.AllowedExtensions {
		value := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
		if value == "" {
			return UploadPolicy{}, fmt.Errorf("upload extension must not be empty")
		}
		normalized[value] = struct{}{}
	}
	policy.AllowedExtensions = normalized
	return policy, nil
}

// isNilDependency 使用反射检查接口内部是否保存了 nil 指针。
// 这里只在进程启动构造依赖时运行一次，不在请求热路径中使用，因此可读性和安全性比微小性能差异更重要。
func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// defaultNewID 生成不依赖数据库自增序列的十六进制标识。
// 系统随机源极少失败；如果失败则退回纳秒时间戳，保证函数合同仍能返回非空值。
func defaultNewID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
