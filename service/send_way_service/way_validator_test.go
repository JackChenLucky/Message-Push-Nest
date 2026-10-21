package send_way_service

import (
	"strings"
	"testing"
)

func TestWayDetailDtalkDingValidate(t *testing.T) {
	cases := []struct {
		name    string
		auth    string
		wantErr string
	}{
		{
			name: "valid",
			auth: `{"client_id":"client-id","client_secret":"client-secret","remind_type":1}`,
		},
		{
			name:    "missing client secret",
			auth:    `{"client_id":"client-id","remind_type":1}`,
			wantErr: "ClientSecret",
		},
		{
			name:    "invalid remind type",
			auth:    `{"client_id":"client-id","client_secret":"client-secret","remind_type":4}`,
			wantErr: "DING类型",
		},
		{
			name:    "invalid json",
			auth:    `{`,
			wantErr: "反序列化失败",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detail := &WayDetailDtalkDing{}
			msg, obj := detail.Validate(tc.auth)

			if tc.wantErr == "" {
				if msg != "" {
					t.Fatalf("Validate() returned error: %s", msg)
				}
				if _, ok := obj.(*WayDetailDtalkDing); !ok {
					t.Fatalf("Validate() returned %T, want *WayDetailDtalkDing", obj)
				}
				return
			}

			if !strings.Contains(msg, tc.wantErr) {
				t.Fatalf("Validate() error = %q, want it to contain %q", msg, tc.wantErr)
			}
		})
	}
}
