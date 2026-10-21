package channels

import (
	"fmt"
	"message-nest/models"
	"message-nest/pkg/message"
	"message-nest/service/send_ins_service"
	"message-nest/service/send_way_service"
	"strings"
)

type DtalkDingChannel struct{ *BaseChannel }

func NewDtalkDingChannel() *DtalkDingChannel {
	return &DtalkDingChannel{BaseChannel: NewBaseChannel(MessageTypeDtalkDing, []string{FormatTypeText})}
}

func (c *DtalkDingChannel) SendUnified(msgObj interface{}, ins models.SendTasksIns, content *UnifiedMessageContent) (string, string) {
	auth, ok := msgObj.(*send_way_service.WayDetailDtalkDing)
	if !ok {
		return "", "类型转换失败"
	}

	insService := send_ins_service.SendTaskInsService{}
	errStr, configInterface := insService.ValidateDiffIns(ins)
	if errStr != "" {
		return "", errStr
	}
	config, ok := configInterface.(models.InsDtalkDingConfig)
	if !ok {
		return "", "钉钉DING config校验失败"
	}

	contentType, formattedContent, err := c.FormatContent(content)
	if err != nil {
		return "", err.Error()
	}
	if contentType != FormatTypeText {
		return "", fmt.Sprintf("未知的钉钉DING发送内容类型：%s", contentType)
	}
	if strings.TrimSpace(formattedContent) == "" {
		return "", "DING消息内容为空"
	}

	cli := message.DtalkDing{
		WayID:        ins.WayID,
		ClientId:     auth.ClientId,
		ClientSecret: auth.ClientSecret,
		RobotCode:    auth.RobotCode,
		RemindType:   auth.RemindType,
		CallVoice:    auth.CallVoice,
		ApiHost:      auth.ApiHost,
		ProxyURL:     auth.ProxyURL,
	}

	res, err := cli.SendDing(config.ToUser, message.BuildDingContent(content.Title, formattedContent))
	if err != nil {
		return "", fmt.Sprintf("发送失败：%s", err)
	}
	return string(res), ""
}
