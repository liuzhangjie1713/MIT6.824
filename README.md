# MIT6.824学习记录
- [x] Lab1 [MapReduce](./src/mr)
- [x] Lab2 [Raft](./src/raft)
  - [x] Part 2A: leader election 
  - [x] Part 2B: log
  - [x] Part 2C: persistence
  - [x] Part 2D: log compaction
- [ ] Lab3 Fault-tolerant Key/Value Service
  - [ ] Part A: Key/value service without snapshots 
  - [ ] Part B: Key/value service with snapshots  
- [ ] Lab4  Sharded Key/Value Service
  - [ ] Part A: The Shard controller
  - [ ] Part B: Sharded Key/Value Server




# Lab2实验概述

MIT6.824 Lab2的内容是要求我们实现一个除了集群成员变化功能的Raft算法，总共分为四个部分，Lab2A：领导人选举，Lab2B：日志，Lab3：持久化，Lab4：日志压缩。在这里，记录一下自己对Raft算法的理解，实验的设计思路，以及遇到的问题。





## 准备工作

在做实验之前，强烈建议先认真阅读以下几篇文章。

1. [可视化raft](https://link.zhihu.com/?target=http://thesecretlivesofdata.com/raft/)和[raft论文](https://pdos.csail.mit.edu/6.824/papers/raft-extended.pdf)：这能够让我们对Raft有一个宏观的认识，图2更是做Lab的终极指南，它所提到的每个要求我们都必须去实现。
2. [MIT 6.824 Lab2实验指导](https://pdos.csail.mit.edu/6.824/labs/lab-raft.html)：这能告诉我们在Lab中需要完成的具体内容。
3. [MIT6.824 Students’ Guide to Raft](https://thesquareplanet.com/blog/students-guide-to-raft/)：这篇文章是非常重要的，因为其中包含了很多初学者易犯的错误，以及图2中没有提到但是在测试中涉及的细节或优化。
4. [Raft Locking Advice](https://pdos.csail.mit.edu/6.824/labs/raft-locking.txt)和[Raft Structure Advice](https://pdos.csail.mit.edu/6.824/labs/raft-structure.txt)：这是对代码结构和锁的建议。

以下是中文翻译版本：

1. [Raft论文](https://github.com/OneSizeFitsQuorum/MIT6.824-2021/blob/master/docs/lab2.md)和[Raft_Figure_2_Chinese](https://gukaifeng.cn/posts/raft-lun-wen-yue-du-bi-ji/Raft_Figure_2_Chinese.png)
2. [MIT 6.824 2020版 Lab2实验指导](https://zhuanlan.zhihu.com/p/248686289)
3. [MIT6.824 Students' Guide to Raft 翻译](https://gukaifeng.cn/posts/mit6.824-students-guide-to-raft-fan-yi/)
4. [Raft Locking Advice](https://zhuanlan.zhihu.com/p/510801912)





## 并发和锁

在并发和锁方面有如下几条建议：

1. 不要在读写通道上持有锁

   当Raft库获取锁往apply channel中写入数据时，如果上层应用调用Start方法就可能发生死锁，因为在Start方法中上层应用试图获取Raft库的锁而发生阻塞，Raft库也会因为上层应用无法读channel而阻塞在写channel上。

2. 不要在发送RPC时持有锁

   Raft库在持锁发送RPC的同时如果收到其他节点发送的RPC就会发生死锁。

3. 一把大锁保平安

   在lab中最好先用粗粒度的锁，想优化的时候在慢慢细化。





## 实验设计

下面我会简单介绍我实现的Raft，在此之前，先将图2贴上，以表敬意。

![](http://blog-liuzhangjie.oss-cn-chengdu.aliyuncs.com/img/2023/20230614002744.png) 



### 结构体

我的Raft结构体基本都是图2中涉及到的字段以及Lab中要求的字段，尽量保持简洁。

```go
// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	State          State       	 // 节点状态
	LeaderId       int         	 // 领导人的 Id
	ElectionTimer  *time.Timer 	 // 选举定时器
	HeartbeatTimer *time.Timer 	 // 心跳定时器
	ApplyCh 	   chan ApplyMsg // 传递给状态机的消息通道
	ApplyCond      *sync.Cond    // 唤醒applier线程的条件变量

	LastLogIndex   int         	 // 最后一条日志条目的索引

	// 所有服务器上的持久性状态 (在响应 RPC 请求之前，已经更新到了稳定的存储设备)
	CurrentTerm    int           // 服务器的任期号（初始化为 0，持续递增）
	VotedFor       int           // 当前投票的候选人的 Id
	Log            []LogEntry    // 日志条目集；每一个条目包含一个用户状态机执行的指令，和收到时的任期号


	// 所有服务器上的易失性状态
	CommitIndex       int // 已知已提交的最高的日志条目的索引（初始值为0，单调递增）
	LastApplied 	  int // 已经被应用到状态机的最高的日志条目的索引（初始值为0，单调递增）
	
	// 领导人上的易失性状态 (选举后已经重新初始化)
	NextIndex  []int // 对于每一台服务器，发送到该服务器的下一个日志条目的索引（初始值为领导人最后的日志条目的索引+1）
	MatchIndex []int //	对于每一台服务器，已知的已经复制到该服务器的最高日志条目的索引（初始值为0，单调递增）

	// 快照状态
	LastIncludedIndex int // 快照中包含的最后日志条目的索引值
	LastIncludedTerm  int // 快照中包含的最后日志条目的任期号

}
```



Make函数是Raft中的主函数，负责Raft节点的初始化和后台线程的开启：

- `ticker`：处理 heartbeat timeout 和 election timeout，我是用go中的timer来实现的定时任务，但实验指导建议使用loop+sleep 来周期性检测。
- `applier`：监听当前的 commitIndex 和 lastApplied，applier() 使用条件变量阻塞，每当 commitIndex 有增加时，则唤醒 applier() ，让 applier() 推进 lastApplied 并提交 applyMsg给上层应用·。

```go
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
    
	//2A
	rf.State = Follower
	rf.VotedFor = -1
	rf.CurrentTerm = 0
	rf.ElectionTimer = time.NewTimer(randomElectionTimeout())
	rf.HeartbeatTimer = time.NewTimer(fixedHeartbeatTimeout())
	//2B
	rf.ApplyCond = sync.NewCond(&rf.mu)
	rf.ApplyCh = applyCh
	rf.NextIndex = make([]int, len(peers))
	rf.MatchIndex = make([]int, len(peers))
	rf.Log = append(rf.Log, LogEntry{Term: 0})
	rf.LastLogIndex = 0
	rf.CommitIndex = 0
	rf.LastApplied = 0
	//2D
	rf.LastIncludedIndex = 0
	rf.LastIncludedTerm = 0

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	
	// start ticker goroutine to start elections
	go rf.ticker()

	// start applyCh goroutine to apply committed log entries
	go rf.applier()

	return rf
}
```



### 领导人选举

这一部分要求我们实现Leader选举和心跳检测，论文中的图4很好的展示了在选举过程服务器状态变化的逻辑关系。

![](http://blog-liuzhangjie.oss-cn-chengdu.aliyuncs.com/img/2023/20230608162935.png) 



#### tiker

ticker 协程会定期收到两个 timer 的到期事件

- 如果是 election timer 到期，则Folloer和Canadidate需要发起新一轮选举，重置选举计时器；
- 如果是 heartbeat timer 到期，则Leader需要发起一轮心跳，重置心跳计时器。

这一部分重要的是对选举超时时间和心跳超时间的设置：

- 选举超时时间为了避免选票被无限的重复瓜分，应该是随机的，论文建议在 150 到 300 毫秒范围内。

- 心跳超时时间是固定的，测试要求 Leader 发送心跳检测 RPC 的频率不超过 10 次/ 秒，建议设置为100毫秒

由于测试脚本的因素，选举超时时间可以更大，心跳超时时间可以更小，从而在Lab2的某些极端测试用例中，给节点更多的时间去选举出Leader或完成日志的同步，但要注意的是超时时间的改变同样会减少代码中bug在测试时出现的频率，所以不能在改变时间后通过全部测试时乐观的认为代码中没有bug。

```go
func (rf *Raft) ticker() {

	for rf.killed() == false {
		// Your code here (2A)
		// Check if a leader election should be started.

		select {
		case <-rf.ElectionTimer.C:
			rf.mu.Lock()
			if rf.State == Leader {
				rf.mu.Unlock()
				continue
			}
			rf.becomeCandidate()
			rf.startElection()
			rf.resetElectionTimer()
			rf.mu.Unlock()
		case <-rf.HeartbeatTimer.C:
			rf.mu.Lock()
			if rf.State == Leader {
				rf.broadcast()
				rf.resetHeartbeatTimer()
			}
			rf.mu.Unlock()
		}
	}
}
```



#### RequestVote RPC hander

基本参照图 2 的描述实现即可，需要注意的是

- 选举限制：`canadiate's log is at least as up-to-date as receiver's log`,这条规则是为了保证在选举的时候新的领导人拥有所有之前任期中已经提交的日志条目。

  Raft 通过比较两份日志中最后一条日志条目的索引值和任期号定义谁的日志比较新。如果两份日志最后的条目的任期号不同，那么任期号大的日志更加新。如果两份日志最后的条目任期号相同，那么日志比较长的那个就更加新。

- 只有在给候选人投票后才重置选举超时时间，这一点 guidance 里面有介绍到。

```go
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	//如果args.Term < rf.CurrentTerm, 返回 false
	if args.Term < rf.CurrentTerm {
		reply.VoteGranted = false
		reply.Term = rf.CurrentTerm
		return
	}

	//如果节点已经投票给了其他节点，就不再投票，返回 false
	if args.Term == rf.CurrentTerm && rf.VotedFor != -1 && rf.VotedFor != args.CandidateId {
		reply.VoteGranted = false
		reply.Term = rf.CurrentTerm
		return
	}

	//如果 args.Term  > rf.CurrentTerm, 切换状态为跟随者
	if args.Term > rf.CurrentTerm {
		rf.State = Follower
		rf.VotedFor = -1
		rf.CurrentTerm = args.Term
		rf.LeaderId = -1
		rf.persist()
	}

	//选举限制：如果候选人的日志没有自己新，那么就不投票给他
	//Raft 通过比较两份日志中最后一条日志条目的索引值和任期号定义谁的日志比较新。
	//如果两份日志最后的条目的任期号不同，那么任期号大的日志更加新。
	//如果两份日志最后的条目任期号相同，那么日志比较长的那个就更加新。
	if args.LastLogTerm < rf.Log[rf.LastLogIndex - rf.LastIncludedIndex].Term {
		reply.VoteGranted = false
		reply.Term = rf.CurrentTerm
		return
	}else if args.LastLogTerm == rf.Log[rf.LastLogIndex - rf.LastIncludedIndex].Term {
		if args.LastLogIndex < rf.LastLogIndex {
			reply.VoteGranted = false
			reply.Term = rf.CurrentTerm
			return
		}
	}

	//投票给候选人
	rf.VotedFor = args.CandidateId
	reply.VoteGranted = true
	reply.Term = rf.CurrentTerm
	rf.resetElectionTimer()
	rf.persist()
}
```



#### RequestVote RPC sender

sender主要是发送 RequestVote RPC request，处理RequestVote RPC reply，需要注意的有以下几点：

- 并行异步投票：发起投票时要异步并行去发起投票，从而不阻塞 ticker 协程。

- RPC args初始化：在开启requestVote协程前初始化，而不是在协程里初始化，因为异步调用时Raft结构体中数据可能发送变化，在这里没有影响，

  但在发送append RPC是就会出错。

- 抛弃过期请求的回复：对于过期请求的回复，直接抛弃就行，不要做任何处理，这一点 guidance 里面也有介绍到，在3C的测试中会涉及到。

- 投票统计：如果接收到大多数服务器的选票，那么就变成领导人，并且在成为Leader后应该立刻开始广播心跳（不要等 heartbeat timer 到期后再发送），让那些相同 term 的 candidate 马上变成 follower（不然人家超时重新选举，term 比你大，你自己刚到手的 leader 地位就保不住了）。

```go
// 开始选举
func (rf *Raft) startElection() {

	if rf.State == Candidate {

		//给自己投票
		rf.VotedFor = rf.me
		rf.persist()
		//选票数
		voteCount := 1

		//给所有其他节点发送投票请求
		for peer := range rf.peers {
			if peer != rf.me {
				args := RequestVoteArgs{}
				args.Term = rf.CurrentTerm
				args.CandidateId = rf.me
				args.LastLogIndex = rf.LastLogIndex
				args.LastLogTerm = rf.Log[args.LastLogIndex - rf.LastIncludedIndex].Term

				reply := RequestVoteReply{}
				DPrintf("raft %v send RequestVote to %v, args.Term %v, args.LastLogIndex %v, args.LastLogTerm %v", rf.me, peer, args.Term, args.LastLogIndex, args.LastLogTerm)
				go rf.requestVote(peer, &args, &reply, &voteCount)
			}
		}
	}
}

// 请求投票
func (rf *Raft) requestVote(peer int, args *RequestVoteArgs, reply *RequestVoteReply, voteCount *int) {

	ok := rf.sendRequestVote(peer, args, reply)

	if ok {
		rf.mu.Lock()
		defer rf.mu.Unlock()
		DPrintf("raft %v receive RequestVote reply from %v, reply.Term %v, reply.VoteGranted %v", rf.me, peer, reply.Term, reply.VoteGranted)

		//如果收到旧的 RPC 的回复
		//将当前任期与您在原始 RPC 中发送的任期进行比较。如果两者不同，请放弃回复并返回
		if rf.CurrentTerm != args.Term || rf.State != Candidate {
			return
		}

		//如果对方的任期号比自己大，就转换为跟随者
		if reply.Term > rf.CurrentTerm {
			rf.State = Follower
			rf.VotedFor = -1
			rf.CurrentTerm = reply.Term
			rf.resetElectionTimer()
			rf.persist()
		}

		if reply.VoteGranted {
			*voteCount++
			//如果获得了大多数的选票，就成为领导人
			if *voteCount > len(rf.peers)/2 {
				rf.becomeLeader()
				rf.broadcast()
				rf.resetHeartbeatTimer()
			}
		}
	}
}
```

### 日志复制

日志复制是Raft算法的核心，2b, 2c, 2d中都有很多针对这部分代码的 corner case ，会有很多容易忽视的细节需要通过不断地debug去完善。



#### becomeLeader

在节点成为Leader后，开始发送日志前，需要对 nextIndex 和 matchIndex进行初始化

- nextIndex ：是对领导者与给定追随者共享什么前缀的猜测，实际要发送每个 Follower 的下一个日志条目。它通常非常乐观（先假定我们共享一切，例如，当一个领导者刚刚被选举出来时，nextIndex 被设置为日志末尾的索引），仅在负面回应时才向前移动。
- matchIndex ：用于安全，它是对领导者与给定追随者共享日志前缀的保守度量，实际表示 Leader 和每个 Follower 匹配到的日志条目，最开始为0，仅在追随者肯定地确认 AppendEntries RPC 时更新。
- 因为用途不同，虽然大多数情况下matchIndex = nextIndex - 1，但你必须单独计算nextIndex和matchIndex 

```go
func (rf *Raft) becomeLeader() {

	if rf.State != Candidate {
		return
	}

	//转换为领导者
	rf.State = Leader
	rf.LeaderId = rf.me
	//初始化 nextIndex 和 matchIndex
	//初始化所有的 nextIndex 值为自己的最后一条日志的 index 加 1
	for i := range rf.peers {
		//rf.NextIndex[i] = rf.LastLogIndex + 1
		rf.NextIndex[i] = Max(rf.LastLogIndex + 1, rf.NextIndex[i])
		rf.MatchIndex[i] = 0
	}

	DPrintf("raft %v become leader, term %v", rf.me, rf.CurrentTerm)
}
```



#### AppendEntries RPC hander

基本都是按照图 2 中的伪代码实现的，LastIncludedIndex是2d snapshot所需要实现的内容，在这里可以看作是0。

这一部分注意的是当Leader和Folloer日志发生冲突时日志回溯的方式

1. 第一种是Leader在被跟随者拒绝之后，减小 nextIndex 值并进行重试，这种实现网络开销较大，能通过2b测试，但无法通过2c
2. 第二种是论文中提到的加速日志回溯优化，guidance进行了详细介绍：通过跟随者返回冲突条目的任期号和该任期号对应的最小索引地址（即conflictterm和conflictIndex ）来让领导人的 nextIndex直接回退到发生冲突的地方，从而减少被拒绝的附加日志 RPCs 的次数

```go
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	//DPrintf("raft %v receive AppendEntries, args.PrevLogIndex %v, args.PrevLogTerm %v, rf.LastLogIndex %v, rf.LastIncludedIndex %v", rf.me, args.PrevLogIndex, args.PrevLogTerm, rf.LastLogIndex, rf.LastIncludedIndex)

	reply.ConflictIndex = -1
	reply.ConflictTerm = -1

	//如果args.Term  < rf.CurrentTerm, 返回 false
	if args.Term < rf.CurrentTerm {
		reply.Success = false
		reply.Term = rf.CurrentTerm
		return
	}

	//如果 args.Term  > rf.CurrentTerm，切换状态为跟随者
	if args.Term > rf.CurrentTerm {
		rf.State = Follower
		rf.VotedFor = -1
		rf.CurrentTerm = args.Term
		rf.persist()
	}

	rf.State = Follower
	rf.LeaderId = args.LeaderId
	rf.resetElectionTimer()

	//如果接收者日志中没有包含这样一个条目，那么就返回 false
	if args.PrevLogIndex < rf.LastIncludedIndex {
		reply.Success = false
		reply.Term = rf.CurrentTerm
		reply.ConflictIndex = rf.LastIncludedIndex + 1
		reply.ConflictTerm = -1
		return
	}


	//在接收者日志中, 如果找不到PrevLogIndex
	//应该返回 conflictIndex = len(log) 和 conflictTerm = None。
	if args.PrevLogIndex > rf.LastLogIndex {
		reply.Success = false
		reply.Term = rf.CurrentTerm
		reply.ConflictIndex = rf.LastLogIndex + 1
		reply.ConflictTerm = -1
		return
	}

	//在接收者日志中, 如果追随者的日志中有 preLogIndex，但是任期不匹配
	//返回 conflictTerm = log[preLogIndex].Term，然后在它的日志中搜索任期等于 conflictTerm 的第一个条目索引。
	if args.PrevLogIndex >= rf.LastIncludedIndex && rf.Log[args.PrevLogIndex - rf.LastIncludedIndex].Term != args.PrevLogTerm {
		reply.Success = false
		reply.Term = rf.CurrentTerm
		reply.ConflictTerm = rf.Log[args.PrevLogIndex - rf.LastIncludedIndex].Term
		index := args.PrevLogIndex
		for index >= rf.LastIncludedIndex && rf.Log[index - rf.LastIncludedIndex].Term == reply.ConflictTerm {
			index--
		}
		reply.ConflictIndex = index + 1
		return
	}

	
	DPrintf("raft %v start AppendEntries, args.PrevLogIndex %v, args.PrevLogTerm %v, rf.LastLogIndex %v, rf.LastIncludedIndex %v", rf.me, args.PrevLogIndex, args.PrevLogTerm, rf.LastLogIndex, rf.LastIncludedIndex)
	//如果一个已经存在的条目和新条目（即刚刚接收到的日志条目）发生了冲突（因为索引相同，任期不同），那么就删除这个已经存在的条目以及它之后的所有条目
	if len(args.Entries) > 0 {
		for i, entry := range args.Entries {
			if args.PrevLogIndex+i+1 > rf.LastLogIndex {
				rf.Log = append(rf.Log, entry)
				rf.LastLogIndex++
			} else {
				if rf.Log[args.PrevLogIndex+i+1 - rf.LastIncludedIndex].Term != entry.Term {
					rf.Log = rf.Log[:args.PrevLogIndex+i+1 - rf.LastIncludedIndex]
					rf.Log = append(rf.Log, entry)
					rf.LastLogIndex = args.PrevLogIndex + i + 1
				}
			}
		}
	}
	
	rf.persist()

	//如果leaderCommit > commitIndex
	//则把commitIndex 重置为  leaderCommit 或者是 上一个新条目的索引 取两者的最小值
	if args.LeaderCommit > rf.CommitIndex {
		rf.CommitIndex = args.LeaderCommit
		if rf.CommitIndex > rf.LastLogIndex {
			rf.CommitIndex = rf.LastLogIndex
		}
		rf.ApplyCond.Broadcast()
	}

	reply.Success = true
	reply.Term = rf.CurrentTerm
}
```



#### AppendEntries RPC sender

可以看到对于 follower 的日志复制，有 snapshot 和 entries 两种方式，需要根据该 peer 的 nextIndex 来判断。而当领导者没有新条目要发送给特定对等方，则 AppendEntries RPC 不包含任何条目，并被视为心跳。

这部分代码需要注意的是：

- 抛弃过期请求的回复：对于过期请求的回复，直接抛弃就行，不要做任何处理，这一点 guidance 里面也有介绍到。
- commit 日志：图 2 中规定，raft leader 只能提交当前 term 的日志，不能提交旧 term 的日志。因此 leader 根据 matchIndex[] 来 commit 日志时需要判断该日志的 term 是否等于 leader 当前的 term，即是否为当前 leader 任期新产生的日志，若是才可以提交。这是因为 Raft 领导者无法确定一个条目是否确实已提交（并且将来永远不会更改），如果它不是来自他们当前的任期。
- 处理被拒绝的RPC：如果一个领导者发出一个 `AppendEntries` RPC，它被拒绝了，但不是因为日志不一致（这只有在我们的任期已过时才会发生），那么你应该立即下台，而不是更新 `nextIndex`。 如果您这样立即重新选举，您可以使用重置的 nextIndex 竞争领导者。

```go
func (rf *Raft) broadcast() {

	if rf.State != Leader {
		return
	}

	for peer := range rf.peers {
		if peer != rf.me {
			//检查是否发送快照
			if rf.NextIndex[peer] <= rf.LastIncludedIndex {
				args := InstallSnapshotArgs{}
				args.Term = rf.CurrentTerm
				args.LeaderId = rf.me
				args.LastIncludedIndex = rf.LastIncludedIndex
				args.LastIncludedTerm = rf.LastIncludedTerm
				args.Data = rf.persister.ReadSnapshot()

				reply := InstallSnapshotReply{}

				DPrintf("raft %v send InstallSnapshot to %v, args.Term %v, args.LastIncludedIndex %v, args.LastIncludedTerm %v, args.Data %v", rf.me, peer, args.Term, args.LastIncludedIndex, args.LastIncludedTerm, args.Data)
				
				go rf.sendSnapshot(peer, &args, &reply)
			} else {
				//发送日志
				args := AppendEntriesArgs{}
				args.Term = rf.CurrentTerm
				args.LeaderId = rf.me
				args.PrevLogIndex = rf.NextIndex[peer] - 1
				args.PrevLogTerm = rf.Log[args.PrevLogIndex - rf.LastIncludedIndex].Term
				args.LeaderCommit = rf.CommitIndex
				//表示心跳， 为空
				args.Entries = make([]LogEntry, 0)
				//如果存在日志条目要发送，就发送日志条目
				if rf.NextIndex[peer] <= rf.LastLogIndex {
					//深拷贝
					args.Entries = append(args.Entries, rf.Log[rf.NextIndex[peer] - rf.LastIncludedIndex:]...)
				}
			
				reply := AppendEntriesReply{}
				DPrintf("raft %v send AppendEntries to %v, args.Term %v, args.PrevLogIndex %v, args.PrevLogTerm %v, args.LeaderCommit %v, args.Entries %v", rf.me, peer, args.Term, args.PrevLogIndex, args.PrevLogTerm, args.LeaderCommit, args.Entries)
				go rf.sendLog(peer, &args, &reply)
			}
		}
	}
}


// 发送日志(空时，表示心跳)
func (rf *Raft) sendLog(peer int, args *AppendEntriesArgs, reply *AppendEntriesReply) {

	ok := rf.sendAppendEntries(peer, args, reply)

	if ok {
		rf.mu.Lock()
		defer rf.mu.Unlock()
		DPrintf("raft %v receive AppendEntries reply from %v, reply.Term %v, reply.Success %v, reply.ConflictIndex %v, reply.ConflictTerm %v", rf.me, peer, reply.Term, reply.Success, reply.ConflictIndex, reply.ConflictTerm)

		//如果收到旧的 RPC 的回复，先记录回复中的任期（可能高于您当前的任期），
		//然后将当前任期与您在原始 RPC 中发送的任期进行比较。如果两者不同，请放弃回复并返回
		if rf.CurrentTerm != args.Term || rf.State != Leader {
			return
		}

		//处理返回体
		//如果成功，更新 nextIndex 和 matchIndex
		if reply.Success == true {
			rf.NextIndex[peer] = Max(args.PrevLogIndex + len(args.Entries) + 1, rf.NextIndex[peer])
			rf.MatchIndex[peer] = Max(args.PrevLogIndex + len(args.Entries), rf.MatchIndex[peer])

			//如果存在一个满足 N > commitIndex 的 N，并且大多数的 matchIndex[i] ≥ N 成立，并且 log[N].term == currentTerm 成立，那么令 commitIndex 等于这个 N
			for N := rf.CommitIndex + 1; N <= rf.LastLogIndex; N++ {
				if rf.Log[N - rf.LastIncludedIndex].Term == rf.CurrentTerm {
					count := 1
					for i := range rf.peers {
						if i != rf.me && rf.MatchIndex[i] >= N {
							count++
						}
					}
					if count > len(rf.peers)/2 {
						rf.CommitIndex = N
						//break
					}
				}
			}
			rf.ApplyCond.Broadcast()
		} else {
			//如果对方的任期号比自己大，就转换为跟随者
			if reply.Term > rf.CurrentTerm {
				rf.State = Follower
				rf.VotedFor = -1
				rf.CurrentTerm = reply.Term
				rf.resetElectionTimer()
				rf.persist()
			} else {
				//如果失败，就递减 nextIndex 重试
				// rf.NextIndex[peer]--
				// if rf.NextIndex[peer] < 1 {
				// 	rf.NextIndex[peer] = 1
				// }

				if reply.ConflictTerm == -1 {
					rf.NextIndex[peer] = reply.ConflictIndex
				} else {
					conflictIndex := -1
					//在收到一个冲突响应后，领导者首先应该搜索其日志中任期为 conflictTerm 的条目。
					//如果领导者在其日志中找到此任期的一个条目，则应该设置 nextIndex 为其日志中此任期的最后一个条目的索引的下一个。
					for i := args.PrevLogIndex; i >= rf.LastIncludedIndex; i-- {
						if rf.Log[i - rf.LastIncludedIndex].Term == reply.ConflictTerm {
							conflictIndex = i
							break
						}
					}
					//如果领导者没有找到此任期的条目，则应该设置 nextIndex = conflictIndex
					if conflictIndex == -1 {
						rf.NextIndex[peer] = reply.ConflictIndex
					} else {
						rf.NextIndex[peer] = conflictIndex + 1
					}
				}
			}
		}
	}
}
```

#### start

start方法由上层应用(在lab即为测试用例)调用，将客户端发送给服务器的请求以日志的形式写到Raft层。

- 只有Leader才能写入日志。

- 写完日志后建议立刻广播开始日志同步。

```go
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).
	//如果不是领导者，返回 false
	if rf.State != Leader {
		return index, term, false
	}

	//如果是领导者，就把命令追加到自己的日志中
	rf.Log = append(rf.Log, LogEntry{Term: rf.CurrentTerm, Command: command})

	//更新最后一条日志的索引
	rf.LastLogIndex++

	DPrintf("raft %v receive command %v, term %v", rf.me, command, rf.CurrentTerm)

	//持久化
	rf.persist()

	//广播
	rf.broadcast()
	rf.resetHeartbeatTimer()

	//返回值
	index = rf.LastLogIndex
	term = rf.CurrentTerm
	isLeader = true

	return index, term, isLeader
}
```

#### applier

appiler协程负责监听当前的 commitIndex 和 lastApplied，任何时候 commitIndex > lastApplied，appiler就会发送特定的日志条目到channel中，让上层服务应用日志到状态机中。

applier的实现有两种方式：

- `loop + sleep`：applier在循环中每睡眠10ms，醒来后比较当前的commitIndex 和 lastApplied。
- `condition variables`：使用go的条件变量`rf.ApplyCond`，在leader 提交了新的日志或者 follower 通过 leader 发来的 leaderCommit 来更新 commitIndex时，主动通知applier协程将[lastApplied + 1, commitIndex] 区间的日志 push 到 applyCh 中去。

很明显第二种方式性能更好，而且调试时日志可读性更好。

这部分代码需要注意的是

- 读写通道时不能持锁：在`rf.ApplyCh <- applymsg`之前释放锁，写完后再加锁。
- 引用之前的 commitIndex：push applyCh 结束之后更新 lastApplied 的时候一定得用之前的 commitIndex 而不是 rf.commitIndex，因为后者很可能在 push channel 期间发生了改变。

```go
func (rf *Raft) applier() {
	for rf.killed() == false {
		rf.mu.Lock()
		for rf.LastApplied >= rf.CommitIndex {
			rf.ApplyCond.Wait()
		}

		lastIncludedIndex := rf.LastIncludedIndex
		lastApplied := rf.LastApplied
		commitIndex := rf.CommitIndex
		entries := make([]LogEntry, commitIndex - lastApplied)
		copy(entries, rf.Log[lastApplied + 1 - lastIncludedIndex:commitIndex + 1 - lastIncludedIndex])
		rf.mu.Unlock()
		for i , entry := range entries {
			applymsg := ApplyMsg{CommandValid: true, Command: entry.Command, CommandIndex: lastApplied + 1 + i}
			rf.ApplyCh <- applymsg
		}
		rf.mu.Lock()
		rf.LastApplied = Max(rf.LastApplied, commitIndex)
		rf.mu.Unlock()
	}
}
```



### 持久化

持久化是为了让重新启动的服务器能够从中断的位置恢复服务。

持久化的部分其实代码写起来非常简单，困难的是2C的测试中相比2B增加了许多 corner case（比如服务器故障，RPC丢失和延迟），你需要不断debug去完善你在2B中写的代码，但2C中打印的日志太长了导致读日志的时候非常难受，导致最后我花在2C上的时间都和2B一样长了。

这部分需要注意的是调用persist和readPersist的地方

- readPersist：只需要在make方法中调用，用于节点的初始化。
- persist：currentTerm, voteFor 和 logs 这三个变量一旦发生变化就一定要在被其他协程感知到之前（释放锁之前，发送 rpc 之前）调用persist持久化，这样才能保证原子性。具体在以下几个地方
  1. 发起选举，更新 term 和 votedFor 时。
  2. 调用 Start 追加日志，rf.log 改变时。
  3. RequestVote 和 AppendEntries 两个 RPC handler 改变相关状态时。
  4. Candidate/Leader 收到 RPC 的 reply 更新自身状态时

```go
func (rf *Raft) persist() {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.CurrentTerm)
	e.Encode(rf.VotedFor)
	e.Encode(rf.Log)
	e.Encode(rf.LastIncludedIndex)
	e.Encode(rf.LastIncludedTerm)
	raftstate := w.Bytes()
	rf.persister.SaveRaftState(raftstate)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}

	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var CurrentTerm int
	var VotedFor int
	var Log []LogEntry
	var LastIncludedIndex int
	var LastIncludedTerm int
	if d.Decode(&CurrentTerm) != nil || d.Decode(&VotedFor) != nil || d.Decode(&Log) != nil || 
	d.Decode(&LastIncludedIndex) != nil || d.Decode(&LastIncludedTerm) != nil{
		//error
	} else {
		rf.CurrentTerm = CurrentTerm
		rf.VotedFor = VotedFor
		rf.Log = Log
		rf.LastIncludedIndex = LastIncludedIndex
		rf.LastIncludedTerm = LastIncludedTerm
		rf.CommitIndex = rf.LastIncludedIndex
		rf.LastApplied = rf.LastIncludedIndex
		rf.LastLogIndex = rf.LastIncludedIndex + len(rf.Log) - 1

		DPrintf("raft %v readPersist, CurrentTerm %v, VotedFor %v, Log %v, LastLogIndex %v", rf.me, rf.CurrentTerm, rf.VotedFor, rf.Log, rf.LastLogIndex)
	}
}
```



### 日志压缩

Raft使用快照的方式对日志进行压缩：将整个系统的状态都以快照的形式写入到稳定的持久化存储中，然后快照之前的日志全部丢弃。这样可以避免日志的无限增长，占用太多的内存空间，也可以避免节点重启后需要从头开始执行大量的l日志，节点仅需读取快照，并重新执行快照后的日志条目即可。

日志压缩的代码实现主要参考论文的section7，和 Raft 交互图

![](http://blog-liuzhangjie.oss-cn-chengdu.aliyuncs.com/img/2023/20230613145058.png) 





#### LastIncludeIndex

在2D中需要对索引系统进行改造，快照功能引入后，由于涉及日志条目的截断操作，截断之前的日志变得不可访问了，故需要设`lastIncludeIndex`这变量来记录快照中最后一个日志的索引，然后在代码中所有需要访问`rf.log`中的日志时，就必须改为用日志的index - lastIncludeIndex才能得到日志在log数组中的实际下标。

lastIncludeTerm可以存到rf.log[0]中，便于后续判断。



#### Snapshot

服务器在通过`applyCh `给应用层发送提交日志信息时，当日志到达一个阈值时，应用层会生成一个快照，并传递给`Snapshot`函数（测试是每10个生成一次快照），`Snapshot`函数就会对生成的快照后无用的日志记录进行裁剪，并像2C一样持久化存储这些信息。

```go
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.killed() == true {
		return
	}

	if index <= rf.LastIncludedIndex || index > rf.CommitIndex{
		return
	}
	DPrintf("raft %v Snapshot, index %v, LastIncludedIndex %v, LastIncludedTerm %v, CommitIndex %v, LastApplied %v", rf.me, index, rf.LastIncludedIndex, rf.LastIncludedTerm, rf.CommitIndex, rf.LastApplied)

	//如果index > rf.LastIncludedIndex，就截断日志
	//更新rf.LastIncludedIndex 和 rf.LastIncludedTerm
	rf.LastIncludedTerm = rf.Log[index - rf.LastIncludedIndex].Term
	
	var newLog []LogEntry = make([]LogEntry, 0)
	newLog = append(newLog, LogEntry{Term: rf.LastIncludedTerm})
	newLog = append(newLog, rf.Log[index+ 1 - rf.LastIncludedIndex:]...)
	rf.Log = newLog
	rf.LastIncludedIndex = index

	//保存状态和快照
	rf.savaStateAndSnapshot(snapshot)
}
```



#### InstallSnapshot RPC hander

有快照机制后，当leader修剪log后，在进行log replication的时候，部分follower缺少已经被leader快照修剪没了的log，那么leader就需要调用该RPC来将自身的快照发送给该follower，来解决这个问题。

这部分需要注意的部分是：

- 节点收到快照后，需要检查发送来的快照是否是过时的，避免旧的快照把本地新的快照给取代了，发生数据回滚。
- 节点收到快照有两种可能：
  - args.LastIncludedIndex比本节点的lastLogIndex都要大，那么节点仅需将本地log全部删除，然后将节点的lastLogIndex变更为args.LastIncludedIndex。
  - args.LastIncludedIndex比本节点的lastLogIndex要小，那么节点仅需将包括LastIncludeIndex和在此之前的全部Log修剪掉即可，无需改动lastLogIndex。
  - 但都必须把args.LastIncludedTerm放在新日志的log[0]处

```go
func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	//如果term < currentTerm就立即回复
	if args.Term < rf.CurrentTerm {
		reply.Term = rf.CurrentTerm
		return
	}

	if args.Term > rf.CurrentTerm {
		rf.State = Follower
		rf.VotedFor = -1
		rf.CurrentTerm = args.Term
		rf.LeaderId = -1
		rf.persist()
	}

	rf.State = Follower
	rf.LeaderId = args.LeaderId
	rf.resetElectionTimer()

	//如果args.LastIncludedIndex <= rf.CommitIndex，就立即回复
	if args.LastIncludedIndex <= rf.LastIncludedIndex {
		reply.Term = rf.CurrentTerm
		return
	}

	DPrintf("raft %v recieve InstallSnapshot, rf.LastIncludedIndex %v, args.LastIncludedIndex %v, rf.LastIncludedIndex %v", rf.me, rf.LastIncludedIndex, args.LastIncludedIndex, rf.LastIncludedIndex)

	if args.LastIncludedIndex >= rf.LastLogIndex {
		rf.LastLogIndex = args.LastIncludedIndex
		var newLog []LogEntry = make([]LogEntry, 0)
		newLog = append(newLog, LogEntry{Term: args.LastIncludedTerm})
		rf.Log = newLog
	}else{
		//截断日志
		var newLog []LogEntry = make([]LogEntry, 0)
		newLog = append(newLog, LogEntry{Term: args.LastIncludedTerm})
		newLog = append(newLog, rf.Log[args.LastIncludedIndex + 1 - rf.LastIncludedIndex:]...)
		rf.Log = newLog
	}
	
	//更新rf.LastIncludedIndex 和 rf.LastIncludedTerm
	rf.LastIncludedIndex = args.LastIncludedIndex
	rf.LastIncludedTerm = args.LastIncludedTerm

	//更新rf.CommitIndex 和 rf.LastApplied
	rf.CommitIndex = Max(rf.CommitIndex, rf.LastIncludedIndex)
	rf.LastApplied = Max(rf.LastApplied, rf.LastIncludedIndex)
	

	//保存状态和快照
	rf.savaStateAndSnapshot(args.Data)

	reply.Term = rf.CurrentTerm

	applyMsg := ApplyMsg{
		SnapshotValid: true,
		Snapshot: args.Data,
		SnapshotTerm: args.LastIncludedTerm,
		SnapshotIndex: args.LastIncludedIndex,
	}

	rf.mu.Unlock()

	rf.ApplyCh <- applyMsg
}	
```



#### InstallSnapshot  RPC sender

在broadcast中，如果`rf.NextIndex[peer] <= rf.LastIncludedIndex`，就说明Leader需要发送给Follower的日志条目已经被存入快照了，在这种情况下Leader就需要使用InstallSnapshot  RPC 来发送快照给太落后的Follower。

Leader在收到RPC reply后仍需要更新自己的nextIndex和matchIndex，在这里使用max主要是为了防止RPC延迟到达造成的index倒退。

```go
func (rf *Raft) sendSnapshot(peer int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) {

	ok := rf.sendInstallSnapshot(peer, args, reply)

	if ok {
		rf.mu.Lock()
		defer rf.mu.Unlock()
		DPrintf("raft %v receive InstallSnapshot reply from %v, reply.Term %v", rf.me, peer, reply.Term)

		//如果收到旧的 RPC 的回复，先记录回复中的任期（可能高于您当前的任期），
		//然后将当前任期与您在原始 RPC 中发送的任期进行比较。如果两者不同，请放弃回复并返回
		if rf.CurrentTerm != args.Term || rf.State != Leader {
			return
		}

		//如果对方的任期号比自己大，就转换为跟随者
		if reply.Term > rf.CurrentTerm {
			rf.State = Follower
			rf.VotedFor = -1
			rf.CurrentTerm = reply.Term
			rf.resetElectionTimer()
			rf.persist()
		}else{
			//rf.NextIndex[peer] = rf.LastIncludedIndex + 1
			rf.NextIndex[peer] = Max(args.LastIncludedIndex + 1, rf.NextIndex[peer])
			rf.MatchIndex[peer] = Max(rf.LastIncludedIndex, rf.MatchIndex[peer])
		}
	}
}
```





## 调试

下图是教授在课程上提到的调试经验，可以参考。但其实在整个Lab2中，最终极也是最简单的办法就是在报错后重新读一遍论文和guidence，看看自己哪些细节没考虑到。

![](http://blog-liuzhangjie.oss-cn-chengdu.aliyuncs.com/img/2023/20230610210030.png) 

具体的debug工具可以使用`util.go`中的`Dprintf`函数：

- 单次测试，用`go test -race -run 2A > out`  命令输出日志 

- 多次测试时，可以使用助教写过的测试脚本[go-test-many.sh](https://gist.github.com/jonhoo/f686cacb4b9fe716d5aa)，用`/go-test-many.sh 100 10 TestFigure8Unreliable2C`命令来输出日志，100表示测试一百次，10表示开10个进程。

如果你想要更高级的调试脚本，可以看 [Debugging by Pretty Printing](https://blog.josejg.com/debugging-pretty/)，这篇文章将会讲解如何使用 python 脚本根据日志的类型和不同的节点编号，进行着色和分割，作者提供了[dslogs](https://link.zhihu.com/?target=https%3A//gist.github.com/JJGO/e64c0e8aedb5d464b5f79d3b12197338) 脚本来美化日志 和[dstest](https://link.zhihu.com/?target=https%3A//gist.github.com/JJGO/0d73540ef7cc2f066cb535156b7cbdab) 脚本来进行批量测试。具体使用办法可以参考这两篇blog，[blog1](https://blog.rayzhang.top/2022/11/09/mit-6.824-lab2-raft/index.html)和[blog2](https://zhuanlan.zhihu.com/p/514654768).



在测试中特别容易报两种错误，特别是在`TestFigure8Unreliable2C`中，：

- `one(%v) failed to reach agreement`：可以看看这个[github issue](https://github.com/springfieldking/mit-6.824-golabs-2018/issues/1),里面有关于导致这种情况常见问题的讨论
- `commit index=%v server=%v %v != server=%v %v`：可以看看这个[github issue](https://github.com/springfieldking/mit-6.824-golabs-2018/issues/2)



我的代码测试结果：

![](http://blog-liuzhangjie.oss-cn-chengdu.aliyuncs.com/img/2023/20231222214547.png) 



但100次测试，仍会报错两次： `one(3930212705376956277) failed to reach agreement`

![](http://blog-liuzhangjie.oss-cn-chengdu.aliyuncs.com/img/2023/20230614002522.png) 

具体情况为选举过程出现活锁，无法选出领导者，但我一直无法明白节点1为什么不会选举超时。

我检查过选举代码，没有逻辑问题，目前猜测是锁的问题，节点1可能阻塞到某个位置。

![](http://blog-liuzhangjie.oss-cn-chengdu.aliyuncs.com/img/2023/20230614002551.png) 


