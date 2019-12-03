package network

import (
	"goPBFT/consensus"
)

// 首先对节点进行定义
type Node struct {
	NodeID        string
	NodeTable     map[string]string
	View          *View
	CurrentState  *consensus.State
	CommitMsgs    []*consensus.RequestMsg
	MsgBuffer     *MsgBuffer
	MsgEntrance   chan interface{}
	MsgDelivery   chan interface{}
	Alarm         chan bool
}
// View 定义
type View struct {
	ID      int64
	Primary string
}

type MsgBuffer struct {
	ReqMsgs []*consensus.RequestMsg
	PrePrepareMsgs []*consensus.PrePrepareMsg
	PrepareMsgs []*consensus.VoteMsg
	CommitMsgs []*consensus.VoteMsg
}

func NewNode(nodeID string) *Node {
	const viewID = 10000000000 // temporary.

	node := &Node {
		nodeID,
		map[string]string{
			"Apple": "localhost:1111",
			"Ball": "localhost:1112",
			"Candy": "localhost:1113",
			"Dog": "localhost:1114",
		},
		&View{
			viewID,
			Primary: "Apple",
		},
		nil,
		make([]*consensus.RequestMsg, 0),
		&MsgBuffer{
			make([]*consensus.RequestMsg, 0),
			make([]*consensus.PrePrepareMsg, 0),
			make([]*consensus.VoteMsg, 0),
			make([]*consensus.VoteMsg, 0),
		},

		// channels
		make(chan interface{}),
		make(chan interface{}),
		make(chan bool),

	}

	//  Start message dispatcher
	go node.dispatchMsg()

	// start alarm trigger
	go node.alarmToDispatcher()

	// start message resolver
	go node.resolveMsg()

	return node
}


func (node *Node) dispatchMsg() {
	for {
		select {
		case msg := <-node.MsgEntrance:
			err := node.routeMsg(msg)
			if err != nil {
				fmt.Println(err)
			}
		case <- node.Alarm:
			err := node.routeMsgWhenAlarmed()
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

func (node *Node) routeMsg(msg interface{}) []error {
	switch msg.(type) {
	// 当信息状态为*请求信息*时
	case *consensus.RequestMsg:
		// 当当前节点状态为 nil 时，需要新建一个信息列表，并将信息拷贝进该切片中
		if node.CurrentState == nil {
			// Copy buffered messages first.
			msgs := make([]*consensus.RequestMsg, len(node.MsgBuffer.ReqMsgs))
			copy(msgs, node.MsgBuffer.ReqMsgs)

			// copy 之后添加新的信息
			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.RequestMsg))

			// 将 buffer 清空
			// Empty the buffer.
			node.MsgBuffer.ReqMsgs = make([]*consensus.RequestMsg, 0)

			// 开始发送消息
			// Send messages.
			node.MsgDelivery <- msgs
		} else {
			// 否则直接添加进 buffer 中
			node.MsgBuffer.ReqMsgs = append(node.MsgBuffer.ReqMsgs, msg.(*consensus.RequestMsg))
		}
	// 当信息状态为*预准备信息*时， 处理方法与前面一直，只不过是放到 PrePrepareMsg 信息列表中
	case *consensus.PrePrepareMsg:
		if node.CurrentState == nil {
			// Copy buffered messages first.
			msgs := make([]*consensus.PrePrepareMsg, len(node.MsgBuffer.PrePrepareMsgs))
			copy(msgs, node.MsgBuffer.PrePrepareMsgs)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.PrePrepareMsg))

			// Empty the buffer.
			node.MsgBuffer.PrePrepareMsgs = make([]*consensus.PrePrepareMsg, 0)

			// Send messages.
			node.MsgDelivery <- msgs
		} else {
			node.MsgBuffer.PrePrepareMsgs = append(node.MsgBuffer.PrePrepareMsgs, msg.(*consensus.PrePrepareMsg))
		}
	// 当信息状态为*投票信息*时
	case *consensus.VoteMsg:
		// 处理 prepare 阶段的投票信息
		if msg.(*consensus.VoteMsg).MsgType == consensus.PrepareMsg {
			// 当当前状态为空或当前阶段不是*预准备结束阶段*，就直接插入信息到 preparemsgs 中
			if node.CurrentState == nil || node.CurrentState.CurrentStage != consensus.PrePrepared {
				node.MsgBuffer.PrepareMsgs = append(node.MsgBuffer.PrepareMsgs, msg.(*consensus.VoteMsg))
			} else {
				// 否则，新建slice，接收信息，清空buffer，发送信息
				// Copy buffered messages first.
				msgs := make([]*consensus.VoteMsg, len(node.MsgBuffer.PrepareMsgs))
				copy(msgs, node.MsgBuffer.PrepareMsgs)

				// Append a newly arrived message.
				msgs = append(msgs, msg.(*consensus.VoteMsg))

				// Empty the buffer.
				node.MsgBuffer.PrepareMsgs = make([]*consensus.VoteMsg, 0)

				// Send messages.
				node.MsgDelivery <- msgs
			}
		// 处理 commit 阶段的投票信息
		} else if msg.(*consensus.VoteMsg).MsgType == consensus.CommitMsg {
			// 当当前状态为空或当前阶段不是*准备结束阶段*，就直接插入信息到 preparemsgs 中
			if node.CurrentState == nil || node.CurrentState.CurrentStage != consensus.Prepared {
				node.MsgBuffer.CommitMsgs = append(node.MsgBuffer.CommitMsgs, msg.(*consensus.VoteMsg))
			} else {
				// 否则，新建slice，接收信息，清空buffer，发送信息
				// Copy buffered messages first.
				msgs := make([]*consensus.VoteMsg, len(node.MsgBuffer.CommitMsgs))
				copy(msgs, node.MsgBuffer.CommitMsgs)

				// Append a newly arrived message.
				msgs = append(msgs, msg.(*consensus.VoteMsg))

				// Empty the buffer.
				node.MsgBuffer.CommitMsgs = make([]*consensus.VoteMsg, 0)

				// Send messages.
				node.MsgDelivery <- msgs
			}
		}
	}

	return nil
}

func (node *Node) routeMsgWhenAlarmed() []error {
	if node.CurrentState == nil {
		// 当 buffer 中有 ReqMsgs 时
		// Check ReqMsgs, send them.
		if len(node.MsgBuffer.ReqMsgs) != 0 {
			msgs := make([]*consensus.RequestMsg, len(node.MsgBuffer.ReqMsgs))
			copy(msgs, node.MsgBuffer.ReqMsgs)
			// 发送信息
			node.MsgDelivery <- msgs
		}
		// 当 buffer 中有 PrePrepareMsgs 时
		// Check PrePrepareMsgs, send them.
		if len(node.MsgBuffer.PrePrepareMsgs) != 0 {
			msgs := make([]*consensus.PrePrepareMsg, len(node.MsgBuffer.PrePrepareMsgs))
			copy(msgs, node.MsgBuffer.PrePrepareMsgs)
			// 发送信息
			node.MsgDelivery <- msgs
		}
	} else {
		// 否则，以同样的方式处理 preparemsgs 和 commitmsgs
		switch node.CurrentState.CurrentStage {
		case consensus.PrePrepared:
			// Check PrepareMsgs, send them.
			if len(node.MsgBuffer.PrepareMsgs) != 0 {
				msgs := make([]*consensus.VoteMsg, len(node.MsgBuffer.PrepareMsgs))
				copy(msgs, node.MsgBuffer.PrepareMsgs)

				node.MsgDelivery <- msgs
			}
		case consensus.Prepared:
			// Check CommitMsgs, send them.
			if len(node.MsgBuffer.CommitMsgs) != 0 {
				msgs := make([]*consensus.VoteMsg, len(node.MsgBuffer.CommitMsgs))
				copy(msgs, node.MsgBuffer.CommitMsgs)

				node.MsgDelivery <- msgs
			}
		}
	}

	return nil
}
