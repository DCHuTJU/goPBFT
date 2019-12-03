package network

import (
	"PBFT/consensus"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type Server struct {
	url string
	node *Node
}

func NewServer(nodeID string) *Server {
	node := NewNode(nodeID)
	server := &Server{node.NodeTable[nodeID], node}
	server.setRoute()
	return server
}

func (server *Server) setRoute() {
	http.HandleFunc("/req", server.getReq)
	http.HandleFunc("/preprepare", server.getPrePrepare)
	http.HandleFunc("/prepare", server.getPrepare)
	http.HandleFunc("/commit", server.getCommit)
	http.HandleFunc("/reply", server.getReply)
}

func (server *Server) getReq(w http.ResponseWriter, r *http.Request) {
	var msg consensus.RequestMsg
	err := json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getPrePrepare(w http.ResponseWriter, r *http.Request) {
	var msg consensus.PrePrepareMsg
	err := json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getPrepare(w http.ResponseWriter, r *http.Request) {
	var msg consensus.VoteMsg
	err := json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getCommit(w http.ResponseWriter, r *http.Request) {
	var msg consensus.VoteMsg
	err := json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getReply(w http.ResponseWriter, r *http.Request) {
	var msg consensus.ReplyMsg
	err := json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.GetReply(&msg)
}

func send(url string, msg []byte) {
	buff := bytes.NewBuffer(msg)
	http.Post("http://" + url, "application/json", buff)
}

func (server *Server) Start() {
	fmt.Printf("Server will be started at %s...\n", server.url)
	if err := http.ListenAndServe(server.url, nil); err != nil {
		fmt.Println(err)
		return
	}
}