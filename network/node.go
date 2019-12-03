package network

import (
	"goPBFT/consensus"
	"encoding/json"
	"fmt"
	"errors"
	"time"
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

func (node *Node) alarmToDispatcher() {
	for {
		time.Sleep(ResolvingTimeDuration)
		node.Alarm <- true
	}
}

func (node *Node) resolveMsg() {
	// 处理的是刚才在 buffer 中保存的信息
	for {
		msgs := <- node.MsgDelivery
		switch msgs.(type) {
		// 处理 requestMsg
		case []*consensus.RequestMsg:
			errs := node.resolveRequestMsg(msgs.([]*consensus.RequestMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}
		case []*consensus.PrePrepareMsg:
			// 处理 prepreparemsg
			errs := node.resolvePrePrepareMsg(msgs.([]*consensus.PrePrepareMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}
		case []*consensus.VoteMsg:
			voteMsgs := msgs.([]*consensus.VoteMsg)
			if len(voteMsgs) == 0 {
				break
			}
			// 处理投票信息中的 prepareMsg
			if voteMsgs[0].MsgType == consensus.PrepareMsg {
				errs := node.resolvePrepareMsg(voteMsgs)
				if len(errs) != 0 {
					for _, err := range errs {
						fmt.Println(err)
					}
					// TODO: send err to ErrorChannel
				}
			} else if voteMsgs[0].MsgType == consensus.CommitMsg {
				// 处理投票信息中的 commitMsg
				errs := node.resolveCommitMsg(voteMsgs)
				if len(errs) != 0 {
					for _, err := range errs {
						fmt.Println(err)
					}
					// TODO: send err to ErrorChannel
				}
			}
		}
	}
}

func (node *Node) resolveRequestMsg(msgs []*consensus.RequestMsg) []error {
	errs := make([]error, 0)

	for _, reqMsg := range msgs {
		err := node.GetReq(reqMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return errs
	}
	return nil
}

// GetReq can be called when the node's CurrentState is nil.
// Consensus start procedure for the Primary.
func (node *Node) GetReq(reqMsg *consensus.RequestMsg) error {
	LogMsg(reqMsg)

	// 为共识创建一个新状态
	err := node.createStateForNewConsensus()
	if err != nil {
		return err
	}

	// 开始执行共识
	prePrepareMsg, err := node.CurrentState.StartConsensus(reqMsg)
	if err != nil {
		return nil
	}

	LogStage(fmt.Sprintf("Consensus Process (ViewID:%d)", node.CurrentState.ViewID), false)

	// 发送 getPrePrepare 信息
	if prePrepareMsg != nil {
		node.Broadcast(prePrepareMsg, "/preprepare")
		LogStage("Pre-prepare", true)
	}
	return nil
}

func (node *Node) resolvePrePrepareMsg(msgs []*consensus.PrePrepareMsg) []error {
	errs := make([]error, 0)

	for _, reqMsg := range msgs {
		err := node.GetPrePrepare(reqMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return errs
	}
	return nil
}

// GetPrePrepare can be called when the node's CurrentState is nil.
// Consensus start procedure for normal participants.
func (node *Node) GetPrePrepare(prePrepareMsg *consensus.PrePrepareMsg) error {
	LogMsg(prePrepareMsg)
	// Create a new state for the new consensus.
	err := node.createStateForNewConsensus()
	if err != nil {
		return err
	}

	prePareMsg, err := node.CurrentState.PrePrepare(prePrepareMsg)
	if err != nil {
		return err
	}

	if prePareMsg != nil {
		// Attach node ID to the message
		prePareMsg.NodeID = node.NodeID

		LogStage("Pre-prepare", true)
		node.Broadcast(prePareMsg, "/prepare")
		LogStage("Prepare", false)
	}
	return nil
}

// 两个是属于同一个类的，因此该函数与后面一个函数参数类型是一样的
func (node *Node) resolvePrepareMsg(msgs []*consensus.VoteMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, prepareMsg := range msgs {
		err := node.GetPrepare(prepareMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (node *Node) GetPrepare(prepareMsg *consensus.VoteMsg) error {
	LogMsg(prepareMsg)

	commitMsg, err := node.CurrentState.Prepare(prepareMsg)
	if err != nil {
		return err
	}

	if commitMsg != nil {
		// Attach node ID to the message
		commitMsg.NodeID = node.NodeID

		LogStage("Prepare", true)
		node.Broadcast(commitMsg, "/commit")
		LogStage("Commit", false)
	}

	return nil
}

func (node *Node) resolveCommitMsg(msgs []*consensus.VoteMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, commitMsg := range msgs {
		err := node.GetCommit(commitMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (node *Node) GetCommit(prepareMsg *consensus.VoteMsg) error {
	LogMsg(prepareMsg)
	replyMsg, committedMsg, err := node.CurrentState.Commit(prepareMsg)
	if err != nil {
		return err
	}

	if replyMsg != nil {
		if committedMsg == nil {
			return errors.New("committed message is nil, even though the reply message is not nil")
		}

		// Attach node ID to the message
		replyMsg.NodeID = node.NodeID

		// Save the last version of committed messages to node.
		node.CommitMsgs = append(node.CommitMsgs, committedMsg)

		LogStage("Commit", true)
		node.Reply(replyMsg)
		LogStage("Reply", true)
	}

	return nil
}

func (node *Node) GetReply(msg *consensus.ReplyMsg) {
	fmt.Printf("Result: %s by %s\n", msg.Result, msg.NodeID)
}

func (node *Node) createStateForNewConsensus() error {
	// 先检查是否有存在的共识机制
	if node.CurrentState != nil {
		return errors.New("another consensus is ongoing")
	}

	// 获取最后一个序列ID
	var lastSequenceID int64
	if len(node.CommitMsgs) == 0 {
		lastSequenceID = -1
	} else {
		lastSequenceID = node.CommitMsgs[len(node.CommitMsgs)-1].SequenceID
	}

	// 创建一个新的共识
	node.CurrentState = consensus.CreateState(node.View.ID, lastSequenceID)
	LogStage("Create the replica status", true)

	return nil
}

func (node *Node) Reply(msg *consensus.ReplyMsg) error {
	for _, value := range node.CommitMsgs {
		fmt.Printf("Committed value: %s, %d, %s, %d", value.ClinetID, value.Timestamp, value.Operation, value.SequenceID)
	}
	fmt.Print("\n")

	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return nil
	}
	send(node.NodeTable[node.View.Primary]+"/reply", jsonMsg)

	return nil
}

func (node *Node) Broadcast(msg interface{}, path string) map[string]error {
	errorMap := make(map[string]error)

	for nodeID, url := range node.NodeTable {
		if nodeID == node.NodeID {
			continue
		}

		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			errorMap[nodeID] = err
			continue
		}

		send(url + path, jsonMsg)
	}

	if len(errorMap) == 0 {
		return nil
	} else {
		return errorMap
	}
}