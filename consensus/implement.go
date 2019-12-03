package consensus

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