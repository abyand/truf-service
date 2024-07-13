package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
)

type Client struct {
	Conn     net.Conn
	Username string
	Ready    bool
}

var (
	connections = make(map[string][]Client)
	mutex       = &sync.Mutex{}
)

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer listener.Close()
	fmt.Println("Server started on port 8080")
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go handleInitialConnection(conn)
	}
}

func handleInitialConnection(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	// Ask for username
	conn.Write([]byte("Enter username: "))
	if !scanner.Scan() {
		fmt.Println("Error reading username")
		return
	}
	username := scanner.Text()
	// Ask for socket ID
	conn.Write([]byte("Enter socket ID: "))
	if !scanner.Scan() {
		fmt.Println("Error reading socket ID")
		return
	}
	socketID := scanner.Text()
	client := Client{Conn: conn, Username: username, Ready: false}
	mutex.Lock()
	connections[socketID] = append(connections[socketID], client)
	mutex.Unlock()
	fmt.Printf("Client %s connected to socket ID: %s\n", username, socketID)
	conn.Write([]byte("Enter 'ready' to signify readiness or 'check' to check the status of the other players.\n"))
	handleConnection(client, socketID)
}

func handleConnection(client Client, socketID string) {
	scanner := bufio.NewScanner(client.Conn)
	for scanner.Scan() {
		message := scanner.Text()
		switch message {
		case "/ready":
			mutex.Lock()
			for i := range connections[socketID] {
				if connections[socketID][i].Conn == client.Conn {
					connections[socketID][i].Ready = true
					client.Conn.Write([]byte("Yeay, you're ready!\n"))
				}
			}
			mutex.Unlock()
		case "/check":
			checkPlayers(client, socketID)
		default:
			fullMessage := fmt.Sprintf("%s: %s", client.Username, message)
			fmt.Printf("Received message on socket ID %s: %s\n", socketID, fullMessage)
			broadcastMessage(socketID, client, fullMessage)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading from connection:", err)
	}
	mutex.Lock()
	connections[socketID] = removeConnection(connections[socketID], client)
	mutex.Unlock()
	client.Conn.Close()
}

func checkPlayers(requester Client, socketID string) {
	mutex.Lock()
	defer mutex.Unlock()
	var response string
	response += "Checking status of members in the socket...\n"
	for _, client := range connections[socketID] {
		var status string
		if client.Ready {
			status = "is ready"
		} else {
			status = "is not ready yet"
		}
		playerStatus := fmt.Sprintf("%s: %s\n", client.Username, status)
		response += playerStatus
	}
	requester.Conn.Write([]byte(response))
}

func broadcastMessage(socketID string, sender Client, message string) {
	mutex.Lock()
	defer mutex.Unlock()
	for _, client := range connections[socketID] {
		if client.Conn != sender.Conn {
			_, err := client.Conn.Write([]byte(message + "\n"))
			if err != nil {
				fmt.Println("Error writing to connection:", err)
			}
		}
	}
}

func removeConnection(slice []Client, client Client) []Client {
	for i, v := range slice {
		if v.Conn == client.Conn {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
