package raft

import (
	"fmt"
	"time"
)

func (rf *Raft) broadcastHeartBeatTimer() {
	timer := time.NewTimer(time.Duration(HeartBeatInterval) * time.Millisecond)
	go rf.broadcastHeartBeat()

	for {
		select {
		case <-rf.kill:
			return
		case <-rf.isStaleCh:
			return
		case <-timer.C:
			timer.Reset(time.Duration(HeartBeatInterval) * time.Millisecond)
			go rf.broadcastHeartBeat()
		}
	}
}

func (rf *Raft) broadcastHeartBeat() {
	replyCh := make(chan *AppendEntriesReply, len(rf.peers))

	for i := 0; i < len(rf.peers); i++ {
		if i == rf.me {
			continue
		}

		args := AppendEntriesArgs{
			Term:         rf.currentTerm,
			LeaderID:     rf.me,
			PrevLogIndex: 0,
			PrevLogTerm:  -1,
			Entries:      []Entry{},
			LeaderCommit: 0,
		}
		reply := &AppendEntriesReply{}

		go func(index int) {
			fmt.Printf("rf[%d] send heartbeat at[%d]\n", rf.me, time.Now().UnixNano()/1000000)
			if rf.sendRequestAppendEntries(index, args, reply) {
				replyCh <- reply
			}
		}(i)
	}

	for reply := range replyCh {
		if !reply.Success {
			// update args
		}

	}
}
