## Run the Client and Server

Start the server:

```bash
go run server/*.go 
```

In a **separate terminal**, run the client:

```bash
go run frontend/*.go 
```

## 4. Test

```bash
curl http://localhost:8080?key=hello

# or 
curl http://10.96.88.88:80?key=hello
```