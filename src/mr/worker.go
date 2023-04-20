package mr

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"

	//"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"sort"
	"strconv"
	"time"
)

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}


//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.
	flag := true
	for flag {
		task := GetTask().Task;
		switch task.TaskType {
		case MapTask: 
			{
				DoMapTask(mapf, task);
				callDone(&CompleteTaskRequest{Task: task})
			}

		case ReduceTask: 
			{
				DoReduceTask(reducef, task);
				callDone(&CompleteTaskRequest{Task: task})
			}

		case WaitingTask:
			{
				time.Sleep(2 * time.Second)
				//log.Println("All tasks are in progress, please wait...")
			}

		case ExitTask:
			{
				//log.Println("Task  is terminated...")
				flag = false
			}
		}
		
	}
	

	// uncomment to send the Example RPC to the coordinator.
	// CallExample()

}

//
// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
//


// GetTask 获取任务（需要知道是Map任务，还是Reduce）
func GetTask() GetTaskResponse {
	request := GetTaskRequest{}
	response := GetTaskResponse{}
	ok := call("Coordinator.PollTask", &request, &response)

	if ok {
		//fmt.Println(response)
	} else {
		log.Printf("call failed!\n")
	}
	return response
}




func DoMapTask(mapf func(string, string) []KeyValue, task Task) {
	//log.Printf("map task working: %v\n" , task)
	var intermediate []KeyValue
	fileName := task.Filenames[0]
	file, err := os.Open(fileName)
	if err != nil {
		log.Fatalf("can't open %v\n", fileName)
	}

	content, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("can't read %v\n", fileName)
	}
	file.Close()
	intermediate = mapf(fileName, string(content))

	nReduce := task.NReduce
	//hashKv := make([][]KeyValue, nReduce)
	var hashKv [][] KeyValue
	for i := 0; i < nReduce; i++ {
		hashKv = append(hashKv, []KeyValue{})
	}
	for _, kv := range intermediate {
		hashKv[ihash(kv.Key)%nReduce] = append(hashKv[ihash(kv.Key)%nReduce], kv)
	}

	for i:=0; i < nReduce; i++ {
		intermediateFileName := "mr-tmp-" + strconv.Itoa(task.Id) + "-" + strconv.Itoa(i)
		intermediateFile, err:= os.Create(intermediateFileName)
		if err != nil {
			log.Fatalf("cant't create %v\n", intermediateFileName)
		}
		enc := json.NewEncoder(intermediateFile)
		for _, kv := range hashKv[i] {
			err := enc.Encode(&kv)
			if(err != nil){
				return
			}
		}
		intermediateFile.Close()
	}
}

func DoReduceTask(reducef func(string, []string) string, task Task) {
	//log.Printf("reduce task working: %v\n" , task)
	intermediate := shuffle(task.Filenames)
	id := task.Id
	dir, _ := os.Getwd()
	tempFile, err := ioutil.TempFile(dir, "mr-tmp-*")
	//tempFile, err := os.Create("mr-out-tmp")
	if err != nil {
		log.Fatal("Can't create temp file", err)
	}
	i := 0
	for i < len(intermediate) {
		j := i + 1
		for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
			j++
		}
		values :=[]string{}
		for k := i; k < j; k++ {
			values = append(values, intermediate[k].Value)
		}
		output := reducef(intermediate[i].Key, values)
		fmt.Fprintf(tempFile, "%v %v\n", intermediate[i].Key, output)
		i = j
	}
	tempFile.Close()
	fn := fmt.Sprintf("mr-out-%d", id)
	os.Rename(tempFile.Name(), fn)
}

// 洗牌方法，得到一组排序好的kv数组
func shuffle(files []string) []KeyValue {
	kva := []KeyValue{}
	for _, fileName := range files{
		file, err := os.Open(fileName)
		if err != nil {
			log.Fatalf("can't open %v\n", fileName)
		}
		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err:=dec.Decode(&kv);err!=nil{
			  break
			}
			kva = append(kva, kv)
		}
		file.Close()
	}
	sort.Sort(ByKey(kva))
	return kva
}

//Call RPC to mark the task as completed
func callDone(request *CompleteTaskRequest) CompleteTaskResponse{
	response := CompleteTaskResponse{}
	ok := call("Coordinator.MarkFinished", &request, &response)

	if ok {
		//fmt.Println(response)
	} else {
		log.Printf("call failed!\n")
	}
	return response
}




func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

//
// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
