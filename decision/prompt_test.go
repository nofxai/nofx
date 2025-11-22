package decision

import (
	"fmt"
	"strings"
	"testing"
)

// TestBuildSystemPrompt_ContainsAllValidActions 测试 prompt 是否包含所有有效的 action
func TestBuildSystemPrompt_ContainsAllValidActions(t *testing.T) {
	// 这是系统中定义的所有有效 action（来自 validateDecision）
	validActions := []string{
		"open_long",
		"open_short",
		"close_long",
		"close_short",
		"update_stop_loss",
		"update_take_profit",
		"partial_close",
		"hold",
		"wait",
	}

	// 构建 prompt
	prompt := buildSystemPrompt(1000.0, 10, 5, "default")

	// 验证每个有效 action 都在 prompt 中出现
	for _, action := range validActions {
		if !strings.Contains(prompt, action) {
			t.Errorf("Prompt 缺少有效的 action: %s", action)
		}
	}
}

// TestBuildSystemPrompt_ActionListCompleteness 测试 action 列表的完整性
func TestBuildSystemPrompt_ActionListCompleteness(t *testing.T) {
	prompt := buildSystemPrompt(1000.0, 10, 5, "default")

	// 检查是否包含关键的缺失 action
	missingActions := []string{
		"update_stop_loss",
		"update_take_profit",
		"partial_close",
	}

	for _, action := range missingActions {
		if !strings.Contains(prompt, action) {
			t.Errorf("Prompt 缺少关键 action: %s（这会导致 AI 返回无效决策）", action)
		}
	}
}

// TestBuildSystemPrompt_NonExistentTemplate_ShouldCallFatal 测试不存在的模板应触发 fatal
func TestBuildSystemPrompt_NonExistentTemplate_ShouldCallFatal(t *testing.T) {
	// 保存原始 fatalFunc
	originalFatal := fatalFunc
	defer func() { fatalFunc = originalFatal }()

	// 测试环境：替换为 panic
	var fatalCalled bool
	var fatalMessage string
	fatalFunc = func(format string, v ...interface{}) {
		fatalCalled = true
		fatalMessage = fmt.Sprintf(format, v...)
		panic("fatal called") // 用 panic 模拟 os.Exit
	}

	// 捕获 panic
	defer func() {
		if r := recover(); r != nil {
			// 验证 fatal 被调用
			if !fatalCalled {
				t.Error("Expected fatalFunc to be called when template does not exist")
			}
			// 验证错误消息
			if !strings.Contains(fatalMessage, "系统无法启动") {
				t.Errorf("Expected fatal message to contain '系统无法启动', got: %s", fatalMessage)
			}
		} else {
			t.Error("Expected panic from fatalFunc, but did not panic")
		}
	}()

	// 触发致命错误：使用不存在的模板
	buildSystemPrompt(1000.0, 10, 5, "non_existent_template_xyz")
}

// TestRealFatal_Documentation 文档测试：验证生产环境行为
// 注意：此测试不会实际运行，仅用于文档说明
func TestRealFatal_Documentation(t *testing.T) {
	t.Skip("文档测试：realFatal 在生产环境会调用 os.Exit(1)")

	// 📚 生产环境行为说明：
	// 1. fatalFunc 默认值是 realFatal
	// 2. realFatal 内部调用 os.Exit(1)
	// 3. 当模板不存在时，系统会立即退出
	//
	// 资金安全保证：
	// - 配置错误的模板 → buildSystemPrompt 调用
	// - GetPromptTemplate 失败 → fatalFunc 调用
	// - realFatal → log.Printf + os.Exit(1)
	// - 进程退出 → 交易员不启动 → 资金安全 ✅
	//
	// 示例场景：
	// 用户配置了 system_prompt_template = "wrong_strategy"
	// 但 prompts/ 目录中只有 [default, Hansen, nof1]
	// → 系统启动时检测到模板不存在
	// → 日志输出：❌ 致命错误：系统提示词模板 'wrong_strategy' 不存在
	// → 日志输出：📋 当前可用的模板列表: [default Hansen nof1]
	// → os.Exit(1) → 系统退出
	// → 不会用错误的策略进行交易 → 100万资金安全 ✅
}

// TestBuildSystemPromptPartialCloseRequiresSLTP 测试 partial_close 必须要求 new_stop_loss 和 new_take_profit
func TestBuildSystemPromptPartialCloseRequiresSLTP(t *testing.T) {
	prompt := buildSystemPrompt(1000.0, 10, 5, "default")

	// 验证 partial_close 指令中包含 new_stop_loss 和 new_take_profit 的要求
	// 这是 Issue #70 的修复：部分平仓后必须重新设置止损止盈
	if !strings.Contains(prompt, "partial_close") {
		t.Fatal("Prompt 缺少 partial_close action")
	}

	// 检查 partial_close 必填字段中是否包含 new_stop_loss 和 new_take_profit
	if !strings.Contains(prompt, "partial_close") || !strings.Contains(prompt, "new_stop_loss") || !strings.Contains(prompt, "new_take_profit") {
		t.Error("Prompt 中 partial_close 应该要求 new_stop_loss 和 new_take_profit（Issue #70 修复）")
	}

	// 检查是否有警告说明原订单会被取消
	if !strings.Contains(prompt, "部分平仓后原订单会被取消") {
		t.Error("Prompt 应该警告 AI 部分平仓后原订单会被取消")
	}
}

// TestGetPromptTemplate_PathTraversalProtection 测试路径遍历攻击防护
func TestGetPromptTemplate_PathTraversalProtection(t *testing.T) {
	maliciousNames := []string{
		"../etc/passwd",
		"..\\windows\\system32",
		"../../sensitive_file",
		"templates/../../../etc/passwd",
		"normal/../../../secret",
	}

	for _, name := range maliciousNames {
		_, err := GetPromptTemplate(name)
		if err == nil {
			t.Errorf("Expected error for malicious template name '%s', but got nil", name)
		}
		if !strings.Contains(err.Error(), "非法的模板名称") {
			t.Errorf("Expected error message to contain '非法的模板名称' for '%s', got: %v", name, err)
		}
	}
}
