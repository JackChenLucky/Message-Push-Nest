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
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

type dtalkWorkSendResponse struct {
	ErrCode   int    `json:"errcode"`
	ErrMsg    string `json:"errmsg"`
	TaskId    int64  `json:"task_id"`
	RequestId string `json:"request_id"`
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
	// 如果有缓存且未过期，直接返回
	if token, ok := d.getCachedToken(); ok {
		return token, nil
	}

	// 否则，请求钉钉 API 获取
	token, expiresIn, err := d.requestNewToken()
	if err != nil {
		return "", err
	}

	// 保存到数据库缓存
	d.saveCachedToken(token, expiresIn)
	return token, nil
}

// getCachedToken 从数据库中获取缓存的 token
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
	// 提前 5 分钟失效
	if time.Now().Add(5 * time.Minute).After(cache.ExpiresAt) {
		return "", false
	}
	return cache.Token, true
}

// saveCachedToken 将 token 保存到数据库缓存
func (d *DtalkWork) saveCachedToken(token string, expiresIn int) {
	if d.WayID == "" {
		return
	}
	cache := dtalkWorkTokenCache{
		Token:     token,
		ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	bytes, err := json.Marshal(cache)
	if err != nil {
		return
	}

	models.DeleteSettingByKey("dtalk_work_token", d.WayID)
	models.AddOneSetting(models.Settings{
		Section: "dtalk_work_token",
		Key:     d.WayID,
		Value:   string(bytes),
	})
}

// requestNewToken 请求钉钉旧版接口获取企业内部应用的 access_token
// GET https://oapi.dingtalk.com/gettoken?appkey=APPKEY&appsecret=APPSECRET
func (d *DtalkWork) requestNewToken() (string, int, error) {
	apiHost := "https://oapi.dingtalk.com"
	if d.ApiHost != "" {
		apiHost = d.ApiHost
	}
	apiURL := fmt.Sprintf("%s/gettoken?appkey=%s&appsecret=%s", apiHost, d.ClientId, d.ClientSecret)

	client := d.getHTTPClient()
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	var r dtalkWorkTokenResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", 0, err
	}

	if r.ErrCode != 0 {
		return "", 0, fmt.Errorf("dtalk work gettoken err: %d - %s", r.ErrCode, r.ErrMsg)
	}

	if r.AccessToken == "" {
		return "", 0, errors.New("dtalk work gettoken returned empty token")
	}

	return r.AccessToken, r.ExpiresIn, nil
}

// SendMessage 发送工作通知消息 (异步发送)
// 使用旧版 API 端点: POST /topapi/message/corpconversation/asyncsend_v2
// 参考: https://open.dingtalk.com/document/orgapp/asynchronous-sending-of-enterprise-session-messages
func (d *DtalkWork) SendMessage(toUser, toDept string, toAll bool, msgType string, msgContent map[string]interface{}) ([]byte, error) {
	token, err := d.GetToken()
	if err != nil {
		return nil, err
	}

	apiHost := "https://oapi.dingtalk.com"
	if d.ApiHost != "" {
		apiHost = d.ApiHost
	}
	apiURL := fmt.Sprintf("%s/topapi/message/corpconversation/asyncsend_v2?access_token=%s", apiHost, token)

	agentId, _ := strconv.Atoi(d.AgentId)
	bodyMap := map[string]interface{}{
		"agent_id":    agentId,
		"to_all_user": toAll,
		"msg": map[string]interface{}{
			"msgtype": msgType,
			msgType:   msgContent,
		},
	}

	if toUser != "" {
		bodyMap["userid_list"] = toUser
	}
	if toDept != "" {
		bodyMap["dept_id_list"] = toDept
	}

	b, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}

	client := d.getHTTPClient()
	resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var r dtalkWorkSendResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return body, err
	}

	if r.ErrCode != 0 {
		return body, fmt.Errorf("dtalk work send err: %d - %s", r.ErrCode, r.ErrMsg)
	}

	return body, nil
}

// SendTextMessage 发送文本工作通知
func (d *DtalkWork) SendTextMessage(toUser, toDept string, toAll bool, text string) ([]byte, error) {
	content := map[string]interface{}{
		"content": text,
	}
	return d.SendMessage(toUser, toDept, toAll, "text", content)
}

// SendMarkdownMessage 发送 Markdown 工作通知
func (d *DtalkWork) SendMarkdownMessage(toUser, toDept string, toAll bool, title, text string) ([]byte, error) {
	content := map[string]interface{}{
		"title": title,
		"text":  text,
	}
	return d.SendMessage(toUser, toDept, toAll, "markdown", content)
}

// getHTTPClient 获取配置了代理的 HTTP 客户端
func (d *DtalkWork) getHTTPClient() *http.Client {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 自定义 API 地址优先级更高，通常用于自建代理服务器，此时不用本地代理
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

// createSOCKS5Dialer 创建 SOCKS5 代理拨号器
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
