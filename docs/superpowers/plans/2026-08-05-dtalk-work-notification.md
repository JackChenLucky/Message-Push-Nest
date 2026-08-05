# 钉钉工作通知推送渠道 (`DtalkWork`) 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 在 Message-Nest 中新增「钉钉工作通知」渠道 `DtalkWork`，使用钉钉新版 OAuth2 API，支持 text/markdown 格式发送和动态接收者群发。

**Architecture:** 完全参照 `QyWeiXinApp`（企业微信自建应用）模式实现，分层为：API 客户端层 (`pkg/message/dtalk_work.go`) → 统一渠道层 (`service/.../channels/dtalk_work.go`) → SDK 层 (`pkg/sdk/.../channels/dtalk_work.go`)，Token 缓存复用 settings 表机制，代理逻辑复用 `QyWeiXinApp` 的 SOCKS5/HTTP 代理模式。

**Tech Stack:** Go 1.x, GORM (settings 表缓存), Vue 3 + TypeScript (前端配置), gin (路由已有)

## Global Constraints

- 类型常量: `DtalkWork`（与现有 `Dtalk` 区分）
- Token 缓存: 数据库 settings 表，section=`dtalk_work_token`，提前 5 分钟刷新
- API 鉴权: 新版 OAuth2（`/v1.0/oauth2/accessToken`），使用 `x-acs-dingtalk-access-token` 头
- 消息格式: text、markdown
- 代理支持: HTTP/HTTPS/SOCKS5，与 QyWeiXinApp 一致
- 命名规范: 文件名 `dtalk_work.go`，结构体 `DtalkWork`，配置 `WayDetailDtalkWork`、`InsDtalkWorkConfig`

---

### Task 1: 添加核心常量定义

**Files:**
- Modify: `pkg/constant/constant.go:74`
- Modify: `service/send_message_service/unified/channels/types.go:13-27`
- Modify: `pkg/sdk/messenger/channels/types.go:62-75`

**Interfaces:**
- Produces: `constant.MessageTypeDtalkWork = "DtalkWork"`, `channels.MessageTypeDtalkWork`, `channels.ChannelDtalkWork`

- [ ] **Step 1: 在 `pkg/constant/constant.go` 添加常量**

在 `MessageTypeGotify` 行后添加：

```go
MessageTypeDtalkWork = "DtalkWork"
```

- [ ] **Step 2: 在 `service/send_message_service/unified/channels/types.go` 添加常量**

在 `MessageTypeGotify` 行后添加：

```go
MessageTypeDtalkWork = constant.MessageTypeDtalkWork
```

- [ ] **Step 3: 在 `pkg/sdk/messenger/channels/types.go` 添加常量**

在 `ChannelGotify` 行后添加：

```go
ChannelDtalkWork = "DtalkWork"
```

- [ ] **Step 4: 验证编译**

```bash
cd E:/code/Message-Push-Nest && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add pkg/constant/constant.go service/send_message_service/unified/channels/types.go pkg/sdk/messenger/channels/types.go
git commit -m "feat: add DtalkWork channel type constants"
```

---

### Task 2: 创建 API 客户端 `pkg/message/dtalk_work.go`

**Files:**
- Create: `pkg/message/dtalk_work.go`

**Interfaces:**
- Produces: `type DtalkWork struct { WayID, ClientId, ClientSecret, AgentId, ApiHost, ProxyURL string }`, `GetToken() (string, error)`, `SendTextMessage(toUser, toDept, toAll bool, text string) ([]byte, error)`, `SendMarkdownMessage(toUser, toDept, toAll bool, title, text string) ([]byte, error)`

- [ ] **Step 1: 创建文件并编写完整实现**

```go
package message

import (
    "bytes"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "message-nest/models"
    "net"
    "net/http"
    "net/url"
    "strconv"
    "strings"
    "time"

    "golang.org/x/net/proxy"
)

type dtalkWorkTokenResponse struct {
    AccessToken string `json:"accessToken"`
    ExpireIn    int    `json:"expireIn"`
    Code        string `json:"code"`
    Message     string `json:"message"`
}

type dtalkWorkSendResponse struct {
    RequestId string `json:"requestId"`
    Code      string `json:"code"`
    Message   string `json:"message"`
}

type DtalkWork struct {
    WayID        string
    ClientId     string
    ClientSecret string
    AgentId      string
    ApiHost      string
    ProxyURL     string
}

type dtalkWorkTokenCache struct {
    Token     string    `json:"token"`
    ExpiresAt time.Time `json:"expires_at"`
}

// GetToken 获取 access_token (优先从数据库 settings 表缓存获取)
func (d *DtalkWork) GetToken() (string, error) {
    if token, ok := d.getCachedToken(); ok {
        return token, nil
    }

    token, expiresIn, err := d.requestNewToken()
    if err != nil {
        return "", err
    }

    d.saveCachedToken(token, expiresIn)
    return token, nil
}

func (d *DtalkWork) getCachedToken() (string, bool) {
    if d.WayID == "" {
        return "", false
    }
    s, err := models.GetSettingByKey("dtalk_work_token", d.WayID)
    if err != nil || s.Value == "" {
        return "", false
    }
    var cache dtalkWorkTokenCache
    if err := json.Unmarshal([]byte(s.Value), &cache); err != nil {
        return "", false
    }
    if time.Now().Add(5 * time.Minute).After(cache.ExpiresAt) {
        return "", false
    }
    return cache.Token, true
}

func (d *DtalkWork) saveCachedToken(token string, expiresIn int) {
    if d.WayID == "" {
        return
    }
    cache := dtalkWorkTokenCache{
        Token:     token,
        ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
    }
    bytes_, err := json.Marshal(cache)
    if err != nil {
        return
    }

    models.DeleteSettingByKey("dtalk_work_token", d.WayID)
    models.AddOneSetting(models.Settings{
        Section: "dtalk_work_token",
        Key:     d.WayID,
        Value:   string(bytes_),
    })
}

func (d *DtalkWork) requestNewToken() (string, int, error) {
    apiHost := "https://api.dingtalk.com"
    if d.ApiHost != "" {
        apiHost = d.ApiHost
    }
    apiURL := fmt.Sprintf("%s/v1.0/oauth2/accessToken", apiHost)

    reqBody := map[string]string{
        "appKey":    d.ClientId,
        "appSecret": d.ClientSecret,
    }
    bodyBytes, _ := json.Marshal(reqBody)

    client := d.getHTTPClient()
    resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(bodyBytes))
    if err != nil {
        return "", 0, err
    }
    defer resp.Body.Close()

    rspBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", 0, err
    }

    var r dtalkWorkTokenResponse
    if err := json.Unmarshal(rspBody, &r); err != nil {
        return "", 0, err
    }

    if r.AccessToken == "" {
        return "", 0, fmt.Errorf("dtalk work gettoken err: code=%s, msg=%s", r.Code, r.Message)
    }

    return r.AccessToken, r.ExpireIn, nil
}

// SendMessage 发送统一包装消息
func (d *DtalkWork) SendMessage(toUser, toDept string, toAll bool, msgKey, msgParam string) ([]byte, error) {
    token, err := d.GetToken()
    if err != nil {
        return nil, err
    }

    apiHost := "https://api.dingtalk.com"
    if d.ApiHost != "" {
        apiHost = d.ApiHost
    }
    apiURL := fmt.Sprintf("%s/v1.0/robot/oToMessages/batchSend", apiHost)

    userIds := []string{}
    if toUser != "" {
        for _, u := range strings.Split(toUser, "|") {
            if trimmed := strings.TrimSpace(u); trimmed != "" {
                userIds = append(userIds, trimmed)
            }
        }
    }

    bodyMap := map[string]interface{}{
        "robotCode": d.ClientId,
        "userIds":   userIds,
        "msgKey":    msgKey,
        "msgParam":  msgParam,
    }

    b, err := json.Marshal(bodyMap)
    if err != nil {
        return nil, err
    }

    req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(b))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("x-acs-dingtalk-access-token", token)

    client := d.getHTTPClient()
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    rspBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }

    var r dtalkWorkSendResponse
    if err := json.Unmarshal(rspBody, &r); err != nil {
        return rspBody, err
    }

    if r.Code != "" && r.Code != "0" {
        return rspBody, fmt.Errorf("dtalk work send err: code=%s, msg=%s", r.Code, r.Message)
    }

    return rspBody, nil
}

func (d *DtalkWork) SendTextMessage(toUser, toDept string, toAll bool, text string) ([]byte, error) {
    msgParam := fmt.Sprintf(`{"content":"%s"}`, escapeJSON(text))
    return d.SendMessage(toUser, toDept, toAll, "sampleText", msgParam)
}

func (d *DtalkWork) SendMarkdownMessage(toUser, toDept string, toAll bool, title, text string) ([]byte, error) {
    msgParam := fmt.Sprintf(`{"title":"%s","text":"%s"}`, escapeJSON(title), escapeJSON(text))
    return d.SendMessage(toUser, toDept, toAll, "sampleMarkdown", msgParam)
}

func escapeJSON(s string) string {
    s = strings.ReplaceAll(s, `\`, `\\`)
    s = strings.ReplaceAll(s, `"`, `\"`)
    s = strings.ReplaceAll(s, "\n", `\n`)
    s = strings.ReplaceAll(s, "\r", `\r`)
    s = strings.ReplaceAll(s, "\t", `\t`)
    return s
}

func (d *DtalkWork) getHTTPClient() *http.Client {
    client := &http.Client{
        Timeout: 30 * time.Second,
    }

    if d.ProxyURL != "" && d.ApiHost == "" {
        proxyURL, err := url.Parse(d.ProxyURL)
        if err == nil {
            if strings.HasPrefix(strings.ToLower(d.ProxyURL), "socks5://") {
                dialer, err := d.createSOCKS5Dialer(proxyURL)
                if err == nil {
                    client.Transport = &http.Transport{
                        DialContext: dialer.DialContext,
                    }
                }
            } else {
                client.Transport = &http.Transport{
                    Proxy: http.ProxyURL(proxyURL),
                }
            }
        }
    }

    return client
}

func (d *DtalkWork) createSOCKS5Dialer(proxyURL *url.URL) (proxy.ContextDialer, error) {
    host := proxyURL.Host
    var auth *proxy.Auth
    if proxyURL.User != nil {
        password, _ := proxyURL.User.Password()
        auth = &proxy.Auth{
            User:     proxyURL.User.Username(),
            Password: password,
        }
    }

    baseDialer := &net.Dialer{
        Timeout:   30 * time.Second,
        KeepAlive: 30 * time.Second,
    }

    dialer, err := proxy.SOCKS5("tcp", host, auth, baseDialer)
    if err != nil {
        return nil, err
    }

    contextDialer, ok := dialer.(proxy.ContextDialer)
    if !ok {
        return nil, errors.New("failed to convert to ContextDialer")
    }

    return contextDialer, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd E:/code/Message-Push-Nest && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add pkg/message/dtalk_work.go
git commit -m "feat: add DtalkWork API client with OAuth2 token caching and proxy support"
```

---

### Task 3: 添加实例配置模型 `InsDtalkWorkConfig`

**Files:**
- Modify: `models/send_ins.go:78`

**Interfaces:**
- Produces: `type InsDtalkWorkConfig struct { ToUser, ToDept string; ToAll bool }`

- [ ] **Step 1: 在 `InsGotifyConfig` 之后添加新结构体**

在 `models/send_ins.go` 第 80 行（`InsGotifyConfig` 结束处）后添加：

```go
// InsDtalkWorkConfig 实例里面的钉钉工作通知config
type InsDtalkWorkConfig struct {
    ToUser string `json:"to_user" label:"接收成员ID"`
    ToDept string `json:"to_dept" label:"接收部门ID"`
    ToAll  bool   `json:"to_all" label:"发送给全部员工"`
}
```

- [ ] **Step 2: 验证编译**

```bash
cd E:/code/Message-Push-Nest && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add models/send_ins.go
git commit -m "feat: add InsDtalkWorkConfig model for DtalkWork channel"
```

---

### Task 4: 创建统一渠道实现

**Files:**
- Create: `service/send_message_service/unified/channels/dtalk_work.go`

**Interfaces:**
- Consumes: `MessageTypeDtalkWork`, `FormatTypeText`, `FormatTypeMarkdown` (from channels/types.go), `WayDetailDtalkWork` (from send_way_service), `InsDtalkWorkConfig` (from models)
- Produces: `DtalkWorkChannel` struct, `NewDtalkWorkChannel()`, `SendUnified(msgObj, ins, content) (string, string)`

- [ ] **Step 1: 创建渠道文件**

创建 `service/send_message_service/unified/channels/dtalk_work.go`：

```go
package channels

import (
    "fmt"
    "message-nest/models"
    "message-nest/pkg/message"
    "message-nest/service/send_ins_service"
    "message-nest/service/send_way_service"
)

type DtalkWorkChannel struct{ *BaseChannel }

func NewDtalkWorkChannel() *DtalkWorkChannel {
    return &DtalkWorkChannel{BaseChannel: NewBaseChannel(MessageTypeDtalkWork, []string{FormatTypeMarkdown, FormatTypeText})}
}

func (c *DtalkWorkChannel) SendUnified(msgObj interface{}, ins models.SendTasksIns, content *UnifiedMessageContent) (string, string) {
    auth, ok := msgObj.(*send_way_service.WayDetailDtalkWork)
    if !ok {
        return "", "类型转换失败"
    }

    insService := send_ins_service.SendTaskInsService{}
    errStr, configInterface := insService.ValidateDiffIns(ins)
    if errStr != "" {
        return errStr, ""
    }

    config, ok := configInterface.(models.InsDtalkWorkConfig)
    if !ok {
        return "钉钉工作通知 config 校验失败", ""
    }

    contentType, formattedContent, err := c.FormatContent(content)
    if err != nil {
        return "", err.Error()
    }

    cli := message.DtalkWork{
        WayID:        ins.WayID,
        ClientId:     auth.ClientId,
        ClientSecret: auth.ClientSecret,
        AgentId:      auth.AgentId,
        ApiHost:      auth.ApiHost,
        ProxyURL:     auth.ProxyURL,
    }

    var res []byte
    var errMsg string
    if contentType == FormatTypeText {
        res, err = cli.SendTextMessage(config.ToUser, config.ToDept, config.ToAll, formattedContent)
        if err != nil {
            errMsg = fmt.Sprintf("发送失败：%s", err.Error())
        }
    } else if contentType == FormatTypeMarkdown {
        res, err = cli.SendMarkdownMessage(config.ToUser, config.ToDept, config.ToAll, content.Title, formattedContent)
        if err != nil {
            errMsg = fmt.Sprintf("发送失败：%s", err.Error())
        }
    } else {
        errMsg = fmt.Sprintf("未知的钉钉工作通知发送内容类型：%s", contentType)
    }

    return string(res), errMsg
}
```

- [ ] **Step 2: 验证编译**

```bash
cd E:/code/Message-Push-Nest && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/send_message_service/unified/channels/dtalk_work.go
git commit -m "feat: add DtalkWorkChannel unified channel implementation"
```

---

### Task 5: 注册渠道到全局注册表

**Files:**
- Modify: `service/send_message_service/unified/channel_registry.go:113`

- [ ] **Step 1: 在 `GetGlobalChannelRegistry()` 中注册新渠道**

在 `channelRegistryOnce.Do(func() {` 块内，`NewGotifyChannel()` 注册行后添加：

```go
globalChannelRegistry.Register(channels.NewDtalkWorkChannel())
```

- [ ] **Step 2: 验证编译**

```bash
cd E:/code/Message-Push-Nest && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/send_message_service/unified/channel_registry.go
git commit -m "feat: register DtalkWorkChannel in global channel registry"
```

---

### Task 6: 添加渠道校验器、测试器和实例校验器

**Files:**
- Modify: `service/send_way_service/way_validator.go:37,53`
- Modify: `service/send_ins_service/ins_validator.go:30`

- [ ] **Step 1: 添加 `WayDetailDtalkWork` 结构体及 Validate/Test 方法**

在 `way_validator.go` 中 `WayDetailGotify` 的 Test 方法之后（第 432 行），添加：

```go
// WayDetailDtalkWork 钉钉工作通知渠道明细字段
type WayDetailDtalkWork struct {
    ClientId     string `json:"client_id" validate:"required,max=100" label:"钉钉应用ClientId"`
    ClientSecret string `json:"client_secret" validate:"required,max=100" label:"钉钉应用ClientSecret"`
    AgentId      string `json:"agent_id" validate:"required,max=50" label:"应用AgentId"`
    ApiHost      string `json:"api_host" validate:"max=200" label:"自定义API地址"`
    ProxyURL     string `json:"proxy_url" validate:"max=200" label:"代理地址"`
}

func (w *WayDetailDtalkWork) Validate(authJson string) (string, interface{}) {
    var empty interface{}
    err := json.Unmarshal([]byte(authJson), w)
    if err != nil {
        return "钉钉工作通知参数反序列化失败！", empty
    }
    _, msg := app.CommonPlaygroundValid(*w)
    return msg, w
}

func (w *WayDetailDtalkWork) Test() (string, string) {
    return "钉钉工作通知配置成功，请直接添加实例任务进行发送测试", ""
}
```

- [ ] **Step 2: 在 `validatorRegistry` 中注册新校验器**

在 `validatorRegistry` map 的 `MessageTypeGotify` 行后添加：

```go
constant.MessageTypeDtalkWork: func() WayValidator { return &WayDetailDtalkWork{} },
```

- [ ] **Step 3: 在 `testerRegistry` 中注册新测试器**

在 `testerRegistry` map 的 `MessageTypeGotify` 行后添加：

```go
constant.MessageTypeDtalkWork: func(m interface{}) WayTester { return m.(*WayDetailDtalkWork) },
```

- [ ] **Step 4: 添加实例校验器**

在 `ins_validator.go` 的 `InsGotifyValidator` 之后添加：

```go
// InsDtalkWorkValidator 钉钉工作通知配置校验
type InsDtalkWorkValidator struct{}

func (v *InsDtalkWorkValidator) Validate(configJson string) (string, interface{}) {
    var empty interface{}
    var Config models.InsDtalkWorkConfig
    err := json.Unmarshal([]byte(configJson), &Config)
    if err != nil {
        return "钉钉工作通知配置反序列化失败！", empty
    }

    // checkAllowMultiRecip 为真代表动态模式，to_user 可以为空
    if checkAllowMultiRecip(configJson) && Config.ToUser == "" {
        return "", Config
    }

    // 固定模式：验证 at least one receiver
    if Config.ToUser == "" && Config.ToDept == "" && !Config.ToAll {
        return "请输入接收成员ID、部门ID 或 勾选全部员工", empty
    }
    return "", Config
}
```

- [ ] **Step 5: 在 `insValidatorRegistry` 中注册实例校验器**

在 `insValidatorRegistry` map 的 `MessageTypeGotify` 行后添加：

```go
constant.MessageTypeDtalkWork: func() InsValidator { return &InsDtalkWorkValidator{} },
```

- [ ] **Step 6: 验证编译**

```bash
cd E:/code/Message-Push-Nest && go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add service/send_way_service/way_validator.go service/send_ins_service/ins_validator.go
git commit -m "feat: add DtalkWork way validator, tester, and ins validator"
```

---

### Task 7: 添加到群发支持列表

**Files:**
- Modify: `service/send_message_service/send_message.go:449,490-503`

- [ ] **Step 1: 在 `supportsDynamicRecipient` 中添加 `DtalkWork`**

在 `supportsDynamicRecipient` 函数中，`MessageTypeQyWeiXinApp` 行后添加：

```go
constant.MessageTypeDtalkWork: true,
```

- [ ] **Step 2: 在 `modifyInsRecipient` 中添加 `DtalkWork` case**

在 `modifyInsRecipient` 函数中，`else if wayType == constant.MessageTypeQyWeiXinApp` 块后添加：

```go
} else if wayType == constant.MessageTypeDtalkWork {
    config["to_user"] = recipient
```

- [ ] **Step 3: 验证编译**

```bash
cd E:/code/Message-Push-Nest && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add service/send_message_service/send_message.go
git commit -m "feat: add DtalkWork to dynamic recipient support"
```

---

### Task 8: 创建 SDK 层渠道

**Files:**
- Create: `pkg/sdk/messenger/channels/dtalk_work.go`

- [ ] **Step 1: 创建 SDK 渠道文件**

创建 `pkg/sdk/messenger/channels/dtalk_work.go`：

```go
package channels

import "message-nest/pkg/message"

type DtalkWorkSDKChannel struct{ *BaseChannel }

func NewDtalkWorkSDKChannel() Channel {
    return &DtalkWorkSDKChannel{NewBaseChannel(ChannelDtalkWork, []string{FormatTypeMarkdown, FormatTypeText})}
}

func (c *DtalkWorkSDKChannel) Send(config ChannelConfig, msg *Message) (*Result, error) {
    clientId := config.GetString("client_id")
    clientSecret := config.GetString("client_secret")
    agentId := config.GetString("agent_id")
    apiHost := config.GetString("api_host")
    proxyURL := config.GetString("proxy_url")

    if clientId == "" {
        return SendError("dtalk_work config missing: client_id is required"), nil
    }
    if clientSecret == "" {
        return SendError("dtalk_work config missing: client_secret is required"), nil
    }

    contentType, formattedContent := c.FormatContent(msg)

    cli := message.DtalkWork{
        ClientId:     clientId,
        ClientSecret: clientSecret,
        AgentId:      agentId,
        ApiHost:      apiHost,
        ProxyURL:     proxyURL,
    }

    toUser := config.GetString("to_user")
    toDept := config.GetString("to_dept")
    toAll := config.GetString("to_all") == "true"

    var res []byte
    var err error

    if contentType == FormatTypeText {
        res, err = cli.SendTextMessage(toUser, toDept, toAll, formattedContent)
    } else if contentType == FormatTypeMarkdown {
        res, err = cli.SendMarkdownMessage(toUser, toDept, toAll, msg.Title, formattedContent)
    } else {
        return SendError("未知的钉钉工作通知发送内容类型：%s", contentType), nil
    }

    if err != nil {
        return ErrorResult(string(res), err), nil
    }
    return SuccessResult(string(res)), nil
}
```

- [ ] **Step 2: 在 SDK messenger 中注册新渠道**

修改 `pkg/sdk/messenger/messenger.go` 的 `init()` 函数，在 `RegisterChannel(ChannelGotify, ...)` 行后添加：

```go
RegisterChannel(ChannelDtalkWork, func() Channel { return channels.NewDtalkWorkSDKChannel() })
```

- [ ] **Step 3: 验证编译**

```bash
cd E:/code/Message-Push-Nest && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add pkg/sdk/messenger/channels/dtalk_work.go pkg/sdk/messenger/messenger.go
git commit -m "feat: add DtalkWork SDK channel with messenger registration"
```

---

### Task 9: 添加前端配置

**Files:**
- Modify: `web/src/constant.js:329`

- [ ] **Step 1: 在 `WAYS_DATA` 数组末尾添加 DtalkWork 配置**

在 `Gotify` 配置块之后的 `]`（WAYS_DATA 结束处）前添加：

```javascript
{
    type: 'DtalkWork',
    label: '钉钉工作通知',
    // 动态接收者配置
    dynamicRecipient: {
        support: true,
        field: 'to_user',
        label: '接收成员ID',
        desc: '成员userId',
    },
    inputs: [
        { subLabel: '应用ClientId', value: '', col: 'client_id', desc: "钉钉应用 ClientId（AppKey）" },
        { subLabel: '应用ClientSecret', value: '', col: 'client_secret', desc: "钉钉应用 ClientSecret" },
        { subLabel: '应用AgentId', value: '', col: 'agent_id', desc: "应用 AgentId" },
        { subLabel: '自定义API地址', value: '', col: 'api_host', desc: "可选，自定义API地址，如 https://api.dingtalk.com" },
        { subLabel: '代理地址', value: '', col: 'proxy_url', desc: "可选，支持 HTTP/HTTPS/SOCKS5 代理" },
        { subLabel: '渠道名', value: '', col: 'name', desc: "想要设置的渠道名字" },
    ],
    tips: {
        text: "钉钉工作通知说明",
        desc: "使用钉钉工作通知发送消息。<br />1. 在钉钉开放平台创建企业内部应用获取 ClientId 和 ClientSecret。<br />2. 支持配置自定义 API 域名（如反向代理）。<br />3. 支持 HTTP/HTTPS/SOCKS5 代理。"
    },
    taskInsRadios: [
        { subLabel: 'text', content: 'text' },
        { subLabel: 'markdown', content: 'markdown' },
    ],
    taskInsInputs: [
        { value: '', col: 'to_user', desc: "接收成员ID (多个用 | 分隔)", label: '接收成员ID' },
        { value: '', col: 'to_dept', desc: "接收部门ID (多个用 | 分隔，可选)", label: '接收部门ID' },
        { value: '', col: 'to_all', desc: "发送给全部员工 (true/false，可选)", label: '全部员工' },
    ],
},
```

- [ ] **Step 2: 更新前端的渠道图标映射**

修改 `web/src/components/pages/sendWays/WaysForm.vue` 中 `getChannelIcon` 函数，在 `Gotify` case 后添加：

```typescript
'DtalkWork': Zap,
```

- [ ] **Step 3: 验证前端编译**

```bash
cd E:/code/Message-Push-Nest/web && npm run build 2>&1 | tail -20
```

- [ ] **Step 4: Commit**

```bash
git add web/src/constant.js web/src/components/pages/sendWays/WaysForm.vue
git commit -m "feat: add DtalkWork frontend channel configuration"
```

---

### Task 10: 整体验证

- [ ] **Step 1: 验证 Go 编译通过**

```bash
cd E:/code/Message-Push-Nest && go build ./...
```
期望: 无错误

- [ ] **Step 2: 验证前端编译通过**

```bash
cd E:/code/Message-Push-Nest/web && npm run build 2>&1 | tail -20
```
期望: 构建成功

- [ ] **Step 3: 运行已有测试（如有）**

```bash
cd E:/code/Message-Push-Nest && go test ./... 2>&1
```

- [ ] **Step 4: 人工验证检查点**

- 启动服务后，在「发送渠道」页面可看到「钉钉工作通知」卡片
- 可成功添加/编辑钉钉工作通知渠道
- 在模板或任务的实例配置中可搜索到该渠道并添加实例
- 实例配置可填写 to_user / to_dept / to_all
- 支持动态接收者模式切换
