package ingress

import (
	"errors"

	"github.com/WuKongIM/WuKongIM/internal/options"
	"github.com/WuKongIM/WuKongIM/internal/service"
	"github.com/WuKongIM/WuKongIM/pkg/wklog"
	"github.com/WuKongIM/WuKongIM/pkg/wkserver"
	"github.com/WuKongIM/WuKongIM/pkg/wkserver/proto"
	"github.com/WuKongIM/WuKongIM/pkg/wkutil"
	wkproto "github.com/WuKongIM/WuKongIMGoProto"
	"go.uber.org/zap"
)

type Ingress struct {
	wklog.Log
}

func New() *Ingress {

	return &Ingress{
		Log: wklog.NewWKLog("Ingress"),
	}
}

func (i *Ingress) SetRoutes() {
	// 获取tag
	service.Cluster.Route("/wk/ingress/getTag", i.handleGetTag)
	// 判断接受者是否允许发送消息
	service.Cluster.Route("/wk/ingress/allowSend", i.handleAllowSend)
	// 更新tag
	service.Cluster.Route("/wk/ingress/updateTag", i.handleUpdateTag)

}

func (i *Ingress) handleGetTag(c *wkserver.Context) {
	req := &TagReq{}
	err := req.decode(c.Body())
	if err != nil {
		i.Error("getTag decode err", zap.Error(err))
		c.WriteErr(err)
		return
	}

	if req.TagKey == "" && req.ChannelId == "" {
		i.Error("tagKey and channelId is nil", zap.Any("req", req))
		c.WriteErr(errors.New("tagKey is nil"))
		return
	}

	if req.TagKey == "" {
		req.TagKey = service.TagManager.GetChannelTag(req.ChannelId, req.ChannelType)
	}

	tag := service.TagManager.Get(req.TagKey)
	if tag == nil {
		i.Error("handleGetTag: tag not exist", zap.Error(err), zap.String("tagKey", req.TagKey))
		c.WriteErr(errors.New("handleGetTag: tag not exist"))
		return
	}
	var uids []string
	var resp *TagResp
	if req.NodeId != 0 {
		resp = &TagResp{
			TagKey: tag.Key,
			Uids:   tag.GetNodeUsers(req.NodeId),
		}
	} else {
		uids = tag.GetUsers()
		resp = &TagResp{
			TagKey: tag.Key,
			Uids:   uids,
		}
	}
	data, err := resp.encode()
	if err != nil {
		i.Error("tagResp encode failed", zap.Error(err))
		c.WriteErr(err)
		return
	}
	c.Write(data)
}

func (i *Ingress) handleAllowSend(ctx *wkserver.Context) {
	req := &AllowSendReq{}
	err := req.decode(ctx.Body())
	if err != nil {
		i.Error("handleAllowSend Unmarshal err", zap.Error(err))
		ctx.WriteErr(err)
		return
	}

	reasonCode, err := service.AllowSendForPerson(req.From, req.To)
	if err != nil {
		i.Error("handleAllowSend: allowSend failed", zap.Error(err))
		ctx.WriteErr(err)
		return
	}

	if reasonCode == wkproto.ReasonSuccess {
		ctx.WriteOk()
		return
	}
	ctx.WriteErrorAndStatus(errors.New("not allow send"), proto.Status(reasonCode))
}

func (i *Ingress) handleUpdateTag(c *wkserver.Context) {
	var req = &TagUpdateReq{}
	err := req.Decode(c.Body())
	if err != nil {
		i.Error("handleUpdateTag: decode failed", zap.Error(err))
		c.WriteErr(err)
		return
	}

	if req.TagKey == "" && req.ChannelId == "" {
		i.Error("tagKey and channelId is nil", zap.Any("req", req))
		c.WriteErr(errors.New("tagKey is nil"))
		return
	}
	if len(req.Uids) == 0 {
		i.Error("uids is nil", zap.Any("req", req))
		c.WriteErr(errors.New("uids is nil"))
		return
	}

	realFakeChannelId := req.ChannelId
	if options.G.IsCmdChannel(req.ChannelId) {
		realFakeChannelId = options.G.CmdChannelConvertOrginalChannel(req.ChannelId)
	}

	tagKey := req.TagKey
	if tagKey == "" {
		if req.ChannelId != "" {
			tagKey = service.TagManager.GetChannelTag(realFakeChannelId, req.ChannelType)
		}
	}
	if tagKey != "" {
		if service.TagManager.Exist(tagKey) {
			if req.Remove {
				err = service.TagManager.RemoveUsers(tagKey, req.Uids)
				if err != nil {
					i.Warn("handleUpdateTag: remove users failed", zap.Error(err))
				}
			} else {
				err = service.TagManager.AddUsers(tagKey, req.Uids)
				if err != nil {
					i.Warn("handleUpdateTag: add users failed", zap.Error(err))
				}
			}
			if req.ChannelTag {
				newTagKey := wkutil.GenUUID()
				err = service.TagManager.RenameTag(tagKey, newTagKey)
				if err != nil {
					i.Warn("handleUpdateTag: rename tag failed", zap.Error(err))
				}
				service.TagManager.SetChannelTag(realFakeChannelId, req.ChannelType, newTagKey)
			}

		} else {
			_, err = service.TagManager.MakeTagWithTagKey(tagKey, req.Uids)
			if err != nil {
				i.Error("handleUpdateTag: make tag failed", zap.Error(err))
				c.WriteErr(err)
				return
			}
		}
	}
	c.WriteOk()

}