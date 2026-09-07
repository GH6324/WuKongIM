//go:build e2e

package webhook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WuKongIM/WuKongIM/pkg/protocol/frame"
	"github.com/WuKongIM/WuKongIM/test/e2e/suite"
	"github.com/stretchr/testify/require"
)

const (
	webhookEventMsgNotify        = "msg.notify"
	webhookEventMsgOffline       = "msg.offline"
	webhookEventUserOnlineStatus = "user.onlinestatus"
)

func TestWukongIMWebhookReceivesNotifyOfflineAndOnlineStatus(t *testing.T) {
	sink := newWebhookSink(t)
	defer sink.close()

	node := suite.New(t).StartSingleNodeCluster(suite.WithNodeConfigOverrides(1, map[string]string{
		"WK_GATEWAY_TOKEN_AUTH_ON":                 "false",
		"WK_WEBHOOK_HTTP_ADDR":                     sink.URL(),
		"WK_WEBHOOK_QUEUE_SIZE":                    "64",
		"WK_WEBHOOK_WORKERS":                       "2",
		"WK_WEBHOOK_MSG_NOTIFY_BATCH_MAX_ITEMS":    "1",
		"WK_WEBHOOK_MSG_NOTIFY_BATCH_MAX_WAIT":     "1ms",
		"WK_WEBHOOK_ONLINE_STATUS_BATCH_MAX_ITEMS": "1",
		"WK_WEBHOOK_ONLINE_STATUS_BATCH_MAX_WAIT":  "1ms",
		"WK_WEBHOOK_OFFLINE_UID_BATCH_SIZE":        "32",
		"WK_WEBHOOK_REQUEST_TIMEOUT":               "2s",
		"WK_WEBHOOK_RETRY_MAX_ATTEMPTS":            "1",
	}))

	client, err := suite.NewWKProtoClient()
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	const (
		fromUID     = "webhooksender"
		toUID       = "webhookrecipient"
		clientSeq   = uint64(1)
		clientMsgNo = "webhook-e2e-msg-1"
		topic       = "webhook-topic"
		expire      = uint32(3600)
		payload     = "hello webhook e2e"
	)

	require.NoError(t, client.Connect(node.GatewayAddr(), fromUID, fromUID+"-device"), node.DumpDiagnostics())
	onlineReq := sink.requireEvent(t, webhookEventUserOnlineStatus, node, func(req webhookRequest) error {
		values, err := decodeOnlineStatus(req.Body)
		if err != nil {
			return err
		}
		for _, value := range values {
			parts := strings.Split(value, "-")
			if len(parts) == 6 && parts[0] == fromUID && parts[1] == "0" && parts[2] == "1" {
				return nil
			}
		}
		return fmt.Errorf("online status values = %#v, want %s-0-1-*", values, fromUID)
	})
	require.Equal(t, http.MethodPost, onlineReq.Method)

	require.NoError(t, client.SendFrame(&frame.SendPacket{
		Setting:     frame.SettingTopic,
		Expire:      expire,
		ClientSeq:   clientSeq,
		ClientMsgNo: clientMsgNo,
		ChannelID:   toUID,
		ChannelType: frame.ChannelTypePerson,
		Topic:       topic,
		Payload:     []byte(payload),
	}), node.DumpDiagnostics())

	sendack, err := client.ReadSendAck()
	require.NoError(t, err, node.DumpDiagnostics())
	require.Equal(t, frame.ReasonSuccess, sendack.ReasonCode, node.DumpDiagnostics())
	require.Equal(t, clientSeq, sendack.ClientSeq)
	require.Equal(t, clientMsgNo, sendack.ClientMsgNo)
	require.NotZero(t, sendack.MessageID)
	require.NotZero(t, sendack.MessageSeq)

	notifyReq := sink.requireEvent(t, webhookEventMsgNotify, node, func(req webhookRequest) error {
		messages, err := decodeNotify(req.Body)
		if err != nil {
			return err
		}
		for _, msg := range messages {
			if err := requireWebhookMessage(msg, sendack.MessageID, sendack.MessageSeq, fromUID, clientMsgNo, topic, expire, payload); err == nil {
				return nil
			}
		}
		return fmt.Errorf("notify messages = %#v, want committed sendack message", messages)
	})
	require.Equal(t, "application/json", notifyReq.ContentType)

	sink.requireEvent(t, webhookEventMsgOffline, node, func(req webhookRequest) error {
		offline, err := decodeOffline(req.Body)
		if err != nil {
			return err
		}
		if err := requireWebhookMessage(offline.webhookMessagePayload, sendack.MessageID, sendack.MessageSeq, fromUID, clientMsgNo, topic, expire, payload); err != nil {
			return err
		}
		if !containsString(offline.ToUIDs, toUID) {
			return fmt.Errorf("offline to_uids = %#v, want %s", offline.ToUIDs, toUID)
		}
		if offline.SourceID == 0 {
			return fmt.Errorf("offline source_id = 0, want sender node id")
		}
		return nil
	})
}

type webhookSink struct {
	server *httptest.Server
	mu     sync.Mutex
	events []webhookRequest
}

type webhookRequest struct {
	Method      string
	ContentType string
	Event       string
	Body        []byte
}

func newWebhookSink(t *testing.T) *webhookSink {
	t.Helper()
	sink := &webhookSink{}
	sink.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sink.mu.Lock()
		sink.events = append(sink.events, webhookRequest{
			Method:      r.Method,
			ContentType: r.Header.Get("Content-Type"),
			Event:       r.URL.Query().Get("event"),
			Body:        append([]byte(nil), body...),
		})
		sink.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	return sink
}

func (s *webhookSink) URL() string {
	if s == nil || s.server == nil {
		return ""
	}
	return s.server.URL + "/webhook"
}

func (s *webhookSink) close() {
	if s != nil && s.server != nil {
		s.server.Close()
	}
}

func (s *webhookSink) requireEvent(t *testing.T, event string, node *suite.StartedNode, check func(webhookRequest) error) webhookRequest {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		for _, req := range s.snapshot() {
			if req.Event != event {
				continue
			}
			if check == nil {
				return req
			}
			if err := check(req); err == nil {
				return req
			} else {
				lastErr = err
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("webhook event %q timed out: lastErr=%v captured=%s\n%s", event, lastErr, s.dump(), node.DumpDiagnostics())
	return webhookRequest{}
}

func (s *webhookSink) snapshot() []webhookRequest {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]webhookRequest, len(s.events))
	copy(out, s.events)
	return out
}

func (s *webhookSink) dump() string {
	var b strings.Builder
	for i, req := range s.snapshot() {
		fmt.Fprintf(&b, "\n[%d] method=%s event=%s content_type=%s body=%s", i, req.Method, req.Event, req.ContentType, strings.TrimSpace(string(req.Body)))
	}
	if b.Len() == 0 {
		return "<none>"
	}
	return b.String()
}

type webhookMessagePayload struct {
	Header struct {
		NoPersist uint8 `json:"no_persist"`
		RedDot    uint8 `json:"red_dot"`
		SyncOnce  uint8 `json:"sync_once"`
	} `json:"header"`
	Setting      uint8  `json:"setting"`
	Topic        string `json:"topic"`
	Expire       uint32 `json:"expire"`
	MessageID    uint64 `json:"message_id"`
	MessageIDStr string `json:"message_idstr"`
	ClientMsgNo  string `json:"client_msg_no"`
	MessageSeq   uint64 `json:"message_seq"`
	FromUID      string `json:"from_uid"`
	ChannelID    string `json:"channel_id"`
	ChannelType  uint8  `json:"channel_type"`
	Timestamp    int32  `json:"timestamp"`
	Payload      []byte `json:"payload"`
}

type webhookOfflinePayload struct {
	webhookMessagePayload
	ToUIDs   []string `json:"to_uids"`
	SourceID int64    `json:"source_id"`
}

func decodeOnlineStatus(body []byte) ([]string, error) {
	var values []string
	if err := json.Unmarshal(body, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func decodeNotify(body []byte) ([]webhookMessagePayload, error) {
	var messages []webhookMessagePayload
	if err := json.Unmarshal(body, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func decodeOffline(body []byte) (webhookOfflinePayload, error) {
	var offline webhookOfflinePayload
	if err := json.Unmarshal(body, &offline); err != nil {
		return webhookOfflinePayload{}, err
	}
	return offline, nil
}

func requireWebhookMessage(msg webhookMessagePayload, messageID int64, messageSeq uint64, fromUID, clientMsgNo, topic string, expire uint32, payload string) error {
	if msg.MessageID != uint64(messageID) {
		return fmt.Errorf("message_id = %d, want %d", msg.MessageID, messageID)
	}
	if msg.MessageIDStr != strconv.FormatInt(messageID, 10) {
		return fmt.Errorf("message_idstr = %q, want %d", msg.MessageIDStr, messageID)
	}
	if msg.MessageSeq != messageSeq {
		return fmt.Errorf("message_seq = %d, want %d", msg.MessageSeq, messageSeq)
	}
	if msg.FromUID != fromUID {
		return fmt.Errorf("from_uid = %q, want %q", msg.FromUID, fromUID)
	}
	if msg.ClientMsgNo != clientMsgNo {
		return fmt.Errorf("client_msg_no = %q, want %q", msg.ClientMsgNo, clientMsgNo)
	}
	if msg.Setting != frame.SettingTopic.Uint8() {
		return fmt.Errorf("setting = %d, want %d", msg.Setting, frame.SettingTopic.Uint8())
	}
	if msg.Topic != topic {
		return fmt.Errorf("topic = %q, want %q", msg.Topic, topic)
	}
	if msg.Expire != expire {
		return fmt.Errorf("expire = %d, want %d", msg.Expire, expire)
	}
	if msg.ChannelType != frame.ChannelTypePerson {
		return fmt.Errorf("channel_type = %d, want %d", msg.ChannelType, frame.ChannelTypePerson)
	}
	if string(msg.Payload) != payload {
		return fmt.Errorf("payload = %q, want %q", string(msg.Payload), payload)
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestBeforeSendWebhookThreeNodeAdmission(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("event") != "msg.before_send" {
			http.Error(w, "event", 400)
			return
		}
		var req struct {
			ClientMsgNo string `json:"client_msg_no"`
			Payload     []byte `json:"payload"`
			ChannelID   string `json:"channel_id"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "JSON", 400)
			return
		}
		mu.Lock()
		counts[req.ClientMsgNo]++
		mu.Unlock()
		switch string(req.Payload) {
		case "deny":
			_ = json.NewEncoder(w).Encode(map[string]any{"allow": false, "reason_code": 200})
		case "modify":
			_ = json.NewEncoder(w).Encode(map[string]any{"allow": true, "payload": base64.StdEncoding.EncodeToString([]byte("modified"))})
		case "timeout":
			<-r.Context().Done()
		case "http_error":
			http.Error(w, "unavailable", 503)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"allow": true})
		}
	}))
	defer sink.Close()
	options := []suite.Option{suite.WithManagerHTTP()}
	for id := uint64(1); id <= 3; id++ {
		timeoutPolicy, errorPolicy := "deny", "deny"
		if id == 2 {
			timeoutPolicy = "allow"
		}
		if id == 3 {
			errorPolicy = "allow"
		}
		options = append(options, suite.WithNodeConfigOverrides(id, map[string]string{
			"WK_GATEWAY_TOKEN_AUTH_ON": "false",
			"WK_PLUGIN_ENABLE":         "false", "WK_WEBHOOK_BEFORE_SEND_ENABLED": "true",
			"WK_WEBHOOK_BEFORE_SEND_HTTP_ADDR": sink.URL, "WK_WEBHOOK_BEFORE_SEND_TIMEOUT": "100ms",
			"WK_WEBHOOK_BEFORE_SEND_ON_TIMEOUT": timeoutPolicy, "WK_WEBHOOK_BEFORE_SEND_ON_ERROR": errorPolicy,
		}))
	}
	cluster := suite.New(t).StartThreeNodeCluster(options...)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	require.NoError(t, cluster.WaitClusterReady(ctx), cluster.DumpDiagnostics())
	_, err := cluster.WaitSlotLeadersStable(ctx, time.Second)
	require.NoError(t, err, cluster.DumpDiagnostics())
	const sender = "before-sender"
	const receiver = "before-receiver"
	const group = "before-send-group"
	require.NoError(t, suite.PostChannel(ctx, cluster.MustNode(1).APIAddr(), map[string]any{
		"channel_id": group, "channel_type": 2, "reset": 1, "subscribers": []string{sender, receiver},
	}), cluster.DumpDiagnostics())
	sendClient, err := suite.NewWKProtoClient()
	require.NoError(t, err)
	defer sendClient.Close()
	recvClient, err := suite.NewWKProtoClient()
	require.NoError(t, err)
	defer recvClient.Close()
	require.NoError(t, sendClient.Connect(cluster.MustNode(1).GatewayAddr(), sender, sender+"-device"))
	require.NoError(t, recvClient.Connect(cluster.MustNode(3).GatewayAddr(), receiver, receiver+"-device"))
	body := func(id, payload string) map[string]any {
		return map[string]any{"from_uid": sender, "channel_id": group, "channel_type": 2, "client_msg_no": id, "payload": base64.StdEncoding.EncodeToString([]byte(payload))}
	}
	warm, err := suite.PostMessageSendEventually(ctx, cluster.MustNode(1).APIAddr(), body("before-warm", "warm"))
	require.NoError(t, err, cluster.DumpDiagnostics())
	require.EqualValues(t, frame.ReasonSuccess, warm.Reason)
	_, err = recvClient.ReadRecv()
	require.NoError(t, err, cluster.DumpDiagnostics())
	for i, payload := range []string{"deny", "modify"} {
		id := "before-wk-" + payload
		require.NoError(t, sendClient.SendFrame(&frame.SendPacket{ClientSeq: uint64(i + 1), ClientMsgNo: id, ChannelID: group, ChannelType: 2, Payload: []byte(payload)}))
		ack, err := sendClient.ReadSendAck()
		require.NoError(t, err, cluster.DumpDiagnostics())
		if payload == "deny" {
			require.EqualValues(t, 200, ack.ReasonCode)
			require.Zero(t, ack.MessageID)
			require.Zero(t, ack.MessageSeq)
		} else {
			require.Equal(t, frame.ReasonSuccess, ack.ReasonCode)
			recv, err := recvClient.ReadRecv()
			require.NoError(t, err, cluster.DumpDiagnostics())
			require.Equal(t, id, recv.ClientMsgNo)
			require.Equal(t, "modified", string(recv.Payload))
		}
	}
	for _, tc := range []struct {
		node        uint64
		id, payload string
		reason      uint8
	}{
		{1, "before-http-deny", "deny", 200},
		{1, "before-timeout-deny", "timeout", uint8(frame.ReasonSystemError)},
		{2, "before-timeout-allow", "timeout", uint8(frame.ReasonSuccess)},
		{2, "before-error-deny", "http_error", uint8(frame.ReasonSystemError)},
		{3, "before-error-allow", "http_error", uint8(frame.ReasonSuccess)},
	} {
		result, err := suite.PostMessageSend(ctx, cluster.MustNode(tc.node).APIAddr(), body(tc.id, tc.payload))
		require.NoError(t, err, cluster.DumpDiagnostics())
		require.Equal(t, tc.reason, result.Reason, tc.id)
		if tc.reason == uint8(frame.ReasonSuccess) {
			recv, err := recvClient.ReadRecv()
			require.NoError(t, err, cluster.DumpDiagnostics())
			require.Equal(t, tc.id, recv.ClientMsgNo)
			require.Equal(t, tc.payload, string(recv.Payload))
		} else {
			require.Zero(t, result.MessageID)
			require.Zero(t, result.MessageSeq)
		}
	}
	transient := body("before-transient", "transient")
	transient["no_persist"] = 1
	transient["sync_once"] = 1
	delete(transient, "channel_id")
	delete(transient, "channel_type")
	transient["subscribers"] = []string{receiver}
	result, err := suite.PostMessageSend(ctx, cluster.MustNode(1).APIAddr(), transient)
	require.NoError(t, err, cluster.DumpDiagnostics())
	require.EqualValues(t, frame.ReasonSuccess, result.Reason)
	recv, err := recvClient.ReadRecv()
	require.NoError(t, err)
	require.Equal(t, "before-transient", recv.ClientMsgNo)
	var history struct {
		Messages []struct {
			ClientMsgNo string `json:"client_msg_no"`
			Payload     []byte `json:"payload"`
		} `json:"messages"`
	}
	_, err = suite.PostJSON(ctx, "http://"+cluster.MustNode(3).APIAddr()+"/channel/messagesync", map[string]any{
		"login_uid": receiver, "channel_id": group, "channel_type": 2, "start_message_seq": 1, "limit": 20, "pull_mode": 1,
	}, &history)
	require.NoError(t, err, cluster.DumpDiagnostics())
	require.Len(t, history.Messages, 4, "rejected and transient sends must not enter committed history")
	for _, msg := range history.Messages {
		require.NotContains(t, msg.ClientMsgNo, "deny")
		if msg.ClientMsgNo == "before-wk-modify" {
			require.Equal(t, "modified", string(msg.Payload))
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for _, id := range []string{"before-wk-deny", "before-wk-modify", "before-http-deny", "before-timeout-deny", "before-timeout-allow", "before-error-deny", "before-error-allow", "before-transient"} {
		require.Equal(t, 1, counts[id], id+": authority forwarding must not repeat the callback")
	}
}
