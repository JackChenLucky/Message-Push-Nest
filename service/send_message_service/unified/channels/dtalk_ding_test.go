package channels

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"message-nest/models"
	"message-nest/pkg/constant"
	"message-nest/service/send_way_service"
)

func newDtalkDingTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = io.WriteString(w, `{"accessToken":"tok-123","expireIn":7200}`)
		case "/v1.0/robot/ding/send":
			var body struct {
				ReceiverUserIdList []string `json:"receiverUserIdList"`
				Content            string   `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode ding body: %v", err)
			}
			if len(body.ReceiverUserIdList) != 2 {
				t.Errorf("receiverUserIdList = %#v, want 2 receivers", body.ReceiverUserIdList)
			}
			if body.Content != "标题\n正文" {
				t.Errorf("content = %q, want title + text", body.Content)
			}
			_, _ = io.WriteString(w, `{"openDingId":"ding-1"}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
}

func TestDtalkDingChannelSendUnified(t *testing.T) {
	server := newDtalkDingTestServer(t)
	defer server.Close()

	channel := NewDtalkDingChannel()
	auth := &send_way_service.WayDetailDtalkDing{
		ClientId:     "client-id",
		ClientSecret: "client-secret",
		RemindType:   1,
		ApiHost:      server.URL,
	}
	ins := models.SendTasksIns{
		WayType:     constant.MessageTypeDtalkDing,
		ContentType: constant.FormatTypeText,
		Config:      `{"to_user":"user1|user2"}`,
	}
	content := &UnifiedMessageContent{
		Title: "标题",
		Text:  "正文",
	}

	res, errMsg := channel.SendUnified(auth, ins, content)
	if errMsg != "" {
		t.Fatalf("SendUnified() error = %q", errMsg)
	}
	if !strings.Contains(res, "openDingId") {
		t.Fatalf("SendUnified() response = %q, want openDingId", res)
	}
}

func TestDtalkDingChannelSendUnifiedEmptyContent(t *testing.T) {
	channel := NewDtalkDingChannel()
	auth := &send_way_service.WayDetailDtalkDing{
		ClientId:     "client-id",
		ClientSecret: "client-secret",
		RemindType:   1,
	}
	ins := models.SendTasksIns{
		WayType:     constant.MessageTypeDtalkDing,
		ContentType: constant.FormatTypeText,
		Config:      `{"to_user":"user1"}`,
	}

	res, errMsg := channel.SendUnified(auth, ins, &UnifiedMessageContent{})
	if res != "" {
		t.Fatalf("SendUnified() response = %q, want empty on error", res)
	}
	if !strings.Contains(errMsg, "内容为空") {
		t.Fatalf("SendUnified() error = %q, want empty content error", errMsg)
	}
}
