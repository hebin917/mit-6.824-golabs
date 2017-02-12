6.824 Lab 2: Raft

Due: Fri Feb 26, 11:59pm

### 介绍

通过[实验一: MapReduce](http://www.jianshu.com/p/5e1b0ad68bff)，我们慢慢熟悉了实验环境和 Go 语言。现在我们要开始构建一个大实验，高性能，分布式 Key/Value 服务。实验分三个阶段，首先我们实现 Raft 协议，接着在 Raft 基础上构建一个 Key/Value 服务，然后将服务分片(shard）化以提高性能，最后给分片间的操作提供事务性保障。

本次我们先来实现 Raft。在一个用备份提高可用性的系统中，Raft 用于管理备份服务器。在高可用系统中，通过备份，小部分的机器故障不会影响系统的正常工作，但问题是每台机器都有可能发生故障，所以不能保证机器集群上的数据始终是一样的，而 Raft 协议就是帮助我们判断，哪些数据才是正确的，哪些需要抛弃并用正确的数据更新。

Raft 的底层思路是实现一个备份状态机。Raft 将所有的客户端请求组织成一个序列，称之为 log，不仅如此，在执行 log 之前，Raft 还会保证所有的备份服务器都同意执行本次 log 的内容。每个备份服务器都会按照前后顺序执行 log，让客户端的请求应用到状态机上。由于每台机器都是按相同顺序执行相同请求，所以他们始终保持相同的状态。如果一个服务器失败后重连了，Raft 负责更新它的 log。只要大多数机器保持健康状态并且能相互通讯，Raft 就能让系统正确的持续运行，否则系统将不再接受新的请求，一旦有足够的机器，系统将会从之前状态开始继续进行。

在本次实验中，我们将用 Go 语言实现 Raft，规模当然比实验一大多了。具体来说就是几个 Raft 实例通过 RPC 的方式通讯已维护各自的 log。Raft 集群应该能接受带有序号的指令或者叫 log 序列。每个 log 都保证可以提交。

注意：只能通过 RPC 实现 Raft 实例之间的沟通。所以，不同的 Raft 实例之间不能共享变量，也不要用文件。

本次实验我们先来实现 Raft 论文中前5章的内容，包括保存需持久的状态，因为当一个服务器重连后需要根据这些信息恢复。

Raft 相关资料：
1. [论文](https://pdos.csail.mit.edu/6.824/papers/raft-extended.pdf)
1. [动画展示](http://thesecretlivesofdata.com/raft/)

虽然实现 Raft 协议的代码量不是最大，但是要让它正确的工作并不是一件容易的事情。需要很多边界情况。当测试不通过时，不易想明白是那种场景出了问题，所以不好debug。

写代码之前要仔细阅读论文，实现基本按照论文的描述，测试也是按照论文设计的。图2是比较完整的伪代码。

### 实验环境

拉取代码

```shell
$ git clone git://g.csail.mit.edu/6.824-golabs-2016 6.824
$ cd 6.824
$ ls
Makefile src
``` 

进入 raft 文件夹，运行测试代码

```
$ cd src/raft
$ GOPATH=~/6.824
$ export GOPATH
$ go test
Test: initial election ...
--- FAIL: TestInitialElection (5.03s)
	config.go:270: expected one leader, got 0
Test: election after network failure ...
--- FAIL: TestReElection (5.03s)
	config.go:270: expected one leader, got 0
...
$
```

当然没有通过，当你写好后，再次测试，看到下面的输出，表示通过测试：

```shell
$ go test
Test: initial election ...
  ... Passed
Test: election after network failure ...
  ... Passed
...
PASS
ok  	raft	162.413s
```

协议的实现代码主要放在 `raft/raft.go` 文件中。这个文件里面已经写好了一些框架代码，一些发送和接受 RPC 的示例，还有一些保存持久化信息的例子。

下面提供了一些接口，我们的 Raft 需要一一实现，测试代码会以它为测试蓝本。

```go
// 创建一个新的 Raft 实例
rf := Make(peers, me, persister, applyCh)

// 处理一个指令
rf.Start(command interface{}) (index, term, isleader)

// ask a Raft for its current term, and whether it thinks it is leader
// 返回该 Raft 实例的当前 term 和是否是 Leader
rf.GetState() (term, isLeader)

// 每当有一个请求被执行了，都应当向服务送一个 ApplyMsg
type ApplyMsg
```

一个服务调用 `Make(peers,me,…)` 创建一个 Raft 实例。peers 是
已经创立的 RPC 连接数组，互相之间可访问；me 是当前创建的实例在 peers 数组中的索引值。`Start(command)` 让 Raft 集群尝试将 command 追加到备份复制的log中。`Start()` 应该马上响应，不需要等待执行结束。服务可通过监听 `applyCh` 的 `ApplyMsg` 到达来判断某个 command 已经完成。

Raft 节点之间使用 `labrpc` 进行通信。`raft.go` 里面有选举Leader的RPC示例，发送 `sendRequestVote()`，处理请求 `RequestVote()`。 

### Step I: 选举Leader 和 heartbeat   

要求：只能有一个 Raft 被选举成为 leader，并且用 heartbeat(发送空的 AppendEntry) 保持 leader 状态。通过前两个测试。

提示：

1. 仔细阅读论文中的图二部分，为 Raft struct 添加必要的属性。同时还需要定义一个新的 struct 表示 log。不要忘了公开的属性即开头大写才可以通过 RPC 传递。

1. 先实现选举部分，完善 RequestVoteArgs 和 RequestVoteReply。在 Make() 方法里面创建一个 gorutine，如果过了一段时间还没有其他 Raft 给它发消息，说明此时没有 leader，即让自己成为 candidate，发送 RequestVote 竞选 leader，当然对于每个 Raft 来说也要能够处理别人发来的 RequestVote，满足一定的规则时则投他一票。

2. 要实现 heartbeat，得先定义 AppendEntries RPC 相关的 struct。然后让 leader 定期给他的小弟(follower) 发送。当然需要实现处理方法，成功处理后需要更新follower选举等候时间，这样 leader 就可通过发 heartbeat 保持它的领导权。

3. 每个 Raft 的选举等待周期应有一定的随机性，以防止总是有多个 Raft 同时竞选而无法选出 leader 的情况。

### Step 2: 接受客户端命令

选出了 leader 然后就可以将这个集群当做一个整体，它能接受指令，并且保证持久性且具备一定容错性。所以我们要让 Start() 方法能够接受指令并把它们放入 log 中。只有 leader 向 log 列表中添加新的内容，follower 的 log 列表通过来自 leader AppendEntries RPC 更新。

要求：leader 和 follower 的 log 列表更新操作。完善 Start() 方法，完善 AppendEntries，实现 sendAppendEntries 和 handler。通过 "basic persistence" 之前所有的测试。

提示：

1. 需要考虑各种失败的情况如无法接收到 RPC，宕机后重连等。同时还得实现论文 5.4.1 中描述的竞选阶段的限制。

2. 尽管只有 leader 会将新的log追加到 log 列表中，但每个 raft 都需要将 log 应用即执行 log 中的命令。所以尽可能将 log 的操作和应用逻辑解耦。

3. 尽量降低 follower 同步 leader 时的沟通次数。

### Step 3: 持久化数据

一个 Raft 服务器重连后能恢复到宕机时候的状态，所以我们需要持久化一些必要状态信息。可参考论文图二。

一个生产环境下的 Raft 服务器需要将状态信息持久化到磁盘中，我们实验为了简化，将持久化操作封装到 Persister 中。用 Make() 创建 Raft 实例时需要传入 Persister 参数，用于初始化状态信息，同时在 Raft 实例状态改变时保存下来。相关方法为 ReadRaftState() 和 SaveRaftState().

要求：

完善 persist 方法，然后在合适的时机调用，即考虑在 Raft 协议中何种情况下需要持久化状态信息。需能通过 "basic persistence" 测试(go test -run 'TestPersist1$')

注意：

1. persist() 需要将数据转化成二进制才能存储

2. 为了避免 OOM(Out Of Memory)，Raft 会定期清理老的 log，下个实验我们会用 snapshotting(论文的第7章) 解决这个问题。

提示：
1. RPC 和 GOB 只会对 struct 的公开属性即首字母大写有效。
2. 有些严格测试需要实现论文的第7页尾到第8页上部灰线标记的部分，即当 AppendEntries 中的 log 与本 follower 冲突时，给 leader 反馈冲突的 term 和下一个 index。
