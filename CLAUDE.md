# Claude 开发备忘

- 本项目使用 **docker compose** 部署
- 编译命令：`./start.sh start --build`
- **本地环境地址**：https://trade.llbrother.org

## 开发流程规则

### 编译和测试检查 - MANDATORY ⚠️🔥

**在提交代码前，必须执行以下检查（按顺序）：**

1. **Go 代码编译检查**（如果修改了 Go 代码）：
   ```bash
   go build .
   ```
   - ✅ 必须编译通过，无任何错误
   - ✅ 检查接口定义：如果添加了新方法，确保接口已更新
   - ❌ 编译失败 = BLOCKING，必须修复

2. **Go 单元测试**（如果修改了 Go 代码）：
   ```bash
   go test ./...
   ```
   - ✅ 所有测试必须通过
   - ❌ 任何测试失败 = BLOCKING，必须修复

3. **Frontend 编译检查**（如果修改了 TypeScript/React 代码）：
   ```bash
   cd web && npm run build
   ```
   - ✅ 必须编译通过，无 TypeScript 类型错误
   - ❌ 编译失败 = BLOCKING，必须修复

4. **Frontend 单元测试**（如果修改了 TypeScript/React 代码）：
   ```bash
   cd web && npm test -- --run
   ```
   - ✅ 所有测试必须通过
   - ❌ 任何测试失败 = BLOCKING，必须修复

**⚠️ 血的教训（2024-11-19）**：
- 修改 Go 代码但未运行 `go build`，导致 Docker 构建失败
- 在实现类中添加了方法，但忘记更新接口定义
- **绝对不允许**提交无法编译的代码！

### TDD（测试驱动开发）- MANDATORY

**所有开发和 bug 修复，默认使用 TDD 方式：**

1. **新功能开发**：
   - ✅ **先写测试**：用测试用例明确需求和预期行为
   - ✅ **运行测试**：确认测试失败（Red）
   - ✅ **实现代码**：让测试通过（Green）
   - ✅ **验证编译**：运行 `go build .` 或 `npm run build` 确保编译通过
   - ✅ **运行所有测试**：确保不影响其他功能
   - ✅ **重构优化**：在测试保护下重构（Refactor）

2. **Bug 修复**：
   - ✅ **先写测试**：复现 bug 的测试用例
   - ✅ **确认失败**：测试应该失败，证明 bug 存在
   - ✅ **修复代码**：让测试通过
   - ✅ **验证编译**：运行编译检查
   - ✅ **验证修复**：确保测试通过且不影响其他测试

3. **测试覆盖率要求**：
   - 核心业务逻辑：≥ 90%
   - 工具函数：≥ 80%
   - UI 组件：重要交互逻辑需要测试

4. **例外情况**（无需 TDD）：
   - 纯 UI 样式调整
   - 配置文件修改
   - 文档更新

## 代码编辑规则

- **只改需要改的行**，不要为了对齐而修改其他行
- 冒号后面不对齐不会有任何问题，语言不关心格式对齐
- 前端的所有显示 label, wording 这些，都要做多语言支持，多语言在 `web/src/i18n/translations.ts` 中，一定不能直接在 tsx 文件中写 raw string
- Never run ./start.sh, 用户会自己在 terminal 运行这个命令

## 数据库操作规则 - HARD RULE

**❌ 严格禁止在没有用户显式允许的情况下对数据库进行任何修改操作：**

- **禁止** UPDATE 操作
- **禁止** DELETE 操作
- **禁止** INSERT 操作
- **只允许** SELECT 查询操作用于诊断和调试

**✅ 如果需要修改数据库，必须：**

1. 先向用户说明需要执行的操作和原因
2. 等待用户明确同意
3. 得到同意后再执行操作

## Git 工作流规则 - MANDATORY

**本项目使用 feature branch 工作流，所有 PR 必须提交到 `next` 分支：**

### PR 提交规则
1. **创建 feature 分支**：从 `next` 分支创建 feature 分支
   ```bash
   git checkout next
   git checkout -b feature/your-feature-name
   # 或 fix/issue-number-description
   ```

2. **提交 PR 到 next**：
   - ✅ **正确**：`feature/xxx` → `next`
   - ❌ **错误**：`feature/xxx` → `main`
   - ❌ **错误**：`next` → `main`

3. **分支命名规范**：
   - 新功能：`feature/description` 或 `feat/description`
   - Bug 修复：`fix/issue-number-description`
   - 重构：`refactor/description`
   - 文档：`docs/description`

4. **Commit Message 规范**：
   - 使用 Conventional Commits 格式
   - 格式：`<type>(<scope>): <description>`
   - 例如：`fix(logger): recover cache on restart`
   - 关联 Issue：在 commit message 中添加 `Fixes #123` 或 `Closes #123`

5. **PR 描述要求**：
   - 必须包含问题描述（Problem）
   - 必须包含解决方案（Solution）
   - 必须包含修改清单（Changes）
   - 必须包含测试说明（Testing）
   - 必须关联相关 Issue（Closes #xxx）

### 分支管理
- **main**: 生产环境分支，只接受从 `next` 合并的稳定版本
- **next**: 开发分支，所有 feature/fix PR 都提交到这里
- **feature/fix 分支**: 从 `next` 创建，完成后提交 PR 到 `next`

### 重要提醒
⚠️ **永远不要直接提交 PR 到 `main` 分支！**
- 除非用户明确要求，否则所有 PR 都应该提交到 `next`
- `main` 分支只接受经过测试和验证的稳定版本

## Git 操作安全规则 - CRITICAL ⚠️🔥

**❌ 绝对禁止的危险操作 - 除非用户明确授权：**

1. **`git reset --hard`** - 会永久丢失未提交的修改！
2. **`git clean -fd`** - 会永久删除未跟踪的文件！
3. **`git checkout -- .`** - 会丢弃所有工作区修改！
4. **`git stash drop`** - 会永久删除 stash 的内容！
5. **切换分支前未保存工作** - 可能导致修改丢失！

**✅ 正确的工作流程：**

### 查看修改状态
```bash
git status                    # 查看当前修改
git diff                      # 查看未暂存的修改
git diff --cached             # 查看已暂存的修改
```

### 保存工作进度
```bash
# 方法 1: 提交到当前分支
git add <files>
git commit -m "WIP: description"

# 方法 2: 使用 stash（但要注意 stash 也可能丢失）
git stash push -m "description"
git stash list                # 确认 stash 成功
```

### 切换分支的安全流程
```bash
# 步骤 1: 检查当前修改
git status

# 步骤 2a: 如果有修改要保留 → 先提交或 stash
git add <files>
git commit -m "WIP: save work before switching"

# 步骤 2b: 如果修改不重要 → 询问用户确认后再丢弃
# ❌ 绝不自动执行 git reset --hard

# 步骤 3: 切换分支
git checkout <branch>
```

### 提交部分文件的正确方式
```bash
# ✅ 正确：只 add 需要提交的文件
git add file1.ts file2.ts
git commit -m "message"

# ❌ 错误：不要为了"清理工作区"而 reset
# 这会丢失其他未提交的修改！
```

### 血的教训（2024-11-19）

**事件：** 在修复 issue #42 时，因为想从 next 分支创建新分支，执行了 `git reset --hard HEAD`，导致：
- ❌ 丢失了过滤功能的前端修改（api.ts, TraderDashboard.tsx）
- ❌ 丢失了过滤功能的后端修改（api/server.go, logger/decision_logger.go）
- ❌ 丢失了 docker-compose.yml 的修改
- ❌ 这些修改都没有 commit，永久丢失，需要重新实现

**正确做法应该是：**
1. 在当前分支直接 `git add` 要提交的文件
2. 提交后再考虑是否需要切换分支
3. 或者询问用户如何处理其他未提交的修改
4. **绝不擅自执行任何可能丢失数据的命令**

**核心原则：**
- 🔴 **宁可多问，绝不擅自重置或删除**
- 🔴 **保护用户的工作成果是第一优先级**
- 🔴 **任何破坏性操作都必须得到用户明确授权**

## 决策规则

- **直接推荐最佳方案**，不要抛出选择题让用户做决策
- 如果有多个方案，直接选择你认为最好的方案并说明理由
- 只有在真正需要用户决策的关键分歧点才询问（例如：架构方向、业务逻辑选择）
- 技术实现细节的选择应该由你直接决定并执行