# Changelog

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
