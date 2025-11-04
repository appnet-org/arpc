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
curl "http://localhost:8080/?op=SET&key=82131353f9ddc8c6&key_size=48&value_size=87"

# Get 
curl "http://localhost:8080/?op=GET&key=82131353f9ddc8c6&key_size=48&value_size=87"

# For Kubernetes:
curl "http://10.96.88.88:80/?op=SET&key=82131353f9ddc8c6&key_size=48&value_size=87"
curl "http://10.96.88.88:80/?op=GET&key=82131353f9ddc8c6&key_size=48&value_size=87"
```