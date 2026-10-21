package message

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"message-nest/models"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

const dtalkDingTokenSection = "dtalk_ding_token"

type dtalkDingTokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpireIn    int    `json:"expireIn"`
	Code        string `json:"code"`
	Message     string `json:"message"`
}

type dtalkDingSendResponse struct {
	OpenDingId string          `json:"openDingId"`
	FailedList json.RawMessage `json:"failedList"`
	Code       string          `json:"code"`
	Message    string          `json:"message"`
	RequestId  string          `json:"requestid"`
}

// DtalkDing 钉钉 DING 消息客户端（专业版/专属版专享接口）
type DtalkDing struct {
	WayID        string
	ClientId     string
	ClientSecret string
	RobotCode    string
	RemindType   int
	CallVoice    string
	ApiHost      string
	ProxyURL     string
}

type dtalkDingTokenCache struct {
	Token      string    `json:"token"`
	ExpiresAt  time.Time `json:"expires_at"`
	ClientID   string    `json:"client_id"`
	SecretHash string    `json:"secret_hash"`
}

// BuildDingContent 将标题和正文组合为 DING 的纯文本内容
func BuildDingContent(title, text string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return text
	}
	if strings.TrimSpace(text) == "" {
		return title
	}
	return title + "\n" + text
}

// parseUserIDs 解析用 | 分隔的接收成员 ID，去空并去重
func parseUserIDs(raw string) []string {
	seen := make(map[string]struct{})
	userIDs := make([]string, 0)
	for _, item := range strings.Split(raw, "|") {
		userID := strings.TrimSpace(item)
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		userIDs = append(userIDs, userID)
	}
	return userIDs
}

// GetToken 获取企业内部应用 access_token，优先使用数据库缓存
func (d *DtalkDing) GetToken() (string, error) {
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

func (d *DtalkDing) credentialFingerprint() string {
	sum := sha256.Sum256([]byte(d.ClientId + "\x00" + d.ClientSecret))
	return hex.EncodeToString(sum[:])
}

func (d *DtalkDing) getCachedToken() (string, bool) {
	if d.WayID == "" {
		return "", false
	}
	s, err := models.GetSettingByKey(dtalkDingTokenSection, d.WayID)
	if err != nil || s.Value == "" {
		return "", false
	}
	var cache dtalkDingTokenCache
	if err := json.Unmarshal([]byte(s.Value), &cache); err != nil {
		return "", false
	}
	// 凭据变更后必须重新获取 token，避免继续使用旧应用的凭证
	if cache.ClientID != d.ClientId || cache.SecretHash != d.credentialFingerprint() {
		return "", false
	}
	// 提前 5 分钟失效
	if time.Now().Add(5 * time.Minute).After(cache.ExpiresAt) {
		return "", false
	}
	return cache.Token, true
}

func (d *DtalkDing) saveCachedToken(token string, expiresIn int) {
	if d.WayID == "" {
		return
	}
	cache := dtalkDingTokenCache{
		Token:      token,
		ExpiresAt:  time.Now().Add(time.Duration(expiresIn) * time.Second),
		ClientID:   d.ClientId,
		SecretHash: d.credentialFingerprint(),
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}

	models.DeleteSettingByKey(dtalkDingTokenSection, d.WayID)
	models.AddOneSetting(models.Settings{
		Section: dtalkDingTokenSection,
		Key:     d.WayID,
		Value:   string(data),
	})
}

// requestNewToken 请求钉钉新版 OAuth2 接口获取企业内部应用 access_token
// POST https://api.dingtalk.com/v1.0/oauth2/accessToken
func (d *DtalkDing) requestNewToken() (string, int, error) {
	apiURL := fmt.Sprintf("%s/v1.0/oauth2/accessToken", d.apiHost())

	bodyMap := map[string]string{
		"appKey":    d.ClientId,
		"appSecret": d.ClientSecret,
	}
	b, err := json.Marshal(bodyMap)
	if err != nil {
		return "", 0, err
	}

	client := d.getHTTPClient()
	resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	var r dtalkDingTokenResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", 0, fmt.Errorf("dtalk ding gettoken response parse err: %s, body=%s", err, string(body))
	}

	if r.AccessToken == "" {
		return "", 0, fmt.Errorf("dtalk ding gettoken err: http=%d, code=%s, msg=%s", resp.StatusCode, r.Code, r.Message)
	}

	expiresIn := r.ExpireIn
	if expiresIn <= 0 {
		expiresIn = 7200
	}
	return r.AccessToken, expiresIn, nil
}

// SendDing 发送 DING 消息
// POST https://api.dingtalk.com/v1.0/robot/ding/send
func (d *DtalkDing) SendDing(toUser, content string) ([]byte, error) {
	userIDs := parseUserIDs(toUser)
	if len(userIDs) == 0 {
		return nil, errors.New("接收成员ID不能为空")
	}
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("DING消息内容为空")
	}

	remindType := d.RemindType
	if remindType == 0 {
		remindType = 1
	}
	if remindType < 1 || remindType > 3 {
		return nil, fmt.Errorf("无效的DING类型：%d，仅支持 1-应用内DING、2-短信DING、3-电话DING", remindType)
	}

	robotCode := strings.TrimSpace(d.RobotCode)
	if robotCode == "" {
		robotCode = d.ClientId
	}
	if robotCode == "" {
		return nil, errors.New("robotCode 不能为空")
	}

	token, err := d.GetToken()
	if err != nil {
		return nil, err
	}

	bodyMap := map[string]interface{}{
		"robotCode":          robotCode,
		"remindType":         remindType,
		"receiverUserIdList": userIDs,
		"content":            content,
	}
	if remindType == 3 {
		if voice := strings.TrimSpace(d.CallVoice); voice != "" {
			bodyMap["callVoice"] = voice
		}
	}

	b, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("%s/v1.0/robot/ding/send", d.apiHost())
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var r dtalkDingSendResponse
	parseErr := json.Unmarshal(body, &r)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parseErr == nil && (r.Code != "" || r.Message != "") {
			return body, fmt.Errorf("dtalk ding send http %d: code=%s, msg=%s", resp.StatusCode, r.Code, r.Message)
		}
		return body, fmt.Errorf("dtalk ding send http %d: %s", resp.StatusCode, string(body))
	}

	if parseErr != nil {
		return body, fmt.Errorf("dtalk ding send response parse err: %s, body=%s", parseErr, string(body))
	}
	if r.Code != "" {
		return body, fmt.Errorf("dtalk ding send err: code=%s, msg=%s", r.Code, r.Message)
	}

	failedList := strings.TrimSpace(string(r.FailedList))
	if failedList != "" && failedList != "null" && failedList != "{}" && failedList != "[]" {
		return body, fmt.Errorf("dtalk ding send partial failure: %s", failedList)
	}

	return body, nil
}

func (d *DtalkDing) apiHost() string {
	apiHost := strings.TrimRight(strings.TrimSpace(d.ApiHost), "/")
	if apiHost == "" {
		return "https://api.dingtalk.com"
	}
	return apiHost
}

// getHTTPClient 获取配置了代理的 HTTP 客户端
func (d *DtalkDing) getHTTPClient() *http.Client {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 自定义 API 地址优先级更高，通常用于自建代理服务器，此时不用本地代理
	if d.ProxyURL != "" && strings.TrimSpace(d.ApiHost) == "" {
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
func (d *DtalkDing) createSOCKS5Dialer(proxyURL *url.URL) (proxy.ContextDialer, error) {
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
