6.824 2016 Lecture 19: Borg Case Study

Reading: "Large-scale cluster management at Google with Borg", Abhishek Verma,
Luis Pedrosa, Madhukar Korupolu, David Oppenheimer, Eric Tune, and John Wilkes.
Proceedings of the 10th European Conference on Computer Systems (EuroSys 2015).

Why are we reading this paper?
  Complex, real-world distributed system incorporating key 6.824 concepts
    Relies on Paxos (similar to RAFT, lab 2, and Zookeeper)
  Explains how your distributed applications would actually run
    e.g., MapReduce (lab 1), distributed KV stores (lab 4)
  As applications increasingly rely on online back-ends, clusters are common
    Many companies run their own to serve or analyze data
    Some problems require them (big data, large-scale machine learning)
  Automated cluster managers are "mainstream" now
    You may well have already used one, or will do in the future
    Neat systems tricks to increase resource utilization => saves real money!

Google Borg
  Developed at Google for internal use on large, shared clusters
    Automates application deployment and resource management on large clusters
    Underpins almost all Google workloads, including "cloud" business, MapReduce
  Inspired several similar open-source systems
    Apache Mesos
    Google Kubernetes
    Docker Swarm
    Hashicorp Nomad
  Hot topic in industry right now ("orchestration")

Motivation: resource sharing ("consolidation") to save money
  Need many servers to serve thousands/millions of users concurrently
    must provision for peak load
    but most of the time, load << peak
    hence, machines are poorly utilized (e.g., 5% CPU load at night)
  But: typically also have low-priority workloads
    can use these to fill in troughs in utilization
    hence, get away with fewer machines overall
  [diagram: 2x resource use timelines, highlight slack resources]
  At scale of thousands of machines, even small efficiency gains save $MM

One solution: just run processes on a pool of time-shared machines
  Developer/sysadmin places applications manually
    To start service, install and set up init to start it on machine boot
    To start batch data processing, logs in, install program and start
  Q: what are the problems with this approach?
    poor security
      many users have ssh login to machine
      ... but cannot give everyone root (= installing things is painful)
      file system, process list, etc., are shared => information leaks
    poor convenience
      shared file system and libraries: version/dependency mess
      user/developer must understand how and where to deploy application
    poor efficiency
      manual human configuration required, nothing is automated
      once a service/application is deployed to a machine, it's static

Borg: an automated cluster manager
  Design goals:
    Hide details of resource allocation and failure handling
    Very high reliability and availability
    Efficiently execute workloads over 10,000+ machines
    Support *all* workloads
      storage servers (GFS, BigTable, MegaStore, Spanner)
      production front-ends (GMail, Docs, web search)
      batch analytics and processing (maps tiles, crawling, index update)
      machine learning (Brain/TensorFlow)
  Challenges:
    How much control and power to expose to the user?
    How to efficiently pack work onto machines?
    How to cope with failures (of machines, or Borg), and network partitions?
    How to isolate different users' workloads from each other?

Borg lifecycle overview
  [diagram: visualize lifecycle, cf. Fig 2 in paper]
  1. User (Google engineer) submits a job, which consists of one or more tasks
  2. Borg decides where to place the tasks
  3. Tasks start running, with Borg monitoring their health
  4. Runtime events are handled
    4a) Task may go "pending" again (due to machine failure, preemption)
    4b) Borg collects monitoring information
  5. Tasks complete, or job is removed by user request
  6. Borg cleans up state, saves logs, etc.
  (This ignores allocs, but see them as larger "tasks" that group other tasks.)

Borg system components
  Mostly generalize to other cluster managers
  cf. Figure 1 in paper
  [diagram: simplified version of Fig 1]
  BorgMaster
    Central "brain": holds cluster state, replicated for reliability (Paxos)
  Borglet
    Per-machine agent, supervises local tasks and interacts with BorgMaster
  Scheduler
    Decides on which machines task are placed (more later)
  Front-ends
    CLI/BCL/API: programmatic job submission, config description
    Dashboards: monitoring of service SLAs (e.g., tail latency)
    Sigma UI: introspection ("why are my tasks pending?")

Reliability
  Replicated state in BorgMaster
    One active (read-write) primary (elected via Paxos)
    Four backups (read-only) who follow primary
      Every replica has state in memory and on disk (Paxos-based store)
    On primary timeout, leader election happens via Paxos/Chubby
      Takes 10 sec to 1 minute
      Post-outage, replicas join as backups and sync with others
    Checkpoints enable debugging and speedy recovery
      Won't have to replay log from the beginning of time
  Borglet: local machine agent
    Regularly contacted by BorgMaster to check health and pass instructions
      Poll-based in order to avoid DoS'ing the BorgMaster by accident
    Disconnection from BorgMaster must be handled (BM failure, net partition)
      All tasks keep running (to protect against all BorgMasters failing)
      Once recovered, kill duplicate tasks rescheduled elsewhere
  Q: what happens in network partition? Several options (not in paper)
     -- need consensus of all BorgMaster replicas => as if all have failed
     -- majority partition elects leader, keeps trucking; minority partition
        behaves as if all BorgMasters failed

Clusters
  A physical data center (building) holds multiple "clusters"
  Each cluster is subdivided into logical "cells"
  Borg manages a cell, users must themselves choose which cell to use
    Geographically distributed, so jobs often replicated

Workload
  Long-running services
    Often serve user (or other jobs') requests: daemon-like
    Never "finish": terminate only due to failure or job termination/upgrade
    Often (but not always) high priority and latency-sensitive
    Stringent fault tolerance required (spread over fault domains)
  Batch processing jobs
    Work toward finite objective, and terminate when done
    Not as sensitive to short-term performance fluctuations
  Notion of priority
    Production ("monitoring" + "prod")
      Guaranteed to run & always receives resources
    Non-production ("batch" + "best effort")
      Second-class citizens
  Examples [diagram / grid]
    prod/Service: BigTable, GMail front-end servers, model serving
    prod/Batch: GFS data migration
    batch/Service: --
    batch/Batch: machine learning model training, data transformation
    best effort/Service: test deployment of new version of service
    best effort/Batch: intern MapReduce job, exploratory ML model training

Admission control
  Must have "quota" to submit a job
  Effectively a human-in-loop fairness system
  Quota costs money (from team budget), so overbuying is expensive
    Except for "low-quality" resources
      Not worse from a HW point of view
      But can get preempted or throttled (and will more often)
    Encourages use of low-priority tiers ("free")
  Quota is purchased for time periods (e.g., monthly basis)
    "Calendaring"
    Mostly useful for prod/Service jobs
  Different to per-task resource request!
    Quota is per-user/per-team
    Qutoa for medium-term capacity planning

Scheduling
  Key part of cluster management: where does each task go?
    Impacts fault tolerance
    Impacts machine load
    Impacts user wait time (until place found)
  Feasibility checking: which machines are candidates?
    Must have enough available resources
      In all dimensions: CPU, memory, disk space, I/O bandwidth
      Notion of "enough" depends on task priority (prod/non-prod differ)
    Consider placement constraints
      Hard: must be satisfied (e.g., "machine must have public IP", "GPGPU")
      Soft: preferences, must not be satisfied (e.g., "faster processor")
      Hard constraints used to ensure fault tolerance 
  Scoring: which of the candidate machines are the best choices?
    Soft constraints influence score
    Minimize disruption: reduce count and priority of preempted tasks
    Higher score = better packing

Utilization/packing
  Q: why is this an issue? Easy to waste resources + humans bad at estimates
  Reasons for waste
    User asked for more resources than they need
    All memory reserved, but plenty of CPU free
    Load well balanced, but no room for large task
    Tasks' needs vary over time (e.g., diurnal service load)
  Paper goes through many design choices that lead to better packing
    But evaluates via contra-positive: worse if had NOT made these choices

Cluster compaction metric
  Used for many graphs in the paper, but somewhat unusual
  How do we assess how good a job the scheduler did?
    Could use utilization, but says little about wastage
    Could measure unusable "holes", but usability of hole is task-dependent
    Could artifically grow the workload, but complex to make realistic
  Key question: "How much smaller a cluster could I have got away with?"
    ... without running out of capacity (many pending tasks)
    If the scheduler packs more tightly, a smaller cluster still works
    Answer can be >100%, if scheduler/setup worse than baseline
  Graphs are CDFs of 15 cells
    [diagram: example graph]
    To right of 100% => bad, would have been worse off than status-quo Borg
    To left of 100% => good, improvement over compacted status-quo Borg cluster

Techniques to avoid idle resources
  Q: we asked you to name some for the lecture. Let's hear some!
  Preemption
    Kick out lower-priority workloads when resources needed
    Allows resources to be utilized, rather than held back
    But: must be prepared to recover from preemption
  Large, shared cells
    Don't give dedicated clusters to different teams
    Share all machines between *everyone*
    Reduces fragmentation due to low load, disincentivizes hoarding
  Fine-grained resource requests
    Allow users to independently vary requests (CPU/memory) in small units
    Idea: accurate estimates avoid "bucketing" waste
    Unlike, e.g., AWS EC2 instance types!
  Resource estimation
    Automatically shrinks the reservation to a safe envelope around usage
    [diagram: example]
    Slow decay protects against repeated preemptions due to spikes
  Overcommit
    Tasks can use resources beyond their limits, at risk of being killed

Scaling: running over 10k+ machines isn't trivial (e.g., Hadoop YARN: 5k max)
  BorgMaster architecture tricks
    Sharded across BorgMaster replicas
      Each responsible for some Borglets (link shard)
      Cf. lab 4 sharding of key ranges
    Separate threads for Borglet communication and read-only RPCs
    Shared-state design for scheduler
      In separate process
      Works on snapshot of cluster state in BorgMaster
      BorgMaster rejects updates based on stale state
  Scheduler tricks
    2011 Borg cell trace: 150k+ tasks
      Preemption requires reconsidering even running ones
    Iteration over everything is prohibitive
      ~20k cycles per task (compare: network packet receive = 16k)
    Equivalence classes
      Only do feasibility checking & scoring once per similar task
    Score caching
      Don't recompute scores if state hasn't changed
    Relaxed randomization
      Don't iterative over the whole cell
      Randomly sample candidate machines until enough are found
      Still takes longer for "pickier" tasks

Task isolation
  Linux containers (chroot + cgroups)
    Kernel namespaces (unlike VMs, kernel still shared)
    Each task has its own root file system
  Packages
    Include statically linked task binary, plus extra files required
    Similar to Docker images
  "Appclasses" customize machine treatment of tasks
    e.g., CPU quantum in kernel scheduler
  Performance isolation
    Goal: prod-priority tasks always get what they asked for
    Compressible resources: rate-limit for lower-priority tasks
    Non-compressible resources: preempt lower-priority tasks to free up

Service discovery
  Automation => user does not know where tasks will be at runtime!
    Lab 1: How to find MR master if you don't know its IP/host?
    Lab 4: How to find shardmaster/replica group members?
  Borg uses Chubby (similar to Zookeeper/lec 8) to store task locations
    RPC system can then look up job:task combination in Chubby
  "Borg Name System" (BNS)
    Integrated with cluster DNS
    50.jfoo.ubar.cc.borg.google.com = task 50, job "foo", cell "cc"
  Thus anyone can make an RPC knowing only cluster+job name and task ID
  This broad problem is called "service discovery" in industry
    Many solutions: Zookeeper, Consul, SkyDNS, ...
    All similar to BNS

Differences to other systems
  YARN (Yahoo! / Microsoft)
    Primarily for batch workloads
    Slot-based resource model
    Application masters do scheduling
  Mesos (Twitter w/ Aurora, AirBnB w/ Marathon)
    Mesos does not implement scheduling (Borg does)
    Mesos makes resource offers to application-specific frameworks
      e.g., MapReduce/Spark scheduler
      Frameworks can accept or reject and wait for better offer
    Some challenges in practice (see Omega paper)
      Information hiding (scoring tricky)
      Resource hoarding (slow schedulers problematic)
      Priority preemption difficult (needs to be part of offer API)
  Bistro/Tupperware (Facebook)
    Bistro uses a resource forest model
    Tries to find a path through the forest to reserve resources
    Little known about Tupperware
  Kubernetes
    Open-source, for smaller scale (1k nodes)
    Not as sophisticated as Borg in terms of policies

Borg user experience
  Failures (especially preemptions!) do happen all the time
    When running long jobs on "best effort", ~5% of MapReduce tasks preempted
    Can get starved in extreme case, though rare
  Resource estimation is super useful and works well in practice
    10 GB memory reservation insufficient => ask for 20 GB, even if need 12 GB


References:
  Cluster compaction metric: http://research.google.com/pubs/archive/43103.pdf
  ACM queue article on Borg, Omega, and Kubernetes:
    http://queue.acm.org/detail.cfm?id=2898444
  Open-source systems:
    https://mesos.apache.org
    https://kubernetes.io