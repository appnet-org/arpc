## Run the Client and Server

Start the server:

```bash
go run kvstore/*.go 
```

In a **separate terminal**, run the client:

```bash
go run frontend/*.go 
```

## 4. Test

```bash
# Set
curl "http://localhost:8080/?operation=set&key=mykey&value=myvalue"

# Get 
curl "http://localhost:8080/?operation=get&key=mykey"
```