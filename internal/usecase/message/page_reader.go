package message

import "context"

// CommittedMessageQuery describes one bounded scan after page semantics have
// been resolved. The adapter must preserve these bounds and scan ordering.
type CommittedMessageQuery struct {
	// ChannelID identifies the canonical Channel whose committed log is read.
	ChannelID ChannelID
	// FromSeq is the inclusive scan starting sequence.
	FromSeq uint64
	// MinSeq and MaxSeq are inclusive scan bounds; zero leaves that bound unset.
	MinSeq uint64
	MaxSeq uint64
	// Limit and MaxBytes bound returned records and payload bytes.
	Limit    int
	MaxBytes int
	// Reverse selects descending storage iteration, not response ordering.
	Reverse bool
}

// CommittedMessageResult owns one scan's messages and payload bytes. The page
// reader may filter and reorder Messages; adapters must not retain mutable aliases.
// Flags.SyncOnce is preserved until page construction excludes command records.
type CommittedMessageResult struct {
	Messages []SyncedMessage
	// Err is scoped to this scan and preserves its underlying cause.
	Err error
}

// CommittedMessageReader reads committed records in request-aligned batches.
// It enforces cluster authority and retention without interpreting page intent.
type CommittedMessageReader interface {
	ReadCommittedMessages(context.Context, []CommittedMessageQuery) ([]CommittedMessageResult, error)
}

// PageReader owns compatible message-page semantics for ordinary sync and
// plugin reads. Authorization and response-specific enrichment belong to callers.
// It is stateless and retains no requests or messages between calls.
type PageReader struct {
	// committed executes bounded scans and transfers ownership of their results.
	committed CommittedMessageReader
}

// NewPageReader constructs page reads over an injected committed-record adapter.
func NewPageReader(committed CommittedMessageReader) *PageReader {
	return &PageReader{committed: committed}
}

var _ ChannelMessageReader = (*PageReader)(nil)
var _ ChannelMessageBatchReader = (*PageReader)(nil)

// SyncMessages uses the same bounded scan and page construction as batch reads.
func (r *PageReader) SyncMessages(ctx context.Context, query ChannelMessageQuery) (ChannelMessagePage, error) {
	results, err := r.SyncMessagesBatch(ctx, []ChannelMessageQuery{query})
	if err != nil {
		return ChannelMessagePage{}, err
	}
	if results[0].Err != nil {
		return ChannelMessagePage{}, results[0].Err
	}
	return results[0].Page, nil
}

// SyncMessagesBatch executes one aligned read wave. A transport or cardinality
// error fails the wave; individual read errors stay at their requested indexes.
func (r *PageReader) SyncMessagesBatch(ctx context.Context, queries []ChannelMessageQuery) ([]ChannelMessageReadResult, error) {
	if r == nil || r.committed == nil {
		return nil, ErrMessageReaderRequired
	}
	plans := make([]messagePagePlan, len(queries))
	reads := make([]CommittedMessageQuery, len(queries))
	for index, query := range queries {
		plans[index] = planMessagePage(query)
		reads[index] = plans[index].scan
	}
	readResults, err := r.committed.ReadCommittedMessages(ctx, reads)
	if err != nil {
		return nil, err
	}
	if len(readResults) != len(queries) {
		return nil, ErrSyncBatchResultMismatch
	}
	results := make([]ChannelMessageReadResult, len(queries))
	for index, read := range readResults {
		if read.Err != nil {
			results[index].Err = read.Err
			continue
		}
		results[index].Page = plans[index].page(read.Messages)
	}
	return results, nil
}

// messagePagePlan keeps scan selection and response construction together so
// applying a visibility floor cannot change the meaning of a latest-page request.
type messagePagePlan struct {
	scan  CommittedMessageQuery
	limit int
	// Exclusive end bounds retain filtering after the bounded storage read.
	excludeThroughSeq uint64
	excludeFromSeq    uint64
}

// planMessagePage resolves page intent and visibility into one bounded scan
// while retaining the exclusive end needed for compatibility filtering.
func planMessagePage(query ChannelMessageQuery) messagePagePlan {
	limit := query.Limit
	if limit <= 0 {
		limit = 1
	}
	latest := query.StartSeq == 0 && query.EndSeq == 0
	plan := messagePagePlan{limit: limit, scan: CommittedMessageQuery{
		ChannelID: query.ChannelID,
		FromSeq:   query.StartSeq,
		MinSeq:    query.MinSeq,
		MaxSeq:    ^uint64(0),
		Limit:     limit + 1,
		MaxBytes:  int(^uint(0) >> 1),
		Reverse:   query.PullMode == PullModeDown || latest,
	}}
	if !latest && query.PullMode == PullModeUp && query.MinSeq > plan.scan.FromSeq {
		plan.scan.FromSeq = query.MinSeq
	}
	if query.PullMode == PullModeUp && query.EndSeq > 0 {
		plan.scan.MaxSeq = query.EndSeq - 1
		plan.excludeFromSeq = query.EndSeq
	} else if query.PullMode == PullModeDown {
		plan.excludeThroughSeq = query.EndSeq
		if query.StartSeq > 0 {
			plan.scan.MaxSeq = query.StartSeq
		}
	}
	if plan.scan.Reverse && plan.scan.FromSeq == 0 {
		plan.scan.FromSeq = ^uint64(0)
		plan.scan.MaxSeq = ^uint64(0)
	} else if !plan.scan.Reverse && plan.scan.FromSeq == 0 {
		plan.scan.FromSeq = 1
	}
	return plan
}

// page consumes owned scan results and applies the same direction and bounds
// that selected the scan, returning ordinary messages in ascending order.
func (p messagePagePlan) page(messages []SyncedMessage) ChannelMessagePage {
	if messages == nil {
		messages = []SyncedMessage{}
	}
	kept := messages[:0]
	for _, msg := range messages {
		if msg.Flags.SyncOnce ||
			(p.excludeThroughSeq > 0 && msg.MessageSeq <= p.excludeThroughSeq) ||
			(p.excludeFromSeq > 0 && msg.MessageSeq >= p.excludeFromSeq) {
			continue
		}
		kept = append(kept, msg)
	}
	// Preserve the compatibility lookahead: filtering may underfill a page;
	// it must not trigger an additional scan or invent HasMore from store cursors.
	hasMore := len(kept) > p.limit
	if hasMore {
		kept = kept[:p.limit]
	}
	if p.scan.Reverse {
		for left, right := 0, len(kept)-1; left < right; left, right = left+1, right-1 {
			kept[left], kept[right] = kept[right], kept[left]
		}
	}
	return ChannelMessagePage{Messages: kept, HasMore: hasMore}
}
