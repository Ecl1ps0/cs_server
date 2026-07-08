package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	agones "agones.dev/agones/sdks/go"
)

func waitForTCP(host string, port int) {
	address := net.JoinHostPort(host, strconv.Itoa(port))

	for {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			fmt.Printf("CS2 is accepting TCP on %s\n", address)
			return
		}

		fmt.Printf("Waiting for CS2 TCP %s: %v\n", address, err)
		time.Sleep(5 * time.Second)
	}
}

func main() {
	gamePort := 27015
	if value := os.Getenv("GAME_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			gamePort = parsed
		}
	}

	sdk, err := agones.NewSDK()
	if err != nil {
		panic(fmt.Errorf("could not create Agones SDK: %w", err))
	}

	// Start health loop immediately so Agones does not mark server unhealthy
	// while CS2 is still downloading/updating/starting.
	go func() {
		for {
			if err := sdk.Health(); err != nil {
				fmt.Printf("Agones health failed: %v\n", err)
			}
			time.Sleep(2 * time.Second)
		}
	}()

	fmt.Println("Waiting for CS2 to open TCP port...")
	waitForTCP("127.0.0.1", gamePort)

	fmt.Println("Marking GameServer as Ready...")
	if err := sdk.Ready(); err != nil {
		panic(fmt.Errorf("could not mark GameServer ready: %w", err))
	}

	select {}
}
