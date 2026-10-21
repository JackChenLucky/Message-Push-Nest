package send_ins_service

import (
	"fmt"
	"strings"
	"testing"

	"message-nest/models"
)

func TestInsDtalkDingValidator(t *testing.T) {
	tooManyIDs := make([]string, 201)
	for i := range tooManyIDs {
		tooManyIDs[i] = fmt.Sprintf("user%d", i)
	}

	cases := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name:   "fixed receivers",
			config: `{"to_user":"user1|user2"}`,
		},
		{
			name:    "fixed empty",
			config:  `{"to_user":""}`,
			wantErr: "接收成员ID",
		},
		{
			name:   "dynamic empty",
			config: `{"to_user":"","allowMultiRecip":true}`,
		},
		{
			name:    "too many receivers",
			config:  fmt.Sprintf(`{"to_user":%q}`, strings.Join(tooManyIDs, "|")),
			wantErr: "200",
		},
		{
			name:    "invalid json",
			config:  `{`,
			wantErr: "反序列化失败",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			validator := &InsDtalkDingValidator{}
			msg, obj := validator.Validate(tc.config)

			if tc.wantErr == "" {
				if msg != "" {
					t.Fatalf("Validate() returned error: %s", msg)
				}
				if _, ok := obj.(models.InsDtalkDingConfig); !ok {
					t.Fatalf("Validate() returned %T, want models.InsDtalkDingConfig", obj)
				}
				return
			}

			if !strings.Contains(msg, tc.wantErr) {
				t.Fatalf("Validate() error = %q, want it to contain %q", msg, tc.wantErr)
			}
		})
	}
}
