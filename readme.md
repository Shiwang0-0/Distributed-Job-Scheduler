### commands to run
- open terminal in the root folder run `PORT=8081 go run cmd/scheduler/main.go`  
- similary for PORT=8082 and 8083 in other terminals  
- additionaly to start the API gateway run `go run cmd/gateway/main.go`  
- also start the Watcher by running `PORT=9001 go run cmd/watcher/main.go`

- send a request to APIGateway run  
    ```
        curl -X POST http://localhost:8080/api/jobs \
        -H "Content-Type: application/json" \
        -d '{
            "payload": "test job"
        }'
    ```
- get jobs  (gateway takes this request)
    ```  
        curl "http://localhost:8080/jobs"   
        curl "http://localhost:8080/jobs?status=pending"  
        curl "http://localhost:8080/job?job_id=69541618f7d4e1295bfa447b"  
    ```
- get watcher stats
    ```
        curl "http://localhost:9001/stats"
    ```
- get queued jobs (by watcher)
    ```
        curl "http://localhost:9001/queue"
    ```

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
                  ┌─────────────────┐
                  │    MONGODB      │
                  │                 │
                  │ - jobs          │
                  │ - job_executions│
                  └─────────────────┘
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
          │                 │                  │
          └─────────────────┼──────────────────┘
                            │
                            ↓ Each adds jobs they claimed
                      ┌───────────┐
                      │   Queue   │
                      └───────────┘
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

```