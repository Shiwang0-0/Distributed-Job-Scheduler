### commands to run

##### using tmux
in the root folder type `bash start_tmux.sh`


##### manual
- open terminal in the root folder run `go run cmd/scheduler/main.go --port=8081`  
- similary for PORT=8082 and 8083 in other terminals  
- start the API gateway by running `go run cmd/gateway/main.go --port=8000 --schedulers=8081,8082,8083`  
- start the Exchange by running `go run cmd/exchange/main.go --port=2001 --normal=7001 --high=7101`

- start the Watcher by running `go run cmd/watcher/main.go --port=9001 --exchanges=2001,2002`
- start the Queue Service by running 
    - `go run cmd/queue_service/main.go --port=7001 --type=normal --index=0`
    - `go run cmd/queue_service/main.go --port=7101 --type=high   --index=0`
- start the Coordinator by running `go run cmd/coordinator/main.go --port=3000 --normal=7001 --high=7101`
- start the Worker by running `go run cmd/worker/main.go --port=10001 --coordinator=3000 --concurrency=5`

#### API Gateway

The API Gateway is the external entry point for clients.
It load-balances requests across multiple schedulers.

- create job (once / cron)
    ```
        curl -X POST http://localhost:8080/api/jobs \
        -H "Content-Type: application/json" \
        -d '{
            "type": "once",
            "payload": "high priority one time",
            "priority":"normal"
        }'
    ```

    ```
        Writing Cron Expression
        Format:     "minute hour"
        "*/5 *"     Every 5 minutes
        "0 *"       Every hour at :00
        "0 9"       9:00 AM daily
        "30 14"     2:30 PM daily
        "0 */2"     Every 2 hours at :00 (0:00, 2:00, 4:00, ...)
        "* 9"       Every minute from 9:00 AM to 9:59 AM

        curl -X POST http://localhost:8080/api/jobs \
        -H "Content-Type: application/json" \
        -d '{
            "type": "cron",
            "cron_expr": "*/2 *",
            "payload": "run every 2 minutes"
        }'
        
        curl -X POST http://localhost:8080/api/jobs \
        -H "Content-Type: application/json" \
        -d '{
            "type": "once",
            "payload": "high priority one time",
            "priority":"high"
        }'
    ```
- get all jobs
    ```
    curl "http://localhost:8000/jobs"
    ```
- get jobs by status
    ```
    curl "http://localhost:8000/jobs?status=pending"
    ```
- get job by id
    ```
    curl "http://localhost:8000/job?job_id=69541618f7d4e1295bfa447b"
    ```
---

### Scheduler
Schedulers persist jobs, handle cron expansion, and participate in leader election.
Only the leader schedules cron jobs.
- scheduler health check
    ```
    curl "http://localhost:8081/health"
    ```
- schedule job (direct scheduler call, usually via gateway)
    ```
    curl -X POST "http://localhost:8081/schedule" \
         -H "Content-Type: application/json" \
         -d '{
              "type": "cron",
              "cron_expr": "*/1 *",
              "payload": "run every minute"
         }'
    ```
- get all jobs from scheduler
    ```
    curl "http://localhost:8081/jobs"
    ```
- get jobs by status
    ```
    curl "http://localhost:8081/jobs?status=active"
    ```
- get job by id
    ```
    curl "http://localhost:8081/job?job_id=69541618f7d4e1295bfa447b"
    ```
---

#### Coordinator (Worker–Queue Assignment)
The coordinator manages worker-to-queue assignments and liveness.
- request queue assignment (worker → coordinator)
    ```
    curl -X POST "http://localhost:3000/coordinator/request-assignment"
    ```
- worker heartbeat
    ```
    curl -X POST "http://localhost:3000/coordinator/heartbeat"
    ```
- release assignment (worker shutdown / rebalance)
    ```
    curl -X POST "http://localhost:3000/coordinator/release-assignment"
    ```
- coordinator stats
    ```
    curl "http://localhost:3000/coordinator/stats"
    ```
---

#### Exchange
Exchanges route jobs to appropriate queues (normal / high priority).
- exchange stats
    ```
    curl "http://localhost:2001/exchange/stats"
    ```

- get jobs  (gateway takes this request)
    ```  
        curl "http://localhost:PORT/jobs"   
        curl "http://localhost:PORT/jobs?status=pending"  
        curl "http://localhost:PORT/job?job_id=69541618f7d4e1295bfa447b"  
    ```
- get watcher stats
    ```
        curl "http://localhost:PORT/stats"
    ```
---

#### Queue Service (Normal & High Priority)
Queues store jobs temporarily and provide leasing semantics.
- push job into queue
    ```
    curl -X POST "http://localhost:7001/queue/push" \
         -H "Content-Type: application/json" \
         -d '{ ...job payload... }'
    ```
- lease a job (worker pulls job)
    ```
    curl -X POST "http://localhost:7001/queue/lease"
    ```
- release lease (job failed / retry)
    ```
    curl -X POST "http://localhost:7001/queue/release-lease"
    ```
- delete job (job completed)
    ```
    curl -X POST "http://localhost:7001/queue/delete"
    ```
- peek queue (non-destructive)
    ```
    curl "http://localhost:7001/queue/peek"
    ```
- list all jobs in queue (debug)
    ```
    curl "http://localhost:7001/queue/all"
    ```
- queue stats
    ```
    curl "http://localhost:7001/queue/stats"
    ```
(Repeat on all queue ports)

---

#### Watcher (DB → Exchange Bridge)
Watchers poll MongoDB for pending jobs and push them into exchanges.
- watcher stats
    ```
    curl "http://localhost:9001/watcher/stats"
    ```
---


### Worker (Job Executor)
Workers pull jobs from queues and execute them.
- worker stats
    ```
    curl "http://localhost:10001/worker/stats"
    ```
---

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        CLIENT                                │
└────────────────────────────┬────────────────────────────────┘
                             │
                             │ HTTP POST /api/jobs
                             ↓
┌─────────────────────────────────────────────────────────────┐
│                     API GATEWAY (Port 8080)                  │
│  - Receives job requests                                     │
│  - Passes to Load Balancer                                   │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ↓
┌─────────────────────────────────────────────────────────────┐
│                      LOAD BALANCER                           │
│  - Has list of scheduler URLs                                │
│  - Checks their health                                       │
│  - Picks next healthy one (round-robin)                      │
│  - Returns the selected scheduler                            │
└────────────────────────────┬────────────────────────────────┘
                             │
                             │ Selects one:
                             ├─→ http://localhost:8081  OR
                             ├─→ http://localhost:8082  OR
                             └─→ http://localhost:8083
                             │
            ┌────────────────┼────────────────┐
            │                │                │
            ↓                ↓                ↓
    ┌───────────┐    ┌───────────┐    ┌───────────┐
    │SCHEDULER 1│    │SCHEDULER 2│    │SCHEDULER 3│
    │ Port 8081 │    │ Port 8082 │    │ Port 8083 │
    │           │    │           │    │           │     LEADER ELECTION RUNNING IN SEPERATE GO ROUTINE
    │  LEADER   │    │           │    │           │                 (To Schedule CRON JOB)
    │           │    │           │    │           │
    │ - Writes  │    │ - Writes  │    │ - Writes  │                 
    │   to DB   │    │   to DB   │    │   to DB   │
    │ - Get     │    │ - Get     │    │ - Get     │
    │   Jobs    │    │   Jobs    │    │   Jobs    │
    │   from DB │    │   from DB │    │   from DB │
    └─────┬─────┘    └─────┬─────┘    └─────┬─────┘
          │                │                │
          └────────────────┼────────────────┘
                           │
                           │ All connect to same DB
                           ↓
                 ┌────────────────────────┐
                 │    MONGODB             │
                 │ - jobs(cron/once)      │
                 │ - job_executions (all) │
                 │ - completed_jobs       |
                 │ - failed_jobs          |
                 └────────────────────────┘
                            │
                            │  All watchers poll concurrently
                            │  (with jitter + atomic FindOneAndUpdate)
                            ↓
          ┌─────────────────┼───────────────────┐
          │                 │                   │
   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
   │   Watcher 1  │  │   Watcher 2  │  │   Watcher 3  │
   │              │  │              │  │              │
   │   claimed    │  │   failed     │  │   failed     │
   │    job       │  │              │  │              │
   └──────────────┘  └──────────────┘  └──────────────┘
          │                 │                   │
          └─────┼─────────────────────┼─────────┘
                │                     │
                │  Push to exchanges  │
                ▼                     ▼
         ┌──────────────┐      ┌──────────────┐
         │ Exchange 1   │      │ Exchange 2   │
         │ (port 2001)  │      │ (port 2002)  │
         └──────┬───────┘      └──────┬───────┘
                │                     │
                │  Fixed % N routing  │     (Route: Hash with job_type + retry_count)
                │                     │   
                ▼                     ▼
            ┌──────────────────────────────┐
            │  Fixed Queue List            │ (if queue failed, the waiting time will eventually expire)
            │  • Normal Priority           │ (Watcher will again pick it from the DB, retry count changes)
            │  • High Priority             │ (Visibility timeout / lease-based)
            └──────────────────────────────┘
                            │
                            |
                            |
                            |
                            ↓
        ┌───────────────────────────────────────────┐
        │      COORDINATOR (Port 3000)              │
        │                                           │
        │  - Worker-to-Queue Assignment             │
        |  - Persists Assignments to DB             |
        │  - Heartbeat Tracking (10s interval)      │
        └───────────────────────────────────────────┘
                            |
                            |
                            | Assignment & Heartbeat
                            |
                            | PULL (LEASE on job for some duration)
                            ↓ 
          ┌─────────────────┼───────────────────┐
    ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
    │   Worker 1   │  │   Worker 2   │  │   Worker 3   │
    │              │  │              │  │              │ (each worker concurreny<=5)
    │   execute    │  │              │  │              │ (pull with jitter)
    │    job       │  │              │  │              │ (exponential retries ==> (RetryCount^2 * 10) seconds)
    └──────────────┘  └──────────────┘  └──────────────┘
          │                 │                  │
          └─────────────────┼──────────────────┘
                            |
                            ↓
                     ┌──────────────┐
                     │   MongoDB    │
                     │              │
                     │ - jobs       │
                     │ - executions │
                     │ - completed  │
                     │ - failed     │
                     └──────────────┘
```

```
    Client ----GET /jobs?status=pending----> Gateway
                                                |
                                                | Forward to a healthy scheduler
                                                v
                                    Scheduler.HandleGetJobs
                                                |
                                                | Query MongoDB jobs collection
                                                v
                                    Return JSON jobs list
                                                ^
                                                |
    Client <------ Gateway returns JSON <-------- Scheduler

--------------------------------------------------------------------------------------------------

    Multiple Watchers (All Active)
        ↓
    Poll DB every 2 seconds
        ↓
    Find jobs: status="pending" AND scheduled_at <= now
        ↓
    ALL watchers try to claim SAME jobs
        ↓
    MongoDB atomic update (only ONE succeeds)
        ↓
    Winner adds job to priority queue
        ↓
    Losers see "already claimed" and move on

--------------------------------------------------------------------------------------------------

    Worker claims job with a LEASE (timeout)
        ↓
    Worker processes job
        ↓
    Case 1: Worker succeeds
        └─ Acknowledge job → Remove from queue
        └─ Write job successful in DB
        
    Case 2: Worker fails/crashes
        └─ Lease expires → Job becomes available again
        └─ Write job pending in DB
        
    Case 3: After N retries (1+N) approach
        └─ write job failed (Dead) in DB 

```
---

### Why this Architecture ? (clarifications and tradeoffs)

- API Gateway
    - A Simple API Gateway that lets Client send HTTP requests

- Load Balancer
    - A simple Load balancer that works on round robin order and distributes the Job to the Schedulers

- Scheduler
    - For Normal Jobs, Scheduler dumps the incoming request to the DB
    - In case of Cron Jobs, Scheduler for the first time dumps the Cron Job in the DB and later by initializing a Leader Election, </br> Leader picks the Cron Job and Create a Normal Job to be executed at the Moment
    - This Creation of new job helps in unqiuely determining every Cron Job independently (each have unique ObjectId)
    - For Cron, a seperate go routine for the Leader Election is present that only interacts with the crons </br> and does not bother the normal job flow of the scheduler

    - Leader election ONLY for cron rule evaluation
    - NO leader election for normal job processing

    - Why DB level locking for Normal Jobs but Leader Election for Cron ?
        - cron jobs because cron scheduling is a time-coordination problem, not a data-claiming problem
        - When the first time Cron is writtern in the DB, it is never meant to be executed </br> it is there only to let the schedulers know that there is some cron job defination (whose instance will be created based on some specified time)
        - Since the Schedulers dont have the ```JobId``` (because there is no job yet) and it is a time based decision in normal multiple scheduler case, </br> multiple watchers might tend to create multiple jobs based on the same job description present in the DB
        - Another case could be that for multiple schedulers time could differ, one might execute an early job because with its respect the time to execute was reached </br> while it is still early running, other scheduler comes and runs it again at the correct time at which it was supposed to run , this will create multiple execution.

        - Leader election gives a single time owner, which simplifies correctness and prevents missed or duplicate executions

- Watchers
    - Watchers Watches the Normal One Time Jobs (both included the actual once and the once create by the Cron)

    - Multiple Watchers + DB level locking (why not leader election)
        - In case of multiple watchers if one watcher dies others can still continue doing the task
        - There will be no need to wait for currTime + IntervalPeriod to select the new leader
        - Watchers are suppose to be dump and disposable
        - With multiple watchers the throughput will be high whereas a single leader might be a bottleneck
        - Multiple watchers dont need coordination
        - In the worst case where the write has already been done but the response was not received, Idempotency at the DB layer will save duplicate reads

- Queue
    - A simple Producer Consumer Based Queue

    - Failing of Queue
        - If somewhere Queue failed, The task marked as "Queued" will be re-polled by the Watcher from the DB
        - This re-polling will ensure that Once the task is writtern to the Db it will ultimately gets executed

- Workers
    - Working Entities that executes the Work
    - Made use of DB level locking (High Execution Rate)
    
    - Failing of a Worker
        - Workers never POP the task from the Queue, they always take a lease on the Job
        - If Worker A is not able to do the task in the given period, an exponential wait is put to it
        - By the time, other workers will try to take the lease and execute them
        - If non of them was able to execute it, the Job is marked as "Failed"




- Locks taken by watchers in case of reading from the DB  
    
    for SQL there are normally 2 ways that you would lock (shared, exclusive) </br>
    the problem with raw locks is that they are time consuming,  
    - Shared locking: (multiple watchers can read same thing ) </br> in case of shared locking if watcher 1 locks the first row the other watcher can also read that row, </br> meaning same task will be executed twice (worse case)
    - Exclusive locking: (both read and write but only to one watcher) </br>
    watcher 1 locks the row 1 (status will be changed from "pending" to "claimed"), </br> watcher 2 comes and sees the row that it is locked (it waits because it doesnt know the status and the row is locked), </br> it wait for its turn but as soon as previous held lock is releases it sees that staus has already changed. (it waited for nothing) </br> this is better than the shared locking, but watcher is waiting at places when locks are held by other watchers which makes the locking slow
    
    making exclusive locking faster </br>
    - SKIP LOCKING: if a row has a lock then skip it, dont wait for the lock to release, continue with the next rows  


    </br>for monogo, FindOneAndUpdate() does atomic document update, so the filtering of task 1 with the new status happens for the watcher 2 immediately </br> (watcher 2 still waits, but for very short period, SQL's SKIP LOCKING overthrows mongo here)