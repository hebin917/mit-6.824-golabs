6.824 2016 Lecture 12: Consistency

New Topic: Consistency
  Plan: Several papers on different consistency models
  It also showed up in previous papers/lectures:
      GFS, Russ's lecture on memory models, Zookeeper
  It showed up in transaction papers under the terms serializability
      Thor, FaRM
  Today's paper in the context of distributed computing
  Two key ideas:
    Lazy-release consistency (introduced by the paper)
    Version vectors (a common implementation technique in distributed systems)

Distributed computing
  The Big Idea: your huge computation on a room full of cheap computers!
  An old and difficult goal; many approaches; much progress; still hot.
  Other cluster computing papers: MapReduce, Spark

Today's approach: distributed shared memory (DSM)
  You all know how to write parallel (threaded) Go programs
  Let's farm the threads out to a big cluster of machines!
  What's missing? shared memory!

DSM plan:
  Programmer writes parallel program: threads, shared variables, locks, &c
  DSM system farms out threads to a cluster of machines
  DSM system creates illusion of single shared memory
  [diagram: LAN, machines w/ RAM, MGR]

DSM advantages
  familiar model -- shared variables, locks, &c
  general purpose (compared to e.g. MapReduce and Spark)
  can use existing apps and libraries written for multiprocessors
  lots of machines on a LAN much cheaper than huge multiprocessor

But:
  machines on a LAN don't actually share memory

General DSM approach (but slow):
  Use hardware's virtual memory protection (r/w vs r/o vs invalid)
  General idea illustrated with 2 machines:
     Part of the address space starts out on M0
	On M1, marked invalid 
     Part of the address space start out on M1
        On M0, marked invalid
  A thread of the application on M1 may refer to an address that lives on M0
    If thread LD/ST to that "shared" address, M1's hardware will take a page fault
       Because page is marked invalid
    OS propagates page fault to DSM runtime
    DSM runtime can fetch page from M0
    DSM on M0, marks page invalid, and sends page to M1
    DSM on M1 receives it from M0, copies it to underlying physical memory
    DSM on M1 marks the page valid
    DSM returns from page fault handler
    Hardware retries LD/ST
  Runs threaded code w/o modification
    e.g. matrix multiply, physical simulation, sort

Challenges:
  Memory model (does memory act like programmers expect it to act?)
    What result does a read observe?
    The last write?
    Stale data?
  Performance (is it fast?)

Example:
  x and y start out = 0
  thread 0:
    x = 1
    if y == 0:
      print yes
  thread 1:
    y = 1
    if x == 0:
      print yes

Would it be OK if both threads printed yes?
  What is the outcome using the general approach?
  Is it even possible to have both threads print "yes"?
  Could they both print "yes" if this were a Go program?
    How could that happen?
    Why is that allowed?
    (See Russ's lecture)

What is a memory model?
  It explains how program reads/writes in different threads interact.
  A contract:
    It gives the compiler/runtime/hardware some freedom to optimize.
    It gives the programmer some guarantees to rely on.
  You need one on a multiprocessor (e.g., for Go).
  You *really* need one for a DSM system.
  You need one for any memory-like or storage system (e.g. the labs).
  There are many memory models!
    With different optimization/convenience trade-offs.
    Often called a consistency model.

Summary of consistency models seen so far

  These terms refer to formal definitions of the results that different
  observers can see from concurrent operations on shared data.
  
  Linearizability and sequential consistency
  
    Individual  reads  and  writes  (or   load  and  store  instructions).  Both
    linearizability  and sequential  consistency  require that  load results  be
    consistent with  all the  operations being  executed one at  a time  in some
    order. They  differ in that  linearizability requires  that if I  observe my
    store to complete, and I tell you so  on the telephone, and then you start a
    load of the same data, you will see my written value. Sequential consistency
    doesn't guarantee that: loads can  become visible to other CPUs considerably
    after  they appear  to  the  issuer to  complete.  Programmers would  prefer
    linearizability, but  real-world CPUs  mostly provide semantics  even weaker
    than  sequential consistency  (e.g. Total  Store Order).  Linearizability is
    seen more often  in the world of  storage systems; Lab 3,  for example, will
    have linearizable puts/gets.

  Strict serializability and serializability
    the corresponding notions for transactions, where a transaction can involve
    reads and writes of multiple records. Both require that the transactions
    yield results as if they executed one at a time in some order. Strict
    serializability additionally requires that if I see my transaction complete,
    and then you start your transaction, that your transaction sees my
    transaction's writes. Programmers often think of databases as providing
    strict serializability, but in fact they usually provide somewhat weaker
    semantics; MySQL/InnoDB, for example, by default provides "Repeatable
    Reads", which can allow a transaction that scans a table twice to observe
    records created by a concurrent transaction.

  The Thor paper uses "external consistency" to mean the same thing as "strict serializability".  

Main trade-off in consistency models
  Lax model => greater freedom to optimize
  Strict model => matches programmer intuition (e.g. read sees latest write)
  This tradeoff is a huge factor in many designs
  Treadmarks is a case study of relaxing to improve performance

Treadmarks high level goals?
  Better DSM performance
  Run existing parallel code

What specific problems with previous DSM are they trying to fix?
  false sharing: two machines r/w different vars on same page
    M1 writes x, M2 writes y
    M1 writes x, M2 just reads y
    Q: what does the general approach do in this situation?
  write amplification: a one byte write turns into a whole-page transfer

First Goal: eliminate write amplification
  don't send whole page, just written bytes

Big idea: write diffs
  on M1 write fault:
    tell other hosts to invalidate but keep hidden copy
    M1 makes hidden copy as well
  on M2 fault:
    M2 asks M1 for recent modifications
    M1 "diffs" current page against hidden copy
    M1 send diffs to M2 (and all machines w/ copy of this page)
    M2 applies diffs to its hidden copy
    M1 marks page r/o

Q: do write diffs change the consistency model?
   At most one writeable copy, so writes are ordered
   No writing while any copy is readable, so no stale reads
   Readable copies are up to date, so no stale reads
   Still sequentially consistent
git
Q: do write diffs fix false sharing?

Next goal: allow multiple readers+writers
  to cope with false sharing
  => don't invalidate others when a machine writes
  => don't demote writers to r/o when another machine reads
  => multiple *different* copies of a page!
     which should a reader look at?
  diffs help: can merge writes to same page
  but when to send the diffs?
    no invalidations -> no page faults -> what triggers sending diffs?

Big idea: release consistency (RC)
  no-one should read data w/o holding a lock!
    so let's assume a lock server
  send out write diffs on release
    to *all* machines with a copy of the written page(s)

Example 1 (RC and false sharing)
x and y are on the same page
M0: a1 for(...) x++ r1
M1: a2 for(...) y++ r2  a1 print x, y r1
What does RC do?
  M0 and M1 both get cached writeable copy of the page
  during release, each computes diffs against original page,
    and sends them to all copies
  M1's a1 causes it to wait until M0's release
    so M1 will see M0's writes

Q: what is the performance benefit of RC?
   What does the general approach do with Example 1?
   multiple machines can have copies of a page, even when 1 or more writes
   => no bouncing of pages due to false sharing
   => read copies can co-exist with writers

Q: does RC change the consistency model? yes!
   M1 won't see M0's writes until M0 releases a lock
   I.e. M1 can see a stale copy of x; not possible w/ general approach
   if you always lock:
     locks force order -> no stale reads

Q: what if you don't lock?
   reads can return stale data
   concurrent writes to same var -> trouble

Q: does RC make sense without write diffs?
   probably not: diffs needed to reconcile concurrent writes to same page

Big idea: lazy release consistency (LRC)
  only send write diffs to next acquirer of released lock,
    not to everyone

Example 2 (lazyness)
x and y on same page (otherwise general-approach avoids copy too)
everyone starts with a copy of that page
M0: a1 x=1 r1
M1:           a2 y=1 r2
M2:                     a1 print x r1
What does LRC do?
  M2 only asks previous holder of lock 1 for write diffs
  M2 does not see M1's y=1, even tho on same page (so print y would be stale)
What does RC do?
What does general-approach do?

Q: what's the performance win from LRC?
   if you don't acquire lock on object, you don't see updates to it
   => if you use just some vars on a page, you don't see writes to others
   => less network traffic

Q: does LRC provide the same consistency model as RC?
   no: LRC hides some writes that RC reveals
   in above example, RC reveals y=1 to M2, LRC does not reveal
   so "M2: print x, y" might print fresh data for RC, stale for LRC
     depends on whether print is before/after M1's release

Q: is LRC a win over general approach if each variable on a separate page?
   or a win over general approach plus write diffs?
   note general approach's fault-driven page reads are lazy at page granularity

Do we think all threaded/locking code will work with LRC?
  Stale reads unless every shared memory location is locked!
  Do programs lock every shared memory location they read?
  No: people lock to make updates atomic.
      if no concurrent update possible, people don't lock.

Example 3 (programs don't lock all shared data)
x, y, and z on the same page
M0: x := 7 a1 y = &x r1
M1:                    a1 a2 z = y r2 r1
M2:                                       a2 print *z r2
will M2 print 7?
LRC as described so far in this lecture would *not* print 7!
  M2 will see the pointer in z, but will have stale content in x's memory.

For real programs to work, Treadmarks must provide "causal consistency":
  when you see a value,
    you also see other values which might have influenced its computation.
  "influenced" means "processor might have read".

How to track which writes influenced a value?
  Number each machine's releases -- "interval" numbers
  Each machine tracks highest write it has seen from each other machine
    a "Vector Timestamp"
  Tag each release with current VT
  On acquire, tell previous holder your VT
    difference indicates which writes need to be sent
  (annotate previous example)

VTs order writes to same variable by different machines:
M0: a1 x=1 r1  a2 y=9 r2
M1:              a1 x=2 r1
M2:                           a1 a2 z = x + y r2 r1
M2 is going to hear "x=1" from M0, and "x=2" from M1.
  How does M2 know what to do?

Could the VTs for two values of the same variable not be ordered?
M0: a1 x=1 r1
M1:              a2 x=2 r2
M2:                           a1 a2 print x r2 r1

Summary of programmer rules / system guarantees
  1. each shared variable protected by some lock
  2. lock before writing a shared variable
     to order writes to same var
     otherwise "latest value" not well defined
  3. lock before reading a shared variable
     to get the latest version
  4. if no lock for read, guaranteed to see values that
     contributed to the variables you did lock

Example of when LRC might work too hard.
M0: a2 z=99 r2  a1 x=1 r1
M1:                            a1 y=x r1
TreadMarks will send z to M1, because it comes before x=1 in VT order.
  Assuming x and z are on the same page.
  Even if on different pages, M1 must invalidate z's page.
But M1 doesn't use z.
How could a system understand that z isn't needed?
  Require locking of all data you read
  => Relax the causal part of the LRC model

Q: could TreadMarks work without using VM page protection?
   it uses VM to
     detect writes to avoid making hidden copies (for diffs) if not needed
     detect reads to pages => know whether to fetch a diff
   neither is really crucial
   so TM doesn't depend on VM as much as general approach does
     General approach used VM faults to decide what data has to be moved, and when
     TM uses acquire()/release() and diffs for that purpose

Performance?

Figure 3 shows mostly good scaling
  is that the same as "good"?
  though apparently Water does lots of locking / sharing

How close are they to best possible performance?
  maybe Figure 5 implies there is only about 20% fat to be cut

Does LRC beat previous DSM schemes?
  they only compare against their own straw-man ERC
    not against best known prior work
  Figure 9 suggests lazyness only a win for Water
    most pages used by most processors, so eager moves a lot of data

What happened to DSM?
  The cluster approach was a great idea
  Targeting *existing* threaded code was not a long-term win
  Overtaken by MapReduce and successors
    MR tolerates faults
    MR guides programmer to good split of data and computation
    BUT people have found MR too rigid for many parallel tasks
  The last word has not been spoken here
    Much recent work on flexible memory-like cluster programming models
    RDDs/Spark, FaRM, Piccolo

References
 http://www.bailis.org/blog/linearizability-versus-serializability/
 http://www.bailis.org/blog/understanding-weak-isolation-is-a-serious-problem/
 https://aphyr.com/posts/313-strong-consistency-models