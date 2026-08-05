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
