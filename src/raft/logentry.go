package raft

// LogEntry for log entry
type LogEntry struct {
	Command interface{}
	Term    int
}
