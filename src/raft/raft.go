package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	//	"bytes"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	//	"6.5840/labgob"
	"6.5840/labrpc"
)

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 2D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 2D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	// 所有服务器上的持久性状态 (在响应 RPC 请求之前，已经更新到了稳定的存储设备)

	CurrentTerm    int         // 服务器的任期号（初始化为 0，持续递增）
	VotedFor       int         // 当前投票的候选人的 Id
	Log            []LogEntry  // 日志条目集；每一个条目包含一个用户状态机执行的指令，和收到时的任期号
	LogLength      int         // 日志长度
	State          State       // 节点状态
	ElectionTimer  *time.Timer // 选举定时器
	HeartbeatTimer *time.Timer // 心跳定时器
	//VoteCount      int         // 获得的选票数
	//applyCond      *sync.Cond  // 应用日志条件变量
	

	// 所有服务器上的易失性状态

	CommitIndex int // 已知已提交的最高的日志条目的索引（初始值为0，单调递增）
	LastApplied int // 已经被应用到状态机的最高的日志条目的索引（初始值为0，单调递增）

	// 领导人上的易失性状态 (选举后已经重新初始化)

	NextIndex  []int // 对于每一台服务器，发送到该服务器的下一个日志条目的索引（初始值为领导人最后的日志条目的索引+1）
	MatchIndex []int //	对于每一台服务器，已知的已经复制到该服务器的最高日志条目的索引（初始值为0，单调递增）


}

// 节点状态
type State int

const (
	Follower  State = iota // 跟随者
	Candidate              // 候选人
	Leader                 // 领导者
)

// 日志条目
type LogEntry struct {
	Term    int         // 领导者收到此条目时的任期号
	Command interface{} // 状态机执行的命令
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	term = rf.CurrentTerm
	isleader = rf.State == Leader
	return term, isleader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// raftstate := w.Bytes()
	// rf.persister.Save(raftstate, nil)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).

}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int //候选人的任期号
	CandidateId  int //请求选票的候选人的 ID
	LastLogIndex int //候选人的最后日志条目的索引值
	LastLogTerm  int //候选人最后日志条目的任期号
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int  //当前任期号，以便于候选人去更新自己的任期号
	VoteGranted bool //候选人赢得了此张选票时为真
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	//DPrintf("server %d send RequestVote to server %d, args.Term %d,  rf.CurrentTerm %d, rf.VotedFor %d", args.CandidateId, rf.me, args.Term, rf.CurrentTerm, rf.VotedFor)
	//defer DPrintf("server %d receive RequestVote from server %d, reply.Term %d, reply.VoteGranted %v", rf.me, args.CandidateId, reply.Term, reply.VoteGranted)

	//如果args.Term < rf.CurrentTerm, 返回 false
	if args.Term < rf.CurrentTerm {
		reply.Term = rf.CurrentTerm
		reply.VoteGranted = false
		return
	}

	//如果 args.Term  > rf.CurrentTerm, 切换状态为跟随者
	if args.Term > rf.CurrentTerm {
		rf.CurrentTerm = args.Term
		rf.VotedFor = -1
		rf.State = Follower
	}

	//如果节点已经投票给了其他节点，就不再投票，返回 false
	if rf.VotedFor != -1 && rf.VotedFor != args.CandidateId {
		reply.Term = rf.CurrentTerm
		reply.VoteGranted = false
		return
	}


	//选举限制：如果候选人的日志没有自己新，那么就不投票给他
	//Raft 通过比较两份日志中最后一条日志条目的索引值和任期号定义谁的日志比较新。
	//如果两份日志最后的条目的任期号不同，那么任期号大的日志更加新。
	//如果两份日志最后的条目任期号相同，那么日志比较长的那个就更加新。
	if args.LastLogTerm < rf.Log[rf.CommitIndex].Term {
		reply.Term = rf.CurrentTerm
		reply.VoteGranted = false
		return
	} else if args.LastLogTerm == rf.Log[rf.CommitIndex].Term{
		if args.LastLogIndex < rf.CommitIndex {
			reply.Term = rf.CurrentTerm
			reply.VoteGranted = false
			return
		}
	}

	

	rf.VotedFor = args.CandidateId
	rf.resetElectionTimer()
	reply.Term = rf.CurrentTerm
	reply.VoteGranted = true
}

// 追加条目RPC参数
type AppendEntriesArgs struct {
	Term         int        //领导者的任期
	LeaderId     int        //领导者的 Id，以便于跟随者重定向请求
	PrevLogIndex int        //紧邻新日志条目之前的那个日志条目的索引
	PrevLogTerm  int        //紧邻新日志条目之前的那个日志条目的任期
	Entries      []LogEntry //准备存储的日志条目（表示心跳时为空；一次性发送多个是为了提高效率）
	LeaderCommit int        //领导人的已知已提交的最高的日志条目的索引
}

type AppendEntriesReply struct {
	Term    int  //当前任期号，用于领导者去更新自己
	Success bool //如果跟随者所含有的条目和 prevLogIndex 以及 prevLogTerm 匹配上了，则为 true
}

// AppendEntries RPC handler.
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	//DPrintf("server %d send AppendEntries to server %d, args.Term %d, args.PrevLogIndex %d, args.PrevLogTerm %d", args.LeaderId, rf.me, args.Term, args.PrevLogIndex, args.PrevLogTerm)
	//defer DPrintf("server %d receive AppendEntries from server %d, reply.Term %d, reply.Success %v", rf.me, args.LeaderId, reply.Term, reply.Success)
	
	//如果args.Term  < rf.CurrentTerm, 返回 false
	if args.Term < rf.CurrentTerm {
		reply.Term = rf.CurrentTerm
		reply.Success = false
		return
	}

	//如果 args.Term  > rf.CurrentTerm，切换状态为跟随者
	if args.Term > rf.CurrentTerm {
		rf.State = Follower
		rf.VotedFor = -1
		rf.CurrentTerm = args.Term
	}


	//在接收者日志中 如果能找到一个和 prevLogIndex 以及 prevLogTerm 一样的索引和任期的日志条目 则继续执行下面的步骤 否则返回假
	if args.PrevLogIndex >= rf.LogLength || (args.PrevLogIndex > 0 && rf.Log[args.PrevLogIndex].Term != args.PrevLogTerm) {	
		reply.Term = rf.CurrentTerm
		reply.Success = false
		rf.resetElectionTimer()
		return
	}

	//如果一个已经存在的条目和新条目（即刚刚接收到的日志条目）发生了冲突（因为索引相同，任期不同），那么就删除这个已经存在的条目以及它之后的所有条目
	for i , entry := range args.Entries {
		if args.PrevLogIndex + 1 + i >= rf.LogLength {
			rf.Log = append(rf.Log, entry)
			rf.LogLength++
		}else{
			if rf.Log[args.PrevLogIndex + 1 + i].Term != entry.Term {
				rf.Log = rf.Log[:args.PrevLogIndex + 1 + i]
				rf.Log = append(rf.Log, entry)
				rf.LogLength = args.PrevLogIndex + 1 + i + 1
			}
		}
	}

	//如果leaderCommit > commitIndex
	//则把commitIndex 重置为  leaderCommit 或者是 上一个新条目的索引 取两者的最小值
	if args.LeaderCommit > rf.CommitIndex {
		rf.CommitIndex = int(math.Min(float64(args.LeaderCommit), float64(rf.LogLength-1)))
		//rf.applyCond.Broadcast()
	}


	rf.resetElectionTimer()
	reply.Term = rf.CurrentTerm
	reply.Success = true
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	//如果不是领导者，返回 false
	if rf.State != Leader {
		return index, term, false
	}

	//如果是领导者，就把命令追加到自己的日志中
	rf.Log = append(rf.Log, LogEntry{Term: rf.CurrentTerm, Command: command})

	//更新日志长度
	rf.LogLength++

	//返回值
	index = rf.LogLength - 1
	term = rf.CurrentTerm
	isLeader = true

	//DPrintf("raft %v receive command %v, term %v", rf.me, command, rf.CurrentTerm)

	return index, term, isLeader
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) ticker() {

	for rf.killed() == false {
		// Your code here (2A)
		// Check if a leader election should be started.
		select {
		case <-rf.ElectionTimer.C:
			if rf.State == Leader {
				continue
			}
			rf.resetElectionTimer()
			rf.becomeCandidate()
			rf.startElection()
		case <-rf.HeartbeatTimer.C:
			if rf.State == Leader {
				rf.resetHeartbeatTimer()
				//rf.broadcastHeartbeat()
				rf.broadcast()
			}
		}
		// pause for a random amount of time between 50 and 350
		// milliseconds.
	    // ms := 50 + (rand.Int63() % 300)
		// time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

// 转换为候选人，开始新一轮选举
func (rf *Raft) becomeCandidate() {
	rf.mu.Lock()
	//增加当前任期号
	rf.CurrentTerm++
	//给自己投票
	rf.VotedFor = rf.me
	//转换为候选人
	rf.State = Candidate
	rf.mu.Unlock()

	//DPrintf("raft %v become candidate, term %v", rf.me, rf.CurrentTerm)
}

// 开始选举
func (rf *Raft) startElection() {

	if rf.State == Candidate {

		//选票数
		voteCount := 1

		//给所有其他节点发送投票请求
		for peer := range rf.peers {
			if peer != rf.me {
				//异步调用自检
				if rf.State != Candidate {
					return
				}
				go rf.requestVote(peer, &voteCount)
			}
		}
	}
}

// 请求投票
func (rf *Raft) requestVote(peer int, voteCount *int) {
	if rf.State != Candidate {
		return
	}
	args := RequestVoteArgs{}
	args.Term = rf.CurrentTerm
	args.CandidateId = rf.me
	args.LastLogIndex = rf.LogLength - 1
	args.LastLogTerm = rf.Log[args.LastLogIndex].Term

	reply := RequestVoteReply{}

	//DPrintf("raft %v send RequestVote to %v, args.Term %v, args.LastLogIndex %v, args.LastLogTerm %v", rf.me, peer, args.Term, args.LastLogIndex, args.LastLogTerm)
	ok := rf.sendRequestVote(peer, &args, &reply)

	//rpc阻塞调用自检
	if rf.State != Candidate {
		return
	}

	if ok {
		//DPrintf("raft %v receive RequestVote reply from %v, reply.Term %v, reply.VoteGranted %v", rf.me, peer, reply.Term, reply.VoteGranted)
		rf.mu.Lock()
		defer rf.mu.Unlock()

		if reply.VoteGranted {
			*voteCount++
			//DPrintf("raft %v get vote from %v, term %v, voteCount %v", rf.me, peer, rf.CurrentTerm, rf.VoteCount)
			//如果获得了大多数的选票，就成为领导人
			if *voteCount > len(rf.peers)/2 {
				rf.becomeLeader()
				rf.resetHeartbeatTimer()
				//rf.broadcastHeartbeat()
				rf.broadcast()
				return
			}
		}else if reply.Term > rf.CurrentTerm {
			rf.CurrentTerm = reply.Term
			rf.State = Follower
			rf.VotedFor = -1
			rf.resetElectionTimer()
		}
	}
}

// 转换为领导者
func (rf *Raft) becomeLeader() {
	if rf.State != Candidate {
		return
	}
	rf.State = Leader

	//初始化 nextIndex 和 matchIndex
	//初始化所有的 nextIndex 值为自己的最后一条日志的 index 加 1
	for i := range rf.peers {
		rf.NextIndex[i] = rf.LogLength
		rf.MatchIndex[i] = -1
	}

	//DPrintf("raft %v become leader, term %v", rf.me, rf.CurrentTerm)
}

// // 广播心跳
// func (rf *Raft) broadcastHeartbeat() {
// 	for peer := range rf.peers {
// 		if peer != rf.me {
// 			//异步调用自检
// 			if rf.State != Leader {
// 				return
// 			}
// 			go rf.sendHeartbeat(peer)
// 		}
// 	}
// }

// // 发送心跳
// func (rf *Raft) sendHeartbeat(peer int) {
// 	if rf.State != Leader {
// 		return
// 	}
// 	args := AppendEntriesArgs{}
// 	args.Term = rf.CurrentTerm
// 	args.LeaderId = rf.me
// 	args.PrevLogIndex = rf.NextIndex[peer] - 1
// 	args.PrevLogTerm = rf.Log[args.PrevLogIndex].Term
// 	args.LeaderCommit = rf.CommitIndex
// 	//表示心跳， 为空
// 	args.Entries = make([]LogEntry, 0)


// 	reply := AppendEntriesReply{}

// 	rf.sendAppendEntries(peer, &args, &reply)

// 	rf.mu.Lock()
// 	defer rf.mu.Unlock()

// 	//rpc阻塞调用自检
// 	if rf.CurrentTerm != args.Term || rf.State != Leader {
// 		return
// 	}

// 	//处理返回体，如果对方的任期号比自己大，就转换为跟随者
// 	if reply.Term > rf.CurrentTerm {
// 		rf.CurrentTerm = reply.Term
// 		rf.State = Follower
// 		rf.VotedFor = -1
// 		rf.resetElectionTimer()
// 	}
// }


//广播
func (rf *Raft) broadcast() {
	if rf.State != Leader {
		return
	}

	for peer := range rf.peers {
		if peer != rf.me {
			//异步调用自检
			if rf.State != Leader {
				return
			}
			go rf.sendLog(peer)
		}
	}
}


// 发送日志
func (rf *Raft) sendLog(peer int) {

	if rf.State != Leader {
		return
	}

	args := AppendEntriesArgs{}
	args.Term = rf.CurrentTerm
	args.LeaderId = rf.me
	args.PrevLogIndex = rf.NextIndex[peer] - 1
	args.PrevLogTerm = rf.Log[args.PrevLogIndex].Term
	args.LeaderCommit = rf.CommitIndex
	//表示心跳， 为空
	args.Entries = make([]LogEntry, 0)
	
	//如果存在日志条目要发送，就发送日志条目
	if rf.NextIndex[peer] < rf.LogLength {
		//深拷贝
		args.Entries = append(args.Entries, rf.Log[rf.NextIndex[peer]:]...)
		//args.Entries = (rf.Log[rf.NextIndex[peer]:])
	}

	reply := AppendEntriesReply{}

	//DPrintf("raft %v send AppendEntries request to %v, args.Term %v, args.PrevLogIndex %v, args.PrevLogTerm %v, args.LeaderCommit %v, args.Entries %v", rf.me, peer, args.Term, args.PrevLogIndex, args.PrevLogTerm, args.LeaderCommit, args.Entries)
	ok := rf.sendAppendEntries(peer, &args, &reply)
	//DPrintf("raft %v receive AppendEntries reply from %v, reply.Term %v, reply.Success %v", rf.me, peer, reply.Term, reply.Success)

	

	//rpc阻塞调用自检
	if rf.CurrentTerm != args.Term || rf.State != Leader {
		return
	}

	if ok {
		rf.mu.Lock()
		defer rf.mu.Unlock()

		//处理返回体
		//如果成功，更新 nextIndex 和 matchIndex
		if reply.Success {
			rf.NextIndex[peer] = args.PrevLogIndex + len(args.Entries) + 1
			rf.MatchIndex[peer] = args.PrevLogIndex + len(args.Entries)
			//如果存在一个满足 N > commitIndex 的 N，并且大多数的 matchIndex[i] ≥ N 成立，并且 log[N].term == currentTerm 成立，那么令 commitIndex 等于这个 N
			for i := rf.CommitIndex + 1; i < rf.LogLength; i++ {
				if rf.Log[i].Term == rf.CurrentTerm {
					count := 1
					for j := range rf.peers {
						if j != rf.me && rf.MatchIndex[j] >= i {
							count++
						}
						if count > len(rf.peers)/2 {
							rf.CommitIndex = i
							break
						}
					}
				}
			}
		
		} else {
			//如果对方的任期号比自己大，就转换为跟随者
			if reply.Term > rf.CurrentTerm {
				rf.CurrentTerm = reply.Term
				rf.State = Follower
				rf.VotedFor = -1
				rf.resetElectionTimer()
			}else{
				//如果失败，就递减 nextIndex 重试
				rf.NextIndex[peer]--
				if rf.NextIndex[peer] < 1 {
					rf.NextIndex[peer] = 1
				}
			}
		}
	}
}


// 重置选举计时器
func (rf *Raft) resetElectionTimer() {
	rf.ElectionTimer.Stop()
	rf.ElectionTimer.Reset(randomElectionTimeout())
}

// 随机选举超时时间,选举超时应该在 300 到 450 毫秒范围内
func randomElectionTimeout() time.Duration {
	return time.Millisecond * time.Duration(rand.Intn(150)+150)
}

// 重置心跳计时器
func (rf *Raft) resetHeartbeatTimer() {
	rf.HeartbeatTimer.Stop()
	rf.HeartbeatTimer.Reset(fixedHeartbeatTimeout())
}

// 固定心跳超时时间
func fixedHeartbeatTimeout() time.Duration {
	return time.Millisecond * 100
}


// 应用已提交的日志条目到状态机
func (rf *Raft) apply(applyCh chan ApplyMsg) {
	for rf.killed() == false {
		// Your code here (2B, 2C).
		rf.mu.Lock()

		//如果 commitIndex > lastApplied，就把 lastApplied 加 1，并把 log[lastApplied] 应用到状态机中
		if rf.CommitIndex > rf.LastApplied {
			for i := rf.LastApplied + 1; i <= rf.CommitIndex; i++ {
				applyMsg := ApplyMsg{}
				applyMsg.CommandValid = true
				applyMsg.Command = rf.Log[i].Command
				applyMsg.CommandIndex = i
				rf.LastApplied = i
				rf.mu.Unlock()
				applyCh <- applyMsg
				//DPrintf("raft %v apply command %v, commandIndx %v, rf.term %v", rf.me, applyMsg.Command, applyMsg.CommandIndex, rf.CurrentTerm)
				rf.mu.Lock()
			}
		}

		rf.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
}



// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	//2A
	rf.State = Follower
	rf.VotedFor = -1
	rf.CurrentTerm = 0
	rf.ElectionTimer = time.NewTimer(randomElectionTimeout())
	rf.HeartbeatTimer = time.NewTimer(fixedHeartbeatTimeout())
	//2B
	rf.NextIndex = make([]int, len(peers))
	rf.MatchIndex = make([]int, len(peers))
	rf.Log = append(rf.Log, LogEntry{Term: 0})
	rf.LogLength = 1
	rf.CommitIndex = 0
	rf.LastApplied = 0



	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.ticker()

	// start applyCh goroutine to apply committed log entries
	go rf.apply(applyCh)

	return rf
}
