package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	echo "github.com/appnet-org/golib/sample/echo-pb"
)

func handler(writer http.ResponseWriter, request *http.Request) {
	requestBody := request.URL.Query().Get("key")
	numHeadersStr := request.URL.Query().Get("header")
	numHeaders, err := strconv.Atoi(numHeadersStr)
	if err != nil {
		log.Printf("Invalid header count: %s, defaulting to 0", numHeadersStr)
		numHeaders = 0
	}
	fmt.Printf("Frontend got request with key: %s !\n", requestBody)

	var conn *grpc.ClientConn

	conn, err = grpc.NewClient(
		"server:9000",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("could not connect: %s", err)
	}
	defer conn.Close()

	c := echo.NewEchoServiceClient(conn)

	// Create and attach metadata with the custom header
	md := metadata.New(map[string]string{
		"key": requestBody, // Here we're setting the custom header "key" to the requestBody
	})

	for i := 0; i < numHeaders; i++ {
		headerName := "appnet-header-" + strconv.FormatUint(uint64(rand.Intn(10000)), 10)
		headerValue := "appnet-value-" + strconv.FormatUint(uint64(rand.Intn(10000)), 10)
		md.Set(headerName, headerValue)
	}

	ctx := metadata.NewOutgoingContext(context.Background(), md)

	message := echo.Msg{
		Body: requestBody,
	}

	var header metadata.MD

	// Make sure to pass the context (ctx) which includes the metadata
	response, err := c.Echo(ctx, &message, grpc.Header(&header))

	if err != nil {
		fmt.Fprintf(writer, "Echo server returns an error: %s\n", err)
		log.Printf("Error when calling echo: %s", err)
	} else {
		fmt.Fprintf(writer, "Response from server: %s\n", response.Body)
		log.Printf("Response from server: %s", response.Body)

		// Print the response headers (metadata)
		log.Println("Response headers:")
		for key, values := range header {
			log.Printf("  %s: %v", key, values)
		}
	}
}

func main() {
	http.HandleFunc("/", handler)

	fmt.Printf("Starting frontend pod at port 8080\n")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
