package migration

import (
	"fmt"

	"github.com/WuKongIM/WuKongIM/pkg/db/message"
	"github.com/WuKongIM/WuKongIM/pkg/db/message/channelcompat"
	"github.com/WuKongIM/WuKongIM/pkg/quorumlog"
)

// PrepareMessageRecord validates that an indivisible source message can be
// recovered under the native default page budget, then returns its exact native
// and storage-neutral representations. It assigns no identity or timestamp.
func PrepareMessageRecord(m channelcompat.Message) (channelcompat.Record, quorumlog.Record, error) {
	row, err := message.EncodeMessageRecord(m, 1)
	if err != nil {
		return row, quorumlog.Record{}, err
	}
	var flags uint8
	for i, set := range [...]bool{m.Framer.NoPersist, m.Framer.RedDot, false, m.Framer.DUP, m.Framer.HasServerVersion, m.Framer.End} {
		if set {
			flags |= 1 << i
		}
	}
	record := quorumlog.Record{ID: m.MessageID, Index: m.MessageSeq, Epoch: 1, FromUID: m.FromUID, ClientMsgNo: m.ClientMsgNo, ServerTimestampMS: m.ServerTimestampMS, Setting: uint8(m.Setting), SyncOnce: m.Framer.SyncOnce, Payload: m.Payload, Protocol: quorumlog.ProtocolFields{FramerFlags: flags, Expire: m.Expire, ClientSeq: m.ClientSeq, StreamID: m.StreamID, StreamFlag: uint8(m.StreamFlag), Timestamp: m.Timestamp, MsgKey: m.MsgKey, StreamNo: m.StreamNo, Topic: m.Topic}}
	if len(row.Payload) > quorumlog.DefaultRecoveryPageBytes || quorumlog.RecoveryRecordBytes(record.FromUID, record.ClientMsgNo, len(record.Payload), record.Protocol) > quorumlog.DefaultRecoveryPageBytes {
		return channelcompat.Record{}, quorumlog.Record{}, fmt.Errorf("source message %d exceeds the native recovery page budget of %d bytes", m.MessageSeq, quorumlog.DefaultRecoveryPageBytes)
	}
	return row, record, nil
}
