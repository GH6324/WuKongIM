package channels

import (
	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	channeltransport "github.com/WuKongIM/WuKongIM/pkg/channel/transport"
)

// payloadHasMessageFlag checks bounded RPC payloads without allocation. Both
// native flags must survive transport; old codecs cannot represent either.
func payloadHasMessageFlag(payload any, syncOnce bool) bool {
	switch v := payload.(type) {
	case ch.AppendRequest:
		return messageHasFlag(v.Message, syncOnce)
	case ch.AppendBatchRequest:
		return messagesHaveFlag(v.Messages, syncOnce)
	case ch.AppendResult:
		return messageHasFlag(v.Message, syncOnce)
	case ch.AppendBatchResult:
		for _, item := range v.Items {
			if messageHasFlag(item.Message, syncOnce) {
				return true
			}
		}
	case channeltransport.PullResponse:
		for _, record := range v.Records {
			if (syncOnce && record.SyncOnce) || (!syncOnce && record.RedDot) {
				return true
			}
		}
	case channeltransport.PullBatchResponse:
		for _, item := range v.Items {
			if payloadHasMessageFlag(item.Response, syncOnce) {
				return true
			}
		}
	case LastVisibleResponse:
		return v.Found && messageHasFlag(v.Message, syncOnce)
	case ConversationHeadsResponse:
		for _, item := range v.Items {
			if payloadHasMessageFlag(lastVisibleResponseFromHead(item.Head), syncOnce) {
				return true
			}
		}
	case CommittedReadsResponse:
		for _, item := range v.Items {
			if messagesHaveFlag(item.Read.Messages, syncOnce) {
				return true
			}
		}
	}
	return false
}

func messagesHaveFlag(messages []ch.Message, syncOnce bool) bool {
	for _, msg := range messages {
		if messageHasFlag(msg, syncOnce) {
			return true
		}
	}
	return false
}

func messageHasFlag(msg ch.Message, syncOnce bool) bool {
	if syncOnce {
		return msg.SyncOnce
	}
	return msg.RedDot
}

func legacyMessageFlagError(payload any) error {
	if payloadHasMessageFlag(payload, false) {
		return errRedDotCodecRequired
	}
	if payloadHasMessageFlag(payload, true) {
		return errSyncOnceCodecRequired
	}
	return nil
}
