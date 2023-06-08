# MIT6.824
MIT6.824学习记录
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


TODO:
- [x] Lab2B优化：加速日志回溯
- [x] Lab2优化：锁,go test -race 通过 
- [ ] Lab2C稳定性优化：目前200次测试, 两次报错：one failed to reach agreement
