容错的 Key/Value 服务

### 介绍

这一次我们要在 实验二：Raft 的基础上创建一个可以容错的键值存储服务。具体来说就是用几个 Raft 实例组成一个可以容错的状态机，而不同客户端操作通过 Raft log 保证一致性。同时当有半数以上的 Raft 实例正常运行且能相互通信时，我们的服务就能正确运行。

在 kvraft 包中，我们要实现 Key/Value 的服务器和客户端，当然服务器同时是一个 Raft 实例。客户端可向服务器发送 Put(), Append() 和 Get() RPC，服务器会将这些命令放到 log 中，然后按顺序执行。客户端可以向任何一个服务器发送请求，但是如果访问的服务器不是 leader 或者没有响应，则需要换一个服务器试试。如果一个客户端指令确定写入 log 中了，则需要给客户端说一声，要是此时服务器宕机或者网络故障了，客户端应该将同样的指令发给另外一个服务器。

我们的实验分两步来做，第一步我们简单的将每个请求的内容追加到 log 列表中，而不用考虑他的空间占用。第二步，我们将实现论文中第7章的内容，用 snapshot 回收无用的 log。

虽然这个实验不要写很多代码，但是可能得花很长时间去搞明白你的实现为啥不行。其实比实验二更难 debug，因为有更多的模块是异步的工作方式。

本实验的代码应在 src/kvraft.go 中实现。需要通过所有测试：

```shell
$ setup ggo_v1.5
$ cd ~/6.824
$ git pull
...
$ cd src/kvraft
$ GOPATH=~/6.824
$ export GOPATH
$ go test
...
$
```

Step 1: 初步 Key/Value 实现

首先明确一下我们的服务对外的接口：

1. `Put(key, value)`, 为 key 设置新的值 value
2. `Get(key)`, 获取 key 的值
3. `Append(key, arg)`, 为 key 的值追加 arg

我们服务由几个 Raft 实例组成一个备份状态集群，客户端应该尝试跟不同的服务器通讯。只要连上了一个 leader，并且此时有半数以上的机器正常运行，则客户端的命令应该被执行。

当然我们先从最简单的场景实现，即没有机器故障。但是我们得保证服务响应的顺序一致性。即客户端的命令记录在每个 Raft 实例上都按相同的顺序执行，且每个命令最多执行一次。Get(key) 应该读取最新的数据。

先看看 Op struct 里面还缺少啥属性，然后实现 PutAppend() 和 Get() 的 handler，它们应该调用 Raft 的 Start() 方法将 Op 放入 log 中，然后等待 Raft 向 applyCh 发送数据，以表示刚才的命令执行成功，在此之前不应该再向 Start() 新的指令。收到成功消息后，向客户端反馈。

完成之后需要能通过 "One client" 测试。

由于我们发送 Start() 和获得结果是异步的，所以得考虑如何处理先后关系。

当一个 leader 接收到 Start() 的命令，但是在 commit 之前失去了 leader 身份时，客户端应该寻找新的 leader 重新发送这个命令。当客户端连上的 leader 被网络隔离了，此时这个 leader 还是认为自己是 leader，只是无法让系统执行命令，而且客户端也不知道有新的 leader，这种情况下只能让客户端无限等待。

最好记住上次的 leader 的 index，这样可以先试试它还是不是 leader，以减少寻找 leader 的时间。

因为我们假设网络和 Raft 实例都不是始终稳定，所以我们需要处理一个请求发送多次的情况，保证同一请求只处理一次。所以我们需要一些额外数据去标示不同的请求。

处理同一请求重发的情况： 一个客户端在 term1 向 leader1 发送请求，然后等待响应超时，然后将这个请求发给了 leader2，此时在 term2。客户端的请求应该只被执行一次。完成 Step 1 后，需要能通过 `TestPersistPartitionUnreliable()` 前面所有测试。

提到一个请求只能被服务器执行一次，这取决于客户端如何识别不同的请求，可以为每个请求加上标示位，当然也可以规定每个客户端同时只能处理一个请求。

###　Step 2: 压缩 log 减少冗余

为了让 Raft 的 log 无限制增长，我们要用快照(snapshot，论文第7章)机制可以定期删除无用的 log。

`StartKVServer()` 传入的参数 `maxraftstate` 用于表明每个 Raft 实例用于保存 log 的空间大小。每当空间不足时需要生成一个 snapshot，然后告诉 Raft 当前的 log 已经被快照存入磁盘(`persister.SaveSnapshot()`)了，可以将内存的 log 扔掉了。

要实现 snapshot 机制，我们需要修改 raft 包的代码，当 follower 需要的同步的 log 已经被快照存入磁盘了，则 leader 需要给 follower 发送一个 `InstallSnapshot` RPC，follower 接收到这种 RPC 后，需要向 kvraft 发送对应的 snapshot。

snapshot 需要保存的不仅仅是 log 列表，还需要相关的信息比如请求序列号等。 
