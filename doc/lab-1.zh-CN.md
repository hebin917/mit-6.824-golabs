实验一：MapReduce

### 介绍

在这个试验中，你会构建一个 MapReduce 库，在这个过程中你会学习Go语言和了解分布式系统中的容错机制。第一部分你会写一个简单的 MapReduce 程序。第二部分你会写一个 Master 分配任务给 worker 并且能处理 worker 执行错误。这个库的接口和容错的和 MapReduce 论文表述的一样。

### 软件

我们已经提供一些基础代码，可以做分布式或者单机实验。

通过 git 获取代码。

```shell
$ git clone git://g.csail.mit.edu/6.824-golabs-2016 6.824
$ cd 6.824
$ ls
Makefile src
```
MapReduce 代码支持顺序执行和分布式执行。顺序执行意味着 Map 先执行，当所有 Map 任务都完成了再执行 Reduce，这种模式可能效率比较低，但是比较便于调试，因为他去掉平行执行所产生的复杂度。分布式执行启动了很多 worker 线程，他们并行执行 Map 任务，然后执行 Reduce 任务，这种模式效率更高，当然更难实现和调试。

### 准备：熟悉代码

`mapreduce`包提供了一个简单的 MapReduce 顺序执行实现。应用只要调用 Distributed() 方法就可以启动一个任务，但是要调试的时候可能需要调用 Sequential().

mapreduce 的运行流程如下：

1. 应用层需要提供输入文件，一个 map 函数，一个 reduce 函数，要启动 reduce 任务的数量。

1. 用这些参数创建一个 master。它会启动一个 RPC 服务器(master_rpc.go)，然后等待 worker 注册(Register())。当有待完成的任务时，schedule() 就会将任务分配给 worker，同时也会进行 worker 的错误处理。

1. master 认为每个输入文件应当交给一个 map 任务处理，然后调用 doMap()，无论直接调用 Sequential() 还是通过RPC给 worker 发送 DoTask 消息都会触发这个操作。每当调用 doMap() 时，它都会去读取相应的文件，以文件内容调用 map 函数并且为每个输入文件产生 nReduce 个文件。因此，每个 map 任务最终会产生 #files x nReduce 个文件。  

1. master 接下来会对每个 reduce 任务至少调用一次 doReduce()。doReduce() 首先会收集 nReduce 个 map 任务产生的文件，然后在每个文件上执行 reduce 函数，最后产生一个结果文件。

1. master 会调用 mr.merge() 方法将上一步产生所有结果文件聚合到一个文件中。

> Note: 本次实验，你需要完善 doMap, doReduce 和 schedule 函数，同时还要写 reduce 方法。

其他的方法基本不需要改动，但是研究一下代码有助于理解其他方法如何与这个架构融合的。

### Part I: Map/Reduce 输入和输出

我们提供的代码中 Map/Reduce 缺少一些实现。在你实现你的第一个 Map/Reduce 方法之前，你首先得实现顺序执行所需要的 doMa() 和 doReduce() 函数，doMap() 函数将 map 的结果分割成多个文件；而 doReuce() 函数为 reduce 函数准备输入数据。这些函数的注释会提供实现思路。

我们提供了 Go 测试组件(test_test.go)帮你检查自己的 doMap() 和 doReduce() 的实现是否正确。要测试 sequential 的实现时候正确可以执行：

```shell
$ cd 6.824
$ export "GOPATH=$PWD"  
$ cd "$GOPATH/src/mapreduce"
$ setup ggo_v1.5
$ go test -run Sequential mapreduce/...
ok  	mapreduce	2.694s
```

如果你的运行结果不是 ok，说明你的代码还有 bug。你可以在 common.go 将 debugEnbale 设置成 true，然后运行 `go test -run Sequential mapreduce/... -v`，可以得到更详细的输出：

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

至此 map 和 reduce 任务可以结合起来了，然后我们可以开始实现 Map/Reduce 了。在本次试验中，我们将实现一个词频统计器。具体来说，你的任务是修改 mapF 和 reduceF，让 wc.go 可以统计出每个单词出现的次数。一个单词就是连续出现字母，字母的判定可以使用标准库 unicode.IsLetter。

输入文件命名是 6.824/src/main/pg-*.txt，可以先尝试编译一下：

```
$ cd 6.824
$ export "GOPATH=$PWD"
$ cd "$GOPATH/src/main"
$ go run wc.go master sequential pg-*.txt
# command-line-arguments
./wc.go:14: missing return at end of function
./wc.go:21: missing return at end of function
```

编译没通过，是因为我们还没实现 mapF he reduceF。在编码之前请阅读论文的第二节。你要实现的 mapF 和 reduceF 可能和论文中描述的有点不同。mapF 会传进来输入文件的名字以及文件的内容，它会将内容分割成单词，然后返回一个 Go slice, 类型是 mapreduce.KeyValue。你的 reduceF 函数会针对每个 key(单词) 调用一次，同时还会传进来一个数组，数组的内容就是 mapF 对这个 key 产生的值，reduceF 应该返回最终得出的值。

你可以测试你的代码：

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

最重的结果数据存在 mrtmp.wcseq 文件中。你可运行 `$ rm mrtmp.*` 删除所有的中间数据文件。

如果你的结果文件内容如下所示，说明你的代码是对的：

```
$ sort -n -k2 mrtmp.wcseq | tail -10
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

或者运行 `$sh ./test-wc.sh`

> 提示：标准库 strings.FieldFunc 可以将一个 string 分割成多个部分，strconv 有将 string 转换成 int 的方法。

### Part III: 分布式 MapReduce

Map/Reduce 最吸引人的地方在于开发者不需要关心他们的代码是在多台机器并行执行的。理论上，我们应该将上一部分的 wc 代码并行执行。

我们现在的实现是 master 把 map 和 reduce 任务一个一个执行。虽然这种实现模式概念上很简单，但是性能并不是很高。在这个部分，你将实现一个新的MapReduce 版本，它会调用多个 worker 线程去执行任务，这样可以更好地利用多核CPU。因为我们的实验不是像真正的 MapReduce 一样部署在多台机器上。所以你的实现是通过 RPC 和 channel 去模拟一个真正的分布式计算。

为了协调并行的任务执行，我们会用一个特殊的 master 线程给 worker 分发任务，而且等待他们完成。为了让我们的实验更加真实，master 只能通过RPC的方式与 worker 通讯。我们提供了 worker 代码(mapreduce/worker.go)，用于启动 worker，还有 rpc 代码用于通讯。

你的任务是完成 mapreduce 包的 schedule.go 代码。具体来说就是你需要修改 schedule()，这个方法负责给 worker 分发 map 和 reduce 任务，当所有任务完成后返回。

请看 master.go 中的 run() 方法，它会调用你的 schedule()，然后调用 merge() 把每个 reduce 任务的输出文件放进一个文件里面。schedule 只需要告诉 worker 输入文件的名字 (mr.files[task]) 和任务 task；worker 自己知道从哪里读取也知道把计算结果写到哪个文件里面。master 通过 RPC 调用 Worker.DoTask 通知 worker 开始新任务，同时还会在 RPC 参数中包含一个 DoTaskArgs 对象。

当一个 worker 开始工作时，它会向 master 发送一个 Register RPC，这部分代码已经在 Master.Register 中实现，注册的同时还会把这个 worker 的相关信息放入 mr.registerChannel。所以你的 schedule 应该通过读取这 channel 处理新 worker 的注册。

当前正在运行的 job 信息都在 Master 中定义。注意，master 不需要知道 Map 或 Reduce 具体执行的是什么代码；当一个 worker 被 wc.go 创建时就已经懈怠了 Map 和 Reduce 函数的信息。

运行 `$ go test -run TestBasic mapreduce/...` 测试本节的代码。测试分布式模式的情况，但是节点不会失败。

> Hint: master 应该并行的发送 RPC 给 worker，这样 worker 可以并发执行任务。可参考 Go RPC 文档。

> Hint: master 应该等一个 worker 完成当前任务后在为它分配一个新的任务。等待 master 响应的线程可以用 channel 作为同步工具。Concurrency in Go
 有详细的 channel 用法。

> Hint: 跟踪 bug 最简单的方法就是在代码加入 debug()，然后执行 `go test -run TestBasic mapreduce/... > out`，out 就会包含调试信息。最后思考输出是否和你认为代码所应该的输出的一样。最后一步是最重要的。

注意：当前的代码试运行在一个 Unix 进程中，而且它能够利用一台机器的多核。如果是要部署在多台机器上，则要修改代码让 worker 通过 TCP 而不是 Unix-domain sockets 通讯。此外还需要一个网络文件系统共享存储。

### Part IV: 处理 worker 执行错误

本小节要让你的 master 能够处理任务执行失败的 worker。由于 MapReduce 中 worker 并没有持久状态，所以处理起来相对容易。如果一个 worker 执行失败了，master 向 worker 发送的任何一个 RPC 都可能失败，例如超时。因此，如果失败，master 应该把这个任务指派给另为一个worker。

一个 RPC 失败并不一定是 worker 失败；仅仅不能获取它的信息但它还在运行。因此，可能有两个 worker 同时执行某个任务。不过因为每个任务都是幂等的，一个任务被执行两次是没事的，两次产生的结果都是一样的。所以对于 worker 失败，其实不需要做什么。

注意：不需要处理 master 失败的情况；我们假设它不会失败。让 master 可以容错是相对困难的，因为它保持着持久的状态，当它失败后我们需要恢复它的状态以保证它可以继续工作。之后的实验会专门处理这个挑战。

你的实现应该通过 test_test.go 剩下的两个实验。第一个是测试你有一个 worker 失败的情况。第二个是测试很多 worker 失败的情况。测试用例会定期开启新的 worker，然后它们会在处理几个任务后失败。测试可运行：`$ go test -run Failure mapreduce/...`

### Part V: 反向索引（可选）

挑战性：

词频统计虽然是 Map/Reduce 最经典的一个应用，但是在大规模数据应用不经常用。在挑战性实验中我们要利用 Map/Reduce 完成创建反向索引的任务。

反向索引在计算机科学中使用广泛，有其在文档搜索领域中非常有用。一般来说，一个反向索引就是一个从数据到数据特征的映射。例如，在文档搜索中，这个映射可能就是关键词与文档名称的映射。


代码库中有个 main/ii.go 文件，它的整体结构跟 wc.go 相似。你应该修改 mapF 和 reduceF 让它们创建反向索引。运行 ii.go 应该输出一个元组列表，每一行的格式如下：

```
$ go run ii.go master sequential pg-*.txt
$ head -n5 mrtmp.iiseq
A: 16 pg-being_ernest.txt,pg-dorian_gray.txt,pg-dracula.txt,pg-emma.txt,pg-frankenstein.txt,pg-great_expectations.txt,pg-grimm.txt,pg-huckleberry_finn.txt,pg-les_miserables.txt,pg-metamorphosis.txt,pg-moby_dick.txt,pg-sherlock_holmes.txt,pg-tale_of_two_cities.txt,pg-tom_sawyer.txt,pg-ulysses.txt,pg-war_and_peace.txt
ABC: 2 pg-les_miserables.txt,pg-war_and_peace.txt
ABOUT: 2 pg-moby_dick.txt,pg-tom_sawyer.txt
ABRAHAM: 1 pg-dracula.txt
ABSOLUTE: 1 pg-les_miserables.txt
```

如果上面不够清晰，他的格式是：

```
word: #documents documents,sorted,and,separated,by,commas
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

你可以运行 src/main/test-mr.sh 测试本次实验的所有内容。如果全部通过，输入会是：

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
