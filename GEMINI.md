# Gemini Slash Commands

This document outlines the available slash commands for Gemini. For additional commands and context, please refer to the original `CLAUDE.md` file.

## Gemini-specific Commands

### `/code-review`

# 代码审查命令

## 审查任务

请对当前工作区的代码修改进行全面审查，重点关注业务逻辑的正确性和技术架构的合理性。

## 审查维度

### 1. 业务层面审查（BLOCKING级别）
- **需求来源验证**：寻找并验证业务需求来源（不能基于代码反推需求）
- **需求实现完整性**：验证代码是否100%满足业务需求
- **业务流程逻辑正确性**：检查状态转换、数据流、条件判断的逻辑
- **数据结构正确性**：验证模型定义、字段约束、业务规则映射
- **Edge Case处理**：评估边界条件和异常场景的处理合理性

### 2. 技术层面审查
- **架构合理性**：模块化、职责分离、依赖关系
- **KISS原则遵循**：避免过度工程化
- **扩展性评估**：未来功能添加的容易程度
- **非Adhoc修改验证**：是否遵循现有代码模式
- **性能问题检测**：查找明显的性能问题（如N+1查询等）
- **单元测试完备性**：核心逻辑90%覆盖率要求

### 3. 契约与连通性专项检查（BLOCKING）
- 端点一致性：前端端点集中配置；路径/动态段/大小写/末尾分隔符与后端路由完全一致；HTTP 方法语义匹配（幂等/副作用）。
- 认证与跨域：统一的认证机制（会话/令牌）；前端传递方式与后端期望一致（Cookie/Authorization 等）；跨域与 CSRF 策略匹配。
- 请求/响应 Schema：字段名、类型、必选/可选一致；时间/数值/布尔的编码一致；分页/排序参数与响应元信息对齐。
- 错误与状态码：4xx/5xx 使用合理；错误负载结构稳定且可解析；前端根据错误类型提供可恢复提示。
- 异步/事件（如有）：事件类型与载荷字段与文档一致；开始/结束/错误/心跳等语义完整；增量合并无丢失/重复；存在降级路径。
- 端到端透传：API→Service→下游（存储/第三方）参数（上下文/会话/区域/幂等键）无遗漏；写操作具备幂等/去重策略。
- 日志与隐私：日志含必要上下文（追踪/用户/会话），敏感信息脱敏；UI 默认不展示内部实现细节。

#### 全链路 Contract Crosswalk（必做）
- 选取关键用户流，建立“参数对齐矩阵”：
  - 前端请求 → 后端路由/处理 → 服务层 → 下游（存储/第三方）→ 持久化字段。
  - 列出每个参数：`name | type | required | default | source-of-truth(文件:行)`。
  - 如存在事件/流式契约，补充事件类型与载荷字段，以及必要的状态回写。
- 输出差异点与最小修复建议（谁改、改哪、如何不破坏现有行为）。

### 4. 高发问题清单（优先排查）
- 命名错位：不同层对同一概念使用了不同命名风格或名称（例如 `camelCase` ↔ `snake_case`、标识符命名不一致）。
- 路径错位：端点路径或动态段命名不一致；末尾分隔符导致重定向或方法失败。
- 类型漂移：数字/字符串/布尔/时间类型在层间编码不一致或未正确转换。
- 认证混用：请求在不同处使用了不同认证方式；跨域请求未按需携带凭证；CSRF 保护缺失。
- 事件缺口：后端新增事件类型或字段未在前端解析；增量合并逻辑导致重复/丢失。
- 可见性/状态：后端状态位或可见性字段被忽略，导致 UI 展示与真实状态不一致。

### 5. 零假设验证（VERIFY-FIRST Gate，BLOCKING）
- 禁止基于“猜测的字段/API/事件”写逻辑。每个关键元素必须给出证据：
  - 模型/字段定义位置（文件:行）
  - 路由/处理方法签名（文件:行）
  - 前端类型/解析/调用代码（文件:行）
- 无证据的假设一律不通过。

### 6. 用户视角 E2E 可见性审计（BLOCKING）
⚠️ **这是最关键的检查** - "代码完美但功能不工作"的主要原因就是跳过了这一步！

**MANDATORY USER FLOW VERIFICATION (必须执行):**
- **完整点击路径追踪**：从用户点击开始，逐步追踪到最终状态
  - 用户点击X → 调用函数Y → 导航到页面Z → 显示内容W
  - **必须验证每个步骤都正确执行**
- **URL路由验证**：所有导航路径在路由配置中存在且正确处理参数
- **状态传递验证**：点击后的状态变化是否正确反映在UI中
- **错误场景测试**：参数缺失、网络错误、权限不足等场景的处理

**具体检查项目:**
- 入口可见：对应功能的入口（按钮/导航/控件）在默认场景与目标设备上可达；不被误判条件隐藏。
- **链接落地（核心）**：页面路由/回跳/深链接一致；从入口到完成形成闭环。
  - 点击通知 → 是否真的跳转到预期页面？
  - 分享链接 → 是否真的加载预期内容？
  - 所有导航路径都必须实际追踪验证！
- 状态完整：加载/空数据/错误/权限不足 均有清晰呈现与可恢复路径。
- 角色/开关：与权限/Feature Flag 的可见性符合预期；默认值不阻断主流程。

**⛔ 禁止行为:**
- ❌ 只看代码结构，不追踪实际执行流程
- ❌ 假设navigate()调用就等于用户到达了目标页面
- ❌ 不验证URL参数处理逻辑
- ❌ 说"看起来正确"而不验证"实际正确"

### 7. Web3 AI 交易系统安全审查（BLOCKING 级别）
🔐 **资金安全是生死线** - 一个安全漏洞可能导致所有资金损失！

#### 7.1 私钥与密钥管理（CRITICAL）
- **零泄露原则**：
  - ❌ 禁止：私钥/助记词出现在日志、错误消息、前端代码、Git 历史中
  - ❌ 禁止：明文存储私钥（环境变量、配置文件、数据库）
  - ✅ 必需：使用硬件钱包、HSM、或加密密钥管理服务（AWS KMS/Vault）
  - ✅ 必需：API Key、密钥材料必须加密存储，运行时解密
- **最小权限原则**：
  - 交易签名密钥与只读查询密钥分离
  - 每个功能使用独立的子账户/权限
  - 定期轮换 API Key 和访问令牌
- **验证检查**：
  - [ ] grep 搜索 `private_key`、`mnemonic`、`seed` 等关键词，确保无硬编码
  - [ ] 检查所有密钥存储位置的加密状态
  - [ ] 验证密钥访问日志和审计追踪

#### 7.2 交易安全（CRITICAL）
- **签名验证**：
  - ✅ 必需：所有交易必须经过签名验证
  - ✅ 必需：验证交易发起者身份（防止伪造）
  - ✅ 必需：使用 nonce/序列号防止重放攻击
- **交易参数验证**：
  - ✅ 必需：验证接收地址合法性（checksum、白名单）
  - ✅ 必需：金额/价格/滑点限制（防止异常大额交易）
  - ✅ 必需：Gas Price/Gas Limit 上限保护（防止 Gas 耗尽攻击）
  - ✅ 必需：Deadline/超时保护（防止过期交易执行）
- **滑点与价格保护**：
  - ✅ 必需：设置合理的滑点容忍度（如 0.5%-2%）
  - ✅ 必需：价格预言机验证（多源对比、时间戳检查）
  - ✅ 必需：异常价格波动拒绝交易
- **验证检查**：
  - [ ] 所有交易调用都有 nonce 或幂等键
  - [ ] 金额/价格参数都有上下限验证
  - [ ] Gas 费用有最大限制
  - [ ] 滑点保护代码存在且正确

#### 7.3 AI 决策安全（CRITICAL）
- **提示注入防护**：
  - ❌ 禁止：直接将用户输入拼接到 AI prompt 中
  - ✅ 必需：用户输入消毒/转义（防止 prompt injection）
  - ✅ 必需：系统提示与用户输入明确分离（使用角色隔离）
  - ✅ 必需：敏感操作需要用户明确确认，AI 不能自主决定大额交易
- **决策审计**：
  - ✅ 必需：记录所有 AI 决策的完整上下文（输入、输出、时间戳、模型版本）
  - ✅ 必需：决策可追溯、可回放、可审计
  - ✅ 必需：异常决策告警（如突然的大额交易建议）
- **模型安全**：
  - ✅ 必需：使用官方 API，避免第三方代理（防止中间人攻击）
  - ✅ 必需：API 响应验证（检测异常输出、格式错误）
  - ✅ 必需：模型输出不直接执行，必须经过参数验证
- **验证检查**：
  - [ ] 搜索用户输入拼接点，确保有消毒处理
  - [ ] 检查决策日志是否完整（包含所有关键参数）
  - [ ] 验证大额交易需要额外确认机制

#### 7.4 智能合约交互安全（CRITICAL）
- **授权范围控制**：
  - ❌ 禁止：无限授权（`approve(spender, type(uint256).max)`）
  - ✅ 必需：按需授权，每次交易前计算精确授权额度
  - ✅ 必需：定期清理过期授权
  - ✅ 必需：监控授权事件，异常授权告警
- **合约调用验证**：
  - ✅ 必需：合约地址白名单（只与已审计合约交互）
  - ✅ 必需：函数选择器验证（防止调用错误函数）
  - ✅ 必需：调用参数类型/范围验证
  - ✅ 必需：模拟执行（dry-run）后再真实执行
- **重入与异常处理**：
  - ✅ 必需：处理合约调用失败情况（revert、out of gas）
  - ✅ 必需：检查返回值，不假设调用成功
  - ✅ 必需：避免在外部调用后修改关键状态（防重入）
- **验证检查**：
  - [ ] grep `approve` 确保无无限授权
  - [ ] 所有合约地址来自配置/白名单，无硬编码
  - [ ] 调用失败有完整的错误处理和回退逻辑

#### 7.5 资金保护机制（BLOCKING）
- **限额控制**：
  - ✅ 必需：单笔交易金额上限（如 $1000）
  - ✅ 必需：日/周/月累计限额
  - ✅ 必需：异常交易频率限制（防止快速耗尽资金）
  - ✅ 必需：大额交易需要多重签名或延迟执行
- **紧急暂停**：
  - ✅ 必需：全局紧急停止按钮（kill switch）
  - ✅ 必需：异常检测自动暂停（如价格异常、Gas 费暴涨）
  - ✅ 必需：暂停后资金安全提取机制
- **余额监控**：
  - ✅ 必需：实时余额监控，低于阈值告警
  - ✅ 必需：异常资金流出告警（大额转出、未知接收方）
  - ✅ 必需：定期对账（链上余额 vs 系统记录）
- **验证检查**：
  - [ ] 限额配置存在且合理
  - [ ] 紧急暂停功能可测试且有权限控制
  - [ ] 余额监控代码存在且接入告警系统

#### 7.6 链上数据验证（CRITICAL）
- **预言机安全**：
  - ❌ 禁止：单一数据源（可被操纵）
  - ✅ 必需：多预言机对比（Chainlink、Band、UMA 等）
  - ✅ 必需：价格偏差检测（多源价格差异超阈值拒绝）
  - ✅ 必需：时间戳验证（数据新鲜度检查，拒绝过期数据）
- **区块确认**：
  - ✅ 必需：等待足够的区块确认（主网建议 ≥12 块，L2 根据实际情况）
  - ✅ 必需：处理链重组可能（pending → confirmed → finalized）
  - ✅ 必需：交易回执验证（status=1 成功）
- **数据完整性**：
  - ✅ 必需：事件日志完整性检查（topic、参数匹配）
  - ✅ 必需：合约状态一致性验证（链上 vs 本地缓存）
  - ✅ 必需：MEV 保护（使用私有内存池或 Flashbots）
- **验证检查**：
  - [ ] 价格数据来自多个预言机
  - [ ] 区块确认数配置合理
  - [ ] 交易状态检查包含 finalized 状态

### 8. 交易生命周期完整性审查（MANDATORY for Trading Systems）
🎯 **这是交易系统最关键的审查** - 交易生命周期任何环节出错都可能导致资金损失或数据不一致！

#### 8.1 交易生命周期概览
```
周期开始 → 账户检查 → 持仓同步 → 被动平仓检测 → AI决策 → 决策执行 → 成交验证 → 状态更新 → 日志持久化 → 周期结束
```

#### 8.2 阶段 1：开仓前检查（Pre-Trade Validation）
**BLOCKING级别** - 开仓前必须验证所有前置条件

| 检查项 | 验证要点 | BLOCKING条件 |
|--------|---------|-------------|
| **防重复开仓** | 检查是否已有同币种同方向持仓 | 未检查或检查不正确 |
| **保证金充足** | 计算所需保证金 + 手续费，验证可用余额 | 未验证或计算错误 |
| **价格有效性** | 验证市场价格数据可用且新鲜 | 使用过期或无效价格 |
| **数量计算** | `quantity = PositionSizeUSD / Price` 正确计算 | 计算公式错误 |
| **杠杆验证** | 验证杠杆倍数在交易所允许范围内 | 超出限制或未验证 |

**验证清单**：
- [ ] 存在防止仓位叠加的检查逻辑
- [ ] 保证金验证包含手续费估算
- [ ] 保证金不足时拒绝开仓
- [ ] 数量计算公式正确
- [ ] 价格来源可靠（非硬编码或过期数据）

#### 8.3 阶段 2：开仓执行（Trade Execution）
**BLOCKING级别** - 执行流程必须完整且顺序正确

| 步骤 | 操作 | BLOCKING条件 |
|------|------|-------------|
| 1. 记录时间 | **在开仓前**记录 `openTime = time.Now()` | 时间记录位置错误 |
| 2. 执行开仓 | 调用交易所API：`OpenLong/OpenShort` | API调用失败未处理 |
| 3. 记录订单 | 提取并保存 `orderId` | 订单ID未记录 |
| 4. 设置止损 | `SetStopLoss(symbol, side, quantity, stopLoss)` | 止损未设置或失败未处理 |
| 5. 设置止盈 | `SetTakeProfit(symbol, side, quantity, takeProfit)` | 止盈未设置或失败未处理 |
| 6. 成交验证 | 查询真实成交价格并更新记录（见8.6） | 未验证或使用预估价 |
| 7. 风险验证 | 基于真实成交价计算实际风险 | 风险超限未处理 |
| 8. 状态记录 | 更新内存状态：`positionFirstSeenTime`, `positionStopLoss`, `positionTakeProfit` | 状态未记录或不一致 |

**关键时序要求**：
```
⚠️ 必须在开仓前记录时间：
✅ CORRECT:   openTime = now(); OpenLong();
❌ INCORRECT: OpenLong(); openTime = now();

原因：时间用于查询成交记录，必须覆盖交易前后
```

**验证清单**：
- [ ] `openTime/closeTime` 在交易执行前记录
- [ ] 止损/止盈设置在开仓后立即执行
- [ ] 止损/止盈失败有日志但不阻断流程
- [ ] 成交价格验证函数被调用
- [ ] 内存状态（positionFirstSeenTime等）被正确更新

#### 8.4 阶段 3：持仓期间（Position Maintenance）
**BLOCKING级别** - 持仓状态必须与交易所同步

| 操作 | 验证要点 | BLOCKING条件 |
|------|---------|-------------|
| **持仓同步** | 每个周期从交易所获取最新持仓 | 未同步或同步失败 |
| **状态清理** | 删除不存在于交易所的持仓内存记录 | 内存泄漏（已平仓但内存未清理） |
| **止损/止盈读取** | 从内存读取止损/止盈价格补充到持仓信息 | 止损/止盈数据丢失 |
| **被动平仓检测** | 检测止损/止盈/强平触发的被动平仓 | 被动平仓未检测到 |
| **止损/止盈调整** | 验证新价格合理性（多单止损<当前价，空单止损>当前价） | 价格合理性未验证 |

**内存状态清理验证**：
```go
// ✅ 必须清理已平仓的内存记录
for key := range positionFirstSeenTime {
    if !currentPositionKeys[key] {
        delete(positionFirstSeenTime[key])
        delete(positionStopLoss[key])
        delete(positionTakeProfit[key])
    }
}
```

**验证清单**：
- [ ] 每个周期调用 `GetPositions()` 同步交易所状态
- [ ] 存在内存状态清理逻辑（删除已平仓记录）
- [ ] 被动平仓检测逻辑存在且正确
- [ ] 被动平仓也有成交价验证（见8.6）
- [ ] 止损/止盈调整有价格合理性验证

#### 8.5 阶段 4：平仓执行（Close Position）
**BLOCKING级别** - 平仓流程必须完整且准确记录

| 类型 | 验证要点 | BLOCKING条件 |
|------|---------|-------------|
| **主动平仓** | `close_long/close_short` 全部平仓 | 未执行或执行失败 |
| **部分平仓** | `partial_close` 按百分比平仓 | 百分比计算错误 |
| **被动平仓** | 止损/止盈触发的自动平仓 | 未检测或未记录 |

**部分平仓特殊检查**：
| 检查项 | 验证要点 | BLOCKING条件 |
|--------|---------|-------------|
| **百分比验证** | `0 < percentage <= 100` | 未验证或范围错误 |
| **最小仓位检查** | 剩余价值 < $10 时自动全部平仓 | 可能产生无法平仓的小额剩余 |
| **止损/止盈恢复** | 部分平仓后为剩余仓位重新设置止损/止盈 | 剩余仓位裸奔（无保护） |

**验证清单**：
- [ ] 所有平仓类型（主动/部分/被动）都有完整流程
- [ ] 平仓时间在执行前记录（`closeTime = now(); CloseLong();`）
- [ ] 部分平仓有最小仓位检查（防止小额剩余）
- [ ] 部分平仓后恢复止损/止盈
- [ ] 所有平仓都调用成交价验证（见8.6）

#### 8.6 阶段 5：成交价格验证（Fill Price Verification）⭐ 核心
**BLOCKING级别** - 成交价格必须 100% 准确，不能使用预估价

**开仓成交价验证流程**：
```
1. 记录开仓时间 openTime（在交易前）
2. 执行开仓 OpenLong/OpenShort
3. 查询成交记录 GetRecentFills(symbol, openTime±10s)
4. 过滤匹配方向（open_long→Buy, open_short→Sell）
5. 计算加权平均价 weightedAvgPrice = Σ(price×qty) / Σ(qty)
6. 更新记录 actionRecord.Price = weightedAvgPrice
7. 基于真实成交价验证风险
```

**平仓成交价验证流程**：
```
1. 记录平仓时间 closeTime（在交易前）
2. 执行平仓 CloseLong/CloseShort/PartialClose
3. 查询成交记录 GetRecentFills(symbol, closeTime±10s)
4. 过滤匹配方向（close_long→Sell, close_short→Buy）
5. 计算加权平均价
6. 更新记录 actionRecord.Price = weightedAvgPrice
```

**关键验证点**：

| 验证项 | 要求 | BLOCKING条件 |
|--------|------|-------------|
| **调用时机** | 在交易执行后立即调用 | 未调用或调用时机错误 |
| **时间窗口** | `tradeTime ± 10秒` 查询成交记录 | 时间窗口不合理 |
| **方向匹配** | 正确过滤成交方向 | 方向匹配错误 |
| **加权平均** | `Σ(price×quantity) / Σ(quantity)` | 计算公式错误 |
| **重试机制** | 3次重试，每次延迟500ms | 无重试或延迟不合理 |
| **降级策略** | 无成交记录时保持原价格，不报错 | 无降级或直接报错阻断流程 |
| **覆盖范围** | 所有开仓/平仓/部分平仓/被动平仓都有验证 | 任何场景遗漏 |

**方向匹配规则（必须正确）**：
```
开仓：
- open_long  → Buy  (买入开多)
- open_short → Sell (卖出开空)

平仓：
- close_long       → Sell (卖出平多)
- close_short      → Buy  (买入平空)
- auto_close_long  → Sell
- auto_close_short → Buy
```

**验证清单**：
- [ ] `verifyAndUpdateActualFillPrice` 用于开仓验证
- [ ] `verifyAndUpdateCloseFillPrice` 用于平仓验证
- [ ] 两个函数都调用 `GetRecentFills` 接口
- [ ] 时间记录在交易执行前（`openTime/closeTime = now(); Trade();`）
- [ ] 方向匹配逻辑正确（参考上表）
- [ ] 加权平均价计算公式正确
- [ ] 有重试机制（建议3次×500ms）
- [ ] 有降级策略（无记录时保持原价格）
- [ ] 所有交易类型都覆盖：
  - [ ] open_long
  - [ ] open_short
  - [ ] close_long
  - [ ] close_short
  - [ ] partial_close
  - [ ] auto_close_long
  - [ ] auto_close_short
- [ ] 三个交易所都实现了 `GetRecentFills`：
  - [ ] Hyperliquid
  - [ ] Binance
  - [ ] Aster

#### 8.7 阶段 6：风险验证（Risk Verification）
**BLOCKING级别** - 基于真实成交价的风险验证

| 验证项 | 要求 | BLOCKING条件 |
|--------|------|-------------|
| **风险计算** | 基于真实成交价计算实际风险 | 使用预估价计算风险 |
| **风险公式** | `risk% = (stopLossUSD + feeUSD) / totalBalance × 100` | 公式错误 |
| **风险限制** | 实际风险 > 2% 时触发保护 | 未检查或阈值不合理 |
| **自动调整止损** | 风险超限时自动调整止损价格 | 未调整或调整逻辑错误 |
| **止损更新** | 调整后更新内存状态和交易所订单 | 状态不一致 |

**验证清单**：
- [ ] 风险计算使用真实成交价（`actionRecord.Price`）
- [ ] 风险计算公式包含止损金额和手续费
- [ ] 存在风险阈值检查（建议2%）
- [ ] 风险超限时自动调整止损
- [ ] 调整止损后更新内存状态（`positionStopLoss[posKey]`）

#### 8.8 阶段 7：日志持久化（Logging & Persistence）
**BLOCKING级别** - 所有关键数据必须准确记录

| 记录项 | 要求 | BLOCKING条件 |
|--------|------|-------------|
| **账户快照** | 余额、可用、未实现盈亏、持仓数 | 数据不完整 |
| **持仓快照** | symbol, side, quantity, entryPrice, markPrice, unrealizedPnL | 数据不完整 |
| **决策记录** | AI决策内容、思维链、执行结果 | 决策未记录 |
| **成交价格** | 使用真实成交价（已验证） | 使用预估价 |
| **订单ID** | 记录交易所返回的 orderId | 订单ID未记录 |
| **盈亏计算** | 基于真实入场价和出场价计算 | 使用预估价计算盈亏 |

**验证清单**：
- [ ] Decision Log 包含账户快照
- [ ] Decision Log 包含持仓快照
- [ ] Decision Log 包含所有决策和执行结果
- [ ] 记录的价格是真实成交价（经过验证的）
- [ ] 盈亏计算基于真实成交价
- [ ] 日志持久化到文件/数据库

#### 8.9 数据流一致性验证（Critical Path Analysis）
**MANDATORY** - 必须追踪关键数据从源头到最终记录的完整路径

**开仓价格数据流**：
```
市场价格 → actionRecord.Price (临时) → OpenLong() → GetRecentFills() → 真实成交价 → actionRecord.Price (更新) → Decision Log
```

**平仓价格数据流**：
```
市场价格 → actionRecord.Price (临时) → CloseLong() → GetRecentFills() → 真实成交价 → actionRecord.Price (更新) → Decision Log
```

**内存状态数据流**：
```
开仓成功 → positionFirstSeenTime[posKey] → 每周期 buildTradingContext → 获取交易所持仓 → 清理不存在的posKey → 内存释放
```

**验证清单**：
- [ ] 开仓价格数据流完整（从市场价到真实成交价）
- [ ] 平仓价格数据流完整（从市场价到真实成交价）
- [ ] 内存状态有创建和清理逻辑
- [ ] 最终日志使用真实成交价（不是临时市场价）
- [ ] 盈亏计算基于真实入场价和出场价

#### 8.10 交易生命周期审查清单（必须100%通过）

**开仓阶段** ✅：
- [ ] 防重复开仓检查存在
- [ ] 保证金验证包含手续费
- [ ] 时间在交易前记录
- [ ] 止损/止盈在开仓后设置
- [ ] 成交价验证被调用
- [ ] 风险验证基于真实成交价
- [ ] 内存状态正确更新

**持仓阶段** ✅：
- [ ] 每周期同步交易所持仓
- [ ] 内存状态清理逻辑存在
- [ ] 被动平仓检测存在
- [ ] 被动平仓有成交价验证
- [ ] 止损/止盈调整有价格验证

**平仓阶段** ✅：
- [ ] 主动平仓有成交价验证
- [ ] 部分平仓有成交价验证
- [ ] 部分平仓有最小仓位检查
- [ ] 部分平仓后恢复止损/止盈
- [ ] 时间在交易前记录

**成交验证** ✅：
- [ ] GetRecentFills 接口存在
- [ ] 三个交易所都实现了接口
- [ ] 时间窗口合理（±10秒）
- [ ] 方向匹配正确
- [ ] 加权平均价公式正确
- [ ] 有重试机制
- [ ] 有降级策略
- [ ] 所有交易类型覆盖

**日志记录** ✅：
- [ ] 账户快照完整
- [ ] 持仓快照完整
- [ ] 决策记录完整
- [ ] 使用真实成交价
- [ ] 盈亏计算准确
- [ ] 日志持久化

**数据流一致性** ✅：
- [ ] 开仓价格数据流完整
- [ ] 平仓价格数据流完整
- [ ] 内存状态管理正确
- [ ] 最终日志数据准确

### ⛔ 交易生命周期审查失败条件（BLOCKING）

如果以下任一项为真，审查必须标记为❌不通过：

**开仓阶段问题**：
- 无防重复开仓检查
- 保证金验证不包含手续费
- 开仓时间在交易后记录（时序错误）
- 止损/止盈未设置或设置失败阻断流程
- 无成交价验证或使用预估价
- 风险验证基于预估价而非真实成交价
- 内存状态未更新或不一致

**平仓阶段问题**：
- 任何平仓类型缺少成交价验证
- 部分平仓无最小仓位检查（可能产生小额剩余）
- 部分平仓后未恢复止损/止盈（剩余仓位裸奔）
- 平仓时间在交易后记录（时序错误）
- 被动平仓未检测或未验证成交价

**成交验证问题**：
- GetRecentFills 接口不存在或未实现
- 任何交易所未实现接口
- 方向匹配逻辑错误
- 加权平均价计算公式错误
- 无重试机制或无降级策略
- 任何交易类型未覆盖（open_long/open_short/close_long/close_short/partial_close/auto_close）

**数据一致性问题**：
- 最终日志使用预估价而非真实成交价
- 盈亏计算基于预估价
- 内存状态泄漏（已平仓但内存未清理）
- 开仓/平仓价格数据流不完整
- 交易所状态与内存状态不同步

**记住：交易系统中，价格不准确 = 盈亏不准确 = 风险计算错误 = 可能导致资金损失！**

## 审查结果

请给出以下三种结果之一：
- ✅ **通过**：可以直接提交
- ❌ **不通过**：存在BLOCKING问题，必须修复
- ⚠️ **需要修复**：有改进空间，建议修复

## 核心原则

1. **白盒逻辑正确性是根本**：业务逻辑错误是生死线
2. **需求驱动**：必须找到真实需求来源
3. **客观分析**：基于实际代码和需求，不自我欺骗
4. **actionable建议**：提供具体的修复指导

## 评审交付物（必须包含）
- **问题清单**：逐条指出"谁与谁不一致"（路径/参数/字段/事件/状态码），附最小复现样本。
- **最小修复建议**：明确"谁改、改哪里、如何不破坏现有调用"（可附 1-3 行级 diff 建议）。
- **兼容/过渡策略**：必要时说明双解析/版本前缀/灰度开关/降级方案。
- **🚨 E2E验证报告**：对每个用户交互流程的完整追踪验证（MANDATORY）

## 强制性E2E验证清单（必须逐项检查）
在给出审查结果前，必须完成以下验证：

### ✅ 用户点击验证
- [ ] 所有onClick处理器都能正确执行
- [ ] 处理器中的navigate()调用指向正确的路径
- [ ] 目标路径在路由配置中存在
- [ ] 目标页面能正确处理URL参数

### ✅ 导航流程验证
- [ ] 从点击到页面加载的完整路径畅通
- [ ] URL参数正确传递和解析
- [ ] 页面状态正确初始化
- [ ] 用户看到预期的内容和界面

### ✅ 状态一致性验证
- [ ] 点击后应用状态正确更新
- [ ] UI界面反映状态变化
- [ ] 没有状态不同步的问题

### ✅ 安全验证（Web3 AI 交易系统 - MANDATORY）
- [ ] **密钥安全**：无私钥泄露（日志/错误/前端/Git）
- [ ] **密钥管理**：私钥加密存储，无明文环境变量
- [ ] **交易验证**：所有交易有签名验证、nonce、金额限制
- [ ] **滑点保护**：价格/滑点验证存在且合理
- [ ] **AI 安全**：用户输入有消毒处理，无直接拼接到 prompt
- [ ] **决策审计**：AI 决策有完整日志（输入/输出/时间戳）
- [ ] **合约安全**：无无限授权，合约地址来自白名单
- [ ] **限额保护**：存在单笔/累计交易限额
- [ ] **紧急机制**：有 kill switch 或暂停功能
- [ ] **预言机安全**：价格数据来自多源，有偏差检测
- [ ] **确认机制**：大额交易需要用户明确确认

### ⛔ 审查失败条件
如果以下任一项为真，审查必须标记为❌不通过：

**功能性问题：**
- 存在navigate()指向不存在或错误的路径
- 用户点击后无法到达预期页面
- 状态更新不完整导致UI不一致
- 关键用户流程无法完成

**安全性问题（Web3 AI 系统）：**
- 私钥/助记词出现在日志、错误消息、前端代码、Git 历史中
- 私钥明文存储（环境变量/配置文件/数据库）
- 交易缺少签名验证、nonce、或金额限制
- 存在无限授权（`approve(spender, type(uint256).max)`）
- 用户输入直接拼接到 AI prompt（prompt injection 风险）
- AI 可以自主决定大额交易（无用户确认）
- 缺少紧急暂停机制
- 单一预言机数据源（可被操纵）
- 大额交易无多重签名或延迟执行

**记住：代码编译通过 ≠ 功能正确工作 ≠ 资金安全**

## 技术验证方法（MANDATORY）

### 🔍 导航路径验证脚本
执行以下检查来验证导航逻辑：
```bash
# 1. 找出所有navigate()调用
grep -r "navigate(" frontend/src --include="*.tsx" --include="*.ts" -n

# 2. 找出所有路由定义
grep -r "path=" frontend/src --include="*.tsx" --include="*.ts" -n

# 3. 检查URL参数处理
grep -r "useSearchParams\|URLSearchParams" frontend/src --include="*.tsx" --include="*.ts" -n
```

### 🔍 状态管理验证
```bash
# 检查状态更新逻辑
grep -r "useState\|useEffect.*navigate" frontend/src --include="*.tsx" --include="*.ts" -n

# 检查onClick处理器
grep -r "onClick.*=>" frontend/src --include="*.tsx" --include="*.ts" -n
```

### 🚨 必须回答的验证问题
对于每个用户交互，审查者必须回答：

1. **点击发生什么？**
   - onClick处理器具体做了什么操作？
   - 调用了哪些函数？传递了什么参数？

2. **导航去哪里？**
   - navigate()的目标路径是什么？
   - 这个路径在路由配置中存在吗？
   - 路径参数格式正确吗？

3. **目标页面做什么？**
   - 目标页面/组件如何处理URL参数？
   - 是否正确提取和使用参数？
   - 用户最终看到什么内容？

4. **状态是否一致？**
   - 点击后应用状态如何变化？
   - UI是否正确反映状态变化？
   - 有没有状态不同步问题？

**如果审查者无法回答这些问题，审查必须标记为❌不通过**

## 快速验证提示
- 端点集中来源：前端禁止硬编码 URL；新增/变更端点已同步到常量/SDK。
- 认证一致：跨域/跨端口请求按需携带凭证（Cookie/Token），不依赖未声明的自定义头。
- 异步降级：在不支持事件/流式或网络异常时具备降级路径与用户提示。
- 可见性扫描：关键入口在默认态与目标设备上可见；空/错误/加载可复现且可恢复。
- 自动化检查：加入简单脚本/CI 规则检查硬编码端点、路径格式、必需认证头/凭证的使用一致性。

### `/design-feature`







**Description:** Analyzes and designs a new feature based on a GitHub issue URL.







**How to use:**



1.  Open the command palette and type `/design-feature`.



2.  Provide the GitHub issue URL when prompted.



3.  Gemini will analyze the issue, research the existing architecture, and propose a design.







**Workflow:**







1.  **Requirement Analysis:** Gemini will analyze the provided GitHub issue to understand the feature requirements.



2.  **Technical Research:** The agent will analyze the existing codebase and research any necessary technologies.



3.  **Architecture Design:** Gemini will design the system, including frontend and backend components.



4.  **Implementation Planning:** The agent will create a detailed plan for implementing the feature.



5.  **Risk Assessment:** Gemini will identify and mitigate potential risks.



6.  **Testing Strategy:** The agent will create a testing plan to ensure the feature is working correctly.







### `/submit-pr`







**Description:** Submits a pull request for a bugfix or feature.







**How to use:**



1.  Ensure your changes are committed.



2.  Open the command palette and type `/submit-pr`.



3.  Gemini will guide you through the process of creating a pull request.







**Workflow:**







1.  **Preparation:** Gemini will check the current branch and repository status.



2.  **Branch Creation:** The agent will create a new branch for the pull request.



3.  **Cherry-pick:** Gemini will cherry-pick the relevant commits.



4.  **Push:** The agent will push the new branch to the remote repository.



5.  **PR Creation:** Gemini will create a pull request with a descriptive title and body.







### `/create-issue`







**Description:** Creates a new GitHub issue.







**How to use:**



1.  Open the command palette and type `/create-issue`.



2.  Provide a title and description for the issue when prompted.



3.  Gemini will create the issue in the appropriate repository.







### `/prepare-x-thread`







**Description:** Prepares a thread for X (formerly Twitter) to announce recent work.







**How to use:**



1.  Open the command palette and type `/prepare-x-thread`.



2.  Gemini will analyze recent commits and generate a draft thread.







**Workflow:**







1.  **Content Gathering:** Gemini will gather information from recent commits.



2.  **Content Analysis:** The agent will analyze the commits to identify key features and bug fixes.



3.  **Thread Organization:** Gemini will organize the content into a thread structure.



4.  **Draft Generation:** The agent will generate a draft of the thread in a markdown file and a text file.












