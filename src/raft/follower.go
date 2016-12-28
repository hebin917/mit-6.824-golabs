package raft

import (
	"fmt"
	"time"
)

func (rf *Raft) isNewTerm(term int) bool {
	return rf.currentTerm >= term
}

func (rf *Raft) convertToFollower(newTerm int, votedFor int) {
	rf.mu.Lock()
	rf.role = Follower
	rf.currentTerm = newTerm
	rf.votedFor = votedFor
	rf.mu.Unlock()
}

func (rf *Raft) resetHeartBeatTimer() {
	go func() {
		rf.isStaleCh <- true
	}()

	if rf.role != Follower {
		fmt.Printf("rf[%d] set heartbeatTime\n", rf.me)
		go rf.heartBeatTimer()
	}
}

func (rf *Raft) heartBeatTimer() {

	timer := time.NewTimer(randInterval())

	// loop hear heartbeat
	for {
		// one turn of waitTime
		select {
		// rf is killed
		case <-rf.kill:
			return

		// receive a heartbeat for leader
		case <-rf.isStaleCh:
			fmt.Printf("rf[%d] heartBeatTimer to be reseted, timestamp=[%d]\n", rf.me, time.Now().UnixNano()/1000000)
			// reset timer, next turn
			timer.Reset(randInterval())

		// time out, to be a candicate and do election, stop heartBeatTimer
		case <-timer.C:
			fmt.Printf("rf[%d, role=[%d], term=[%d]] timeout to election, timestamp=[%v]\n", rf.me, rf.role, rf.currentTerm, time.Now().UnixNano()/1000000)
			go rf.electionTimer()
			return
		}
	}
}
