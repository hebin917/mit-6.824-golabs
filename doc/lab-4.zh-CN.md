 ## sharded kvraft

 ### 介绍

 基于上个实验的成果 kvraft，我们将 kv 分成多个部分(shard)以提高性能。怎么分呢？比如讲以 "a" 开头的 key 分到一个 shard，以 "b" 开头的 key 分到另一个 shard。如此一来，每个 shard 只需要处理与他相关的 key 的操作，其他 shard 于此同时就可以接受其他的请求，系统就可增加 shard 以提高吞吐量。

我们的整个系统有两个基本组件：shard master 和 shard group。整个系统有一个 master 和多个 group，master 一个 raft 集群，每一个 shard group 是由 kvraft 实例构成的集群。shard master 负责调度，客户端向 shard master 发送请求，master 会根据配置(config)来告知客户端服务这个 key 的是哪个 group。 每个 group 负责部分 shard。

对于各个 group，我们需要实现 group 之间的 shard 数据转移。因为我们 group 是动态变化的，有新加入的，也有要退出，有时负载均衡需要重新配置各个 group 的 shard。 

本次实验最大的挑战是如何处理在服务的过程中修改配置文件，即 group 与 shard 的对应关系。比如说，客户端此时请求的数据恰好是正在迁移的数据，我们需要确认本次请求是在迁移操作之前，还是之后，如果是之前，请求的数据所对应的新 shard 会收到通知；若是之后，请求只需重试。所以我们 Raft 不要把 `Put`, `Appends`, `Gets` 写进 log 中，`Reconfiguration` 也要写进去。总之，我们要确保每个请求只有一个 group 服务。

我们的代码主要在 src/shardmastet 和 src/shardkv 中实现。

Part A: Shard Master

shardmaster 负责管理所有的 configuration 改动，每个 configuration 记录了所有的 shardgroup 以及 shards 与 group 的对应关系。每当有修改时，master 都会生成一个新的 configuration。

在 shardmaster/common.go 中，描述了 configuration 的对外接口：Join, Leave, Move 和 Query。

1. Join 主要是用来增加 group。参数是 GID(Grop ID) 及其对应的 server 列表。master 需要重新分配 group 与 shard 的映射关系，当然数据移动的越少，数据分布的越平均越好，然后创建新的 configuration。

2. Leave 和 Join 相反，用于删除一个 group。参数是 GID。 master 需要重新分配 group 与 shard 的映射关系，当然数据移动的越少，数据分布的越平均越好，然后创建新的 configuration。

3. Move 的参数是 shard 和 GID，用于将 shard 数据移动到 GID 对应的 group 中去。主要用于测试方便。

4. Query 用于获取某个版本的 configuration，参数即为版本号。若参数是 -1 或大于最新的版本号，则返回最新的 configuration，如此时 master 正在执行 Join, Leave 或 Move，则需等操作完成再处理 Query(-1)。

在数值方面，configuration 应该从 0 开始计算，然后自增。shard 的数量应该大于 group 的数量。

别忘了 Go 的 map 变量是引用。

Part B: Sharded 键值服务

有了 master，我们就可以开始构建 shard group 了，代码主要在 shardkv/client.go，shardkv/common.go，and shardkv/server.go。

我们要实现的 shardkv 其实就是组成一个 group 集群的一个备份服务器。一个 group 集群对外接受 Get，Put 和 Append 请求，但只是部分 shard，例如以 "a" 或 "b" 开头的 key。使用基础代码中的 `key2shard()` 方法得到 key 与 shard 之间的映射关系。全部的 group 集群会服务所有的 shard。master 负责将分配 shard 与 group 的映射关系，当映射关系发生变化后，集群之间需要转移数据，以确保 client 的操作结果一致性。

作为一个分布式存储系统，基本的要求对外看起来和单机没啥区别即保证客户端请求的顺序执行。`Get()` 应该看到最新的结果，所有的操作需要在有 master 配置改动的同时，保持正确性。除此之外，我们还要保证可用性，即当有多数 group 正常运行，彼此能沟通且能和 master 集群通信时，整个系统依然能对外服务和内部配置自动调整。

一个 shardkv 只属于一个 group，这个关系在本实验中假设不会改变的。

基础代码中的 client.go 会将 PRC 发送给正确的 group，如果 group 响应说你要的 key 不是我负责的，client 会向 master 请求最新的 configuration，然后确定向那个 group 发送 RPC。我们要做是为每个 RPC 增加 ID，以实现判断重复请求的逻辑，就像上个实验 kvraft 一样。

写完代码测试一下，通过 Challenge 以前的测试用例。

```shell
$ cd ~/6.824/src/shardkv
$ go test
Test: static shards ...
  ... Passed
Test: join then leave ...
  ... Passed
Test: snapshots, join, and leave ...
  ... Passed
Test: servers miss configuration changes...
  ... Passed
Test: concurrent puts and configuration changes...
  ... Passed
Test: more concurrent puts and configuration changes...
  ... Passed
Test: unreliable 1...
  ... Passed
Test: unreliable 2...
  ... Passed
Test: shard deletion (challenge 1) ...
  ... Passed
Test: concurrent configuration change and restart (challenge 1)...
  ... Passed
Test: unaffected shard access (challenge 2) ...
  ... Passed
Test: partial migration shard access (challenge 2) ...
  ... Passed
PASS
ok  	shardkv	206.132s
$
```

我们的实现基本不需要主动向 master 发送 Join 请求，这些逻辑都放在基础代码的测试用例中。

#### Step 1 从单 group 开始

第一步我们先假设所有的 shard 都分配到一个 group 上，所以代码实现和 Lab 3 非常类似。只是要在 group 启动时，根据 configuration 确定自己的应该接受那些 key 的请求。完成后能通过第一个测试用例。

然后我们要处理 configuration 更改时的逻辑。首先每个 group 都要监听 configuration 的改变，当其版本更新后，我们要开始进行 shard 迁移。当某个 shard 不在归该 group 管理时，该 group 应该立刻停止响应与这个 shard 相关的请求，然后开始将这个 shard 的数据迁移到另一个负责管理它的 group 那里。当一个 group 有了一个新的 shard 所有权时，它需要先等待 shard 的数据完全迁移完成后，才能开始接受这个 shard 相关的请求。 

要注意的是在 shard 数据迁移的过程中，要保证所有一个 group 内所有的副本服务器同时进行数据迁移，然后让整个 group 对并发请求保持结果溢脂性。

代码中应该定期从 shardmaster 中拉取 configuration 信息。测试用例会测试 100ms 左右时间间隔下，configuration 改变是，逻辑是否依然正确。

服务器之间需要使用 RPC 的方式进行数据迁移。shardmaster 的 Config 里面有服务器的名字，但是我们的 RPC 需要向 labrpc.ClientEnd 发送信息。没事，在调用 StartServer() 时，传入了一个 make_end() 方法，调用他可以将服务器的名字转化为一个 labrpc.ClientEnd.

当 group 收到请求的数据不在自己管辖范围内时，应当返回 ErrWrongGroup，但是我们得在 configuration 变化的情况下，做出正确的判断。

由于我们要能判断出请求的重复性，所以当一个 shard 从一个 group 转移到另一个 group 时，不仅要带有 Key/Value 数据还得带上一些判断重复请求的信息。考虑一下在何种情况下接受新 shard 的 group 需要更新自己状态，完全覆盖本地数据一定就是对的吗？

接着还得考虑服务器和客户端如何处理 ErrWrongGroup。收到 ErrWrongGroup 响应的客户端需要改变自己的 sequence number 吗？服务器在给 Get/Put 请求恢复 ErrWrongGroup 的时候需要更新本地对这个客户端的状态记录吗？

其实当 group 对一个 shard 失去了管理权后，没必要立即将其删除，虽然在生产环境下可能会造成空间浪费，但是在我们本次实验中可以简化逻辑。

> Hint: When group G1 needs a shard from G2 during a configuration change, does it matter at what point during its processing of log entries G2 sends the shard to G1?
You can send an entire map in an RPC request or reply, which may help keep the code for shard transfer simple.

用 RPC 传送 map 的时候记得复制到一个新的 map 里面去哦，传引用就会有问题。

Challenge exercises

For this lab, we have two challenge exercises, both of which are fairly complex beasts, but which are also essential if you were to build a system like this for production use.
Garbage collection of state

When a replica group loses ownership of a shard, that replica group should eliminate the keys that it lost from its database. It is wasteful for it to keep values that it no longer owns, and no longer serves requests for. However, this poses some issues for migration. Say we have two groups, G1 and G2, and there is a new configuration C that moves shard S from G1 to G2. If G1 erases all keys in S from its database when it transitions to C, how does G2 get the data for S when it tries to move to C?

> CHALLENGE: Modify your solution so that each replica group will only keep old shards for as long as is absolutely necessary. Bear in mind that your solution must continue to work even if all the servers in a replica group like G1 above crash and are then brought back up. You have completed this challenge if you pass TestChallenge1Delete and TestChallenge1Concurrent.

> Hint: gob.Decode will merge into the object that it is given. This means that if you decode, say, a snapshot into a map, any keys that are in the map, but not in the snapshot, will not be deleted.

Client requests during configuration changes

The simplest way to handle configuration changes is to disallow all client operations until the transition has completed. While conceptually simple, this approach is not feasible in production-level systems; it results in long pauses for all clients whenever machines are brought in or taken out. A better solution would be if the system continued serving shards that are not affected by the ongoing configuration change.

> CHALLENGE: Modify your solution so that, if some shard S is not affected by a configuration change from C to C', client operations to S should continue to succeed while a replica group is still in the process of transitioning to C'. You have completed this challenge when you pass TestChallenge2Unaffected.

While the optimization above is good, we can still do better. Say that some replica group G3, when transitioning to C, needs shard S1 from G1, and shard S2 from G2. We really want G3 to immediately start serving a shard once it has received the necessary state, even if it is still waiting for some other shards. For example, if G1 is down, G3 should still start serving requests for S2 once it receives the appropriate data from G2, despite the transition to C not yet having completed.

> CHALLENGE: Modify your solution so that replica groups start serving shards the moment they are able to, even if a configuration is still ongoing. You have completed this challenge when you pass TestChallenge2Partial.

Handin procedure

Before submitting, please run all the tests one final time. You are responsible for making sure your code works.

Also, note that your Lab 4 sharded server, Lab 4 shard master, and Lab 3 kvraft must all use the same Raft implementation. We will re-run the Lab 2 and Lab 3 tests as part of grading Lab 4.

Before submitting, double check that your solution works with:

```shell
$ go test raft/...
$ go test kvraft/...
$ go test shardmaster/...
$ go test shardkv/...
```

Submit your code via the class's submission website, located at https://6824.scripts.mit.edu:444/submit/handin.py/.

You may use your MIT Certificate or request an API key via email to log in for the first time. Your API key (XXX) is displayed once you logged in, which can be used to upload the lab from the console as follows.

For part A:

```shell
$ cd "$GOPATH"
$ echo "XXX" > api.key
$ make lab4a
```

For part B:

```shell
$ cd "$GOPATH"
$ echo "XXX" > api.key
$ make lab4b
```
