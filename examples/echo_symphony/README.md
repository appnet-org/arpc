## Run the client and server

Start the server:

```bash
go run server/server.go 
```

In a **separate terminal**, run the client:

```bash
go run frontend/frontend.go frontend/metrics.go
```

## 4. Test

```bash
curl http://localhost:8080?key=hello
```