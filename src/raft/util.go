package raft

import (
	"log"
	"math/rand"
	"time"
)

// Debugging
const Debug = 0

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}

func randInterval() time.Duration {
	rand.Seed(time.Now().UnixNano())
	waitMs := HeartHeatTimeOutBase + rand.Intn(HeartBeatTimeOutRange)
	return (time.Millisecond * time.Duration(waitMs))
}
