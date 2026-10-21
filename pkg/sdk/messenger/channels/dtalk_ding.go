package channels

import (
	"message-nest/pkg/message"
	"strconv"
	"strings"
)

type DtalkDingSDKChannel struct{ *BaseChannel }

func NewDtalkDingSDKChannel() Channel {
	return &DtalkDingSDKChannel{NewBaseChannel(ChannelDtalkDing, []string{FormatTypeText})}
}

func (c *DtalkDingSDKChannel) Send(config ChannelConfig, msg *Message) (*Result, error) {
	clientId := config.GetString("client_id")
	clientSecret := config.GetString("client_secret")
	if clientId == "" {
		return SendError("dtalk_ding config missing: client_id is required"), nil
	}
	if clientSecret == "" {
		return SendError("dtalk_ding config missing: client_secret is required"), nil
	}
	toUser := config.GetString("to_user")
	if strings.TrimSpace(toUser) == "" {
		return SendError("dtalk_ding config missing: to_user is required"), nil
	}

	remindType := 1
	if raw := strings.TrimSpace(config.GetString("remind_type")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 3 {
			return SendError("dtalk_ding config invalid: remind_type must be 1, 2 or 3"), nil
		}
		remindType = parsed
	}

	contentType, formattedContent := c.FormatContent(msg)
	if contentType != FormatTypeText || strings.TrimSpace(formattedContent) == "" {
		return SendError("dtalk_ding message content is empty"), nil
	}

	cli := message.DtalkDing{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		RobotCode:    config.GetString("robot_code"),
		RemindType:   remindType,
		CallVoice:    config.GetString("call_voice"),
		ApiHost:      config.GetString("api_host"),
		ProxyURL:     config.GetString("proxy_url"),
	}

	res, err := cli.SendDing(toUser, message.BuildDingContent(msg.Title, formattedContent))
	if err != nil {
		return ErrorResult(string(res), err), nil
	}
	return SuccessResult(string(res)), nil
}
