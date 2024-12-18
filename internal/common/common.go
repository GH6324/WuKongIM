package common

import (
	"context"
	"time"

	"github.com/WuKongIM/WuKongIM/internal/errors"
	"github.com/WuKongIM/WuKongIM/internal/ingress"
	"github.com/WuKongIM/WuKongIM/internal/options"
	"github.com/WuKongIM/WuKongIM/internal/service"
	"github.com/WuKongIM/WuKongIM/internal/types"
	"github.com/WuKongIM/WuKongIM/pkg/wklog"
	wkproto "github.com/WuKongIM/WuKongIMGoProto"
	"go.uber.org/zap"
)

// CommonService 通用服务
type Service struct {
	client *ingress.Client
	wklog.Log
}

func NewService() *Service {

	return &Service{
		client: ingress.NewClient(),
		Log:    wklog.NewWKLog("common.Service"),
	}
}

// 获取或请求tag
// 先根据tagKey在本地取，如果没有，则去频道领导者节点取
// 取到后设置在本地缓存中
func (s *Service) GetOrRequestAndMakeTag(fakeChannelId string, channelType uint8, tagKey string) (*types.Tag, error) {

	realFakeChannelId := fakeChannelId
	if options.G.IsCmdChannel(fakeChannelId) {
		realFakeChannelId = options.G.CmdChannelConvertOrginalChannel(fakeChannelId)
	}

	var tag *types.Tag
	if tagKey == "" {
		tagKey = service.TagManager.GetChannelTag(realFakeChannelId, channelType)
	}
	if tagKey != "" {
		tag = service.TagManager.Get(tagKey)
	}
	if tag != nil {
		return tag, nil
	}
	if channelType == wkproto.ChannelTypePerson {
		return s.getOrMakePersonTag(realFakeChannelId)
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	leader, err := service.Cluster.LeaderOfChannel(timeoutCtx, realFakeChannelId, channelType)
	cancel()
	if err != nil {
		wklog.Error("GetOrRequestTag: getLeaderOfChannel failed", zap.Error(err), zap.String("channelId", realFakeChannelId), zap.Uint8("channelType", channelType))
		return nil, err
	}

	// tagKey在频道的领导节点是一定存在的，
	// 如果不存在可能就是失效了，这里直接忽略,只能等下条消息触发重构tag
	if leader.Id == options.G.Cluster.NodeId {
		wklog.Warn("tag not exist in leader node", zap.String("tagKey", tagKey), zap.String("fakeChannelId", realFakeChannelId), zap.Uint8("channelType", channelType))
		return nil, errors.TagNotExist(tagKey)
	}

	// 去领导节点请求
	tagResp, err := s.client.RequestTag(leader.Id, &ingress.TagReq{
		TagKey: tagKey,
		NodeId: options.G.Cluster.NodeId,
	})
	if err != nil {
		s.Error("GetOrRequestTag: get tag failed", zap.Error(err), zap.String("channelId", realFakeChannelId), zap.Uint8("channelType", channelType))
		return nil, err
	}

	tag, err = service.TagManager.MakeTagWithTagKey(tagKey, tagResp.Uids)
	if err != nil {
		s.Error("GetOrRequestTag: make tag failed", zap.Error(err))
		return nil, err
	}
	service.TagManager.SetChannelTag(realFakeChannelId, channelType, tagKey)
	return tag, nil
}

// 获取个人频道的投递tag
func (s *Service) getOrMakePersonTag(fakeChannelId string) (*types.Tag, error) {
	// 处理普通假个人频道
	u1, u2 := options.GetFromUIDAndToUIDWith(fakeChannelId)
	subscribers := []string{u1, u2}
	tag, err := service.TagManager.MakeTag(subscribers)
	if err != nil {
		return nil, err
	}
	service.TagManager.SetChannelTag(fakeChannelId, wkproto.ChannelTypePerson, tag.Key)
	return tag, nil
}
