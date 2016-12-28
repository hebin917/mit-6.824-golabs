package raft

import (
	"fmt"
	"time"
)

func (rf *Raft) electionTimer() {
	timer := time.NewTimer(randInterval())

	win := make(chan bool)
	go rf.election(win)

	for {
		select {
		case <-rf.kill:
			return

		// win to be a leader
		case <-win:
			return

		// receive a leader request
		case <-rf.isStaleCh:
			return

		// re-election
		case <-timer.C:
			randInterval := randInterval()
			timer.Reset(randInterval)
			fmt.Printf("rf[%d] re-election after[%d]\n", rf.me, randInterval)
			win = make(chan bool)
			go rf.election(win)
		}
	}
}

func (rf *Raft) election(win chan bool) {
	rf.mu.Lock()
	rf.role = Candicate
	rf.votedFor = rf.me
	rf.currentTerm++
	rf.mu.Unlock()
	grantedChan := make(chan *RequestVoteReply, len(rf.peers))

	// sends voting request to all peers
	for i := 0; i < len(rf.peers); i++ {
		if i == rf.me {
			continue
		}

		args := RequestVoteArgs{
			Term:         rf.currentTerm,
			CandidateID:  rf.me,
			LastLogIndex: len(rf.logs),
			LastLogTerm:  rf.logs[len(rf.logs)-1].Term,
		}
		reply := &RequestVoteReply{}

		go func(index int) {
			// fmt.Printf("send vote request to rf[%d]\n", index)
			if rf.sendRequestVote(index, args, reply) {
				grantedChan <- reply
			}
		}(i)
	}

	grantedCount := 1
	for replty := range grantedChan {
		if replty.VoteGranted {
			grantedCount++
		}

		// convert to be a leader and stop election timer
		if grantedCount > len(rf.peers)/2 {
			fmt.Printf("Term[%d] - rf[%d] to be a leader, grantedCount=%d\n", rf.currentTerm, rf.me, grantedCount)
			rf.mu.Lock()
			rf.role = Leader
			rf.mu.Unlock()
			go rf.broadcastHeartBeatTimer()
			win <- true
			return
		}
	}
}
