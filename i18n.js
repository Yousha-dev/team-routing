/* Dashboard-only localization. API identifiers, policy values and raw diagnostics stay unchanged. */
window.TeamRoutingI18n = (() => {
  'use strict';
  const storageKey = 'team-routing.language';
  const messages = {
    'Account availability and client routing': '账号可用状态与客户端路由',
    'Language': '界面语言',
    'Automatic (browser language)': '自动（跟随浏览器）',
    'Disconnected': '未连接', 'Connected': '已连接', 'Loading': '加载中',
    'Connect to your proxy': '连接代理服务',
    'The management key stays in this page’s memory and is sent only to this proxy.': '管理密钥仅保存在当前页面内存中，且只会发送给此代理服务。',
    'Management key': '管理密钥', 'Connect': '连接',
    'Team Routing sections': 'Team Routing 功能分区',
    'Overview': '概览', 'Client routing': '客户端路由', 'Policy': '策略', 'Diagnostics': '诊断',
    'Reload settings': '重新加载设置', 'Refresh due quota': '刷新到期配额数据',
    'Connected accounts': '已接入账号',
    'Decisions use the active policy and current provider readings.': '路由决策依据当前生效的策略及提供商配额数据。',
    'Filter accounts': '筛选账号', 'Account or provider': '账号或提供商',
    'Account': '账号', 'Five-hour remaining': '5 小时剩余额度', 'Weekly remaining': '每周剩余额度', 'Routing status': '路由状态',
    'Client keys and model overrides': '客户端密钥与模型重定向',
    'Exact requested-model rules are optional and take precedence over each client default. Providers and models are discovered from the proxy. Start a fresh conversation after changing model families.': '可按请求模型的精确 ID 配置规则，其优先级高于该客户端的默认路由。提供商及模型从代理服务动态获取。切换模型家族后，请新建会话。',
    'Manage client keys': '管理客户端密钥', 'Claude Code auto-mode compatibility': 'Claude Code 自动模式兼容性',
    'Server-side classifier eligibility requires genuine safeguard results from the upstream. Provider translation cannot create those results or guarantee no-charge classifiers. Local classifier fallback stays enabled.': '服务端分类器资格需要上游返回真实的安全检查结果。提供商协议转换无法生成这些结果，也不能保证分类器免费；客户端分类器回退仍保持启用。',
    'Anthropic gateway compatibility details': 'Anthropic 网关兼容性说明',
    'Routing policy': '路由策略',
    'Fields, limits and evaluated rules come from the plugin’s versioned schema.': '字段、取值范围及规则解释均来自插件当前版本的配置定义。',
    'Advanced settings': '高级设置', 'Rule precedence': '规则优先级',
    'Availability and protection are checked before ranking and cache affinity.': '先检查账号可用性与保护规则，再进行调度排序和缓存亲和性判断。',
    'Check route availability': '检查路由可用性',
    'Explain an effective model and its eligible accounts without inference or forced quota refresh.': '查看最终使用的模型及符合条件的账号，不发起模型推理，也不强制刷新配额。',
    'Client key': '客户端密钥', 'Requested model': '请求模型', 'Check route': '检查路由',
    'Prompt cache and routing evidence': '提示词缓存与路由依据',
    'Measured provider token usage. Cache-read share is not subscription billing weight.': '提供商实际报告的 Token 用量。缓存读取占比不等于订阅额度的计费权重。',
    'Recent decisions': '最近的路由决策', 'Time': '时间', 'Model': '模型', 'Decision': '决策原因',
    'Engine parameters': '引擎参数', 'Read-only parameters supplied by this plugin version.': '以下为插件当前版本提供的只读参数。',
    'Settings saved': '设置已保存', 'Saved policy changes apply to new requests.': '保存后的策略变更对新请求生效。',
    'Save changes': '保存更改', 'Ready to connect.': '等待连接。', 'Account details': '账号详情', 'Close account details': '关闭账号详情', 'Close': '关闭',
    'Unsaved changes': '有未保存的更改', 'Not started': '尚未开始',
    'Available': '可用', 'Protected / unavailable': '受保护／不可用', 'Disabled': '已禁用', 'Provider managed': '由提供商管理',
    'Enter the management key.': '请输入管理密钥。',
    'The proxy returned an unexpected response.': '代理服务返回了无法识别的响应。',
    'Request failed ({status}).': '请求失败（HTTP {status}）。',
    'The request timed out. Try again later.': '请求超时，请稍后重试。',
    'Unable to reach the proxy. Check the connection and management URL.': '无法连接代理服务，请检查网络连接及管理 API 地址。',
    'Provider': '提供商', 'Advertised models': '已公布的模型', 'None': '无', 'Last quota reading': '最近配额采集时间', 'Next due refresh': '下次计划刷新时间',
    'Refresh state': '刷新状态', 'Not loaded': '尚未加载', 'Configured reserves': '已配置的额度预留',
    '{five} five-hour / {weekly} weekly · {source}': '5 小时预留 {five}／每周预留 {weekly} · {source}',
    'Custom': '自定义', 'Defaults': '默认值', 'Owned account: exempt': '自有账号：豁免默认预留', 'Reserve protection disabled': '未开启预留保护',
    '{window} reserve': '{window}额度预留', 'Unlocks {date}': '将于 {date} 释放预留', 'Unlocked {date}': '已于 {date} 释放预留',
    'Provider credits': '提供商额外付费用量', 'Enabled at provider': '提供商侧已开启', 'Disabled at provider': '提供商侧已关闭', 'Unknown': '未知',
    'Numeric assessment and quota windows': '数值评估与配额窗口（原始 JSON）',
    'Accounts with available measured quota': '有可用实测配额的账号', 'Active requests': '正在处理的请求', 'Active client keys': '有效客户端密钥',
    ' · Owned': ' · 自有账号', '{count} active': '{count} 个进行中请求', 'Details': '详情', 'No matching accounts.': '没有符合条件的账号。',
    'Sun': '周日', 'Mon': '周一', 'Tue': '周二', 'Wed': '周三', 'Thu': '周四', 'Fri': '周五', 'Sat': '周六',
    '{title}: {day}': '{title}：{day}', 'Proxy time: {zone}': '代理服务器时区：{zone}', 'Effective values': '生效参数（原始 JSON）',
    'Measured input read from cache': '实测输入缓存读取占比', 'Cache-write tokens': '缓存写入 Token 数', 'Recorded account switches': '已记录的账号切换次数',
    'No eligible account': '没有符合条件的账号', 'Requested model only': '仅按请求模型检查', 'Choose a requested model': '选择请求模型',
    'Settings or active keys changed elsewhere. Reload before saving your edits.': '设置或有效密钥已在其他位置变更。请先重新加载，再编辑并保存。',
    'Last checked {date}': '最近检查：{date}', 'Check {field}.': '请检查“{field}”。',
    'Connected. Account status and policy are loaded from the proxy.': '已连接，已从代理服务加载账号状态和策略。',
    'Discard unsaved changes and reload settings?': '要放弃未保存的更改并重新加载设置吗？',
    'Loaded the saved policy.': '已加载保存的策略。', 'Saved. New requests use the updated policy.': '已保存，新请求将使用更新后的策略。',
    '{eligible} of {matching} matching accounts have usable cached quota': '匹配的 {matching} 个账号中，{eligible} 个账号的缓存配额可用',
    'Effective model: {model}': '最终使用的模型：{model}', 'Routing rule: {reason}': '路由规则：{reason}',
    'No runtime credential advertises this provider/model pair.': '运行时没有凭证公布支持此提供商／模型组合。',
    'Filter supported models': '筛选支持的模型', 'Filter models for {name}': '筛选 {name} 的模型',
    'Choose a model': '选择模型', 'Choose a provider first': '请先选择提供商', ' · selected': ' · 已选', ' · unavailable': ' · 不可用',
    'Choose a provider': '选择提供商', 'Target model': '目标模型', 'Target provider': '目标提供商',
    'Choose a provider and model for {name}.': '请为 {name} 选择提供商和模型。',
    'Label': '备注名称', 'Use requested model': '使用请求指定的模型', 'Override unmatched requests': '重定向未匹配规则的请求', 'Default routing': '默认路由',
    '{name} default': '{name} 的默认路由', 'Add model rule': '添加模型规则', 'Optional rules for specific requested models': '针对特定请求模型的可选规则',
    'Exact model rules take precedence over the default above. Unmatched models keep the default. Targets are resolved once, without chained remapping.': '精确模型规则优先于上方默认路由；未匹配的模型沿用默认路由。目标只解析一次，不进行链式重定向。',
    'Remove rule': '删除规则', 'Exact requested model ID': '请求模型的精确 ID', 'Search available requested models': '搜索可选的请求模型',
    'Filter requested models for {name} rule {number}': '筛选 {name} 的第 {number} 条规则的请求模型', ' · custom ID': ' · 自定义 ID',
    'Override this model': '将此模型重定向至目标', 'Use this requested model': '原样使用此请求模型', '{name} model rule': '{name} 的模型规则',
    'Requested model ID (optional)': '请求模型 ID（可手动填写）', 'Action': '处理方式',
    'Enter the exact requested model for {name}.': '请填写 {name} 的精确请求模型 ID。', 'Duplicate requested model {model} for {name}.': '{name} 的请求模型 {model} 存在重复规则。',
    'Check {field} for {name}.': '请检查 {name} 的“{field}”。', '{field} for {name}': '{name}：{field}', '{name} {field}: {day}': '{name}：{field}，{day}',
    'Provider reserve policies': '提供商额度预留策略',
    'Each provider has its own defaults and schedule. Only reported quota windows are used. Providers without a quota adapter keep native quota management.': '每个提供商使用独立的默认值和时间计划，仅作用于实际报告的配额窗口。没有配额适配器的提供商仍由原生机制管理配额。',
    ' · disconnected': ' · 未接入',
    'Quota-aware policy. Only windows reported for the effective model participate.': '基于配额的保护策略，仅使用最终模型实际报告的配额窗口。',
    'No normalized quota adapter is available. Ownership, model routing and native cooldowns still work; this plugin cannot promise percentage reserves.': '没有可用的标准化配额适配器。账号归属、模型路由及原生冷却仍有效，但插件无法保证按百分比预留额度。',
    'Protection and weekly release schedule': '保护时段与每周预留释放计划',
    'Disconnected credential': '未接入的凭证', 'Saved rules are retained for this credential ID.': '为此凭证 ID 保留已保存的规则。',
    'Ownership and reserves are explicit per credential.': '账号归属和预留规则按凭证分别设置。',
    'Use default ({ownership})': '沿用默认值（{ownership}）', 'owned': '自有', 'shared': '共享', 'Owned': '自有账号', 'Shared': '共享账号',
    'Ownership': '账号归属', 'Inherit provider defaults / ownership': '继承提供商默认值／账号归属规则', 'Custom five-hour / weekly reserves': '自定义 5 小时／每周预留',
    'Reserve settings': '额度预留设置', 'Other native quota windows': '其他原生配额窗口', ' · no current reading': ' · 暂无配额数据',
    'No other normalized quota windows have been reported.': '尚未报告其他标准化配额窗口。',
    'Shared account protection': '共享账号保护', 'Release schedule': '预留释放计划', 'Quota monitoring': '配额监控',
    'Ranking estimates': '调度排序估算', 'Client compatibility': '客户端兼容性', 'Billing safeguards': '付费保护',
    'Owned Claude accounts': '自有 Claude 账号',
    'Exempt from default reserves unless you set account-specific reserves. Ownership only breaks equal ranking scores.': '豁免默认预留，但仍遵守显式设置的账号预留。自有身份仅用于调度评分相同时的选择。',
    'Enable reserve protection': '启用额度预留保护',
    'Enable default and account-specific reserves. Owned accounts are exempt from defaults.': '启用默认及逐账号额度预留。自有账号仅豁免默认预留。',
    'Default weekly reserve': '默认每周剩余额度预留', 'Default five-hour reserve': '默认 5 小时剩余额度预留',
    'Weekly reserve': '每周剩余额度预留', 'Five-hour reserve': '5 小时剩余额度预留',
    'Percentage preserved until release when an account has no custom reserve.': '账号未设置自定义预留时，在释放时间之前保留的剩余额度百分比。',
    'Percentage preserved for a whole overlapping session when an account has no custom reserve.': '账号未设置自定义预留时，为与保护时段重叠的整个 5 小时窗口保留的剩余额度百分比。',
    'Percentage preserved until release.': '在释放时间之前保留的剩余额度百分比。',
    'Percentage preserved for a whole overlapping session.': '为与保护时段重叠的整个 5 小时窗口保留的剩余额度百分比。',
    'Weekly reserve release': '每周预留释放方式', 'Use a fixed lead time, or the final daytime window / preceding evening schedule.': '可选择固定提前时间，或根据最后一个日间窗口／前一晚的计划释放预留。',
    'Fixed lead time': '固定提前时间', 'Final daytime window / preceding evening': '最后一个日间窗口／前一晚',
    'Release lead time (hours)': '提前释放时间（小时）',
    'Fixed-hours mode: time before reset. Scheduled mode: final window for daytime resets on protected days.': '固定时间模式：在重置前多少小时释放。计划模式：保护日内日间重置前的最后一个窗口时长。',
    'Protected weekdays': '保护星期', 'Weekly daytime releases and five-hour session protection use these days.': '每周日间预留释放计划和 5 小时窗口保护均使用这些星期。',
    'Protected start hour': '保护开始小时', 'Local hour when protected time starts.': '按所选时区计算的保护开始小时。',
    'Protected end hour': '保护结束小时',
    'The end minute is included. An end hour of 20 covers through 20:00; night starts at 20:01.': '包含结束时刻所在的分钟。例如结束小时为 20 时，保护持续到 20:00 这一分钟，20:01 起视为夜间。',
    'Schedule time zone': '计划时区', 'IANA time zone, such as Asia/Karachi. Leave empty to use proxy system time.': 'IANA 时区名称，例如 Asia/Shanghai。留空则使用代理服务器系统时区。',
    'Quota polling interval (seconds)': '配额轮询间隔（秒）', 'Refresh due readings; provider backoff is always respected.': '刷新已到期的配额数据；始终遵守提供商的退避等待时间。',
    'Quota age upper bound (seconds)': '配额数据最大有效期（秒）',
    'Claude also uses the stricter polling interval plus grace. The effective limit is shown in evaluated rules.': 'Claude 还使用更严格的“轮询间隔加宽限时间”限制。实际生效值可在规则解释中查看。',
    'Initial weekly conversion': '每周额度初始换算系数',
    'Estimated five-hour allowances per weekly allowance. Valid paired samples learn a per-account conversion.': '估算一份每周额度相当于多少份 5 小时额度。系统会依据有效的配对样本学习各账号的换算系数。',
    'Full-history Claude requests': 'Claude 请求携带完整历史',
    'Negotiate account-local Message Threads off. Clients must handle the unsupported-beta retry; normal prompt caching remains.': '协商关闭绑定单个账号的 Message Threads 功能。客户端须能处理不支持该测试功能时的重试；普通提示词缓存不受影响。',
    'Require provider credits disabled': '要求关闭提供商额外付费用量',
    'Strict mode excludes Claude accounts whose credits are enabled or unconfirmed. Off allows best-effort included-quota routing; zero paid spillover cannot be guaranteed.': '严格模式排除额外付费用量已开启或状态未确认的 Claude 账号。关闭此要求后仅尽力使用订阅内额度，无法保证不产生额外费用。',
    'Record sanitized diagnostics': '记录脱敏诊断日志', 'Record private routing, quota and cache evidence without prompts or credentials.': '记录私有路由、配额及缓存依据，不记录提示词或凭证。',
    'Legacy Claude owner emails': '旧版 Claude 自有账号邮箱',
    'Existing email-based Claude ownership. For every provider, use the account ownership selector below. Explicit account choices take precedence.': '兼容旧版按邮箱识别 Claude 自有账号的设置。所有提供商均建议使用下方的逐账号归属选择器，显式账号设置优先。',
    'Remaining reserve': '剩余额度预留', 'Percentage to keep for this native quota window.': '为此原生配额窗口保留的剩余额度百分比。',
    'Release before reset (minutes)': '重置前提前释放（分钟）', '0 always preserves the reserve; a positive value unlocks it before a confirmed reset.': '0 表示始终保留预留额度；正数表示在已确认的重置时间之前提前释放。',
    'Accounts and reserves': '账号与额度预留',
    'Ownership works for every connected provider. Defaults and schedules are provider-specific. Reserve controls require reported quota; unknown limits remain provider-managed. Explicit account percentages override owned exemptions. Credential IDs keep rules attached across token refreshes.': '账号归属适用于所有已接入的提供商。默认值和时间计划按提供商区分；预留控制需要实际配额数据，未知限额仍由提供商管理。显式账号预留优先于自有账号默认豁免。规则绑定凭证 ID，不受令牌刷新影响。',
    'Parallel request distribution': '并行请求分配', 'Eligible accounts across providers': '各提供商中符合条件的账号',
    'Spread concurrent work across the least-active eligible accounts before deadline ranking. Cache affinity yields when its account is busier. Native cooldowns and quota reserves still apply; no artificial concurrency quota is imposed.': '先向进行中请求较少的可用账号分散并行任务，再按额度截止时间排序。原账号较忙时可让出缓存亲和绑定。仍遵守原生冷却和额度预留，不另设人为并发配额。',
    'Client model rules': '客户端模型规则', 'All providers': '所有提供商',
    'Exact requested-model rules win over the client default; pass-through exceptions preserve the requested model. Overrides are resolved once.': '精确请求模型规则优先于客户端默认路由；直通例外保留原请求模型。重定向仅解析一次。',
    'Account ownership': '账号归属',
    'Explicit account ownership wins over legacy Claude email ownership. Ownership exempts provider defaults and only breaks equal ranking scores; explicit account reserves still apply.': '显式账号归属优先于旧版 Claude 邮箱归属规则。自有身份仅豁免提供商默认预留，并在评分相同时用于选择；显式账号预留仍然生效。',
    'Provider availability': '提供商可用性', 'All requests': '所有请求',
    'Enabled credentials must advertise the exact requested model and pass native cooldown checks.': '启用的凭证必须明确支持请求模型的精确 ID，并通过原生冷却检查。',
    'Included subscription usage': '订阅内额度使用',
    'Paid-only requests are rejected. Paid-use signals block an account until fresh usable allowance is confirmed in a newer window.': '拒绝仅可付费使用的请求。检测到额外付费用量后暂停该账号，直至新窗口的最新数据确认有可用订阅内额度。',
    'Quota freshness': '配额数据时效性', 'Claude default reserves': 'Claude 默认额度预留', 'Shared Claude': '共享 Claude 账号',
    'Account-specific reserves': '逐账号额度预留', 'Configured accounts with reported quota': '已配置且有配额数据的账号',
    'Provider-specific defaults and schedule apply only to reported quota windows, with explicit account overrides taking precedence.': '提供商默认值和时间计划仅作用于已报告的配额窗口，显式逐账号设置优先。',
    'Configured quota windows': '已配置的配额窗口',
    'Native daily/hourly or other limits keep their own identity and reset. Custom floors release only before a confirmed reset when configured.': '原生每日、每小时等限额保持各自的窗口身份和重置时间。自定义预留仅在设置了提前释放且重置时间已确认时释放。',
    'Weekly release schedule': '每周预留释放计划', 'Request admission': '请求准入控制',
    'Keep completed work reserved until a newer reading covers it. Response headers may lower cached headroom; old responses cannot raise it.': '已完成请求的预计消耗会继续占用额度，直到更新的配额数据覆盖该消耗。响应头可下调缓存中的可用空间，旧响应不能将其调高。',
    'Reset-aware ranking': '感知重置时间的调度排序', 'Eligible accounts': '符合条件的账号',
    'Rank usable weekly allowance against remaining five-hour opportunities. Ownership only breaks exactly equal scores; learned conversion replaces the initial estimate.': '根据可用每周额度及剩余 5 小时使用机会进行排序。自有身份只在评分完全相同时用于选择，学习得到的换算系数会替代初始估算。',
    'Cache affinity': '缓存亲和性', 'Similar-urgency conversations': '紧迫程度相近的会话',
    'Keep warm conversations on their account unless eligibility or materially greater expiry urgency requires a switch. Prompts and cache controls are preserved.': '尽量让缓存仍有效的会话留在原账号；账号不再符合条件或其他账号额度即将到期的紧迫程度明显更高时可切换。保留提示词及缓存控制设置。',
    'Provider translation does not guarantee Anthropic server-side safeguards or no-charge classifier eligibility.': '提供商协议转换不保证支持 Anthropic 服务端安全检查，也不保证具备免费分类器资格。',
    'Native Claude traffic preserves safeguards fields; eligibility still requires genuine upstream safeguard results.': '原生 Claude 流量保留安全检查字段；是否符合资格仍取决于上游真实的安全检查结果。',
    'Uses cached quota and model registration. Native model-specific cooldowns are checked by the host at selection; no inference or forced quota refresh is performed.': '使用缓存配额及模型注册信息。宿主在选择账号时检查模型级原生冷却；此检查不发起推理，也不强制刷新配额。',
    'requested model': '请求指定的模型', 'exact model pass-through': '精确模型规则：原样直通', 'exact client model override': '精确客户端模型规则：重定向', 'client default model override': '客户端默认路由：重定向',
    'The runtime provider registry is unavailable. Unmapped models cannot be selected for an override.': '运行时提供商注册表不可用，无法选择未映射的模型作为重定向目标。',
    'Disabled in proxy': '已在代理服务中禁用', 'No enabled models advertised by the proxy': '代理服务未公布任何已启用模型',
    'Provider/model eligibility is checked for each request': '每次请求都会检查提供商／模型是否可用',
    'Eligible above account-specific reserves': '可用，剩余额度高于账号自定义预留要求', 'Eligible above shared-account reserves': '可用，剩余额度高于共享账号预留要求', 'Eligible': '符合条件',
    'Host eligibility; no normalized quota available': '使用宿主可用性判断，暂无标准化配额数据', 'Quota reading is stale or unavailable': '配额数据已过期或不可用',
    'Shared reserve requires a recent quota reading': '共享账号预留需要近期的配额数据',
    'Configured quota window has no recent reading': '已配置的配额窗口缺少近期数据', 'Claude account requires complete five-hour and weekly quota': 'Claude 账号需要完整的 5 小时及每周配额数据',
    'Available included quota; no future weekly reset reported': '订阅内额度可用，但尚未报告未来的每周重置时间',
    'Available native quota windows; no future reset reported': '原生配额窗口可用，但尚未报告未来的重置时间',
    'Request admission state is unavailable': '请求准入状态不可用', 'Credit safety state is unavailable': '付费保护状态不可用',
    'Paid usage was detected; waiting for usable included quota in a newer window': '检测到额外付费用量，正在等待新窗口确认可用的订阅内额度',
    'A recent included quota and billing reading is required': '需要近期的订阅内额度及计费状态数据',
    'Extra usage setting is unknown; account is excluded': '额外付费用量设置未知，已排除此账号',
    'Extra usage is enabled at the provider; account is excluded': '提供商侧已开启额外付费用量，已排除此账号',
    'Quota temporarily unavailable; cached readings retained.': '配额暂时不可用，已保留缓存数据。',
    'Provider rate limited quota reads; refresh is paused.': '提供商限制了配额查询频率，已暂停刷新。',
    'Quota authentication rejected; check account sign-in.': '配额查询认证被拒绝，请检查账号登录状态。',
    'Quota unavailable.': '配额不可用。', 'quota refresh is still in progress': '配额刷新仍在进行中',
    'not_loaded': '尚未加载', 'cached': '已缓存', 'fresh': '有效', 'stale': '已过期', 'refreshing': '刷新中', 'backoff': '退避等待中',
    'invalid management URL': '管理 API 地址无效', 'no management request in progress': '当前没有可复用身份的管理请求', 'no active client key': '没有有效的客户端密钥',
    'Usage rate limited. Please try again later.': '用量受限，请稍后重试。',
    'affinity similar urgency': '紧迫程度相近，保持会话绑定', 'warm cache similar urgency': '紧迫程度相近，保持缓存亲和',
    'best deadline score': '额度截止时间综合评分最高', 'imminent five hour waste': '5 小时额度即将到期，优先避免浪费',
    'parallel work spread': '分散并行请求负载', 'previous account ineligible or unavailable': '原账号不再符合条件或已不可用',
    'material deadline advantage': '其他账号的额度截止紧迫程度明显更高', 'weekly deadline at risk': '每周额度可能来不及用完',
    'affinity less active': '原绑定账号较空闲', 'least active deadline score': '优先较空闲账号，再按截止时间评分',
    'Unable to load proxy state. Retry after the proxy finishes starting.': '无法加载代理状态，请等待代理服务启动完成后重试。',
    'Evaluation request is too large.': '路由检查请求过大。', 'Invalid evaluation request.': '路由检查请求无效。',
    'Settings are too large.': '设置数据过大。', 'Invalid settings JSON.': '设置 JSON 无效。', 'Unknown dashboard endpoint.': '未知的管理面板接口。',
    'Saved. New requests use these settings immediately. Start a fresh conversation after changing model families.': '已保存，新请求立即使用这些设置。切换模型家族后，请新建会话。',
    'A quota refresh is already running.': '已有配额刷新任务正在运行。',
    'Checking due quota readings. Fresh cached data and provider cooldowns are respected; repeated clicks do not force upstream requests.': '正在检查到期配额数据。仍遵守缓存有效期和提供商冷却，重复点击不会强制向上游发送查询。',
    'conflict: settings changed elsewhere; reload before saving': '保存冲突：设置已在其他位置更改，请重新加载后再保存',
    'too many accounts or routes': '账号或路由数量过多', 'enter valid owned-account email addresses': '请输入有效的自有账号邮箱地址',
    'enter plain owned-account email addresses': '请输入不带显示名称的自有账号邮箱地址',
    'unknown client name': '未知的客户端', 'client list changed; reload settings': '客户端列表已变更，请重新加载设置',
    'client list changed; reload before editing model rules': '客户端列表已变更，请重新加载后再编辑模型规则',
    'client labels must be at most 80 characters on one line': '客户端备注名称必须为单行，长度不超过 80 个字符',
    'quota age upper bound must be at least the polling interval': '配额数据最大有效期不得短于轮询间隔',
    'invalid weekly reserve release mode': '每周预留释放方式无效', 'invalid shared Claude reserves or protected hours': '共享账号额度预留或保护时段无效',
    'invalid or duplicate protected weekday': '保护星期无效或重复', 'unknown shared reserve timezone': '无法识别额度预留计划的时区',
    'account reserves require both five-hour and weekly values; remove the entry to inherit defaults': '账号预留必须同时填写 5 小时及每周数值；如需继承默认值，请移除此自定义规则',
    'too many account reserve rules': '账号预留规则过多', 'invalid account reserve identity': '账号预留规则的凭证 ID 无效',
    'unable to verify new account rules; reload after credentials finish loading': '无法验证新账号规则，请等待凭证加载完成后重新加载',
    'account rules must target connected credentials; reload the account list': '账号规则必须指向已接入的凭证，请重新加载账号列表',
    'too many subscription policy entries': '订阅策略条目过多', 'invalid ownership identity': '账号归属规则的凭证 ID 无效',
    'invalid provider policy; Claude uses its existing settings field': '提供商策略无效；Claude 必须使用现有的专用设置字段',
    'provider reserve settings must be an object': '提供商额度预留设置必须是对象', 'invalid account quota-window rules': '账号配额窗口规则无效',
    'quota-window rule does not match its group/window identity': '配额窗口规则与其分组／窗口标识不匹配',
    'use standard reserves for five-hour/weekly windows; credit balances cannot be reserved as subscription quota': '5 小时／每周窗口请使用标准额度预留；额外付费余额不能作为订阅配额预留',
    'invalid native quota-window reserve': '原生配额窗口的预留设置无效',
    'new window rules must reference a reported quota window; reload the account': '新窗口规则必须引用已报告的配额窗口，请重新加载账号',
    'unable to verify provider settings': '无法验证提供商设置', 'new provider policies must reference connected providers': '新提供商策略必须引用已接入的提供商',
    'too many client model rule sets': '客户端模型规则组过多', 'invalid client model rule set': '客户端模型规则组无效',
    'enter an exact requested model ID on one line': '请在一行内输入请求模型的精确 ID',
    'pass-through rules cannot also override a target': '原样直通规则不能同时指定重定向目标',
    'choose pass-through or a complete override target': '请选择原样直通，或完整填写重定向目标',
    'client key is no longer active': '客户端密钥已失效', 'choose a requested model to evaluate': '请选择要检查的请求模型',
    'runtime model registry is unavailable': '运行时模型注册表不可用',
    'credential unavailable': '凭证不可用', 'credential has no access token': '凭证缺少访问令牌',
    'Antigravity project identity unavailable': 'Antigravity 项目标识不可用', 'quota monitoring unsupported for this provider': '此提供商暂不支持配额监控',
    'invalid quota response': '配额响应无效', 'no recognized quota windows returned': '未返回可识别的配额窗口',
    'service temporarily unavailable; please try again later': '服务暂时不可用，请稍后重试',
    'requested model is currently unavailable': '请求的模型当前不可用',
    'five_hour': '5 小时', 'seven_day': '每周', 'weekly': '每周', 'monthly': '每月', 'daily': '每日', 'hourly': '每小时',
    'extra_usage': '额外付费用量', 'configured estimate': '按配置估算', 'initial estimate': '初始估算', 'learned': '学习所得'
  };
  // Only application-owned message formats are recognized. Captured model IDs,
  // account names, timestamps and unknown upstream text are never rewritten.
  const patterns = [
    [/^Blocked: (.+)$/s, m => '已阻止：' + translate(m[1])],
    [/^Provider cooldown until (.+)$/, m => '提供商冷却中，结束时间：' + m[1]],
    [/^Included (\S+) quota exhausted$/, m => '订阅内' + translate(m[1]) + '额度已耗尽'],
    [/^Projected (\S+) quota exhausted$/, m => '预计' + translate(m[1]) + '额度已耗尽'],
    [/^Included (\S+) next-request safety buffer$/, m => '已触及订阅内' + translate(m[1]) + '额度的下一请求安全缓冲'],
    [/^Shared (\S+) reserve and next-request buffer$/, m => '已触及' + translate(m[1]) + '额度预留及下一请求安全缓冲'],
    [/^Weekly deadline demand ([\d.]+); ([\d.]+) five-hour equivalents before (.+)$/, m => `每周额度截止需求系数 ${m[1]}；在 ${m[3]} 前有相当于 ${m[2]} 份 5 小时额度可用`],
    [/^Five-hour expiry: ([\d.]+)% usable capacity at risk in ([\d.]+) min$/, m => `5 小时额度即将到期：${m[2]} 分钟内有 ${m[1]}% 的可用额度可能作废`],
    [/^Native quota deadline: ([\d.]+)% usable before (.+)$/, m => `原生配额截止时间：${m[2]} 前有 ${m[1]}% 可用额度`],
    [/^Claude readings must be no older than (\d+) seconds\. Provider refresh backoff remains in force\.$/, m => `Claude 配额数据不得超过 ${m[1]} 秒，仍遵守提供商刷新退避。`],
    [/^Default reserves: ([\d.]+)% weekly and ([\d.]+)% five-hour on non-owned Claude accounts without a custom rule\. Five-hour protection covers the whole overlapping session\.$/, m => `未设自定义规则的非自有 Claude 账号默认预留每周总额度的 ${m[1]}% 和 5 小时总额度的 ${m[2]}%。5 小时保护覆盖与保护时段重叠的整个窗口。`],
    [/^Unlock the weekly reserve (\d+) hours before reset\.$/, m => `在每周额度重置前 ${m[1]} 小时释放预留。`],
    [/^Protected-day daytime resets unlock (\d+) hours before reset\. All other resets unlock at the preceding (\d+):00, including unprotected-day daytime resets\.$/, m => `保护日内日间重置时，提前 ${m[1]} 小时释放预留。其他重置均在此前最近的 ${m[2]}:00 释放，包括非保护日内的日间重置。`],
    [/^(\d+) custom account rules replace provider default percentages, including owned accounts when explicitly configured\. The provider policy must be enabled; its protection and release schedules still apply\.$/, m => `${m[1]} 条逐账号自定义规则覆盖提供商默认百分比；显式配置后，自有账号也遵守这些规则。须开启提供商保护，且仍遵守其保护时段和释放计划。`],
    [/^(.+) reserves$/, m => m[1] + ' 额度预留'],
    [/^account reserves must be between 0 and ([\d.]+)%$/, m => `账号预留必须在 0 到 ${m[1]}% 之间`],
    [/^(.+) is outside its allowed range$/, m => `“${translate(m[1])}”超出允许范围`],
    [/^(.+) has an unsupported value$/, m => `“${translate(m[1])}”的值不受支持`],
    [/^(.+) must be (integer|number|text|true or false|a list)$/, m => `“${translate(m[1])}”必须是${({integer:'整数',number:'数值',text:'文本','true or false':'布尔值（true 或 false）','a list':'列表'})[m[2]]}`],
    [/^(account_reserves|model_routes|account_ownership|provider_reserves|window_reserves) must be an object; use \{\} to remove rules$/, m => `${m[1]} 必须是对象；使用 {} 清除规则`],
    [/^model ("(?:[^"\\]|\\.)*") is not advertised by provider ("(?:[^"\\]|\\.)*"); reload the catalog or choose a supported pair$/, m => `提供商 ${m[2]} 未公布支持模型 ${m[1]}；请重新加载目录或选择支持的组合`],
    [/^quota request failed \(HTTP (\d+)\)$/, m => `配额查询失败（HTTP ${m[1]}）`],
    [/^local proxy HTTP (\d+)$/, m => `本地代理返回 HTTP ${m[1]}`]
  ];
  let preference = 'auto';
  try { const saved = localStorage.getItem(storageKey); if (['auto', 'en', 'zh-CN'].includes(saved)) preference = saved; } catch { /* Storage may be blocked in an embedded dashboard. */ }
  function browserLanguage() {
    for (const language of navigator.languages?.length ? navigator.languages : [navigator.language || 'en']) {
      if (/^zh(?:-|$)/i.test(language)) return 'zh-CN';
      if (/^en(?:-|$)/i.test(language)) return 'en';
    }
    return 'en';
  }
  function locale() { return preference === 'auto' ? browserLanguage() : preference; }
  function translate(source, values = {}) {
    if (source == null) return '';
    if (typeof source === 'object' && typeof source.translationKey === 'string') { values = source.values; source = source.translationKey; }
    source = String(source);
    let result = source;
    if (locale() === 'zh-CN') {
      if (Object.hasOwn(messages, source)) result = messages[source];
      else for (const [pattern, render] of patterns) { const match = pattern.exec(source); if (match) { result = render(match); break; } }
    }
    // Substitute once, so user-controlled braces cannot become new placeholders.
    return result.replace(/\{([a-zA-Z_]+)\}/g, (match, key) => Object.hasOwn(values, key) ? String(typeof values[key] === 'function' ? values[key]() : values[key]) : match);
  }
  const bindings = new Map(), textNodes = new WeakMap();
  function setText(element, source, values) {
    let text = textNodes.get(element);
    if (!text || text.parentNode !== element) { text = document.createTextNode(''); element.replaceChildren(text); textNodes.set(element, text); }
    bindings.set(text, { source, values }); text.nodeValue = translate(source, values);
    return element;
  }
  function attribute(element, name, source, values) {
    let binding = bindings.get(element);
    if (!binding || !binding.attributes) { binding = { attributes: new Map() }; bindings.set(element, binding); }
    binding.attributes.set(name, { source, values }); element.setAttribute(name, translate(source, values));
  }
  function refresh() {
    document.documentElement.lang = locale();
    for (const [target, binding] of bindings) {
      if (!target.isConnected) { bindings.delete(target); continue; }
      if (binding.attributes) for (const [name, value] of binding.attributes) target.setAttribute(name, translate(value.source, value.values));
      else target.nodeValue = translate(binding.source, binding.values);
    }
  }
  function localize(root) {
    root.querySelectorAll('[data-i18n]').forEach(element => setText(element, element.dataset.i18n));
    for (const name of ['aria-label', 'placeholder', 'title']) root.querySelectorAll('[data-i18n-' + name + ']').forEach(element => attribute(element, name, element.getAttribute('data-i18n-' + name)));
    refresh();
  }
  function setLanguage(value) {
    preference = ['auto', 'en', 'zh-CN'].includes(value) ? value : 'auto';
    try { localStorage.setItem(storageKey, preference); } catch { /* Language switching still works without persistence. */ }
    refresh(); window.dispatchEvent(new CustomEvent('team-routing-languagechange'));
  }
  function failure(translationKey, values = {}) {
    const result = new Error(translate(translationKey, values));
    result.localization = { translationKey, values };
    return result;
  }
  function error(message) {
    if (message?.localization) return message.localization;
    if (message?.name === 'TimeoutError' || message?.name === 'AbortError') return 'The request timed out. Try again later.';
    if (message instanceof TypeError && /fetch|network|load failed/i.test(message.message)) return 'Unable to reach the proxy. Check the connection and management URL.';
    return message?.message || String(message);
  }
  window.addEventListener('languagechange', () => { if (preference === 'auto') { refresh(); window.dispatchEvent(new CustomEvent('team-routing-languagechange')); } });
  return { t: translate, text: setText, attribute, refresh, localize, locale, preference: () => preference, setLanguage, error, failure };
})();
