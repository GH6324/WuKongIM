package quorumlog

// ProtocolFields preserves durable protocol fields beyond the ordinary append
// projection. Strings are immutable and the value owns no mutable buffers.
// Version 2 proposals bind these fields; version 1 hashes remain unchanged.
type ProtocolFields struct {
	// FramerFlags uses the durable six-bit layout, excluding SyncOnce, which
	// remains on Record. Unknown bits and a duplicated SyncOnce bit are invalid.
	FramerFlags uint8
	Expire      uint32
	ClientSeq   uint64
	StreamID    uint64
	StreamFlag  uint8
	Timestamp   int32
	MsgKey      string
	StreamNo    string
	Topic       string
}

// SizeBytes counts the additional fixed and variable protocol content for
// queue and I/O budgets. The empty ordinary-append projection costs no bytes.
func (p ProtocolFields) SizeBytes() int {
	if p == (ProtocolFields{}) {
		return 0
	}
	return 38 + len(p.MsgKey) + len(p.StreamNo) + len(p.Topic)
}

// Valid reports whether the durable flags have an unambiguous interpretation.
func (p ProtocolFields) Valid() bool { return p.FramerFlags & ^uint8(0x3b) == 0 }
