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
	// MaxUploadBytes 是读取上传流时允许的最大真实字节数。
	MaxUploadBytes int64
	// AllowedExtensions 使用小写、无点号扩展名作为集合键，查询时间复杂度稳定为 O(1)。
	AllowedExtensions map[string]struct{}
}

// AllowsExtension 判断去掉点号后的扩展名是否在允许集合内。
func (p UploadPolicy) AllowsExtension(extension string) bool {
	// HTTP 文件名可能包含大写或前导点号，先统一成策略集合使用的格式。
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
	// 读取 map 的第二个返回值即可判断键是否存在，不需要给集合元素保存额外布尔值。
	_, allowed := p.AllowedExtensions[normalized]
	// true 表示上传用例可以继续读取文件，false 表示应在外部调用前拒绝。
	return allowed
}

// Dependencies 列出 Service 构造时需要的全部长期依赖。
// 组合根用真实实现填充它，测试用 fake 和固定函数填充它；Gin Context 不参与依赖保存。
type Dependencies struct {
	// Repository 提供文档和分块持久化能力。
	Repository DocumentRepository
	// Storage 提供原始文件的写入、读取和补偿删除能力。
	Storage ObjectStorage
	// Embedder 把分块文本转换成向量。
	Embedder Embedder
	// Policy 保存上传大小和扩展名约束。
	Policy UploadPolicy
	// Logger 记录补偿失败等不能覆盖主错误的附加信息。
	Logger *slog.Logger
	// Now 可由测试替换为固定时钟，生产为空时使用 time.Now。
	Now func() time.Time
	// NewID 可由测试替换为确定性 ID 生成器。
	NewID func() string
}

// Service 是知识文档应用用例的入口。
// 它只保存接口和纯函数，因此构造完成后可以被 HTTP Handler 复用，也可以在单元测试中直接调用。
type Service struct {
	// repo 是构造完成后不可替换的仓储接口。
	repo DocumentRepository
	// storage 是当前配置选中的 LocalStorage 或 S3Storage。
	storage ObjectStorage
	// embedder 是 OpenAI 兼容 Embedding 客户端接口。
	embedder Embedder
	// policy 是已经完成归一化和校验的上传策略副本。
	policy UploadPolicy
	// logger 用于记录不应覆盖主错误的附加失败。
	logger *slog.Logger
	// now 提供业务时间，避免用例直接绑定真实系统时钟。
	now func() time.Time
	// newID 为文档和分块生成唯一标识。
	newID func() string
}

// NewService 校验并保存应用服务需要的依赖。
// 构造阶段失败比处理请求时 panic 更安全，也能让启动日志明确指出究竟缺少哪一项。
func NewService(deps Dependencies) (*Service, error) {
	// 接口可能装着“有类型的 nil 指针”，此时 interface 本身不等于 nil。
	// isNilDependency 同时识别普通 nil 和这种 Go 初学者很容易踩到的情况。
	if isNilDependency(deps.Repository) {
		return nil, fmt.Errorf("repository is required")
	}
	// 对象存储缺失时上传和处理流程都无法工作，所以构造阶段立即失败。
	if isNilDependency(deps.Storage) {
		return nil, fmt.Errorf("object storage is required")
	}
	// Embedder 缺失时文档永远无法进入 completed 状态。
	if isNilDependency(deps.Embedder) {
		return nil, fmt.Errorf("embedder is required")
	}
	// 日志器用于记录补偿和失败回写异常，不能等到故障发生时再因为 nil 触发 panic。
	if deps.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// 策略归一化会复制扩展名集合，防止配置对象在服务构造后继续影响运行行为。
	policy, err := normalizeUploadPolicy(deps.Policy)
	// 非法大小或扩展名应作为启动配置错误原样返回。
	if err != nil {
		return nil, err
	}

	// 时钟和 ID 生成器也是依赖。生产环境可使用默认实现，测试传入固定函数后就不会依赖真实时间和随机数。
	now := deps.Now
	// 生产组合根通常不传时钟，此时选择标准库当前时间。
	if now == nil {
		now = time.Now
	}
	newID := deps.NewID
	// 生产组合根不传 ID 生成器时使用安全随机默认实现。
	if newID == nil {
		newID = defaultNewID
	}

	// 所有依赖均合法后一次性构造 Service，后续请求只读取这些稳定字段。
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
	// 非正数上限会让限长读取逻辑失去正确边界。
	if policy.MaxUploadBytes <= 0 {
		return UploadPolicy{}, fmt.Errorf("upload max bytes must be positive")
	}
	// 没有任何允许扩展名意味着所有上传都会失败，通常属于配置错误。
	if len(policy.AllowedExtensions) == 0 {
		return UploadPolicy{}, fmt.Errorf("at least one upload extension is required")
	}

	// 创建新集合而不是原地修改输入，避免配置层与应用层共享 map。
	normalized := make(map[string]struct{}, len(policy.AllowedExtensions))
	// 每个配置项都统一去空格、转小写并去掉前导点号。
	for extension := range policy.AllowedExtensions {
		value := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
		// 空字符串无法表示真实文件类型，发现后立即指出策略配置错误。
		if value == "" {
			return UploadPolicy{}, fmt.Errorf("upload extension must not be empty")
		}
		// 空结构体不占用额外数据，只利用 map 键表达集合成员。
		normalized[value] = struct{}{}
	}
	// 用规范化副本替换返回值中的集合，原输入仍保持不变。
	policy.AllowedExtensions = normalized
	// 返回可直接用于请求热路径的稳定策略。
	return policy, nil
}

// isNilDependency 使用反射检查接口内部是否保存了 nil 指针。
// 这里只在进程启动构造依赖时运行一次，不在请求热路径中使用，因此可读性和安全性比微小性能差异更重要。
func isNilDependency(value any) bool {
	// 普通 nil 接口可以直接识别，不需要反射。
	if value == nil {
		return true
	}
	// ValueOf 暴露接口内部保存的真实动态值和类型。
	reflected := reflect.ValueOf(value)
	// 只有这些可为 nil 的种类才能安全调用 IsNil，其他种类调用会 panic。
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		// 有类型的 nil 指针会在这里返回 true。
		return reflected.IsNil()
	default:
		// 普通结构体、数字和字符串等值类型不可能是 nil。
		return false
	}
}

// defaultNewID 生成不依赖数据库自增序列的十六进制标识。
// 系统随机源极少失败；如果失败则退回纳秒时间戳，保证函数合同仍能返回非空值。
func defaultNewID() string {
	// 16 字节随机数提供 128 位空间，碰撞概率足够低，可脱离数据库生成。
	var random [16]byte
	// 正常情况使用系统密码学随机源。
	if _, err := rand.Read(random[:]); err == nil {
		// 十六进制编码得到只含安全 ASCII 字符的稳定 ID。
		return hex.EncodeToString(random[:])
	}
	// 极端随机源故障时用纳秒时间保证返回非空，避免整个服务因生成 ID 直接崩溃。
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
