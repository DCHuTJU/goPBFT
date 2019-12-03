# goPBFT
A simple consensus of PBFT
#### 目标

使用 Go 实现简单的 PBFT 共识。共识具体示意图如下：

<img src="https://img2018.cnblogs.com/blog/1146526/201901/1146526-20190109001706309-977129049.png" alt="PBFT共识" style="zoom:80%;" />

#### 1. 节点定义

既然是共识，肯定会有几点，在本共识中，节点定义如下：

``` go
type Node struct {
	NodeID        string
	NodeTable     map[string]string
	View          *View
	CurrentState  *State
	CommitMsgs    []*RequestMsg
	MsgBuffer     *MsgBuffer
	MsgEntrance   chan interface{}
	MsgDelivery   chan interface{}
	Alarm         chan bool
}
```

其中比较重要的是 `CurrentState`，它决定着每一个节点处于什么状态并且需要执行什么操作，其具体定义如下：

```go
type State struct {
	...
	CurrentStage Stage
}
type Stage int
const (
	Idle       Stage = iota // Node is created successfully, but the consensus process is not started yet.
	PrePrepared              // The ReqMsgs is processed successfully. The node is ready to head to the Prepare stage.
	Prepared                 // Same with `prepared` stage explained in the original paper.
	Committed                // Same with `committed-local` stage explained in the original paper.
)
```

`NodeTable`是一个`map`类型，主要用于存放相关节点的`NodeID`及其`url`。其他的变量都与信息的传递有关，我们之后会讲到。

再来说一下`View`字段，在`PBFT`中有一个视图`view`的概念，在一个视图里，一个是主节点，其余的都叫备份节点。主节点负责将来自客户端的请求排好序，然后按序发送给备份节点。但主节点可能是拜占庭的：它会给不同的请求编上相同的序号，或者不去分配序号，或者让相邻的序号不连续。备份节点应当有职责来主动检查这些序号的合法性，并能通过`timeout`机制检测到主节点是否已经宕掉。当出现这些异常时，这些备份节点就会触发视图更换`view change`协议选举出新的主节点。

视图是一个连续编号的整数。主节点由公式$p=v\ mod\ |R|$得到，$v$是视图编号，$p$是副本编号，$|R|$是副本集合的个数。View`字段中应该不只有当前视图`ID`，还应保存着当前主节点的`ID`，故`View`字段的定义如下：

```go
type View struct {
	ID      int64
	Primary string
}
```

#### 2. 消息定义

`MsgBuffer`是`Node`节点中用于接收信息的一个`buffer`，`PBFT`中消息的类型主要有四种：

* `ReqMsgs`：请求信息
* `PrePrepareMsgs`：预准备阶段信息
* `PrepareMsgs`：准备阶段信息
* `CommitMsgs`： 提交阶段信息

`MsgBuffer`就是用于接收这些信息的：

```go
type MsgBuffer struct {
	ReqMsgs        []*RequestMsg
	PrePrepareMsgs []*PrePrepareMsg
	PrepareMsgs    []*VoteMsg
	CommitMsgs     []*VoteMsg
}
```

在具体实现中，`PrepareMsgs`和`CommitMsgs`都属于投票信息，因此可以使用同样的结构进行定义。

##### 2.1 RequestMsg

`RequestMsg`定义如下：

```go
type RequestMsg struct {
	Timestamp  int64  `json:"timestamp"`
	ClinetID   string `json:"clientID"`
	Operation  string `json:"operation"`
	SequenceID int64  `json:"sequenceID"`
}
```

`SequenceID`表示的是发送的信息的序列号，因为需要对请求进行排序。`ClientID`表示的是发送信息的客户端`ID`。

##### 2.2 PrePrepareMsgs

`PrePrepareMsgs`定义如下：

```go
type PrePrepareMsg struct {
	ViewID     int64       `json:"viewID"`
	SequenceID int64       `json:"sequenceID"`
	Digest     string      `json:"digest"`
	RequestMsg *RequestMsg `json:"requestMsg"`
}
```

除了RequestMsg之外，其余节点都是处于视图里面的操作，因此都会涉及到`ViewID`字段。而且，为保证信息准确，会涉及到签名`Digest`字段。

##### 2.3 VoteMsg

整个`PBFT`共识会涉及到两次投票，因此投票信息既适用于`PrepareMsgs`，也适用于`CommitMsgs`。具体定义如下：

```go
type VoteMsg struct {
	ViewID     int64  `json:"viewID"`
	SequenceID int64  `json:"sequenceID"`
	Digest     string `json:"digest"`
	NodeID     string `json:"nodeID"`
	MsgType           `json:"msgType"`
}
```

`MsgType`就决定着是属于哪一阶段的投票信息：

```go
type MsgType int
const(
	PrepareMsg MsgType = iota
	CommitMsg
)
```

##### 2.4 ReplyMsg

在整个共识结束后，还会将消息传递回客户端，此时需要一个`ReplyMsg`，具体定义如下：

```go
type ReplyMsg struct {
	ViewID    int64  `json:"viewID"`
	Timestamp int64  `json:"timestamp"`
	ClientID  string `json:"clientID"`
	NodeID    string `json:"nodeID"`
	Result    string `json:"result"`
}
```

`Result`即返回的最后结果。

#### 3. 启动节点

基本定义介绍完后，我们就可以考虑开始研究共识内部的内容了，第一步肯定是要先创建节点了，其实就是初始化`Node`结构体的过程：

```go
func NewNode(nodeID string) *Node {
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
	return node
}
```

因为我们暂时还没有`ViewID`，所以我们暂定`const viewID = 10000000000`。

除了基本的初始化外，我们还需要**对信息进行调度**，**处理信息**，**设立一个负责检测节点正常运行的通知器**。

```go
	//  Start message dispatcher
	go node.dispatchMsg()
	// start alarm trigger
	go node.alarmToDispatcher()
	// start message resolver
	go node.resolveMsg()
```

##### 3.1 node.dispatchMsg

在处理信息时，信息可能会有两大类，即**普通信息**和**通知信息**，普通信息正常处理即可，通知信息则需要额外的处理方式：

```go
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
```

##### 3.1.1 node.routeMsg

其实就是对于前面所提及的四种信息进行顺序处理(`RequestMsg`, `PrePrepareMsg`, `PrepareMsg`, `CommitMsg`).

处理时仍然分成三种情况来处理，但`RequestMsg`, `PrePrepareMsg`,这两种处理方式实质上是基本一致的，具体执行操作流程图如下：

<img src="https://github.com/dengchengH/goPBFT/blob/master/routeMsg1.png?raw=true" alt="routeMsg1" style="zoom:75%;" />

逻辑代码如下：

```go
		if node.CurrentState == nil {
			msgs := make([]*consensus.(MsgType), len(node.MsgBuffer.(MsgType)))
			copy(msgs, node.MsgBuffer.(MsgType))
			// copy 之后添加新的信息
			msgs = append(msgs, msg.(*consensus.(MsgType))
			// 将 buffer 清空
			node.MsgBuffer.(MsgType) = make([]*consensus.(MsgType), 0)
			// 开始发送消息
			node.MsgDelivery <- msgs
		} else {
			// 否则直接添加进 buffer 中
			node.MsgBuffer.(MsgType) = append(node.MsgBuffer.(MsgType), msg.(*consensus.(MsgType)))
		}
```

`(MsgType)`既可以表示为`RequestMsg`，也可以表示为`PrePrepareMsg`。

而`PrepareMsg`, `CommitMsgs`的`routeMsg`与前面两种信息处理逻辑完全相反，且在判断条件上稍加了修改，对于`PrepareMsg`来说：

```go
// 判断条件
if node.CurrentState == nil || node.CurrentState.CurrentStage != consensus.PrePrepared
```

对于`CommitMsg`来说：

```go
// 判断条件
if node.CurrentState == nil || node.CurrentState.CurrentStage != consensus.Prepared
```

流程图如下所示：

<img src="https://github.com/dengchengH/goPBFT/blob/master/routeMsg2.png?raw=true" alt="routeMsg2" style="zoom:75%;" />

##### 3.1.2 node.routeMsgWhenAlarmed

在该方法中，对于四种信息的处理方式是完全一致的，唯一的区别的就是执行条件（同上一节的内容）。具体处理流程如下：

<img src="https://github.com/dengchengH/goPBFT/blob/master/routeMsgWhenAlarmed.png?raw=true" alt="routeMsgWhenAlarmed" style="zoom:75%;" />

逻辑代码如下：

```go
msgs := make([]*consensus.(MsgType), len(node.MsgBuffer.(MsgType)))
copy(msgs, node.MsgBuffer.(MsgType))
node.MsgDelivery <- msgs
```

`(MsgType)`表示的是前面所提及的`RequestMsg`, `PrePrepareMsg`, `PrepareMsg`, `CommitMsg`四种信息。

#####  3.2 node.alarmToDispatcher

该方法是当出现特殊情况时才会触发，这里先假定每隔一段时间执行一次。

```go
func (node *Node) alarmToDispatcher() {
	for {
		time.Sleep(20 * time.Second)
		node.Alarm <- true
	}
}
```

##### 3.3 node.resolveMsg

该方法是对前面通过`node.MegDelivery`接收的信息进行处理，对于每一个类型的信息又有不同的处理方式。

##### 3.3.1  node.resolveRequestMsg

该方法对`Message`序列进行依次处理，处理方法为 `GetReq()`。

当节点的`CurrentState`为空时即可调用该方法。此方法会初始化共识，并开始执行共识：

```go
func (node *Node) GetReq(reqMsg *consensus.RequestMsg) error {
	err := node.createStateForNewConsensus()
	if err != nil {
		return err
	}
	// 开始执行共识
	prePrepareMsg, err := node.CurrentState.StartConsensus(reqMsg)
	if err != nil {
		return nil
	}
	// 发送 getPrePrepare 信息
	if prePrepareMsg != nil {
		node.Broadcast(prePrepareMsg, "/preprepare")
	}
	return nil
}
```

在开始共识时，信息类型就变成了`PrePrepareMsg`，当开始执行共识时，会返回该类型的信息序列，当序列不为空时，即可对所有的节点进行广播。

##### 3.3.2 node.resolvePrePrepareMsg

该方法对`Message`序列进行依次处理，处理方法为 `GetPrePrepare()`。

当节点的`CurrentState`为空时即可调用该方法。此方法会将共识转换至`Prepare`状态：

```go
func (node *Node) GetPrePrepare(prePrepareMsg *consensus.PrePrepareMsg) error {
	err := node.createStateForNewConsensus()
	if err != nil {
		return err
	}
	prePareMsg, err := node.CurrentState.PrePrepare(prePrepareMsg)
	if err != nil {
		return err
	}
	if prePareMsg != nil {
		prePareMsg.NodeID = node.NodeID
		node.Broadcast(prePareMsg, "/prepare")
	}
	return nil
}
```

通过该方法处理后，信息类型此时转化成`PrepareMsg`类型，同样需要将信息进行广播。

##### 3.3.3 node.resolvePrepareMsg

该方法对`Message`序列进行依次处理，处理方法为 `GetPrepare()`。

当节点的`CurrentState`不为空是才可调用该方法。此方法会将共识转换至`Commit`状态：

```go
func (node *Node) GetPrepare(prepareMsg *consensus.VoteMsg) error {
	commitMsg, err := node.CurrentState.Prepare(prepareMsg)
	if err != nil {
		return err
	}
	if commitMsg != nil {
		commitMsg.NodeID = node.NodeID
		node.Broadcast(commitMsg, "/commit")
	}
	return nil
}
```

通过该方法处理后，信息类型此时转化成`CommitMsg`类型，将信息进行广播。

##### 3.3.4 node.resolveCommitMsg

该方法对`Message`序列进行依次处理，处理方法为 `GetPrepare()`。

当节点的`CurrentState`不为空是才可调用该方法。此方法会将共识转换至`Reply`状态：

```go
func (node *Node) GetCommit(prepareMsg *consensus.VoteMsg) error {
	replyMsg, committedMsg, err := node.CurrentState.Commit(prepareMsg)
	if err != nil {
		return err
	}
	if replyMsg != nil {
		if committedMsg == nil {
			return errors.New("committed message is nil, even though the reply message is not nil")
		}
		replyMsg.NodeID = node.NodeID
		node.CommitMsgs = append(node.CommitMsgs, committedMsg)
		node.Reply(replyMsg)
	}
	return nil
}
```

通过该方法处理后，信息类型此时转化成`ReplyMsg`类型，将信息进行回复，注意在`Commit`阶段内，信息不能为空，若为空时，应进行错误处理。

将回复信息进行简单的输出：

```go
func (node *Node) GetReply(msg *consensus.ReplyMsg) {
	fmt.Printf("Result: %s by %s\n", msg.Result, msg.NodeID)
}
```

##### 3.3.5 node.createStateForNewConsensus

在前两种信息处理过程中，发现在每次执行开始时，由于`CurrentState`为空，需要创建一个新状态：

```go
func (node *Node) createStateForNewConsensus() error {
	// 先检查是否有存在的状态
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
	// 创建一个新的状态
	node.CurrentState = consensus.CreateState(node.View.ID, lastSequenceID)
	return nil
}
```

##### 3.3.6 node.Broadcast

在进行广播时，需要用到在一开始`Node`中的`NodeTable`进行广播。广播的消息需要进行`json`处理：

```go
func (node *Node) Broadcast(msg interface{}, path string) map[string]error {
	errorMap := make(map[string]error)
	for nodeID, url := range node.NodeTable {
		if nodeID == node.NodeID {
			continue
		}
		jsonMsg, err := json.Marshal(msg)
		send(url + path, jsonMsg)
	}
	return errorMap
}
```

##### 3.3.7 node.Reply

`Reply`即将信息最终`ReplyMsg`信息发送给客户端。

```go
func (node *Node) Reply(msg *consensus.ReplyMsg) error {
	for _, value := range node.CommitMsgs {
		fmt.Printf("Committed value: %s, %d, %s, %d", value.ClinetID, value.Timestamp, value.Operation, value.SequenceID)
	}
	jsonMsg, err := json.Marshal(msg)
	...
	return nil
}
```

#### 4. 网络信息传递

网络信息传递其实就是使用`http`进行信息处理。

首先需要设置路由：

```go
func (server *Server) setRoute() {
	http.HandleFunc("/req", server.getReq)
	http.HandleFunc("/preprepare", server.getPrePrepare)
	http.HandleFunc("/prepare", server.getPrepare)
	http.HandleFunc("/commit", server.getCommit)
	http.HandleFunc("/reply", server.getReply)
}
```

不同的请求对应的路由不同。但每个路由的处理方法基本上是一致的。

##### 4.1 getXXX

处理信息的方式都是按以下流程执行：

```go
func (server *Server) getXXX(w http.ResponseWriter, r *http.Request) {
	var msg consensus.XXXMsg
	err := json.NewDecoder(r.Body).Decode(&msg)
	server.node.MsgEntrance <- &msg
}
```

##### 4.2 send

```go
func send(url string, msg []byte) {
	buff := bytes.NewBuffer(msg)
	http.Post("http://" + url, "application/json", buff)
}
```

信息发送方法实质上就是调用了`http.Post`方法。

#### 5. 共识过程

最后是共识部分，该部分控制着主要的共识操作。

##### 5.1 f 的选择

根据`PBFT`的原理，节点个数$N$应当大于等于$3f+1$，这里的$f$是指出现故障/拜占庭节点的数量。在进行`Node`初始化的时候启动了4个节点，故此时f的选择为1。

##### 5.2 CreateState

在第3部分中，当`node.CurrentState`为空时，需要进行状态创建，具体`State`结构如下：

```go
type State struct {
	ViewID         int64
	MsgLogs        *MsgLogs
	LastSequenceID int64
	CurrentStage   Stage
}
type MsgLogs struct {
	ReqMsg         *RequestMsg
	PrepareMsgs    map[string]*VoteMsg
	CommitMsgs     map[string]*VoteMsg
}
```

`Stage`的定义在前面提及过，在这里不再赘述。

对于`CreateState`来说，可以认为是对`State`状态执行初始化：

```go
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
```

第3部分中所提及的对于各种状态的处理函数中，都需要执行共识部分的一些转换函数以保证共识状态实现转换。

##### 5.2.1 StartConsensus

`StartConsensus`对应的是`Request`阶段，其目的是将共识状态转换至`PrePrepare`状态：

```go
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
	// 将状态转换为 pre-prepared
	state.CurrentStage = PrePrepared
	return PrePrepareMsg
}
```

在执行签名时，使用的是`sha256`方法进行签名，最终返回的是`PrePrepareMsg`，表示已经转换至`PrePrepare`状态。

##### 5.2.2 PrePrepare

`PrePrepare`方法对应的是`PrePrepare`阶段，其目的是将共识状态转换至`Prepare`状态：

```go
func (state *State) PrePrepare(prePrepareMsg *PrePrepareMsg) (*VoteMsg, error) {
	// 获取 msg 并将其放入 log 中
	state.MsgLogs.ReqMsg = prePrepareMsg.RequestMsg
	// 检验信息正确与否
	if !state.verifyMsg(prePrepareMsg.ViewID, prePrepareMsg.SequenceID, prePrepareMsg.Digest) {
		return nil, errors.New("pre-prepare message is corrupted")
	}
	// 将状态更改为 pre-prepare
	state.CurrentStage = PrePrepared
	return VoteMsg
}
```

在对由前一阶段转换后的`PrePrepareMsg`进行验证后，将状态转换至`Prepare`，并生成`PrepareMsg`。

##### 5.2.3 Prepare

`Prepare`方法对应的是`Prepare`阶段，其目的是将共识状态转换至`Commit`状态：

```go
func (state *State) Prepare(prepareMsg *VoteMsg) (*VoteMsg, error) {
	if !state.verifyMsg(prepareMsg.ViewID, prepareMsg.SequenceID, prepareMsg.Digest) {
		return nil, errors.New("Prepare message is corrupted.")
	}
	// 输出当前投票信息
	fmt.Printf("[Prepare-Vote]: %d\n", len(state.MsgLogs.PrepareMsgs))
	if state.prepared() {
		// 更改当前状态至 prepared
		state.CurrentStage = Prepared
		return VoteMsg
	}
	return nil, nil
}
```

在对由前一阶段转换后的`PrepareMsg`进行验证后，将状态转换至`Commit`，并生成`CommitMsg`。需要注意的是，在对其进行转换前，还需要对状态进行一次判定，以确定该状态为`prepared`，这样才能够将其转换至`CommitMsg`类型。

`prepared`方法具体逻辑如下：

```go
func (state *State) prepared() bool {
	if state.MsgLogs.ReqMsg == nil {
		return false
	}
	if len(state.MsgLogs.PrepareMsgs) < 2 * f {
		return false
	}
	return true
}
```

该方法的主要作用实际上就是在检查该共识的得票数是否达到标准，若达到标准则继续执行后面的操作，否则本次共识失败。

##### 5.2.4 Commit

`Commit`方法对应的是`Commit`阶段，其目的是将共识状态转换至`Reply`状态：

```go
func (state *State) Commit(commitMsg *VoteMsg) (*ReplyMsg, *RequestMsg, error) {
	if !state.verifyMsg(commitMsg.ViewID, commitMsg.SequenceID, commitMsg.Digest) {
		return nil, nil, errors.New("commit message is corrupted")
	}
	if state.committed() {
		// 此节点在本地执行请求的操作并获取结果。
		result := "Executed"
		// 更改状态至 committed
		state.CurrentStage = Committed
		return ReplyMsg
	}
	return nil, nil, nil
}
```

其中涉及到`committed`方法，该方法与前面所述的`prepared`具有同样的功能。

##### 5.2.5 verifyMsg

该方法主要是用来检查消息的准确性：

```go
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
	// 检验 digest
	if digestGot != digest {
		return false
	}
	return true
}
```

经过上述步骤，完整的`PBFT`基本实现。