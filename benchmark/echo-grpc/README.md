# gRPC echo server

This is a simple Echo server built using Go and gRPC.

## Run Application
`go run server/server.go`
`go run frontend/frontend.go`

## Test
`curl "http://localhost:8080?key=hello&header=1"`

## wrk
`./wrk -d 20s -t 1 -c 1 http://localhost:8080 -s wrk.lua`


## Build Application and Push to Dockerhub
`bash build_images.sh`  (Remember to run `docker login` and change your username)
