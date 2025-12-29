### commands to run
- open terminal in the root folder run `PORT=8081 go run cmd/scheduler/main.go`  
- similary for PORT=8082 and 8083 in other terminals  
- additionaly to start the API gateway run `go run cmd/gateway/main.go`  
- send a request to APIGateway run  
    ```
        curl -X POST http://localhost:8080/api/jobs \
        -H "Content-Type: application/json" \
        -d '{
            "message": "test job"
        }'
    ```
