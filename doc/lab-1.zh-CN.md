### 介绍

通过[分布式系统系列文章]() ，我们了解了分布式的一些基本概念，若是写点代码实践一下，那就更好了。先做个简单的实验练练手，还记得 [MapReduce]() 吗？，本次实验中会构建一个 MapReduce 库，主要能熟悉 Go 语言外加了解分布式系统中的容错机制。首先写个一个简单的 MapReduce 程序，再写一个 Master，它不仅能分配任务给 worker 而且能处理 worker 执行错误。接口参考论文描述。

### 实验环境

不会让你从零开始撸代码啦，还不快 git clone ?

```shell
$ git clone git://g.csail.mit.edu/6.824-golabs-2016 6.824
$ cd 6.824
$ ls
Makefile src
```

MapReduce 代码支持顺序执行和分布式执行。顺序执行意味着 Map 先执行，当所有 Map 任务都完成了再执行 Reduce，这种模式可能效率比较低，但是比较便于调试，毕竟串行。分布式执行启动了很多 worker 线程，他们并行执行 Map 任务，然后执行 Reduce 任务，这种模式效率更高，当然更难实现和调试。

### 准备：熟悉代码

`mapreduce` 包提供了一个简单的 MapReduce 顺序执行实现。应用只要调用 `Distributed()` 方法就可以启动一个任务，但是要调试的时候可能需要调用 `Sequential()`.

mapreduce 的运行流程如下：

1. 应用层需要提供输入文件，一个 map 函数，一个 reduce 函数，要启动 reduce 任务的数量。

1. 用这些参数创建一个 master。它会启动一个 RPC 服务器(master_rpc.go)，然后等待 worker 注册(`Register()`)。当有待完成的任务时，`schedule()` 就会将任务分配给 worker，同时也会进行 worker 的错误处理。

1. master 认为每个输入文件应当交给一个 map 任务处理，然后调用 `doMap()`，无论直接调用 `Sequential()` 还是通过 RPC 给 worker 发送 DoTask 消息都会触发这个操作。每当调用 doMap() 时，它都会去读取相应的文件，以文件内容调用 map 函数并且为每个输入文件产生 nReduce 个文件。因此，每个 map 任务最终会产生 `#files x nReduce` 个文件。  

1. master 接下来会对每个 reduce 任务至少调用一次 `doReduce()`。`doReduce()` 首先会收集 nReduce 个 map 任务产生的文件，然后在每个文件上执行 reduce 函数，最后产生一个结果文件。

1. master 会调用 `mr.merge()` 方法将上一步产生所有结果文件聚合到一个文件中。

所以本次实验就是到填空题，空是：doMap, doReduce，schedule 和 reduce。

其他的方法基本不需要改动，有时间的研究研究有助于理解整体架构。

### Part I: Map/Reduce 输入和输出

第一个空 `doMap()` 函数的功能是读取指定文件的内容，执行 mapF 函数，将结果保存在新的文件中；而 `doReuce()` 读取 `doMap` 的输出文件，执行 reduceF 函数，将结果存在磁盘中。

写完了就测试测试，测试文件(test_test.go)已经写好了。串行模式测试可执行：

```shell
$ cd 6.824
$ export "GOPATH=$PWD"  
$ cd "$GOPATH/src/mapreduce"
$ setup ggo_v1.5
$ go test -run Sequential mapreduce/...
ok  	mapreduce	2.694s
```

如果你看到的不是 ok，说明还有 bug 哦。在 common.go 将 debugEnbale 设置成 true，然后运行 `go test -run Sequential mapreduce/... -v`，可以看到更详细的输出：

```
$ env "GOPATH=$PWD/../../" go test -v -run Sequential mapreduce/...
=== RUN   TestSequentialSingle
master: Starting Map/Reduce task test
Merge: read mrtmp.test-res-0
master: Map/Reduce task completed
--- PASS: TestSequentialSingle (1.34s)
=== RUN   TestSequentialMany
master: Starting Map/Reduce task test
Merge: read mrtmp.test-res-0
Merge: read mrtmp.test-res-1
Merge: read mrtmp.test-res-2
master: Map/Reduce task completed
--- PASS: TestSequentialMany (1.33s)
PASS
ok  	mapreduce	2.672s
```

### Part II: 单机词频统计

完成了第一部分，我们可以开始构建自己第一个 MapReduce 系统：词频统计器。没错还是填空题：mapF 和 reduceF，让 wc.go 可以统计出每个单词出现的次数。我们的测试文件里面只有英文，所以一个单词就是连续出现字母，判断一个字母参考标准库 `unicode.IsLetter`。

测试文件是 6.824/src/main/pg-*.txt，不妨先编译试试：

```
$ cd 6.824
$ export "GOPATH=$PWD"
$ cd "$GOPATH/src/main"
$ go run wc.go master sequential pg-*.txt
# command-line-arguments
./wc.go:14: missing return at end of function
./wc.go:21: missing return at end of function
```

当然通过不了，毕竟空还没填呢。mapF 的参数是测试文件名和其内容，分割成单词，返回 []mapreduce.KeyValue，KeyValue：单词-频次。轮到 reduceF 函数了，它会针对每个 key(单词) 调用一次，参数是某个单词以及这个单词在所有测试文件中的 mapF 结果。

写好了，便可测试：

```
$ cd "$GOPATH/src/main"
$ time go run wc.go master sequential pg-*.txt
master: Starting Map/Reduce task wcseq
Merge: read mrtmp.wcseq-res-0
Merge: read mrtmp.wcseq-res-1
Merge: read mrtmp.wcseq-res-2
master: Map/Reduce task completed
14.59user 3.78system 0:14.81elapsed
```

最终的结果保存在 mrtmp.wcseq 文件中。运行 `$ rm mrtmp.*` 删除所有的中间数据文件。

运行 `sort -n -k2 mrtmp.wcseq | tail -10`，如果看到的和下面的一样，说明你写对了。

```
$ 
he: 34077
was: 37044
that: 37495
I: 44502
in: 46092
a: 60558
to: 74357
of: 79727
and: 93990
the: 154024
```

亦可直接运行 `$sh ./test-wc.sh`

> 小提示: `strings.FieldFunc` 可以将一个 string 分割成多个部分，`strconv` 包中有函数可将 string 转换成 int。

### Part III: 分布式 MapReduce

MapReduce 让开发者最爽的地方是不需要关心代码是在多台机器并行执行的。但我们现在的实现是 master 把 map 和 reduce 任务一个一个执行。虽然这种实现模式概念上很简单，但是性能并不是很高。接下来我们来实现一个并发的 MapReduce，它会调用多个 worker 线程去执行任务，这样可以更好地利用多核CPU。当然我们的实验不是真署在多台机器上而是用 channel 去模拟分布式计算。

由于是并发，所以需要调度者 master 线程，它负责给 worker 分发任务，而且一直等待直到所有 worker 完成任务。为了让我们的实验更加真实，master 只能通过 RPC 的方式与 worker 通讯。worker 代码(mapreduce/worker.go)已经准备好了，它用于启动 worker。

下一个空是 schedule.go 中的 `schedule()`，这个方法负责给 worker 分发 map 和 reduce 任务，当所有任务完成后返回。

master.go 中的 `run()` 方法会先调用 `schedule()`，然后调用 `merge()` 把每个 reduce 任务的输出文件整合到一个文件里面。schedule 只需要告诉 worker 输入文件的名字 (`mr.files[task]`) 和任务 task，worker 自己知道从哪里读取也知道把结果写到哪个文件里面。master 通过 RPC 调用 `Worker.DoTask` 通知 worker 开始新任务，同时还会在 RPC 参数中包含一个 `DoTaskArgs` 对象。

当一个 worker 准备完毕可以工作时，它会向 master 发送一个 Register RPC，注册的同时还会把这个 worker 的相关信息放入 `mr.registerChannel`。所以 `schedule` 应该通过读取这个 channel 处理新 worker 的注册。

当前正在运行的 job 信息都在 Master 中定义。注意，master 不需要知道 Map 或 Reduce 具体执行的是什么代码；当一个 worker 被 wc.go 创建时就已经携带了 Map 和 Reduce 函数的信息。

运行 `$ go test -run TestBasic mapreduce/...` 可进行基础测试。

> 小提示: master 应该并行的发送 RPC 给 worker，这样 worker 可以并发执行任务。可参考 Go RPC 文档。

> 小提示: master 应该等一个 worker 完成当前任务后马上为它分配一个新任务。等待 master 响应的线程可以用 channel 作为同步工具。Concurrency in Go 有详细的 channel 用法。

> 小提示: 跟踪 bug 最简单的方法就是在代码加入 debug()，然后执行 `go test -run TestBasic mapreduce/... > out`，out 就会包含调试信息。最重要的思考你原以为的输出和真正的输出为何不一样。

注：当前的代码试运行在一个 Unix 进程中，而且它能够利用一台机器的多核。如果是要部署在多台机器上，则要修改代码让 worker 通过 TCP 而不是 Unix-domain sockets 通讯。此外还需要一个网络文件系统共享存储。

### Part IV: 处理 worker 执行错误

本小节要让你的 master 能够处理任务执行失败的 worker。由于 MapReduce 中 worker 并没有持久状态，所以处理起来相对容易。如果一个 worker 执行失败了，master 向 worker 发送的任何一个 RPC 都可能失败，例如超时。因此，如果失败，master 应该把这个任务指派给另为一个worker。

一个 RPC 失败并不一定代表 worker 失败，有可能是某个 worker 正常运行但 master 无法获取到它的信息。所以可能会出两个 worker 同时执行同一个任务。不过因为每个任务都是幂等的，一个任务被执行两次是没啥影响。

我们假设它不会失败，所以不需要处理 master 失败的情况。让 master 可以容错是相对困难的，因为它保持着持久的状态，当它失败后我们需要恢复它的状态以保证它可以继续工作。

test_test.go 还剩最后两个测试。测有一个 worker 失败的情况和有很多 worker 失败的情况。运行可测试：`$ go test -run Failure mapreduce/...`

### Part V: 反向索引（可选）

挑战性：

词频统计虽然是 MapReduce 最经典的一个应用，但是在大规模数据应用不经常用。试试写个反向索引应用。

反向索引在计算机科学中使用广泛，尤其在文档搜索领域中非常有用。一般来说，一个反向索引就是一个从数据到数据特征的映射。例如，在文档搜索中，这个映射可能就是关键词与文档名称的映射。


main/ii.go 的整体结构跟 wc.go 相似。修改 mapF 和 reduceF 让它们创建反向索引。运行 ii.go 应该输出一个元组列表，每一行的格式如下：

```
$ go run ii.go master sequential pg-*.txt
$ head -n5 mrtmp.iiseq
A: 16 pg-being_ernest.txt,pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-metamorphosis.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
ABC: 2 pg-les_miserables.txt,pg-war_and_peace.txt
ABOUT: 2 pg-moby_dick.txt,pg-tom_sawyer.txt
ABRAHAM: 1 pg-dracula.txt
ABSOLUTE: 1 pg-les_miserables.txt
```

你的代码应该通过 test-ii.sh 的测试：

```
$ sort -k1,1 mrtmp.iiseq | sort -snk2,2 mrtmp.iiseq | grep -v '16' | tail -10
women: 15 pg-being_ernest.txt,pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-metamorphosis.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
won: 15 pg-being_ernest.txt,pg-dorian_gray.txt,pg-dracula.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-metamorphosis.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
wonderful: 15 pg-being_ernest.txt,pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
words: 15 pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-metamorphosis.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
worked: 15 pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-metamorphosis.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
worse: 15 pg-being_ernest.txt,pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
wounded: 15 pg-being_ernest.txt,pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
yes: 15 pg-being_ernest.txt,pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-metamorphosis.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
younger: 15 pg-being_ernest.txt,pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
yours: 15 pg-being_ernest.txt,pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
```

### 通过全部测试

运行 src/main/test-mr.sh 可测试本次实验的所有内容。如果全部通过，可以看到：

```
$ sh ./test-mr.sh
==> Part I
ok  	mapreduce	3.053s

==> Part II
Passed test

==> Part III
ok  	mapreduce	1.851s

==> Part IV
ok  	mapreduce	10.650s

==> Part V (challenge)
Passed test
```