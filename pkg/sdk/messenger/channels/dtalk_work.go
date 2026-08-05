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
