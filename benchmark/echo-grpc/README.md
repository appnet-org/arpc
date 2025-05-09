# gRPC echo server

This is a simple Echo server built using Go and gRPC.

## Build Application
`go build -o server server.go`
.,,,,,,,,,,`go build -o frontend frontend.go`

## Build Application and Push to Dockerhub
`bash build_images.sh`  (Remember to run `docker login` and change your username)


## Test
`curl "http://localhost:8080?key=hello&header=1"`

## wrk
`./wrk -d 20s -t 1 -c 1 http://10.96.88.88:80 -s wrk.lua`