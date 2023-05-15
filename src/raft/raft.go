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
	CurrentTerm    int         // 服务器的任期号（初始化为 0，持续递增）
	VotedFor       int         // 当前投票的候选人的 Id
	Log            []LogEntry  // 日志条目集；每一个条目包含一个用户状态机执行的指令，和收到时的任期号
	State          State       // 节点状态
	ElectionTimer  *time.Timer // 选举定时器
	HeartbeatTimer *time.Timer // 心跳定时器
	VoteCount      int         // 获得的选票数
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

	//如果term < currentTerm返回 false
	if args.Term < rf.CurrentTerm || (args.Term == rf.CurrentTerm && rf.VotedFor != -1 && rf.VotedFor != args.CandidateId) {
		reply.Term = rf.CurrentTerm
		reply.VoteGranted = false
		return
	}

	if args.Term > rf.CurrentTerm {
		rf.CurrentTerm = args.Term
		rf.VotedFor = -1
		rf.State = Follower
	}

	rf.VotedFor = args.CandidateId
	rf.resetElectionTimer()
	reply.Term = rf.CurrentTerm
	reply.VoteGranted = true
}

// 追加条目RPC参数
type AppendEntriesArgs struct {
	Term         int        //领导者的任期号
	LeaderId     int        //领导者的 Id，以便于跟随者重定向请求
	PrevLogIndex int        //紧挨着新日志条目的上一个索引
	PrevLogTerm  int        //prevLogIndex 条目的任期号
	Entries      []LogEntry //存储的日志条目（表示心跳时为空）
	LeaderCommit int        //领导者已经提交的日志的索引值(commitIndex)
}

type AppendEntriesReply struct {
	Term    int  //当前任期号，用于领导者去更新自己
	Success bool //跟随者包含了匹配上 prevLogIndex 和 prevLogTerm 的日志时为真
}

// AppendEntries RPC handler.
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	//DPrintf("server %d send AppendEntries to server %d, args.Term %d, args.PrevLogIndex %d, args.PrevLogTerm %d", args.LeaderId, rf.me, args.Term, args.PrevLogIndex, args.PrevLogTerm)
	//defer DPrintf("server %d receive AppendEntries from server %d, reply.Term %d, reply.Success %v", rf.me, args.LeaderId, reply.Term, reply.Success)
	//如果term < currentTerm, 返回 false
	if args.Term < rf.CurrentTerm {
		reply.Term = rf.CurrentTerm
		reply.Success = false
		return
	}

	//如果 RPC 请求或响应包含的任期号 T > currentTerm，切换状态为跟随者
	if args.Term > rf.CurrentTerm {
		rf.State = Follower
		rf.VotedFor = -1
		rf.CurrentTerm = args.Term
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
				rf.broadcastHeartbeat()
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

// 请求投票
func (rf *Raft) requestVote(peer int) {
	if rf.State != Candidate {
		return
	}
	args := RequestVoteArgs{}
	args.Term = rf.CurrentTerm
	args.CandidateId = rf.me

	reply := RequestVoteReply{}

	ok := rf.sendRequestVote(peer, &args, &reply)

	//rpc阻塞调用自检
	if rf.State != Candidate {
		return
	}

	if ok {
		rf.mu.Lock()
		defer rf.mu.Unlock()

		if reply.VoteGranted {
			rf.VoteCount++
			//DPrintf("raft %v get vote from %v, term %v, voteCount %v", rf.me, peer, rf.CurrentTerm, rf.VoteCount)
			//如果获得了大多数的选票，就成为领导人
			if rf.VoteCount > len(rf.peers)/2 {
				rf.becomeLeader()
				rf.resetHeartbeatTimer()
				rf.broadcastHeartbeat()
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

// 开始选举
func (rf *Raft) startElection() {

	if rf.State == Candidate {

		//选票数
		rf.VoteCount = 1

		//给所有其他节点发送投票请求
		for peer := range rf.peers {
			if peer != rf.me {
				//异步调用自检
				if rf.State != Candidate {
					return
				}
				go rf.requestVote(peer)
			}
		}
	}
}

// 转换为领导者
func (rf *Raft) becomeLeader() {
	rf.State = Leader

	//DPrintf("raft %v become leader, term %v", rf.me, rf.CurrentTerm)
}

// 广播心跳
func (rf *Raft) broadcastHeartbeat() {
	for peer := range rf.peers {
		if peer != rf.me {
			//异步调用自检
			if rf.State != Leader {
				return
			}
			go rf.sendHeartbeat(peer)
		}
	}
}

// 发送心跳
func (rf *Raft) sendHeartbeat(peer int) {
	if rf.State != Leader {
		return
	}
	args := AppendEntriesArgs{}
	args.Term = rf.CurrentTerm
	args.LeaderId = rf.me

	reply := AppendEntriesReply{}

	rf.sendAppendEntries(peer, &args, &reply)

	rf.mu.Lock()
	defer rf.mu.Unlock()

	//rpc阻塞调用自检
	if rf.CurrentTerm != args.Term || rf.State != Leader {
		return
	}

	//处理返回体，如果对方的任期号比自己大，就转换为跟随者
	if reply.Term > rf.CurrentTerm {
		rf.CurrentTerm = reply.Term
		rf.State = Follower
		rf.VotedFor = -1
		rf.resetElectionTimer()
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

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.ticker()

	return rf
}
