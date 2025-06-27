package main

import (
	"bytes"
	"fmt"

	echo "github.com/appnet-org/arpc/examples/echo_symphony/symphony"
)

func main() {
	original := &echo.EchoRequest{
		Id:       42,
		Score:    100,
		Username: "alice",
		Content:  "helloworld",
	}

	// Marshal the message
	data, err := original.MarshalSymphony()
	if err != nil {
		fmt.Println("Marshal failed:", err)
		return
	}
	fmt.Println("Marshaled data:", data)

	// Unmarshal into a new object
	var decoded echo.EchoRequest
	err = decoded.UnmarshalSymphony(data)
	if err != nil {
		fmt.Println("Unmarshal failed:", err)
		return
	}

	// Print both objects
	fmt.Println("Original:", original)
	fmt.Println("Decoded: ", &decoded)

	// Simple field comparisons
	if original.Id != decoded.Id {
		fmt.Println("Id mismatch")
	}
	if original.Score != decoded.Score {
		fmt.Println("Score mismatch")
	}
	if original.Username != decoded.Username {
		fmt.Println("Username mismatch")
	}
	if original.Content != decoded.Content {
		fmt.Println("Content mismatch")
	}

	// Check marshal determinism
	data2, err := decoded.MarshalSymphony()
	if err != nil {
		fmt.Println("Second marshal failed:", err)
		return
	}
	if !bytes.Equal(data, data2) {
		fmt.Println("Second marshal does not match first — not deterministic!")
	}
}
