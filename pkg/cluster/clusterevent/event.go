package clusterevent

import (
	"fmt"

	"github.com/WuKongIM/WuKongIM/pkg/cluster/clusterconfig/pb"
)

type EventType int

const (
	EventTypeNone EventType = iota
	// EventTypeNodeAdd 节点添加
	EventTypeNodeAdd
	// EventTypeNodeUpdate 节点更新
	EventTypeNodeUpdate
	// EventTypeNodeDelete 节点删除
	EventTypeNodeDelete
	// EventTypeNodeAddResp 节点添加响应
	EventTypeNodeAddResp
	// EventTypeSlotAdd 槽添加
	EventTypeSlotAdd
	// EventTypeSlotUpdate 槽更新
	EventTypeSlotUpdate
	// EventTypeSlotDelete 槽删除
	EventTypeSlotDelete
	// EventTypeApiServerAddrUpdate api服务地址更新
	EventTypeApiServerAddrUpdate
	// EventTypeNodeOnlieChange 节点在线状态改变
	EventTypeNodeOnlieChange
	// EventTypeSlotNeedElection 槽需要重新选举
	EventTypeSlotNeedElection
)

func (e EventType) String() string {
	switch e {
	case EventTypeNodeAdd:
		return "EventTypeNodeAdd"
	case EventTypeNodeUpdate:
		return "EventTypeNodeUpdate"
	case EventTypeNodeDelete:
		return "EventTypeNodeDelete"
	case EventTypeNodeAddResp:
		return "EventTypeNodeAddResp"
	case EventTypeSlotAdd:
		return "EventTypeSlotAdd"
	case EventTypeSlotUpdate:
		return "EventTypeSlotUpdate"
	case EventTypeSlotDelete:
		return "EventTypeSlotDelete"
	case EventTypeApiServerAddrUpdate:
		return "EventTypeApiServerAddrUpdate"
	case EventTypeNodeOnlieChange:
		return "EventTypeNodeOnlieChange"
	case EventTypeSlotNeedElection:
		return "EventTypeSlotNeedElection"
	}
	return fmt.Sprintf("EventTypeNone[%d]", e)

}

type Message struct {
	Type         EventType
	Nodes        []*pb.Node
	Slots        []*pb.Slot
	SlotMigrates []*pb.SlotMigrate
}