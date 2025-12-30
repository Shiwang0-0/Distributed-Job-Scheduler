### commands to run
- open terminal in the root folder run `PORT=8081 go run cmd/scheduler/main.go`  
- similary for PORT=8082 and 8083 in other terminals  
- additionaly to start the API gateway run `go run cmd/gateway/main.go`  
- send a request to APIGateway run  
    ```
        curl -X POST http://localhost:8080/api/jobs \
        -H "Content-Type: application/json" \
        -d '{
            "payload": "test job"
        }'
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
    │ - Polls   │    │ - Polls   │    │ - Polls   │
    │   DB      │    │   DB      │    │   DB      │
    │ - Executes│    │ - Executes│    │ - Executes│
    │   jobs    │    │   jobs    │    │   jobs    │
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
```