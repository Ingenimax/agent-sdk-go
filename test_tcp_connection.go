package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

func main() {
	fmt.Println("=== Testing Basic TCP Connection ===")
	
	// Test if we can establish basic TCP connections on this system
	
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	
	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Printf("TCP server listening on port %d\n", port)
	
	// Start a simple TCP server
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			fmt.Println("TCP connection accepted")
			conn.Close()
		}
	}()
	
	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)
	
	// Test TCP connection
	fmt.Println("Testing TCP connection...")
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Fatalf("Failed to connect via TCP: %v", err)
	}
	
	fmt.Println("✅ TCP connection successful")
	conn.Close()
	listener.Close()
	
	fmt.Println("TCP test completed successfully")
}