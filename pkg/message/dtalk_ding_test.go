package message

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestParseUserIDs(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: []string{}},
		{name: "single", raw: "user1", want: []string{"user1"}},
		{name: "multiple", raw: "user1|user2|user3", want: []string{"user1", "user2", "user3"}},
		{name: "trim and dedupe", raw: " user1 | |user2|user1 ", want: []string{"user1", "user2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseUserIDs(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseUserIDs(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestBuildDingContent(t *testing.T) {
	cases := []struct {
		name  string
		title string
		text  string
		want  string
	}{
		{name: "title and text", title: "标题", text: "正文", want: "标题\n正文"},
		{name: "only text", title: "", text: "正文", want: "正文"},
		{name: "only title", title: "标题", text: "", want: "标题"},
		{name: "both empty", title: "", text: "", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildDingContent(tc.title, tc.text)
			if got != tc.want {
				t.Fatalf("BuildDingContent(%q, %q) = %q, want %q", tc.title, tc.text, got, tc.want)
			}
		})
	}
}

func TestDtalkDingSendSuccess(t *testing.T) {
	var tokenCalls, sendCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			tokenCalls++
			if r.Method != http.MethodPost {
				t.Errorf("token method = %s, want POST", r.Method)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode token body: %v", err)
			}
			if body["appKey"] != "client-id" || body["appSecret"] != "client-secret" {
				t.Errorf("unexpected token body: %#v", body)
			}
			_, _ = io.WriteString(w, `{"accessToken":"tok-123","expireIn":7200}`)
		case "/v1.0/robot/ding/send":
			sendCalls++
			if got := r.Header.Get("x-acs-dingtalk-access-token"); got != "tok-123" {
				t.Errorf("access token header = %q, want tok-123", got)
			}
			var body struct {
				RobotCode          string   `json:"robotCode"`
				RemindType         int      `json:"remindType"`
				ReceiverUserIdList []string `json:"receiverUserIdList"`
				Content            string   `json:"content"`
				CallVoice          string   `json:"callVoice"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode ding body: %v", err)
			}
			if body.RobotCode != "client-id" {
				t.Errorf("robotCode = %q, want fallback client-id", body.RobotCode)
			}
			if body.RemindType != 1 {
				t.Errorf("remindType = %d, want default 1", body.RemindType)
			}
			if want := []string{"user1", "user2"}; !reflect.DeepEqual(body.ReceiverUserIdList, want) {
				t.Errorf("receiverUserIdList = %#v, want %#v", body.ReceiverUserIdList, want)
			}
			if body.Content != "标题\n正文" {
				t.Errorf("content = %q, want %q", body.Content, "标题\n正文")
			}
			if body.CallVoice != "" {
				t.Errorf("callVoice = %q, want empty for app DING", body.CallVoice)
			}
			_, _ = io.WriteString(w, `{"openDingId":"ding-1"}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cli := DtalkDing{
		ClientId:     "client-id",
		ClientSecret: "client-secret",
		ApiHost:      server.URL,
	}
	res, err := cli.SendDing("user1| user2 |user1", BuildDingContent("标题", "正文"))
	if err != nil {
		t.Fatalf("SendDing returned error: %v", err)
	}
	if tokenCalls != 1 || sendCalls != 1 {
		t.Fatalf("calls: token=%d send=%d, want 1 and 1", tokenCalls, sendCalls)
	}
	if !strings.Contains(string(res), "ding-1") {
		t.Fatalf("response = %s, want openDingId", string(res))
	}
}

func TestDtalkDingSendPhoneDing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/oauth2/accessToken" {
			_, _ = io.WriteString(w, `{"accessToken":"tok-123","expireIn":7200}`)
			return
		}
		var body struct {
			RobotCode  string `json:"robotCode"`
			RemindType int    `json:"remindType"`
			CallVoice  string `json:"callVoice"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode ding body: %v", err)
		}
		if body.RobotCode != "robot-code" {
			t.Errorf("robotCode = %q, want robot-code", body.RobotCode)
		}
		if body.RemindType != 3 {
			t.Errorf("remindType = %d, want 3", body.RemindType)
		}
		if body.CallVoice != "Standard_Female_Voice" {
			t.Errorf("callVoice = %q, want Standard_Female_Voice", body.CallVoice)
		}
		_, _ = io.WriteString(w, `{"openDingId":"ding-2"}`)
	}))
	defer server.Close()

	cli := DtalkDing{
		ClientId:     "client-id",
		ClientSecret: "client-secret",
		RobotCode:    "robot-code",
		RemindType:   3,
		CallVoice:    "Standard_Female_Voice",
		ApiHost:      server.URL,
	}
	if _, err := cli.SendDing("user1", "hello"); err != nil {
		t.Fatalf("SendDing returned error: %v", err)
	}
}

func TestDtalkDingSendAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/oauth2/accessToken" {
			_, _ = io.WriteString(w, `{"accessToken":"tok-123","expireIn":7200}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":"invalid.chatbotId","message":"无效的机器人id","requestid":"req-1"}`)
	}))
	defer server.Close()

	cli := DtalkDing{
		ClientId:     "client-id",
		ClientSecret: "client-secret",
		ApiHost:      server.URL,
	}
	_, err := cli.SendDing("user1", "hello")
	if err == nil {
		t.Fatal("SendDing returned nil error, want API error")
	}
	if !strings.Contains(err.Error(), "invalid.chatbotId") {
		t.Fatalf("error = %v, want code invalid.chatbotId", err)
	}
}

func TestDtalkDingSendFailedList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/oauth2/accessToken" {
			_, _ = io.WriteString(w, `{"accessToken":"tok-123","expireIn":7200}`)
			return
		}
		_, _ = io.WriteString(w, `{"openDingId":"ding-1","failedList":{"无效用户":["user2"]}}`)
	}))
	defer server.Close()

	cli := DtalkDing{
		ClientId:     "client-id",
		ClientSecret: "client-secret",
		ApiHost:      server.URL,
	}
	_, err := cli.SendDing("user1|user2", "hello")
	if err == nil {
		t.Fatal("SendDing returned nil error, want partial failure")
	}
	if !strings.Contains(err.Error(), "无效用户") {
		t.Fatalf("error = %v, want failedList detail", err)
	}
}
