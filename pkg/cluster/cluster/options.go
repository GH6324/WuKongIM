package cluster

import (
	"context"
	"time"

	replica "github.com/WuKongIM/WuKongIM/pkg/cluster/replica2"
	"github.com/WuKongIM/WuKongIM/pkg/wkserver/proto"
	"go.uber.org/zap/zapcore"
)

type Options struct {
	NodeID                       uint64
	Addr                         string
	ApiServerAddr                string            // 节点api地址
	InitNodes                    map[uint64]string // 初始化节点列表
	ChannelGroupScanInterval     time.Duration
	ShardLogStorage              IShardLogStorage
	MessageLogStorage            IShardLogStorage // 消息日志存储
	MaxChannelActivitiesPerGroup int              // 每个channelGroup最大处理活动的channel数量
	Transport                    ITransport
	ChannelInactiveTimeout       time.Duration // channel不活跃超时时间, 如果频道超过这个时间没有活跃, 则会被移除，等下次活跃时会重新加入
	SlotInactiveTimeout          time.Duration // slot不活跃超时时间，如果槽超过这个时间没有活跃，则会被移除，等下次活跃时会重新加入
	AdvanceCountOfBatch          int           // 每批次处理Advance的最大数量
	DataDir                      string        // 数据存储目录
	SlotCount                    uint32        // 槽数量
	SlotMaxReplicaCount          uint32        // 每个槽位最大副本数量
	ChannelMaxReplicaCount       uint16        // 每个频道最大副本数量
	ProposeTimeout               time.Duration
	ReqTimeout                   time.Duration
	GetAppliedIndex              func(shardNo string) (uint64, error)          // 获取分区已应用的日志索引
	ChannelGroupCount            int                                           // channelGroup数量
	OnSlotApply                  func(slotId uint32, logs []replica.Log) error // 槽数据应用
	LogLevel                     zapcore.Level                                 // 日志级别
	ChannelClusterStorage        ChannelClusterStorage                         // 频道分布式存储
	LogSyncLimitOfEach           int                                           // 每次日志同步数量

	nodeOnlineFnc      func(nodeID uint64) (bool, error) // 节点是否在线
	requestSlotLogInfo func(ctx context.Context, nodeId uint64, req *SlotLogInfoReq) (*SlotLogInfoResp, error)

	Send func(to uint64, m *proto.Message) error
}

func NewOptions(optList ...Option) *Options {
	opts := &Options{
		Addr:                         "0.0.0.0:10001",
		ChannelGroupScanInterval:     time.Millisecond * 200,
		SlotInactiveTimeout:          time.Millisecond * 200,
		MaxChannelActivitiesPerGroup: 1000,
		ChannelInactiveTimeout:       time.Hour * 1,
		AdvanceCountOfBatch:          50,
		SlotCount:                    256,
		SlotMaxReplicaCount:          3,
		ProposeTimeout:               time.Second * 5,
		ChannelGroupCount:            128,
		ChannelMaxReplicaCount:       3,
		ReqTimeout:                   time.Second * 10,
		LogLevel:                     zapcore.InfoLevel,
		LogSyncLimitOfEach:           100,
	}
	for _, opt := range optList {
		opt(opts)
	}
	return opts
}

func (o *Options) Replicas() []uint64 {
	replicas := make([]uint64, 0, len(o.InitNodes))
	for nodeID := range o.InitNodes {
		replicas = append(replicas, nodeID)
	}
	return replicas
}

type Option func(opts *Options)

func WithNodeID(nodeID uint64) Option {
	return func(opts *Options) {
		opts.NodeID = nodeID
	}
}

func WithAddr(addr string) Option {
	return func(opts *Options) {
		opts.Addr = addr
	}
}

func WithInitNodes(initNodes map[uint64]string) Option {
	return func(opts *Options) {
		opts.InitNodes = initNodes
	}
}

func WithChannelGroupScanInterval(interval time.Duration) Option {
	return func(opts *Options) {
		opts.ChannelGroupScanInterval = interval
	}
}

func WithShardLogStorage(storage IShardLogStorage) Option {
	return func(opts *Options) {
		opts.ShardLogStorage = storage
	}
}

func WithMaxChannelActivitiesPerGroup(count int) Option {
	return func(opts *Options) {
		opts.MaxChannelActivitiesPerGroup = count
	}
}

func WithTransport(transport ITransport) Option {
	return func(opts *Options) {
		opts.Transport = transport
	}
}

func WithChannelInactiveTimeout(timeout time.Duration) Option {
	return func(opts *Options) {
		opts.ChannelInactiveTimeout = timeout
	}
}

func WithSlotInactiveTimeout(timeout time.Duration) Option {
	return func(opts *Options) {
		opts.SlotInactiveTimeout = timeout
	}
}

func WithAdvanceCountOfBatch(count int) Option {
	return func(opts *Options) {
		opts.AdvanceCountOfBatch = count
	}
}

func WithDataDir(dataDir string) Option {
	return func(opts *Options) {
		opts.DataDir = dataDir
	}
}

func WithSlotCount(count uint32) Option {
	return func(opts *Options) {
		opts.SlotCount = count
	}
}

func WithSlotMaxReplicaCount(count uint32) Option {
	return func(opts *Options) {
		opts.SlotMaxReplicaCount = count
	}
}

func WithProposeTimeout(timeout time.Duration) Option {
	return func(opts *Options) {
		opts.ProposeTimeout = timeout
	}
}

func WithGetAppliedIndex(f func(shardNo string) (uint64, error)) Option {
	return func(opts *Options) {
		opts.GetAppliedIndex = f
	}
}

func WithMessageLogStorage(storage IShardLogStorage) Option {
	return func(opts *Options) {
		opts.MessageLogStorage = storage
	}
}

func WithApiServerAddr(addr string) Option {
	return func(opts *Options) {
		opts.ApiServerAddr = addr
	}
}

func WithChannelMaxReplicaCount(count uint16) Option {
	return func(opts *Options) {
		opts.ChannelMaxReplicaCount = count
	}
}

func WithOnSlotApply(f func(slotId uint32, logs []replica.Log) error) Option {
	return func(opts *Options) {
		opts.OnSlotApply = f
	}
}

func WithLogLevel(level zapcore.Level) Option {
	return func(opts *Options) {
		opts.LogLevel = level
	}
}

func WithChannelClusterStorage(storage ChannelClusterStorage) Option {
	return func(opts *Options) {
		opts.ChannelClusterStorage = storage
	}
}