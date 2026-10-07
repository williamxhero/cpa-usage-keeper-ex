# SPEC-001 — 渠道与上游凭证定价（第一期）

- 状态：**READY FOR AGENT／已审阅规格；功能尚未实现，未部署。**
- 目标项目：`williamxhero/cpa-usage-keeper-ex`，不涉及现有安装环境或其他项目修改。
- 源码基线：`main`，`ced1c4316c6a83d366253d2efdd0ffec5e573f6d`（2026-10-07 核查）。
- 规格来源：已确认的第一期需求及目标仓库现有行为。
- 金额口径：USD、单价按每 1,000,000 tokens；所有费用均为配置价估算，不是正式账单。

## Problem Statement

Keeper 用户通过多个具体渠道和上游凭证调用相同模型，实际约定的单价或折扣不同，但当前价格表以模型为唯一入口。现有规则虽然能用 `auth_index` 匹配凭证，却仍须逐模型重复配置，用户还需要手工识别索引，无法直接选择友好的渠道／凭证名称，也无法给同一模型的不同渠道分别设置完整单价。

“提供商类型”不等于“具体渠道”：两个 OpenAI-compatible 配置都可能显示为 `openai`，但它们的命名、连接端点和商业价格不同。仅按 provider 类型归类会把本应独立的费用混在一起。上游凭证索引也不等于下游 API Key 分组；混用会把费用绑定到错误对象。

当前费用会用最新价格回算历史。用户需要清楚理解每个请求命中了哪一层定价，以及 Overview、各维度汇总、请求明细为何得到相同的金额。如果缺少基准价或绑定失效，不能把未知成本伪装成免费，更不能把部分已知成本显示为完整总额。

第一期要解决的是 Keeper 内部的可维护、可解释、兼容旧配置的渠道／凭证估算定价，不是改造 CPA 或同步真实财务账本。

## Solution

在 Keeper 的现有价格设置体验中提供渠道和上游凭证选择、绑定及定价管理。

- 从现有身份目录展示经过安全筛选的名称、别名、类型、状态和识别信息。用户不需要查看 API key 或认证文件秘密，也不需要靠手工输入索引识别凭证。
- 使用 Keeper 自有稳定渠道标识，将一个具体命名渠道与选定的上游凭证显式关联。同为 `openai` 的两个渠道可以独立配置；目录信息不足时由用户显式选择成员，不猜测归属。
- 渠道和凭证均支持一条全模型默认倍率，以及针对某个模型的专属倍率或完整四段固定单价。默认倍率覆盖后续使用的其他模型，不需要复制到每个模型。
- 倍率支持 `0.2x`、`20%`、`1.2x` 等输入；界面清楚区分“未配置／继承”“显式 0”“缺少价格”。
- 使用唯一的覆盖顺序：**凭证＋模型 > 凭证全模型默认倍率 > 渠道＋模型 > 渠道全模型默认倍率 > 现有 legacy 定价**。新覆盖之间不连乘，新覆盖也不再次叠加旧折扣。
- 展示“配置价估算费用”和“基准参考费用”，请求明细解释命中范围、模式、模型匹配、倍率／四段单价及缺价原因。两种金额分别具有完整性状态。
- 保存后持久化并原子发布，所有费用查询复用后端解析结果；同一数据范围与同一定价快照的明细之和、Overview 和汇总相符。
- 明确提示修改价格、渠道成员或凭证绑定会影响历史回算；索引变化以失效／未绑定状态提示，经显式确认迁移，不按同名自动继承。

## User Stories

1. As a Keeper administrator, I want to select an upstream credential by a safe friendly name, so that I can configure its pricing without inspecting an API key or guessing an auth index.
2. As a Keeper administrator, I want to distinguish named channels that use the same provider type, so that independent upstream prices are not merged into one generic provider bucket.
3. As a Keeper administrator, I want to create a stable Keeper pricing channel and explicitly bind its member credentials, so that a channel remains identifiable independently of mutable upstream names.
4. As a Keeper administrator, I want to see unresolved or ambiguous channel membership, so that I do not accidentally assign a price to the wrong upstream configuration.
5. As a Keeper administrator, I want to set one channel-wide multiplier for all models, so that I do not have to repeat the same discount for each model.
6. As a Keeper administrator, I want to set one credential-wide multiplier for all models, so that a credential-specific agreement automatically applies to every model it uses.
7. As a Keeper administrator, I want to override a channel default for a specific model, so that exceptional model pricing can coexist with the channel-wide default.
8. As a Keeper administrator, I want to override a credential default for a specific model, so that a model-specific credential agreement takes precedence over broader settings.
9. As a Keeper administrator, I want to enter equivalent multiplier forms such as 0.2x and 20%, so that I can use the pricing notation familiar to me.
10. As a Keeper administrator, I want to enter multipliers above one and an explicit zero, so that markups and intentionally zero-priced usage are represented correctly.
11. As a Keeper administrator, I want invalid, negative, non-finite, or unsafe numeric inputs to be rejected without losing saved settings, so that a typo cannot corrupt estimated costs.
12. As a Keeper administrator, I want to distinguish an inherited setting from an explicit zero, so that clearing a setting does not accidentally make usage free.
13. As a Keeper administrator, I want to set independent input, output, cache-read, and cache-write prices for a channel and model, so that my channel-specific four-part tariff is reflected in estimates.
14. As a Keeper administrator, I want to set a different four-part tariff for the same model on an individual credential, so that credential-specific prices remain independent of channel prices.
15. As a Keeper administrator, I want an incomplete fixed-price form to be rejected rather than silently filling missing parts with zero, so that unknown prices are not mistaken for free usage.
16. As a Keeper user, I want a complete fixed tariff to estimate a model that has no baseline price, so that I can evaluate known contractual prices while seeing that the baseline comparison is unavailable.
17. As a Keeper user, I want a multiplier with no usable baseline to show unavailable cost rather than invented cost, so that discounts do not conceal unknown pricing.
18. As a Keeper user, I want credential pricing to take precedence over channel pricing without multiplying the two, so that the displayed cost matches the single applicable agreement.
19. As an existing Keeper user, I want requests without a matching new override to retain all existing model multipliers and matching legacy rules, so that upgrading does not silently change my current cost outputs.
20. As a Keeper user, I want configured-price estimates and unadjusted baseline references to be displayed separately, so that I can understand both my configured agreement and its comparison basis.
21. As a Keeper user, I want each request to explain its selected scope, pricing mode, model match, and applied prices, so that I can diagnose an unexpected estimate.
22. As a Keeper user, I want Overview, time, model, channel, credential, and request-detail costs to use the same backend pricing, so that each breakdown reconciles with the total for the same data scope.
23. As a Keeper user, I want existing token normalization, model matching, and pricing styles to be preserved while replacements of legacy tier adjustments are clearly explained, so that a new override neither double-counts tokens nor hides a change in applied pricing.
24. As a Keeper user, I want missing prices to mark a partially known total as incomplete, so that a small unpriced portion cannot be hidden inside an apparently complete total.
25. As a Keeper administrator, I want saved pricing and bindings to survive restart and become visible atomically, so that readers never receive half of a pricing update.
26. As a Keeper administrator, I want to see stale credential bindings and explicitly migrate them after index rotation, so that a renamed or replaced credential cannot silently inherit another credential's price.
27. As a Keeper administrator, I want a clear warning that changing prices or bindings recalculates historical estimates, so that I do not mistake the current view for a frozen historical bill.
28. As a read-only user or API consumer, I want existing access limits and cost-field contracts to remain compatible without exposing secrets, so that the feature does not expand my permissions or break existing integrations.

## Implementation Decisions

### 1. 领域边界与身份术语

- **提供商类型（provider type）**：现有身份元数据中的协议／提供商分类，例如 `openai`。它用于说明和筛选，不是本期价格分组的唯一键。
- **渠道（pricing channel）**：Keeper 拥有的具体命名定价分组，具有持久化、不随改名改变的 opaque ID、显示名称、可选说明和显式成员关系。它代表选定的具体配置／商业渠道，不是“所有 OpenAI 请求”。
- **上游凭证（credential）**：现有使用身份解析出的 CPA 凭证。`auth_index` 是上游身份线索，`api_group_key` 是下游调用者分组；本期不能互换二者，也不扩展旧规则的字段集合来模拟渠道。
- **凭证定价主体**：Keeper 自有稳定 ID，显式关联既有目录身份关系；保留身份类型及原始 Identity 等已有解析所需信息。UsageIdentity 的数据库主键可作为引用，但不被宣称为上游永不变化的 `auth_id`；LookupKey 只在既有内部用途使用，不成为新对外定价标识。
- 通过既有身份解析链结合 `auth_index` 和现有可信类型线索获得唯一身份。不能唯一解析时标记未绑定／歧义，不能凭名称、provider 类型、模型名或凭证列表顺序猜测。

### 2. 具体渠道映射与安全目录

- 现有 OpenAI-compatible 元数据来源用配置 Name、BaseURL 和每条 AuthIndex 表达多个凭证，但没有独立稳定 channel ID；缺少必要 key 或 AuthIndex 的条目当前会被跳过。本期不假定这些条目已经可被唯一选中。
- 渠道 ID 由 Keeper 生成；管理员以安全元数据选择具体配置对应的凭证成员。创建渠道时可用明确的配置分组作为候选建议，但必须显式确认成员，不按 Name＋BaseURL 自动赋予永久等价关系。
- Name、经过安全处理的连接端点、别名和 provider 类型只用于识别／解释。即使名称和端点相同，也不自动合并两个独立渠道；有歧义时显示候选凭证并要求显式选择。
- 一个凭证定价主体同一时刻至多属于一个渠道；一个渠道可有多个凭证。同一主体的重复／冲突成员关系须拒绝，不能用保存顺序决定费用。未绑定渠道的凭证仍可有独立定价。
- **稳定渠道 ID 不能补出用量中不存在的归属证据。** 同类型＋endpoint＋key 的重复上游配置可能共享同一个 AuthIndex；已有目录归一化的首项保留／去重也不证明该首项就是请求实际渠道。只有可靠身份证据能区分的渠道才可独立绑定和定价；管理员给两个名称各造一个 ID 并不能使共享身份的用量可拆分。
- 缺身份、共享身份无法区分具体配置、冲突绑定或已知歧义时，拒绝保存声称独立归属的冲突配置；查询不得随机挑一个渠道或凭证覆盖。已存配置后来出现这种冲突时，新渠道／凭证覆盖均视为未匹配，提示“归属冲突／无法区分”，归入“未绑定／未知渠道”，并完整回退到原 legacy 路径。
- 上述回退只表示能否沿用原计价，不证明渠道归属正确：legacy 可计算时保留原估算结果并附归属告警，legacy 缺基准且有计价 token 时继续显示费用不可用／不完整；不能为满足渠道报表随意填价或标成免费。界面说明覆盖未适用及原因，不把未知归属误显示成某个命名渠道。
- 新选择器、接口、日志、校验错误和费用解释均使用安全 allowlist。禁止 API key、LookupKey、管理密钥、认证文件内容、访问／刷新 token、认证文件完整路径和其他认证秘密进入新对外结果。端点展示须移除 userinfo、含秘密的查询参数等敏感内容；不为方便展示新增密钥尾段。
- 沿用既有身份可见性和访问控制。需要区分同名凭证时可展示权限允许的已有安全身份标识／Keeper ID，不开放新的秘密查询端点。

### 3. 定价配置模型与持久化

- 新覆盖独立于现有 ModelPriceSetting 和 ModelPriceRule 存储；不迁移、不改写或删除用户现有模型单价、PriceMultiplier、旧规则。
- 覆盖的唯一性由“主体类型＋稳定主体 ID＋范围（全模型／指定模型）”确定；指定模型再包含规范化后的模型标识。每个唯一范围只有一个生效模式，不能同时保存倍率和固定价。
- 渠道和凭证的全模型范围只允许倍率。指定模型范围允许二选一：倍率或四段固定单价。固定价绝不成为所有模型的通用默认价。
- 选模型时允许从已有使用记录或既有模型目录中选择，包括已有使用但没有基准价格的模型。固定价覆盖不能因为缺少全局 ModelPriceSetting 而无法创建。
- 使用已有模型标识规范化和匹配约定，不引入大小写折叠、模糊匹配或新别名推断。新规则列表按安全名称友好呈现，持久化和解析仍用稳定 ID。
- 以可兼容的数据库变更保存渠道、凭证定价主体、身份绑定、成员关系和覆盖。重启恢复后相同配置得到相同结果；没有任何新记录时原功能无需重新配置。
- 移除一个覆盖等于回到下一层继承，不写入零倍率。删除仍有成员／覆盖的主体须明确处理依赖并确认历史回算影响，不静默级联丢失用户价格配置。

### 4. 数值、空值和完整四段价

- 倍率文本接受去掉首尾空白后的普通十进制数、十进制数加 `x`／`X`、十进制数加 `%`；百分比除以 100。`0.2`、`0.2x`、`20%` 的规范化数值均为 0.2；`1.2x` 为 1.2，`120%` 同样为 1.2。
- 保存和 API 回读统一为有限、非负数值，不能只保存原始字符串，也不能把倍率限制在 1 以内。拒绝负数、NaN、Infinity、混合单位、无法解析的输入以及会使受支持计价范围溢出的数值组合；沿用现有计价的安全数值校验能力。
- API 保存覆盖时必须显式提供有效数值。表单空白及未配置表示继承，不能默认转成 0 或保存一条隐含倍率为 1 的覆盖。已有覆盖的继承操作通过明确移除／清除完成；这不改变旧规则 API 对 omitted／null 的既有默认行为。
- 显式倍率 1 是生效覆盖：按无 legacy 调整的基准价计费，并阻断低优先级覆盖；显式倍率 0 同样是生效覆盖，不等于继承。
- 固定价单位统一为 USD／1M，完整包含普通输入、输出、缓存读取、缓存写入四段，每段必须显式提供有限非负数。任一字段为空、null、缺失或非法时整个保存失败，不从基准、渠道或其他凭证逐段拼接。
- 四段中的显式 0 是已知零单价，可全部为 0；“缺失”不是 0。固定价模式没有额外倍率；切换模式须提交完整目标模式，不让隐藏的旧模式字段继续参与计价。
- 沿用现有模型基准的存储语义：已有四段字段中的 0 仍按原语义处理，不推断它原本是缺失还是免费，也不为本期重新判定历史 legacy 可用性。新固定价接口必须保留缺失与显式 0 的区别；不存在可用基准记录时不能用默认零值伪造参考费用。

### 5. 覆盖选择与 legacy 组合：唯一算法

每个费用主体按以下顺序选择**第一条有效且绑定明确的配置**：

| 优先级 | 范围 | 允许模式 |
| --- | --- | --- |
| 1 | 凭证＋指定模型 | 倍率或完整固定价 |
| 2 | 凭证全模型默认 | 倍率 |
| 3 | 渠道＋指定模型 | 倍率或完整固定价 |
| 4 | 渠道全模型默认 | 倍率 |
| 5 | 未命中新覆盖 | 完整保留现有模型匹配和 legacy 计价 |

- 在每个“指定模型”优先级内先匹配请求 Model，再按既有约定尝试 ModelAlias。范围优先级先于这两个候选：命中凭证级 alias 覆盖仍优先于渠道级精确模型覆盖。解释结果须标明实际匹配名称和来源。
- **基准参考费用**：用当前匹配到的模型基准四段单价及既有归一化 token 计算，**不乘 PriceMultiplier，也不乘任何旧 ModelPriceRule**。它是 Keeper 当前配置的参考，不被宣称为官方实时合同价。
- **新倍率覆盖的配置价估算费用**：上述未调整基准费用乘唯一选中的新倍率；四段费用均按同一倍率缩放。PriceMultiplier 及所有旧规则均不再执行。
- **新固定价覆盖的配置价估算费用**：用所选覆盖完整四段单价和相同 token 口径计算；不再乘渠道默认倍率、凭证默认倍率、PriceMultiplier 或任何旧规则。
- **未命中新覆盖的配置价估算费用**：完整调用原 legacy 路径，包括原模型／alias 选择、PriceMultiplier、所有命中旧规则相乘、显式零倍率短路、缺价处理和其他既有结果语义。不存在新覆盖的对象，其原金额、分段金额、可用性和已有匹配元数据必须与升级前一致。
- 新覆盖是该范围的完整定价选择，不是额外叠加优惠。原来通过 service tier、reasoning effort、auth_index 等旧规则产生的调整也被所选新覆盖替代；只在未命中新覆盖时继续按旧规则计算。界面必须明确这一点，不能暗中保留某一种旧乘数。
- 命中倍率但基准缺失时，所选配置仍是该倍率，估算不可用；**不能跳过它转用低优先级固定价或 legacy 价格，也不能因倍率为 0 而把有 token 的未知基准变成可用零费用**。用户需要改成完整固定价或补齐基准。
- 命中完整固定价即使没有模型基准也可估算；基准参考费用为不可用。若有基准则两种费用分别计算，不能以固定价替代“基准”。
- 无任何需要计价的 token 时沿用当前零费用可用性约定；金额为 0 本身不用于判断是否缺价。

例如：未调整基准为 $10，旧 PriceMultiplier 为 0.5，两条命中旧规则为 2 和 3。没有新覆盖时结果仍为 $30；渠道默认 0.2x 得到 $2；同时有凭证默认 0.3x 时得到 $3，而不是 $0.60、$9 或其他叠乘结果。凭证指定模型倍率 1.2x 再覆盖为 $12。基准参考始终为 $10。

### 6. 后端统一解析、Token 与聚合一致性

- 扩展现有后端定价领域、快照及费用输入／结果契约，在一个 resolver 中完成身份绑定解析、覆盖选择、配置价估算、基准参考及解释。前端只提交配置和显示结果，不独立计算账面费用或选择规则。
- 延续现有四段 token 归一化：各段非负，普通输入为输入减缓存读取及缓存写入后的非负部分；缓存两段各算一次，不能再次按普通输入价收费。沿用已有事件处理器、executor、pricing style 和 reasoning token 口径，不重新解释已保存使用事件。
- 固定价继承可用基准的 pricing style；没有基准时保存时要求显式选择现有受支持 style。该选择用于沿用现有计价／展示契约，不用于重写历史 token。不得引入新的 style 或依据显示名称猜测协议。
- 保留请求中的模型／alias、tier 等现有属性及 legacy 匹配含义；本期不新增 tier 定价层。新覆盖替代旧 tier 倍率是上一节的明确选择，不意味着重写请求 tier 或丢弃这些字段。
- Overview、时间序列、模型／渠道／凭证汇总、请求明细，以及其他仍返回现有费用字段的查询，统一使用当前解析器；若某已有费用视图未增加双价展示，其原金额也不能继续走独立旧算法。
- 聚合在计价前必须保留分辨模型／alias、上游身份与渠道绑定、token 归一化差异、legacy 活跃规则等所需维度。不能先把同模型的不同凭证 token 合并，再按任意一个凭证价格计费。
- 对聚合行延续先逐行归一化再聚合的既有原则。新绑定所需维度须参与投影，即使没有旧 auth_index 规则也不能被优化掉；必要时调整读取投影，而非另造采集／事件去重系统。
- 相同过滤条件、时间范围、时区、可用事件范围和定价快照下，各种汇总等于对应明细的配置价估算之和，基准参考亦然；独立取整仅发生在展示边界，后端聚合不累计前端舍入值。

### 7. 双价展示、缺价和 API 兼容性

- 主金额标签为“配置价估算费用”，参考标签为“基准参考费用”；不能把前者标成“真实扣款”，不能把后者标成未经证实的官方价格。
- 既有金额字段（如 `cost_usd`、`total_cost_usd`、Overview 的 `total_cost` 及现有分段金额）保留字段名、数值类型、币种和归一化口径，表示**配置价估算费用**。无新匹配时其值保持原样；有新匹配时按新选择变化，这是管理员配置后的预期行为。不会把已有 cost 字段改成原始基准费用。
- 既有 `cost_available` 等字段保留**各自端点的原契约**，不能一概重新定义为同一种完整性含义。已核查的 Overview 在任一计价行不可用时将 Summary 的 CostAvailable 置为 false，并由服务直接透传，因此其现有字段已经表达估算不完整，无需改变该语义。其他视图仍按各自既有契约处理；未命中新覆盖时其原可用性值完全保持。
- 双价结果分别表达估算完整性与基准完整性，不能用同一个标志代表两种费用。估算可复用本来就表示完整性的既有字段；若某视图的既有可用性字段不是完整性契约，则通过新增独立完整性标志表达未知部分，不覆盖旧字段语义。基准金额有独立的完整性信息。新增字段向后兼容，不要求既有客户端改请求参数。
- 单请求无可用费用时，新参考金额可为 null，并附带明确原因；保留已有数值字段的既有占位与 availability 行为，客户端不得仅凭数值 0 识别免费。聚合可返回已知部分的小计，但任一有计价需求的主体缺价时对应费用的**完整性**必须为 false，界面标“部分估算／不完整”；不得据此重定义其他视图的旧 availability 字段。完全没有可估价部分时显示“不可用”，不是完整的 $0。
- 返回两种费用各自的缺价原因／涉及模型与身份等安全覆盖信息，沿用已有 cost availability／incomplete 告警体验。若给出缺价数量或覆盖率，必须真实可由当前查询数据支持，不能把聚合行数冒充请求数或编造 100% 完整率。
- 请求明细解释至少包含：命中 scope、稳定主体标识及安全名称、指定模型／默认范围、模型或 alias 匹配、倍率或完整四段价、实际应用的分段费用、是否走 legacy、是否存在被新覆盖替代的 legacy 调整、估算及参考的可用性原因。解释必须来自用于计算金额的同一快照。
- legacy 解释保留其命中规则和最终倍率；新覆盖解释明确“未叠加模型通用倍率与旧规则”，不让用户把两个界面中的折扣再次相乘。
- 在新／旧响应契约中提供兼容的定价快照一致性标识，以区分跨请求更新期间的不同快照。该标识只用于一致性判断与刷新，不保存历史价格版本，也不承诺能够查询旧快照。
- 保持既有查询访问范围：只读用户不能修改渠道、绑定或价格，也不能通过双价字段看到不属于自己的使用记录。管理接口沿用管理员鉴权；原本被禁止的目录／定价管理读取不会因友好选择器而开放。只读费用解释仅含其权限范围内的安全摘要。
- UI 沿用现有国际化与响应式布局约定，新增文案覆盖英文、简体中文、繁体中文（en／zh／zh-TW），复用现有组件和布局。桌面与移动窄屏均可完成选择、输入、保存及查看解释，长名称／错误提示不遮挡关键操作；本期不做视觉重设计。

### 8. 保存原子性、失效与历史回算

- 沿用“串行写事务、事务内校验完整候选、提交成功后原子发布快照”的既有机制，将新价格及必要绑定纳入同一解析快照。身份成员关系变化也须形成完整、一致的新候选，不能让新成员对应旧规则或反过来。
- 一个响应绑定一个不可变快照；并发读只能观察整个旧候选或整个新候选。非法配置、候选构建失败或提交失败不得改变数据库有效配置或对外活跃快照；保存成功后后续查询看到新配置。
- 原子性保证的是费用查询视图及保存结果，不要求把上游元数据抓取和所有服务合并成分布式事务。新增身份暂未解析时保守显示未绑定；发布失败不得对外形成半套价格。
- key、BaseURL、认证文件路径等变化可能导致 AuthIndex 改变，Keeper 不将该索引承诺为永久稳定身份，也不按同名、相同 endpoint 或相同 provider 自动迁移价格。
- 目录确认原身份不再有效时显示“绑定失效／历史身份”，新身份显示“未绑定”；单轮同步失败或超时不能被误判为身份已删除，沿用现有 scoped stale／restore 和同步状态判断。
- 已记录的精确历史身份关联保留，用于历史请求的当前价格回算；“当前凭证离线／失效”不等于抹除其历史归属。新索引在显式迁移前不继承旧主体的覆盖；其请求按可用绑定选择或走 legacy。
- 显式迁移由管理员确认原主体与新目录身份，保留原历史身份引用并把新身份加入该稳定主体，沿用其覆盖与渠道归属；若新身份已属于其他定价主体必须先解决冲突，不默默抢占。纠正错误历史绑定属于单独明确的解绑／重绑操作，需提示历史金额和归属将重新计算。
- 修改价格、删除覆盖、调整渠道成员及迁移／纠正绑定时显示“当前配置作用于历史回算，不是请求发生时冻结的账单”。配置变化可以改变旧请求的费用；绑定归属修正还可以改变历史渠道／凭证汇总。无需新增账单快照或价格审计系统。

## Testing Decisions

### 测试边界与既有先例

用户已确认主 seam：**经现有价格保存 API／服务持久化，再经现有费用查询取得 Overview、汇总和请求明细**。测试以外部结果和契约为依据，而非断言内部表结构、函数调用次数或 resolver 的私有实现。

- 主集成测试使用隔离的临时数据库、受控身份目录／配置元数据和真实定价服务。通过管理员保存配置后查询，不仅用价格 stub 证明路由接受 JSON；同一夹具覆盖所有费用查询。
- 复用现有 API 定价校验／管理员鉴权测试、repository 的 Overview hourly／daily 规则测试、分析和请求明细规则测试、无新规则兼容性测试，以及服务价格保存／重启／原子快照的已有辅助设施。
- 现有 hourly 测试证明默认 $1 与 priority $2 聚合为 $3；daily 测试证明两条命中旧规则 2 与 3 连乘为 6。保留这些先例，并加入新覆盖旁路 legacy 及身份维度保留的集成断言。
- 数值规范化、完整四价和关键安全异常可增加少量领域／服务边界测试，但不另造多套计算引擎。
- 辅助 UI 测试沿用现有 PriceRulesModal、表单草稿及 pricing API 的组件测试习惯，验证用户实际选择、输入、提交、错误反馈、继承／零区别和双价标签；金额正确性由后端主 seam 验证。
- 使用最小而具代表性的已知 token 夹具，金额断言考虑现有数值精度，不使用大范围宽松误差掩盖重复计价。回归要求是同输入、同范围、同快照的金额／availability 一致，不是证明 exactly-once 采集。

### 可测试验收矩阵

| 编号 | 场景／操作 | 必须观察到的外部结果 |
| --- | --- | --- |
| T01 | 不创建任何新覆盖；覆盖旧 PriceMultiplier、两条以上命中旧规则、零倍率及未匹配规则 | 所有既有费用输出与原行为一致；旧规则仍全命中相乘，原可用性及 alias 结果不变。 |
| T02 | 两个 provider 类型都为 openai 的具体渠道 A／B，同模型各有 1M 普通输入，基准 $10；默认倍率分别 0.2x／0.5x | 渠道小计 $2／$5，总计 $7；渠道独立，不合并成一个 openai 定价主体。 |
| T03 | 基准 $10，旧模型倍率 0.5，旧规则 2、3；渠道默认 0.2x，凭证默认 0.3x | 新估算 $3、参考 $10；不为 $0.60 或 $9。清除全部新覆盖恢复 legacy $30，旧配置未被改写。 |
| T04 | 依次增加／移除凭证模型覆盖、凭证默认、渠道模型覆盖、渠道默认 | 严格按五层顺序逐层生效；凭证默认优先于渠道模型覆盖；清除一层立即继承下一层。 |
| T05 | 凭证默认 0.3x；模型 M 专属 1.2x，另一个模型无专属；之后出现新模型 N | M 用 1.2x，其他有基准模型和 N 用 0.3x，不需为 N 复制配置。 |
| T06 | 输入 0.2、0.2x、20%、1.2x、120%、0x；保存并回读／重启 | 等价值规范化一致，大于 1 不被截断；0 是明确覆盖，1 也阻断低层和 legacy。 |
| T07 | 负数、NaN、Infinity、混合单位、错误文本、极大有限但组合溢出值 | API／服务拒绝，界面给字段错误；之前已保存配置和所有查询结果不变。 |
| T08 | 表单空白、切换继承、显式 0；旧规则 omitted／null 请求 | 空白不产生隐含零或 1 覆盖，明确移除恢复继承；新覆盖的 0 被保存；旧 API 默认行为不变。 |
| T09 | 同模型同渠道内两凭证分别固定四价（1,2,3,4）和（2,4,6,8），各段各 1M；输入总量含两个缓存段，为 3M | 估算分别 $10／$20，总计 $30；不再乘任何其他倍率；两凭证单价独立持久化。 |
| T10 | 固定四价任一段 omitted／null／空白／负数，再测试四段显式 0 | 缺／非法字段整体拒绝、不补价；完整零价请求为可用零费用，与缺价状态可区分。 |
| T11 | 已有使用模型没有基准；给该凭证模型完整四价（1,2,3,4），使用 T09 第一组 tokens | 配置价估算 $10 可用，基准参考不可用；无需先造零基准记录，也不以固定价冒充参考。 |
| T12 | 无基准，最高层命中倍率 0.3x 或 0x，低层存在固定价 | 有计价 token 时估算仍不可用，解释指向所选倍率和缺基准；不降级、不假造免费。 |
| T13 | 完全无需计价的 token 输入，无模型基准 | 延续现有可用零费用约定，和“有 token 但缺价”区分。 |
| T14 | 基准四价（10,20,2,4），input=1M、output=0.1M、cache-read=0.2M、cache-write=0.1M；倍率 0.2x | 普通输入 0.7M；基准 $9.8、估算 $1.96，缓存不重复计入普通输入。逐请求及聚合结果相同。 |
| T15 | 单请求缓存 token 超过 input；另有正常请求，与其一起聚合 | 先逐请求／行归一化再汇总；不得用总 input 减总 cache 代替既有逐行非负处理。 |
| T16 | Model 精确匹配、仅 ModelAlias 匹配、两候选均有配置，以及凭证 alias 对渠道精确模型 | 每层先 Model 后 ModelAlias，范围优先级不变；解释实际匹配来源，模型名不模糊猜测。 |
| T17 | 现有受支持 pricing styles、executor／reasoning token、service tier 旧规则；有／无新覆盖 | 已保存 token 和模型口径不变；legacy tier 连乘保留，新覆盖时旧 tier 乘数明确不再执行；缺基准固定价要求选择合法 style。 |
| T18 | 同模型不同身份和价格、同身份不同模型／tier，覆盖小时／日边界与下游 Key 过滤 | 同范围、同快照的 Overview、时间、模型、渠道、凭证和明细金额及四段小计一致；没有因投影丢 auth_index 而错价。 |
| T19 | 可估价 $2 的请求与有 token 但缺价请求混合；再测试固定价可用但基准不可用 | 已知估算小计保留但总额标不完整，Overview 原 cost_available 为 false；其他视图保留各自旧字段契约，使用已有或新增完整性标志表达未知部分。估算与参考分别判断完整性，缺价原因可见。 |
| T20 | 保存价格／绑定、刷新、重建服务并查询历史 | 配置、稳定 ID 和关联持久化；当前价格作用于旧请求，UI 明确回算提示；重启结果一致。 |
| T21 | 并发查询与覆盖／成员更新；非法候选及写事务提交失败 | 单响应只出现完整旧或新配置；解释、估算、参考一致；失败时数据库有效配置和活跃快照均保持原值。 |
| T22 | 索引变化但 Name／BaseURL 与旧凭证相同；随后显式迁移；同时模拟一次 metadata 抓取失败 | 迁移前新身份未绑定，不自动继承价格；原历史关联保留；迁移后新旧精确身份归同一主体；抓取失败不误删除绑定。 |
| T23 | 同名同端点歧义、同类型＋endpoint＋key 的重复配置共享 AuthIndex（含目录首项去重）、缺 AuthIndex、重复成员及迁移到已占用身份 | 新渠道 ID 不被当作归属证据，不能猜选／随机应用覆盖或保存独立归属冲突；已有冲突的新覆盖不匹配，提示冲突并归未知渠道。legacy 可算则保持原金额并告警，缺价则不可用／不完整；错误无秘密，既有价格不被改写。 |
| T24 | 下游 api_group_key 改变、上游凭证不变；以及反向情况 | 上游覆盖只随实际凭证选择；下游分组只影响既有过滤／legacy 规则，不被当成 credential。 |
| T25 | 无登录／只读用户访问管理接口；带特殊端点、秘密字段的元数据进入新接口和错误路径 | 沿用管理员鉴权，禁止越权保存／读取管理目录；API、选择器、日志、解释及错误不含 key、LookupKey、token、认证内容／完整路径或端点秘密。 |
| T26 | 旧客户端请求已有费用 API，新客户端显示双价；多个查询跨价格更新 | 原字段名称／类型／币种不变，无新匹配时值不变；新增字段可忽略；跨快照标识可识别并刷新，不误宣称瞬间金额一致。 |
| T27 | UI 选择友好渠道／凭证、保存全模型倍率／模型四价、切换模式／继承、显示失效绑定和部分费用 | 不需手工查看密钥；前后端校验和错误一致；新旧规则组合说明、历史回算提示、双价标签及独立缺价状态清晰。 |
| T28 | 在 en／zh／zh-TW 和桌面／移动窄屏下使用新配置与双价展示 | 新文案有完整翻译，不显示缺失的翻译键；关键操作、长名称及错误提示沿用现有响应式布局，不遮挡或挤出可用区域；不引入视觉重设计。 |

### 实现完成后的验证门

功能尚未实现，以下功能测试和构建门待实施完成后执行，不作为规格已通过的测试证据。实施者应先运行最小相关后端集成／组件测试，再按项目 Makefile 的现有门验证：

- `go test ./cmd/... ./internal/...`
- 前端依赖使用 `npm --prefix ./web ci`。
- `npm --prefix ./web run test`
- `npm --prefix ./web run lint`
- `npm --prefix ./web run typecheck`
- `npm --prefix ./web run build`

身份和价格夹具必须使用合成数据及隔离数据库；不连接生产服务，不读取本机安装数据库或以真实凭证验证不泄密。无法运行的门须记录原因，不能标记通过。新增契约测试应在实施前能暴露原功能缺口，并在实现后通过；保留所有有效旧回归。

## Out of Scope

- CPAMP／CPAMC 费用同步、前端或源码修改；其他独立项目写入。
- CPA 核心修改、上游凭证索引生成算法修改，或为本期虚构一个永不变化的上游 auth_id。
- 本机安装目录的环境配置、数据库、二进制、可执行文件及任何服务变更。
- 生产部署、发布安装包、服务重启或切换、数据回填。
- 前端视觉重设计。
- 发票、余额结算、真实扣款、会计账本、汇率及多币种。
- 价格版本档案、按请求发生时价格冻结、历史账单冻结或完整审计／结算系统。
- 采集管线重构、事件去重重构、exactly-once 承诺和重复事件修复。
- 抓取任意官方实时价格、私有合同价格或自动猜测商业折扣。
- 新的 provider 协议、pricing style、通用规则表达式语言、额外 tier 定价层及新渠道／凭证覆盖连乘机制。
- 规格不要求引入新生产依赖；只有现有工具无法合理满足已批准范围时才另行说明必要性。

## Further Notes

- 公开源码基线为目标 fork 的 main 提交 `ced1c4316c6a83d366253d2efdd0ffec5e573f6d`，核查日期 2026-10-07。
- 官方发布版本 v1.15.9 不等于本 fork 的 main；应以目标源码基线判断现有能力和兼容行为。
- 配置价估算与基准参考均不是正式账单、真实扣款或官方实时合同价格；历史结果随当前配置回算。
- 规格验收要求“同范围、同快照、同 token 口径的估算一致”，不证明采集数据本身无重复、exactly-once 或与供应商账单精确一致。
