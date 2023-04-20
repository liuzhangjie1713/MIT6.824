package mr

import (
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Coordinator struct {
	// Your definitions here.
	Files             []string   // 输入文件
	MapTasks          []Task     // mapTask类数组
	ReduceTasks       []Task     // reduceTask类数组
	//IntermediateFiles [][]string // 映射数组，保存中间文件
	Phase             Phase      // 记录master的状态
	NReduce           int        // 记录每个map任务划分多少份
	End               bool       // 标志master是否结束任务
	Mutex             sync.Mutex // 锁
}

type Task struct {
	TaskType  TaskType  // 任务类型，map/reduce
	Id        int       // 任务id，唯一标志任务，并且与中间文件对应
	Filenames []string  // 任务中对应的文件名
	State     State     // 任务状态，记录任务初始化，运行中，已完成等状态
	NReduce   int       // 一个文件map后产生多少个renduce文件
	Time      time.Time // 每个任务设定时长，超时重启
}

// 任务类型
type TaskType int

const (
	MapTask TaskType = iota
	ReduceTask
	WaitingTask // Waitting任务代表此时为任务都分发完了，但是任务还没完成，阶段未改变
	ExitTask    // exit
)

// 任务状态
type State int

const (
	Working State = iota // 此阶段在工作
	Waiting              // 此阶段在等待执行
	Done
)

// 任务阶段
type Phase int

const (
	MapPhase    Phase = iota // 此阶段在分发MapTask
	ReducePhase              // 此阶段在分发ReduceTask
	AllDone                  // 此阶段已完成
)

// Your code here -- RPC handlers for the worker to call.

// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {
	ret := false

	// Your code here.
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	if c.Phase == AllDone {
		//log.Printf("All tasks are finished,the coordinator will be exit! !")
		ret = true
	}
	return ret
}

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{
		Files:   files,
		NReduce: nReduce,
		End:     false,
	}

	c.makeMapTasks()

	c.server()

	go c.CrashDetector()

	return &c
}

// 初始化map任务
func (c *Coordinator) makeMapTasks() {
	c.Phase = MapPhase
	for index, filename := range c.Files {
		maptask := Task{
			TaskType: MapTask,
			Id:       index,
			State:    Waiting,
			NReduce:  c.NReduce,
		}
		maptask.Filenames = append(maptask.Filenames, filename) 
		c.MapTasks = append(c.MapTasks,  maptask)
		//log.Printf("make a map task : %v", maptask)
	}
}

// 初始化reduce任务
func (c *Coordinator) makeReduceTasks() {
	for i := 0; i < c.NReduce; i++ {
		reducetask := Task{
			TaskType:  ReduceTask,
			Id:        i,
			State:     Waiting,
			Filenames: selectReduceFileNames(i),
		}
		c.ReduceTasks = append(c.ReduceTasks,  reducetask)
		//log.Printf("make a reduce task : %v", reducetask)
	}
}

func selectReduceFileNames(reduceNum int) []string {
	var reduceFileNames []string
	path, _ := os.Getwd()
	files, _ := ioutil.ReadDir(path)
	for _, file := range files {
		// 匹配对应的reduce文件
		if strings.HasPrefix(file.Name(), "mr-tmp") && strings.HasSuffix(file.Name(), strconv.Itoa(reduceNum)) {
			reduceFileNames = append(reduceFileNames, file.Name())
		}
	}
	return reduceFileNames
}

// 分发任务
func (c *Coordinator) PollTask(request *GetTaskRequest, response *GetTaskResponse) error {
	// 分发任务应该上锁，防止多个worker竞争，并用defer回退解锁
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	switch c.Phase {
	case MapPhase:
		{
			for index, task := range c.MapTasks {
				if task.State == Waiting {
					c.MapTasks[index].State = Working
					c.MapTasks[index].Time = time.Now()
					response.Task = c.MapTasks[index]
					//log.Printf("Maptask[%d] begin working", task.Id)
					return nil
				}
			}

			response.Task.TaskType = WaitingTask
			for _, task := range c.MapTasks {
				if task.State != Done {
					return nil
				}
			}
			c.toNextPhase()
			return nil
		}
	case ReducePhase:
		{
			for index, task := range c.ReduceTasks {
				if task.State == Waiting {
					c.ReduceTasks[index].State = Working
					c.ReduceTasks[index].Time = time.Now()
					response.Task = c.ReduceTasks[index]
					//log.Printf("Reducetask[%d] begin working", task.Id)
					return nil
				}
			}
			response.Task.TaskType = WaitingTask
			for _, task := range c.ReduceTasks {
				if task.State != Done {
					return nil
				}
			}
			c.toNextPhase()
			return nil
		}
	case AllDone:
		{
			response.Task.TaskType = ExitTask
		}
	default:
		{
			panic("The phase undefined!")
		}
	}
	return nil
}

// 任务阶段转换
func (c *Coordinator) toNextPhase() {
	if c.Phase == MapPhase {
		c.makeReduceTasks()
		c.Phase = ReducePhase
	}else if c.Phase == ReducePhase {
		c.Phase = AllDone
	}
}

// 标记任务已完成
func (c *Coordinator) MarkFinished(request *CompleteTaskRequest, response *CompleteTaskResponse) error {
	// 标记任务应该上锁，防止多个worker竞争，并用defer回退解锁
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	task := request.Task
	switch task.TaskType {
	case MapTask:
		{
			//prevent a duplicated work which returned from another worker
			if task.State == Working {
				//task.State = Done
				c.MapTasks[task.Id].State = Done
				//log.Printf("Maptask[%d] is finished.\n", task.Id)
			} else {
				//log.Printf("Maptask[%d] is finished already.\n", task.Id)
			}
		}
	case ReduceTask:
		{
			if task.State == Working {
				//task.State = Done
				c.ReduceTasks[task.Id].State = Done
				//log.Printf("Reducetask[%d] is finished.\n", task.Id)
			} else {
				//log.Printf("Reducetask[%d] is finished already.\n", task.Id)
			}
		}
	default:
		{
			panic("The task type undefined!")
		}
	}

	return nil
}

func (c *Coordinator) CrashDetector() {
	for {
		time.Sleep(time.Second * 2)
		c.Mutex.Lock()
		if c.Phase == AllDone {
			c.Mutex.Unlock()
			break
		}
		for index, task := range c.MapTasks {
			if task.State == Working && time.Since(task.Time) > 9*time.Second {
				//log.Printf("Maptask[ %d ] is crash,take [%d] s\n", task.Id, time.Since(task.Time))
				c.MapTasks[index].State = Waiting
			}
		}

		for index, task := range c.ReduceTasks {
			if task.State == Working && time.Since(task.Time) > 9*time.Second {
				//log.Printf("Reducetask[ %d ] is crash,take [%d] s\n", task.Id, time.Since(task.Time))
				c.ReduceTasks[index].State = Waiting
			}
		}
		c.Mutex.Unlock()
	}
}
