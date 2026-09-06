package api

import (
	"net/http"
	"strings"

	cmdsyncusecase "github.com/WuKongIM/WuKongIM/internal/usecase/cmdsync"
	messageusecase "github.com/WuKongIM/WuKongIM/internal/usecase/message"
	"github.com/gin-gonic/gin"
)

type messageSyncRequest struct {
	UID        string `json:"uid"`
	MessageSeq uint64 `json:"message_seq"`
	Limit      int    `json:"limit"`
}

type messageSyncAckRequest struct {
	UID            string `json:"uid"`
	LastMessageSeq uint64 `json:"last_message_seq"`
}

type messageCMDBindingRequest struct {
	UID         string `json:"uid"`
	ChannelID   string `json:"channel_id"`
	ChannelType uint8  `json:"channel_type"`
}

func (s *Server) handleMessageSync(c *gin.Context) {
	var req messageSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeJSONError(c, "数据格式有误！")
		return
	}
	uid := strings.TrimSpace(req.UID)
	if uid == "" {
		writeJSONError(c, "uid不能为空！")
		return
	}
	if req.Limit < 0 {
		writeJSONError(c, "limit不能为负数！")
		return
	}
	if s == nil || s.cmdSync == nil {
		writeJSONError(c, "cmd sync usecase not configured")
		return
	}

	result, err := s.cmdSync.Sync(c.Request.Context(), cmdsyncusecase.SyncQuery{
		UID:        uid,
		MessageSeq: req.MessageSeq,
		Limit:      req.Limit,
	})
	if err != nil {
		writeJSONError(c, err.Error())
		return
	}

	resp := make([]legacyMessageResp, 0, len(result.Messages))
	for _, msg := range result.Messages {
		resp = append(resp, newLegacyMessageResp(uid, cmdSyncMessageToLegacy(msg)))
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleMessageSyncAck(c *gin.Context) {
	var req messageSyncAckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeJSONError(c, "数据格式有误！")
		return
	}
	uid := strings.TrimSpace(req.UID)
	if uid == "" {
		writeJSONError(c, "uid不能为空！")
		return
	}
	if req.LastMessageSeq == 0 {
		writeJSONError(c, "last_message_seq不能为空！")
		return
	}
	if s == nil || s.cmdSync == nil {
		writeJSONError(c, "cmd sync usecase not configured")
		return
	}

	if err := s.cmdSync.SyncAck(c.Request.Context(), cmdsyncusecase.SyncAckCommand{UID: uid, LastMessageSeq: req.LastMessageSeq}); err != nil {
		writeJSONError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

func (s *Server) handleMessageCMDBind(c *gin.Context) {
	req, ok := s.bindMessageCMDBinding(c)
	if !ok {
		return
	}
	if err := s.cmdSync.Bind(c.Request.Context(), cmdsyncusecase.BindCommand{
		UID: req.UID, ChannelID: req.ChannelID, ChannelType: req.ChannelType,
	}); err != nil {
		writeJSONError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

func (s *Server) handleMessageCMDUnbind(c *gin.Context) {
	req, ok := s.bindMessageCMDBinding(c)
	if !ok {
		return
	}
	if err := s.cmdSync.Unbind(c.Request.Context(), cmdsyncusecase.UnbindCommand{
		UID: req.UID, ChannelID: req.ChannelID, ChannelType: req.ChannelType,
	}); err != nil {
		writeJSONError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

func (s *Server) bindMessageCMDBinding(c *gin.Context) (messageCMDBindingRequest, bool) {
	var req messageCMDBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeJSONError(c, "数据格式有误！")
		return messageCMDBindingRequest{}, false
	}
	req.UID = strings.TrimSpace(req.UID)
	req.ChannelID = strings.TrimSpace(req.ChannelID)
	if req.UID == "" || req.ChannelID == "" || req.ChannelType == 0 {
		writeJSONError(c, "uid、channel_id和channel_type不能为空！")
		return messageCMDBindingRequest{}, false
	}
	if s == nil || s.cmdSync == nil {
		writeJSONError(c, "cmd sync usecase not configured")
		return messageCMDBindingRequest{}, false
	}
	return req, true
}

func cmdSyncMessageToLegacy(msg cmdsyncusecase.SyncedMessage) messageusecase.SyncedMessage {
	timestamp := msg.Protocol.Timestamp
	if timestamp == 0 && msg.ServerTimestampMS != 0 {
		timestamp = int32(msg.ServerTimestampMS / 1000)
	}
	return messageusecase.SyncedMessage{
		Setting: msg.Setting, Topic: msg.Protocol.Topic, Expire: msg.Protocol.Expire,
		Flags: messageusecase.MessageFlags{
			NoPersist: msg.Protocol.FramerFlags&1 != 0, RedDot: msg.Protocol.FramerFlags&2 != 0, SyncOnce: msg.SyncOnce,
		},
		MessageID:   msg.MessageID,
		ClientMsgNo: msg.ClientMsgNo,
		MessageSeq:  msg.MessageSeq,
		FromUID:     msg.FromUID,
		ChannelID:   msg.ChannelID,
		ChannelType: msg.ChannelType,
		Timestamp:   timestamp,
		Payload:     append([]byte(nil), msg.Payload...),
	}
}
