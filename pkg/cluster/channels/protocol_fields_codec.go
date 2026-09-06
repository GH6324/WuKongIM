package channels

import (
	"encoding/binary"
	ch "github.com/WuKongIM/WuKongIM/pkg/channel"
)

// Version 8 adds a bounded fixed-width protocol header followed by the three
// existing length-delimited strings. Earlier frame bodies remain unchanged.
func appendProtocolFields(dst []byte, p ch.ProtocolFields, syncOnce bool) []byte {
	dst = appendBool(dst, syncOnce)
	dst = append(dst, p.FramerFlags, p.StreamFlag)
	dst = binary.BigEndian.AppendUint32(dst, p.Expire)
	dst = binary.BigEndian.AppendUint64(dst, p.ClientSeq)
	dst = binary.BigEndian.AppendUint64(dst, p.StreamID)
	dst = binary.BigEndian.AppendUint32(dst, uint32(p.Timestamp))
	dst = appendString(dst, p.MsgKey)
	dst = appendString(dst, p.StreamNo)
	return appendString(dst, p.Topic)
}
func readProtocolFields(body []byte, offset int) (ch.ProtocolFields, bool, int, error) {
	var p ch.ProtocolFields
	syncOnce, next, err := readBool(body, offset, "message sync once")
	if err != nil {
		return p, false, offset, err
	}
	offset = next
	if len(body)-offset < 26 {
		return p, false, offset, errInvalidCodecFrame
	}
	p.FramerFlags = body[offset]
	p.StreamFlag = body[offset+1]
	p.Expire = binary.BigEndian.Uint32(body[offset+2 : offset+6])
	p.ClientSeq = binary.BigEndian.Uint64(body[offset+6 : offset+14])
	p.StreamID = binary.BigEndian.Uint64(body[offset+14 : offset+22])
	p.Timestamp = int32(binary.BigEndian.Uint32(body[offset+22 : offset+26]))
	offset += 26
	if !p.Valid() {
		return p, false, offset, errInvalidCodecFrame
	}
	if p.MsgKey, offset, err = readString(body, offset); err != nil {
		return p, false, offset, err
	}
	if p.StreamNo, offset, err = readString(body, offset); err != nil {
		return p, false, offset, err
	}
	if p.Topic, offset, err = readString(body, offset); err != nil {
		return p, false, offset, err
	}
	return p, syncOnce, offset, nil
}
