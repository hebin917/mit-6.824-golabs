package raft

import (
	"fmt"
	"log"
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

		go func(server int) {

			fmt.Printf("rf[%d, term=%d] with logs= %v, nextIndex= %v, matchIndex= %v\n", rf.me, rf.currentTerm, rf.logs, rf.nextIndex, rf.matchIndex)
			entries := []Entry{}
			pervLog := Entry{Term: 0, Command: nil}

			if rf.nextIndex[server] <= len(rf.logs)-1 {
				entries = rf.logs[rf.nextIndex[server]:]
			}

			if rf.nextIndex[server] < len(rf.logs)+1 {
				fmt.Printf("rf.nextIndex[%d]=%d, len(rf.logs)=%d\n", server, rf.nextIndex[server], len(rf.logs))
				pervLog = rf.logs[rf.nextIndex[server]-1]
			}
			args := AppendEntriesArgs{
				Term:         rf.currentTerm,
				LeaderID:     rf.me,
				PrevLogIndex: rf.nextIndex[server] - 1,
				PrevLogTerm:  pervLog.Term,
				Entries:      entries,
				LeaderCommit: rf.commitIndex,
			}
			reply := &AppendEntriesReply{server: server}
			fmt.Printf("rf[%d] send heartbeat to rf[%d] at[%d]\n", rf.me, server, time.Now().UnixNano()/1000000)
			if rf.sendRequestAppendEntries(server, args, reply) {
				replyCh <- reply
			}
		}(i)
	}

	for reply := range replyCh {
		rf.mu.Lock()

		fmt.Printf("reply.server=[%v]\n", reply)
		if reply.Term > rf.currentTerm {
			rf.resetHeartBeatTimer()
			rf.convertToFollower(reply.Term, -1)
		}
		// update nextIndex and matchIndex
		if reply.Success {
			// update nextIndex and matchIndex
			rf.nextIndex[reply.server] = len(rf.logs)
			rf.matchIndex[reply.server] = len(rf.logs) - 1
		} else {
			// decrement nextIndex
			rf.nextIndex[reply.server]--
		}

		rf.mu.Unlock()

		// check and update commitIndex
		// get min log index in currentTerm
		min := 0
		for i := len(rf.logs) - 1; i > 0; i-- {
			if rf.logs[i].Term == rf.currentTerm {
				min = i
			}

			if rf.logs[i].Term > rf.currentTerm {
				log.Fatal("has log with heigher term than currentTerm\n")
			}

			if rf.logs[i].Term < rf.currentTerm {
				// get previous i
				min = i + 1
			}
		}
		// get max of replicated log index
		max := len(rf.logs) - 1

		// check if
		for i := max; i >= min; i-- {
			count := 0
			for _, index := range rf.matchIndex {
				if index >= i {
					count++
				}
			}

			if count > len(rf.peers)/2 {
				if i > rf.commitIndex {
					go rf.commit(rf.commitIndex+1, max)
					rf.mu.Lock()
					rf.commitIndex = i
					rf.mu.Unlock()
				}
				break
			}
		}
	}
}
