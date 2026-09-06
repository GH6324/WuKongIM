package channels

import (
	"errors"

	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
	channeltransport "github.com/WuKongIM/WuKongIM/pkg/channel/transport"
)

var errFullProtocolCodec = errors.New("channels: durable protocol data requires channel codec v8")

func messageNeedsFullProtocol(message ch.Message) bool {
	return message.SyncOnce || message.Protocol != (ch.ProtocolFields{})
}

// payloadNeedsFullProtocol prevents an old peer from acknowledging a response
// whose durable header, stream, topic or command flag it cannot represent.
func payloadNeedsFullProtocol(payload any) bool {
	switch v := payload.(type) {
	case channeltransport.PullResponse:
		for _, record := range v.Records {
			if record.SyncOnce || record.Protocol != (ch.ProtocolFields{}) {
				return true
			}
		}
	case channeltransport.PullBatchResponse:
		for _, item := range v.Items {
			if payloadNeedsFullProtocol(item.Response) {
				return true
			}
		}
	case LastVisibleResponse:
		return v.Found && messageNeedsFullProtocol(v.Message)
	case ConversationHeadsResponse:
		for _, item := range v.Items {
			if item.Head.Found && messageNeedsFullProtocol(item.Head.Message) {
				return true
			}
		}
	case CommittedReadsResponse:
		for _, item := range v.Items {
			for _, message := range item.Read.Messages {
				if messageNeedsFullProtocol(message) {
					return true
				}
			}
		}
	}
	return false
}
