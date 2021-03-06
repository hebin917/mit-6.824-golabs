6.824 2016 Lecture 6: Raft (2)

*** continued from previous lecture

reminder:
  new leader forces followers' logs to be identical to leader's log
  through AppendEntries prevLogTerm mechanism

could new leader roll back *executed* entries from end of previous term?
  i.e. could an executed entry be missing from the new leader's log?
  this would be a disaster -- violates State Machine Safety
  solution: Raft won't elect a leader that might not have an executed entry

could we choose leader with longest log?
  example:
    S1: 5 6 7
    S2: 5 8
    S3: 5 8
  first, could this scenario happen? how?
    S1 leader in term 6; crash+reboot; leader in term 7; crash and stay down
      both times it crashed after only appending to its own log
    S2 leader in term 8, only S2+S3 alive, then crash
  who should be next leader?
    S1 has longest log, but entry 8 could have been executed !!!
    so new leader can only be one of S2 or S3
    i.e. the rule cannot be simply "longest log"

end of 5.4.1 explains "at least as up to date" voting rule
  compare last entry -- higher term wins
  if equal terms, longer log wins
  so only S3 or S3 can be leader, will force S1 to discard 6,7
    ok since no majority -> not executed -> no client reply

the point:
  "at least as up to date" rule ensures new leader's log contains
    all potentially executed entries
  so new leader won't roll back any executed operation

The Question (from last lecture)
  figure 7, top server is dead; which of a/d/f can be elected?
  i.e. majority of votes from "less up to date" servers?

depending on who is elected leader in Figure 7, different entries
  will end up committed or discarded
  c's 6 and d's 7,7 may be discarded OR committed
  some will always remain committed: 111445566

how to roll back quickly
  the Figure 2 design backs up one entry per RPC -- slow!
  lab tester probably requires faster roll-back
  S1: 4 4 4 4
  S2: 4 4 5 5 6 6
  S3: 4 4 5 5
  S3 just elected term=7 leader (S2 not in its election majority)
  paper outlines a scheme towards end of Section 5.3:
  if follower rejects, includes this in reply:
    the term of the conflicting entry
    the # of the first entry for conflicting term
  if leader knows about the conflicting term:
    move nextIndex[i] back to its last entry for the conflicting term
  else:
    move nextIndex[i] back to follower's first index

why "log[N].term==currentTerm" in figure 2's Rules for Servers?
  why can't we execute any entry that's on a majority?
    how could such an entry be discarded?
  figure 8 describes an example
  S1: 1 2     1 2 4
  S2: 1 2     1 2
  S3: 1   --> 1 2
  S4: 1       1
  S5: 1       1 3
  S1 was leader in term 2, sends out two copies of 2
  S5 leader in term 3
  S1 in term 4, sends one more copy of 2 (b/c S3 rejected op 4)
  what if S5 now becomes leader?
    S5 can get a majority (w/o S1)
    S5 will roll back 2 and replace it with 3
  so "present on a majority" != "committed"

is it OK to forget 2? could it have been executed?
  not when S1 initially sent it, since not initially on majority
  could S1 have mentioned it in leaderCommit after re-elected for term=4?
  no! very end of Figure 2 says "log[N].term == currentTerm"
  and S1 was in term 4 when sending 3rd copy of 2

so an entry becomes commited if:
  1) it reached a majority in the term it was initially sent out, or
  2) if a subsequent log entry becomes committed.
     2 *could* have committed if S1 hadn't lost term=4 leadership

this is a consequence of:
  the "more up to date" voting rule favoring higher term, and
  the leader imposing its log on followers

*** topic: persistence and performance

if a server crashes and restarts, what must it remember?
  Figure 2 lists "persistent state":
    currentTerm, votedFor, log[]
  a Raft server can only re-join after restart if these are intact
  thus it must save them to non-volatile storage after changing them
    before sending any RPC or RPC reply
  non-volatile = disk, SSD, &c
  why log[]?
    if a rebooted server was in server's majority for committing an entry,
      but then forgets, a future server might not see this log entry
  why currentTerm/votedFor?
    to prevent a client from voting for one candidate, then reboot,
      then vote for a different candidate in the same (or older!) term
    could lead to two leaders for a single term

what about the service's state -- the state-machine state?
  does not need to be saved, since Raft persists the entire log
  after restart, Raft will send all entries to service,
    since lastApplied will reset to zero on re-start
  this replay will regenerate complete state-machine state
  (but snapshots change this story a bit)

the need for persistence is often the bottleneck for performance
  a hard disk write takes 10 ms, SSD write takes 0.1 ms
  an RPC takes well under 1 ms (within a single data center)
  so we expect 100 to 10,000 ops/second

how to get better performance?
  (note many services don't need high performance!)
  batch many concurrent client operations in each RPC
    and in each disk write
  use faster non-volatile storage (e.g. battery-backed RAM), 

how to get even higher throughput?
  not by adding servers to a single Raft cluster!
    more servers -> tolerate more simultaneous server failures
    more servers *decrease* throughput: more leader work / operation
  to get more throughput, "shard" or split the data across
    many separate Raft clusters, each with 3 or 5 servers
    different clusters can work in parallel, since they don't interact
    lab 4
  or use Raft for fault-tolerant master and something else
    that replicates data more efficiently than Raft, e.g. GFS chunkservers

Does Raft give up performance to get clarity?
  Most replication systems have similar common-case performance:
    One RPC exchange and one disk write per agreement.
  Some Raft design choices might affect performance:
    Raft follower rejects out-of-order AppendEntries RPCs.
      Rather than saving for use after hole is filled.
      Might be important if network re-orders packets a lot.
    A slow leader may hurt Raft, e.g. in geo-replication.
  What's important for performance in practice?
    What you use it for (every put/get vs occasional re-configuration).
    Batching into RPCs and disk writes.
    Fast path for read-only requests.
    Efficient RPC library.
    Raft is probably compatible with most techniques of this kind.
  Papers with more about performance:
    Zookeeper/ZAB; Paxos Made Live; Harp

*** topic: client behavior, duplicate operations, read operations

how should lab 3 key/value client work?
  client sends Put/Append/Get RPCs to the servers
  how to find leader?
  what if leader crashes?

client RPC loop
  send RPC to a server
  RPC call() will return -- either error (time out), or reply
  if reply, and server says it was leader, return
  if no reply, or server says not leader, try another server
  client should cache last known leader

problem: this will produce duplicate client requests
  client sends to leader S1, S1 gets into log of majority,
    S1 crashes before replying to client
  client sees RPC error, waits for new leader, re-sends to new leader
  new leader puts request in log *again*

we need something like the at-most-once machinery from lecture 2
  each server should remember executed client requests
  [diagram: each server keeps duplicate table in service module]
  if a request appears again the the log
    don't execute it
    reply to client with result from previous execution
  the service does this, not Raft
  lecture 2 explains how to detect duplicates efficiently
    you'll need it for lab 3

all replicas need to maintain a duplicate detection table
  not just leader
  since any server might become leader, and client might re-send to it

is it OK that the saved return value might no longer be up to date?
  C1 sends get(x), servers execute with result 1, leader crashes before reply
  C2 sends put(x, 2)
  C2 re-sends get(x) to new leader, which replies 1 from table
  intuition for answering questions about correct results:
    could application have seen 1 from a non-replicated server?
  yes:
    C1 sends get(x), executes quickly, reply very slow through network,
      so it arrives after C2's put(x,2)
  the formal name for this kind of semantics is "linearizable"
    each operation appears to execute at some point in time,
      and reflects all operations that executed a previous times
    an operation's time is somewhere between send and recv time

an important aside: 
  can leader execute read-only operations locally?
    without sending to followers in AppendEntries and waiting for commit?
    very tempting, since r/o ops may dominate, and don't change state
  why might that be a bad idea?
  how could we make the idea work?

*** topic: log compaction and Snapshots

problem:
  log will get to be huge -- much larger than state-machine state!
  will use lots of memory
  will take a long time to
    read and re-evaluate after reboot
    send to a newly added replica

what constrains how a server can discard old parts of log?
  can't forget un-committed operations
  need to replay if crash and restart
  may be needed to bring other servers up to date

solution: service periodically creates persistent "snapshot"
  [diagram: service with state, snapshot on disk, raft log, raft persistent]
  copy of entire state-machine state through a specific log entry
    e.g. k/v table, client duplicate state
  service writes snapshot to persistent storage (disk)
  service tells Raft it is snapshotted through some entry
  Raft discards log before that entry
  a server can create a snapshot and discard log at any time

you'll implement this in lab 3
  your Raft should *not* store the discarded log entries
    i.e. Go garbage collector must be able to free the memory
  so you'll need a slightly clever data structure
    not, for example, a slice indexed by log index
    perhaps a slice whose zero'th element is index X

relation of snapshot and logs
  snapshot can only contain committed log entries
  so server will only discard committed prefix of log
    anything not known to be committed will remain in log

so a server's on-disk state consists of:
  service's snapshot up to a certain log entry
  Raft's persisted log w/ following log entries
  the combination is equivalent to the full log

what happens on crash+restart?
  service reads snapshot from disk
  Raft reads persisted log from disk
    sends service entries that are committed but not in snapshot

what if leader discards a log entry that's not in follower i's log?
  nextIndex[i] will back up to start of leader's log
  so leader can't repair that follower with AppendEntries RPCs
  (Q: why not have leader discard only entries that *all* servers have?)

what's in an InstallSnapshot RPC? Figures 12, 13
  term
  lastIncludedIndex
  lastIncludedTerm
  snapshot data

what does a follower do w/ InstallSnapshot?
  reject if term is old (not the current leader)
  reject (ignore) if we already have last included index/term,
    or if we've already executed last included index
    it's an old/delayed RPC
  empty the log, replace with fake "prev" entry
  set lastApplied to lastIncludedIndex
  send snapshot in applyCh to service
  service *replaces* its lastApplied, k/v table, client dup table

The Question:
  Could a received InstallSnapshot RPC cause the state machine to go
  backwards in time? That is, could step 8 in Figure 13 cause the state
  machine to be reset so that it reflects fewer executed operations? If
  yes, explain how this could happen. If no, explain why it can't
  happen.

*** topic: configuration change

configuration change (Section 6)
  configuration = set of servers
  every once in a while you might want to
    move to an new set of servers, or
    increase/decrease the number of servers
  human initiates configuration change, Raft manages it
  we'd like Raft to execute correctly across configuration changes

why doesn't a straightforward approach work?
  suppose each server has the list of servers in the current config
  change configuration by telling each server the new list
    using some mechanism outside of Raft
  problem: they will learn new configuration at different times
  example: want to replace S3 with S4
    S1: 1,2,3  1,2,4
    S2: 1,2,3  1,2,3
    S3: 1,2,3  1,2,3
    S4:        1,2,4
  OOPS! now *two* leaders could be elected!
    S2 and S3 could elect S2
    S1 and S4 could elect S1

Raft configuration change
  idea: "joint consensus" stage that includes *both* old and new configuration
  leader of old group logs entry that switches to joint consensus
    Cold,new -- contains both configurations
  during joint consensus, leader gets AppendEntries majority in both old and new
  after Cold,new commits, leader sends out Cnew
  S1: 1,2,3  1,2,3+1,2,4
  S2: 1,2,3
  S3: 1,2,3
  S4:        1,2,3+1,2,4
  no leader will use Cnew until Cold,new commits in *both* old and new.
    so there's no time at which one leader could be using Cold
    and another could be using Cnew
  if crash but new leader didn't see Cold,new
    then old group will continue, no switch, but that's OK
  if crash and new leader did see Cold,new,
    it will complete the configuration change