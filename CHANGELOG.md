# Changelog

## 0.2.16 — 2026-11-27

### 品牌图标更新

- 插件主 Logo 与国内版登录图标改用全分辨率圆角版本（1960×1957），不再压缩成 1024 或 256 像素。
- 圆角弧度与原插画底部蓝弧一致，上下对称，四角透明，浅色和深色面板都不会出现白边。
- 国际版 Dola 图标保持不变。

### 商店首个公开版本

- 本次商店更新同时包含 0.2.15 的登录改动：二维码登录改为手机号、密码或短信验证码的账号登录，旧二维码页面已移除。

## 0.2.15 — 2026-11-25

### 登录不再出现二维码

- 主入口 `doubao`、`dola` 以及旧入口 `doubao-qr`、`dola-qr` 全部改为账号登录，不再请求或返回登录二维码。
- 移除 `/qr.html` 资源路由，旧链接也不会再渲染二维码页面。
- 选择“不自动打开”时，面板只返回账号登录页链接，不启动浏览器；用户点“打开”后，登录页会启动隔离浏览器，使用手机号、密码或短信验证码完成登录。
- 登录页新增 `account-launch` 资源接口，按已签名且 5 分钟有效的登录状态启动浏览器；重复点击不会重复打开窗口。
- 非 Windows 主机无法使用隔离浏览器读取 Cookie，登录会返回明确错误，不再回退到二维码。

### 升级说明

- 需要重启 CPA core 或重新加载插件后生效。
- 已有的“不自动打开”选择会保留，但结果从二维码页变为可手动打开的账号登录页。
- 已有账号凭据不需要重新登录。

## 0.2.13 — 2026-11-23

### 动态模型目录

- 新增服务端模型目录发现：通过账号凭证调用豆包客户端的 `POST /alice/slot/action_bar_v3/brief_list`，读取 `menu_conf_v2` 中的模式、模型、上下文窗口和推理强度配置。
- 目录按凭证缓存 10 分钟；请求失败时直接返回错误，不再回退静态文本模型；失败结果缓存 45 秒避免反复重试。
- `need_upgrade=true` 的模型不再注册，避免把未开通模型宣告为可用。
- 新增 `Qwen 4 Max`、`Qwen 3.8 Max`、`Qwen 3.8 Flash` 等由服务端下发的模型；后续新增模型不再需要改插件代码。
- 国内版不保留静态文本模型或兼容别名；请求的模型不在当前账号菜单中时直接返回 `400 model_unavailable`。Dola 仍使用其独立单模型入口。

### 思考强度与上下文

- 模型元数据改为使用服务端 `reasoning_effort_config.option_list` 和 `default_level`；不再把所有模型统一宣告为 `low/medium/high + 可关闭`。
- 请求中的 `reasoning_effort`、Responses 的 `reasoning.effort`、以及模型名后缀（如 `doubao-pro(high)`）现在会真正参与档位选择；不支持的档位回退到该模型默认档。
- 支持服务端下发的 `3=low / 4=medium / 5=high / 6=xhigh / 7=max`，并修正 Auto、Lite 等模型的默认档位。
- `ContextLength` 改为读取 `model_extra_params.total_window_size`；菜单不提供输出上限，因此不再设置 `MaxCompletionTokens` / `OutputTokenLimit`，输入上限等于总窗口。

### 兼容说明

- 动态目录不依赖 `a_bogus` 或外部签名器；只使用现有 Cookie 和公共请求参数。
- `model.static` 只保留视频模型；文本模型完全由 `model.for_auth` 的动态目录提供。

## 0.2.12 — 2026-11-18

### 模型信息

- 修正 Kimi K3.1 Pro、Kimi K3.1 Pro 高速、Kimi K3 的上下文长度为 300K，与豆包客户端实际下发的 `total_window_size=300000` 一致。此前目录写 256K，客户端会显示成 250K。
- DeepSeek V4.1 Flash、DeepSeek V4 Pro、GLM 5.3、GLM 5.3 Flash 保持 300K；豆包原生模型保持 256K。客户端按 1024 换算显示为 293K/250K 属于正常单位显示。

### 图标

- 插件主 Logo 与国内版登录入口图标替换为新版人物形象。
- Dola 国际版登录图标保持不变。

### 升级说明

- 重启 CPA core 或重新加载插件后模型目录会刷新，不需要重新登录。
- 面板从 GitHub main 读取图标；如果仍显示旧图，刷新面板或重启 core 后即可更新。

## 0.2.10 — 2026-11-18

### 模型 ID 前缀

- 插件暴露的模型 ID 现在默认带 `doujie/` 前缀（例如 `doujie/glm-5.3-flash`、`doujie/doubao-video`），避免和其他供应商的同名模型（`glm-5.3-flash`、`kimi-k3` 等）混在同一路由里。
- `/v1/models` 只注册带前缀的模型。请求侧仍兼容无前缀和旧的 `doubao/` 前缀，现有客户端不修改也能继续调用。
- 新增 `model_prefix` 配置项：默认 `doujie/`，可改成任意前缀（缺少结尾 `/` 时会自动补上）；设为 `none`、`-` 或 `off` 关闭前缀。前缀不能包含空白字符。

### 升级说明

- 升级后请重启 CPA core 或重新加载插件。
- 在客户端里固定填写模型名的，请把 `glm-5.3-flash` 这类名称改成 `doujie/glm-5.3-flash`；不改仍可调用，但和其他供应商同名模型的路由优先级不变。
- 不需要重新登录，现有 auth 文件和配额数据不受影响。

## 0.2.9 — 2026-11-17

### 模型选择与客户端一致

- 修正 Kimi K3.1 Pro、Kimi K3.1 Pro 高速、Kimi K3 和 DeepSeek V4 Pro 的 `model_item_key`。现在使用豆包工作模式菜单里的真实 key（`person-kimi-*-261111`、`person-deepseek-v4-pro-ga-260813`），不会再因为 key 不存在而静默回退到“自动/豆包”。
- 同步修正 Kimi 的上下文参数为 `memory_profile=300k_230`、`total_window_size=300000`，与客户端菜单一致。
- 未知的国内版模型现在返回 `400 model_unavailable`，不再把请求悄悄发送到默认模型。
- 模型元数据的输入上限改为“总上下文 - 最大输出”（例如 256k 窗口显示 224k 输入 + 32k 输出），避免客户端把输入和输出相加后提示超限。

### 普通对话不再被当成工作任务

- 对话模式（`mode_id=1`）不再携带 `general_task_param` / `agent_task_param`，也不会创建云电脑任务；会话保持普通聊天形态，默认机器人也会使用普通对话 bot，而不是工作任务 bot。
- 补齐 `STREAM_MSG_NOTIFY` 文本解析，普通对话的“收到”这类回复不会再被解析成空回答。

### 本地电脑工作模式

- 新增 `runtime`、`workspace`、`device_name`、`sandbox_auth_type`、`local_app_id`、`client_os` 配置。`runtime: local` 时按桌面客户端的真实结构发送 `runtime_type=2`、`local_device_id`、`local_app_id`、`workspace`、`sandbox_auth_type` 和 `client_option`，会话在豆包客户端显示为“本地电脑”。
- `runtime: auto` 在配置了 `workspace` 时使用本地电脑，否则使用云电脑。云电脑任务只发送 `{"runtime_type":1}`，不再附带空本地字段。
- 本地任务要求豆包桌面端正在运行且本地机器服务可用；如果桌面端未运行，请改回 `runtime: cloud`。

### 升级说明

- 现有 auth 文件不需要重新登录；升级后重启 CPA core 或重新加载插件即可。
- 需要本地电脑模式时，在插件配置中增加：

  ```yaml
  runtime: local
  workspace: D:\WenPaul\Documents
  sandbox_auth_type: ask_risky
  ```

- 只使用普通对话模型时，可以设置 `runtime: cloud`；对话模式不会再创建工作任务。

## 0.2.8 — 2026-11-16

### 模型选择真正生效

- 国内版文本模型现在按豆包客户端的模型菜单发送 `model_item_key`，不再只把模型名透传给上游。已覆盖自动、豆包 2.1 Lite、豆包 2.1 Pro，以及工作模式中的第三方模型。
- 有模型菜单映射的请求自动使用 `/chat/completion` 模型选择协议，并同时携带 `option.model_config`、`aggregate_params`、`conversation_init_ext` 和 `reasoning_effort`，避免出现“请求成功但实际仍由 Auto 回答”的情况。
- `doubao-pro` / `doubao-2.1-pro` 会进入豆包 2.1 Pro 的深度思考链路；`doubao-seed-2.0-lite` / `doubao-2.1-lite` 会映射到 `seed-lite-7b`。旧模型标识继续可用，不需要改客户端配置。

### 工作模式第三方模型

- 新增 DeepSeek V4.1 Flash、DeepSeek V4 Pro、Kimi K3.1 Pro、Kimi K3.1 Pro 高速、Kimi K3、GLM 5.3、GLM 5.3 Flash 的模型清单和路由映射。
- DeepSeek 和 GLM 使用豆包客户端的 `person-` 网关键，Kimi 使用 `kimi-` 前缀；第三方网关模型统一使用 `item_id=10001`，并沿用工作模式 `mode_id=3`、`agent_mode=1`、`conversation_mode=2`。
- 第三方模型是否出现在菜单、价格倍率和调用权限由豆包账号、套餐和服务端灰度决定。插件只做请求映射，不绕过上游的订阅、权限或额度校验。

### 账号能力与缺失模型

- Dola 国际版只展示与国内版兼容的 `doubao` 文本模型，避免把国内工作模式模型错误暴露给不支持的账号。
- 如果当前账号没有下发或没有开通某个模型，插件返回 `400 model_unavailable`，明确提示切换模型或先在豆包端开通；不再把这种情况误报成通用 `502 upstream_error`。
- 模型目录仍允许运营商通过宿主配置排除模型；实际可用性以当前登录账号的豆包菜单为准。

### 响应解析修复

- 修复深度思考响应中 `10040` 思考块之后的正文被错误归入 thinking 的解析问题，正文和思维链现在会分开输出。
- 补充模型选择、第三方模型目录、账号缺失模型和 Dola 目录过滤的测试。

### 升级说明

- 现有 auth 文件不需要重新登录，模型选择会在下一次请求时生效。
- 升级后需要重启 CPA core 或重新加载插件，新的插件版本才会被宿主载入。
- 如果账号看不到某个第三方模型，请以豆包客户端当前菜单为准；插件不会自动购买、开通或降级到未请求的模型。

## 0.2.7 — 2026-11-15

### 登录默认不弹浏览器

- `auto_open_browser` 默认值改为 `false`。点击登录默认只返回二维码/登录链接，不再由插件自行拉起浏览器。
- 只有显式设置 `auto_open_browser: true`、宿主传入 `auto_open=true`，或明确指定 `browser` 时才会打开浏览器。
- 修复在面板选择“不自动打开”后插件仍自动弹浏览器的问题。
- 不自动打开时返回可在浏览器打开的 `qr.html#...` 二维码页面链接，兼容只接受 http(s) 链接的面板。

### 升级说明

- 需要浏览器账号登录的用户，请在插件配置中设置 `auto_open_browser: true`。
- 只扫码登录的用户无需修改配置。
## 0.2.6 — 2026-11-15

### 账号归属规范化

- 凭据文件统一写入 `oauth_provider` 和账号标签 `email`：国内版为 `doubao`，国际版为 `dola`。
- 桌面端导入不再把 `doubao-desktop` 作为入口字段，统一归到国内版 `doubao`，原值保留在 `legacy_provider`。
- 旧凭据解析时自动归一化入口，避免国内版与 Dola 共用 `type: doubao` 时账号串卡。
- 扫码、浏览器登录、桌面导入等路径统一使用同一套入口映射。
- 管理面板可按入口正确显示“已有 N 个账号”并列出对应账号。

### 升级说明

- 建议升级；现有凭据无需手动迁移，加载解析时会自动归一化。
## 0.2.5 — 2026-11-15

### 登录体验

- 浏览器登录不再无条件自动弹出：新增 `auto_open_browser` 设置；设为 `false` 时不启动浏览器，改为返回扫码登录。
- 新增 `browser` 设置，可选 `auto`、`chrome`、`edge`，或浏览器可执行文件的绝对路径；默认仍跟随系统默认 Chromium。
- 如果宿主通过 OAuth metadata 传入 `auto_open`、`browser`、`browser_name`、`browser_path` 等字段，插件会优先遵循宿主设置。
- 未配置时保持原有行为：自动打开系统默认 Chromium，并使用隔离临时配置读取登录态。

### 升级说明

- 默认行为不变，现有部署无需改配置。
- 如果希望“只生成登录链接/扫码、不自动弹浏览器”，设置 `auto_open_browser: false`。
- 如果指定浏览器，设置 `browser: chrome`、`browser: edge` 或绝对路径。

## 0.2.4 — 2026-11-15

### 安全

- 外部签名服务不再收到 `msToken`、`sessionid` 等 Cookie 派生的查询参数；签名 URL 在发送给 signer 和上游前会统一剥离这些参数。
- Cookie 本身仍保留在本地请求头中，不会发送给外部 signer，账号登录态不受影响。
- 新增从 `buildRequest` 开始的端到端测试：使用合成 `msToken`，检查 signer 收到的 URL、请求头和请求体都不包含该 token。

### 升级说明

- 使用 `signer: external` 的部署建议升级。
- 外部 signer 仍会收到完整请求体，其中可能包含用户提示词；请只配置可信的 signer 服务。

### 发布流程

- 发版说明改为从 `CHANGELOG.md` 自动提取对应版本段落；缺少该段落或内容过短会让发布流水线失败。
- 后续每个版本都必须写用户能看懂的更新日志，不能只放 GitHub compare 链接。

## 0.2.3 — 2026-11-11

文档与发布整理，无运行时行为变更。

- 新增本更新日志，汇总 0.2.0–0.2.3 的功能、修复与安全变更。
- README / README.zh-CN 增加更新日志入口。
- 发布说明改为可读的变更列表，不再只有 compare 链接。

## 0.2.2 — 2026-11-11

### 视频任务隔离与额度冷却

- 每个 credential 增加独立视频任务租约：同一个 auth 同时只允许一个任务，第二个请求返回 `409 video_busy`。
- 提交前调用 `query_video_gen_info` 检查 `has_generating_task`，避免与浏览器或其他任务重复生成。
- 解析豆包 SSE 中的 `reset_at`，把额度错误转换为该账号的冷却；后续请求返回 `429` + `Retry-After`，不再重复打上游。
- 401/验证失败写入 10 分钟冷却。
- CPA host 会把 429 继续转换为账号/模型级 `model_cooldown`，第二次请求在 host 层直接被拒绝。

### 管理接口

- 新增认证后的管理路由：
  - `GET /v0/management/doujie-oauth/video/status`
  - `POST /v0/management/doujie-oauth/video/cooldowns/clear`
- 状态包含任务租约、阶段、开始时间、冷却剩余时间；支持按 `auth_id` 或全部清除。
- 公共资源路由仍严格静态，不承载动态状态。

### 安全与日志

- 上游错误信息中的 URL 统一脱敏，去掉 query、fragment、userinfo，避免签名 CDN 地址进入日志。
- 成功结果中的 MP4 地址保持完整返回给客户端。
- 新增任务租约、冷却、SSE reset 解析、管理接口和脱敏的单元测试。

## 0.2.1 — 2026-11-11

### 浏览器会话身份对齐

- 视频请求使用 `tea_uuid = web_id`，与网页端一致。
- 不再回放旧 `msToken`，避免浏览器导入会话被判定为验证挑战。
- `pc_version` / `doubao_pc_version` 固定为 `3.45.1`，`fp` 使用 `s_v_web_id`。

### 两轮确认生成

- 解析第一轮 SSE 的 `SSE_ACK`，提取 `conversation_id`、`last_section_id`、`msg_cursor`。
- 自动发送第二轮“确认生成”，请求体与网页端一致：`agent_mode=2`、`mode_id=1`、`model_item_key=0`、`need_create_conversation=false`，不再重复 `chat_ability`。
- 合并第二轮返回的 `video_model.video_list`、`vid`、`task_id`；必要时调用 `get_play_info`。

## 0.2.0 — 2026-11-11

### 安全修复（CPA 插件商店审计）

- 公共 `/v0/resource/plugins/` 只返回静态资源；移除动态 `/qr.png` 和带 token 的 `/manual.html` 重试页。
- 二维码改为认证接口 `auth.login.start` 返回 `data:image/png;base64,...`。
- 桌面端 Cookies 被占用时返回明确重试提示，不再暴露未认证的导入动作。
- 登录完成重定向限制为当前站点所属的豆包/Dola HTTPS 域，流程 Cookie 不发送到上游提供的任意 origin。
- 外部 signer 只接收 method、URL、body 和 `Content-Type` / `Accept` / `User-Agent` / `Agw-Js-Conv`；Cookie、Authorization、CSRF 不转发。
- `signer_url` 要求 HTTPS，或仅回环地址允许 HTTP。
- 增加静态资源、重定向白名单、signer 凭据剥离和传输校验测试。

### 视频协议

- 视频生成改为顶层 `chat_ability` 对象，`ability_type=17`。
- `ability_param` 使用 `ratio/model/duration/resolution/input_box_content`，`conversation_mode=1`，`mention_skill_list` / `mention_ext` 保持为空。
- 通过 `/samantha/skill/pack` 校验账号实际可用模型，不静默切换未请求模型。

## 0.1.x 基线

- 0.1.1：首次提交 CLIProxyAPI 插件商店 registry。
- 0.1.5–0.1.9：登录流程加固、桌面会话导入、浏览器账号登录、豆包视频模型与独立视频 Skill、每日桌面端 credit 状态检查。
- 独立视频 Skill 已拆分到 `wenbaolu6666-design/doubao-session-video`。
