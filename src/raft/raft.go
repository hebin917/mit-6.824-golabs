package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"labrpc"
	"math"
	"reflect"
	"sync"
)

// Role is Raft role
type Role int

const (
	// Follower is initial role
	Follower Role = 1 + iota
	// Candicate is ready to elect a leader
	Candicate
	// Leader is win the election
	Leader
)

// const for timer
const (
	HeartBeatInterval     int = 60
	HeartHeatTimeOutBase  int = 150
	HeartBeatTimeOutRange int = 150
	ElectionTimeOutBase   int = HeartHeatTimeOutBase
	ElectionTimeOutRange  int = HeartBeatTimeOutRange
)

// import "bytes"
// import "encoding/gob"

// ApplyMsg as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

// Raft is a Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex
	peers     []*labrpc.ClientEnd
	persister *Persister
	me        int // index into peers[]

	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	// persistent states on all servers
	currentTerm int
	votedFor    int
	logs        []Entry

	// volatile state on all server
	commitIndex int
	lastApplied int

	// volatile state on leaders
	nextIndex  []int
	matchIndex []int

	// memory
	role      Role
	isStaleCh chan bool
	applyCh   chan ApplyMsg

	// kill single
	kill chan bool
}

// Entry for log entry
type Entry struct {
	Term    int
	Command interface{}
}

// GetState returns currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	return rf.currentTerm, rf.role == Leader
}

// persists save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
func (rf *Raft) persist() {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.logs)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

// readPersist restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	r := bytes.NewBuffer(data)
	d := gob.NewDecoder(r)
	d.Decode(&rf.currentTerm)
	d.Decode(&rf.votedFor)
	d.Decode(&rf.logs)
}

func (rf *Raft) intializeNextIndex() {
	for i := range rf.nextIndex {
		rf.nextIndex[i] = len(rf.logs)
	}
}

func (rf *Raft) commit(fromIndex, toIndex int) {
	for index := fromIndex; index <= toIndex; index++ {
		msg := ApplyMsg{Index: index, Command: rf.logs[index].Command}
		rf.applyCh <- msg
		fmt.Printf("rf[%d] send ApplyMsg: %v\n", rf.me, msg)
	}
}

// RequestVoteArgs examples RequestVote RPC arguments structure.
type RequestVoteArgs struct {
	Term         int
	CandidateID  int
	LastLogIndex int
	LastLogTerm  int
}

// RequestVoteReply examples RequestVote RPC reply structure.
type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

// RequestVote for voting RPC handler.
func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	// rf.mu.Lock()
	// defer rf.mu.Unlock()

	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm {
		reply.VoteGranted = false
		return
	}

	fmt.Printf("rf[%d, term=%d] vote to a candicate, args=%#v\n", rf.me, rf.currentTerm, args)
	// rf is stale or ready to be a follower
	if (args.Term > rf.currentTerm) ||
		((rf.votedFor == -1 || rf.votedFor == args.CandidateID) && len(rf.logs) <= args.LastLogIndex) {

		rf.resetHeartBeatTimer()
		rf.convertToFollower(args.Term, args.CandidateID)
		reply.VoteGranted = true
		fmt.Printf("rf[%d] voteFor rf[%d]\n", rf.me, rf.votedFor)
	}
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// returns true if labrpc says the RPC was delivered.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

// AppendEntriesArgs for arguments
type AppendEntriesArgs struct {
	Term         int
	LeaderID     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []Entry
	LeaderCommit int
}

// AppendEntriesReply for reply
type AppendEntriesReply struct {
	Term    int
	Success bool
	server  int
}

// RequestAppendEntries handles request for appending entries
func (rf *Raft) RequestAppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) {
	fmt.Printf("rf[%d] args of appendEntries: %#v\n", rf.me, args)

	// request is stale
	if args.Term < rf.currentTerm {
		fmt.Printf("rf[%d, term=%d] stale args:%v", rf.me, rf.currentTerm, args)
		reply.Success = false
		reply.Term = rf.currentTerm
		return
	}

	rf.resetHeartBeatTimer()

	// rf is stale
	if args.Term > rf.currentTerm {
		rf.convertToFollower(args.Term, args.LeaderID)
	}

	// entries is too new
	if len(rf.logs)-1 < args.PrevLogIndex || rf.logs[args.PrevLogIndex].Term != args.PrevLogTerm {
		reply.Success = false
		return
	}

	// entries is ok
	reply.Success = true

	// update rf.logs
	rf.mu.Lock()
	rf.logs = append(rf.logs[:args.PrevLogIndex+1], args.Entries...)
	rf.mu.Unlock()
	fmt.Printf("rf[%d].logs=%v, commitIndex=%d\n", rf.me, rf.logs, rf.commitIndex)

	// update rf.commitIndex
	if args.LeaderCommit > rf.commitIndex {
		newCommitIndex := int(math.Min(float64(args.LeaderCommit), float64(len(rf.logs)-1)))
		fmt.Printf("newCommitIndex=%d\n", newCommitIndex)
		go rf.commit(rf.commitIndex+1, newCommitIndex)
		rf.mu.Lock()
		rf.commitIndex = newCommitIndex
		rf.mu.Unlock()
	}
}

// AppendEntriesReply.Entries is empty
func (rf *Raft) sendRequestHeartbeat(server int, args AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendRequestAppendEntries(server int, args AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.RequestAppendEntries", args, reply)
	return ok
}

// Start the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (index int, term int, isLeader bool) {
	index = -1
	term, isLeader = rf.GetState()
	for i, log := range rf.logs {
		if reflect.DeepEqual(log.Command, command) {
			index = i
		}
	}
	if isLeader && index == -1 {
		index = len(rf.logs)
	}

	if !isLeader {
		return
	}

	// save into rf.logs
	rf.mu.Lock()
	rf.logs = append(rf.logs, Entry{Term: term, Command: command})
	rf.nextIndex[rf.me] = len(rf.logs)
	rf.matchIndex[rf.me] = len(rf.logs) - 1
	fmt.Printf("rf[%d] receive command[%v]\n", rf.me, command)
	rf.mu.Unlock()
	return
}

// Kill the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	rf.kill <- true
}

// Make the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.role = Follower
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.votedFor = -1
	rf.applyCh = applyCh
	rf.logs = []Entry{Entry{0, nil}}
	rf.nextIndex = make([]int, len(peers), len(peers))
	rf.matchIndex = make([]int, len(peers), len(peers))
	rf.isStaleCh = make(chan bool)
	rf.kill = make(chan bool, 1)

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	rf.intializeNextIndex()
	fmt.Printf("rf[%d] set heartbeatTime\n", rf.me)
	go rf.heartBeatTimer()
	return rf
}
