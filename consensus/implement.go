package consensus

import (
	"encoding/json"
	"fmt"
	"errors"
	"time"
)

type State struct {
	ViewID int64
	MsgLogs *MsgLogs
	LastSequenceID int64
	CurrentStage Stage
}

type MsgLogs struct {
	ReqMsg *RequestMsg
	PrepareMsgs map[string]*VoteMsg
	CommitMsgs map[string]*VoteMsg
}

type Stage int
const (
	Idle        Stage = iota // Node is created successfully, but the consensus process is not started yet.
	PrePrepared              // The ReqMsgs is processed successfully. The node is ready to head to the Prepare stage.
	Prepared                 // Same with `prepared` stage explained in the original paper.
	Committed                // Same with `committed-local` stage explained in the original paper.
)

const f = 1

func CreateState(viewID int64, lastSequenceID int64) *State{
	return &State{
		ViewID: viewID,
		MsgLogs: &MsgLogs{
			ReqMsg:nil,
			PrepareMsgs:make(map[string]*VoteMsg),
			CommitMsgs:make(map[string]*VoteMsg),
		},
		LastSequenceID: lastSequenceID,
		CurrentStage: Idle,
	}
}

func (state *State) StartConsensus(request *RequestMsg)(*PrePrepareMsg, error) {
	sequenceID := time.Now().UnixNano()

	// 找到当前序列 id 中的最大值
	if state.LastSequenceID != -1 {
		for state.LastSequenceID >= sequenceID {
			sequenceID += 1
		}
	}
	// 为请求消息对象分配一个新的序列ID
	request.SequenceID = sequenceID

	// 向日志中保存 reqMsgs
	state.MsgLogs.ReqMsg = request

	// 获取请求消息的签名
	digest, err := digest(request)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	// 将状态转换为 pre-prepared
	state.CurrentStage = PrePrepared

	return &PrePrepareMsg{
		ViewID: state.ViewID,
		SequenceID: sequenceID,
		Digest: digest,
		RequestMsg: request,
	}, nil
}

func digest(object interface{}) (string, error) {
	msg, err := json.Marshal(object)

	if err != nil {
		return "", nil
	}
	return Hash(msg), nil
}

func (state *State) PrePrepare(prePrepareMsg *PrePrepareMsg) (*VoteMsg, error) {
	// 获取 msg 并将其放入 log 中
	state.MsgLogs.ReqMsg = prePrepareMsg.RequestMsg
	// 检验信息正确与否
	if !state.verifyMsg(prePrepareMsg.ViewID, prePrepareMsg.SequenceID, prePrepareMsg.Digest) {
		return nil, errors.New("pre-prepare message is corrupted")
	}
	// 将状态更改为 pre-prepare
	state.CurrentStage = PrePrepared

	return &VoteMsg {
		ViewID: state.ViewID,
		SequenceID: prePrepareMsg.SequenceID,
		Digest: prePrepareMsg.Digest,
		MsgType: PrepareMsg,
	}, nil
}

func (state *State) Prepare(prepareMsg *VoteMsg) (*VoteMsg, error) {
	if !state.verifyMsg(prepareMsg.ViewID, prepareMsg.SequenceID, prepareMsg.Digest) {
		return nil, errors.New("Prepare message is corrupted.")
	}

	// 将信息添加到 logs
	state.MsgLogs.PrepareMsgs[prepareMsg.NodeID] = prepareMsg

	// 输出当前投片信息
	fmt.Printf("[Prepare-Vote]: %d\n", len(state.MsgLogs.PrepareMsgs))

	if state.prepared() {
		// 更改当前状态至 prepared
		state.CurrentStage = Prepared

		return &VoteMsg{
			ViewID: state.ViewID,
			SequenceID: prepareMsg.SequenceID,
			Digest: prepareMsg.Digest,
			MsgType: CommitMsg,
		}, nil
	}

	return nil, nil
}

func (state *State) Commit(commitMsg *VoteMsg) (*ReplyMsg, *RequestMsg, error) {
	if !state.verifyMsg(commitMsg.ViewID, commitMsg.SequenceID, commitMsg.Digest) {
		return nil, nil, errors.New("commit message is corrupted")
	}

	// 将 msg 加入 log
	state.MsgLogs.CommitMsgs[commitMsg.NodeID] = commitMsg

	// 输出当前投票状态
	fmt.Printf("[Commit-Vote]: %d\n", len(state.MsgLogs.CommitMsgs))

	if state.committed() {
		// 此节点在本地执行请求的操作并获取结果。
		result := "Executed"

		// 更改状态至 prepared
		state.CurrentStage = Committed

		return &ReplyMsg{
			ViewID: state.ViewID,
			Timestamp: state.MsgLogs.ReqMsg.Timestamp,
			ClientID: state.MsgLogs.ReqMsg.ClinetID,
			Result: result,
		}, state.MsgLogs.ReqMsg, nil
	}
	return nil, nil, nil
}

func (state *State) committed() bool {
	if !state.prepared() {
		return false
	}
	if len(state.MsgLogs.CommitMsgs) < 2 * f {
		return false
	}
	return true
}

func (state *State) verifyMsg(viewID int64, sequenceID int64, digestGot string) bool {
	// 试图错误，将导致无法启动共识
	if state.ViewID != viewID {
		return false
	}

	// 检查是否传递错误序列号
	if state.LastSequenceID != -1 {
		// 要保证传递的 sequenceID 是比 LastSequenceID 大的
		if state.LastSequenceID >= sequenceID {
			return false
		}
	}

	digest, err := digest(state.MsgLogs.ReqMsg)
	if err != nil {
		fmt.Println(err)
		return false
	}

	// 检验 digest
	if digestGot != digest {
		return false
	}

	return true
}

func (state *State) prepared() bool {
	if state.MsgLogs.ReqMsg == nil {
		return false
	}
	if len(state.MsgLogs.PrepareMsgs) < 2 * f {
		return false
	}
	return true
}
