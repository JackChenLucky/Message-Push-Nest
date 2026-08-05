# 钉钉工作通知推送渠道设计

**日期**: 2026-08-05
**状态**: 待实现

## 概述

在现有 Message-Nest 消息推送平台中新增「钉钉工作通知」推送渠道，类型常量为 `DtalkWork`。该渠道使用钉钉开放平台新版 OAuth2 鉴权，通过企业自建应用向员工发送工作通知。

## 与现有钉钉机器人的区别

| 特性 | 钉钉机器人 (Dtalk) | 钉钉工作通知 (DtalkWork) |
|------|-------------------|-------------------------|
| 鉴权 | webhook access_token + secret 加签 | ClientId + ClientSecret (OAuth2) |
| 发送对象 | 群聊 | 指定员工/部门/全部 |
| API | `/robot/send` | `/v1.0/robot/oToMessages/batchSend` |
| 实例配置 | 无 | to_user / to_dept / to_all |

## 架构设计

完全参照 `QyWeiXinApp`（企业微信自建应用）的实现模式，包括：
- Token 缓存机制（数据库 settings 表，提前 5 分钟刷新）
- HTTP/SOCKS5 代理支持
- 动态接收者（群发）支持

## API 客户端

### 获取 AccessToken

```
POST https://api.dingtalk.com/v1.0/oauth2/accessToken
Body: { "appKey": "<clientId>", "appSecret": "<clientSecret>" }
Response: { "accessToken": "...", "expireIn": 7200 }
```

### 发送工作通知

```
POST https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend
Headers: { "x-acs-dingtalk-access-token": "<accessToken>" }
Body: {
  "userIds": [...],
  "msgKey": "sampleText" | "sampleMarkdown",
  "msgParam": "{ \"content\": \"...\" }"
}
```

## 渠道配置 (WayDetailDtalkWork)

| 字段 | JSON Key | 必填 | 说明 |
|------|----------|------|------|
| ClientId | `client_id` | 是 | 钉钉应用 ClientId（AppKey） |
| ClientSecret | `client_secret` | 是 | 钉钉应用 ClientSecret |
| AgentId | `agent_id` | 是 | 应用 AgentId |
| 自定义API | `api_host` | 否 | 自定义 API 地址 |
| 代理 | `proxy_url` | 否 | HTTP/HTTPS/SOCKS5 代理 |

## 实例配置 (InsDtalkWorkConfig)

| 字段 | JSON Key | 必填 | 说明 |
|------|----------|------|------|
| 接收成员 | `to_user` | 否 | 成员 userId，多个用 `\|` 分隔 |
| 接收部门 | `to_dept` | 否 | 部门 ID，多个用 `\|` 分隔 |
| 全部员工 | `to_all` | 否 | 布尔值，是否发送给全部员工 |

支持动态接收者模式（allowMultiRecip）。

## 支持的消息格式

- text（文本）
- markdown（Markdown）

## 需修改/新增的文件清单

### 新增文件
1. `pkg/message/dtalk_work.go` — 钉钉工作通知 API 客户端
2. `service/send_message_service/unified/channels/dtalk_work.go` — 统一渠道实现
3. `pkg/sdk/messenger/channels/dtalk_work.go` — SDK 层渠道

### 修改文件
4. `pkg/constant/constant.go` — 添加 `MessageTypeDtalkWork` 常量
5. `pkg/sdk/messenger/channels/types.go` — 添加 `ChannelDtalkWork` 常量
6. `service/send_message_service/unified/channels/types.go` — 添加常量重导出
7. `service/send_message_service/unified/channel_registry.go` — 注册新渠道
8. `service/send_way_service/way_validator.go` — 添加校验器和测试器
9. `service/send_ins_service/ins_validator.go` — 添加实例校验器
10. `models/send_ins.go` — 添加 `InsDtalkWorkConfig` 结构体
11. `service/send_message_service/send_message.go` — 添加到群发支持列表
12. `web/src/constant.js` — 前端 WAYS_DATA 配置

## 测试策略

- 渠道连接测试：通过测试按钮验证 ClientId/ClientSecret 能否成功获取 token
- 实例发送测试：通过创建任务/模板发送测试消息到指定员工

## 自检清单

- [x] 常量命名无歧义（DtalkWork vs Dtalk）
- [x] Token 缓存复用现有机制
- [x] 代理逻辑复用现有 SOCKS5/HTTP 代理
- [x] 动态接收者列表在 send_message.go 中注册
- [x] 前端 UI 配置完整
