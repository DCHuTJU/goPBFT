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

其实就是对于前面所提及的四种信息进行处理(`RequestMsg`, `PrePrepareMsg`, `PrepareMsg`, `CommitMsg`).

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

<img src="E:\gitRepository\goPBFT\routeMsgWhenAlarmed.png" alt="routeMsgWhenAlarmed" style="zoom:75%;" />

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

