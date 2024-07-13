package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
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
	conn.Write([]byte("Enter username: "))
	if !scanner.Scan() {
		fmt.Println("Error reading username")
		return
	}
	username := scanner.Text()
	conn.Write([]byte("Enter socket ID: "))
	if !scanner.Scan() {
		fmt.Println("Error reading socket ID")
		return
	}
	socketID := scanner.Text()
	client := Client{Conn: conn, Username: username, Ready: false}
	mutex.Lock()
	if len(connections[socketID]) >= 4 {
		mutex.Unlock()
		conn.Write([]byte("Sorry, the maximum number of players in a game has been reached.\n"))
		return
	}
	connections[socketID] = append(connections[socketID], client)
	mutex.Unlock()
	fmt.Printf("Client %s connected to socket ID: %s\n", username, socketID)
	conn.Write([]byte("Enter '/ready' to signify readiness, '/check' to check the status of the other players, '/chat' followed by your message to chat, '/help' to see these instructions again.\n"))
	handleConnection(client, socketID)
}

func handleConnection(client Client, socketID string) {
	scanner := bufio.NewScanner(client.Conn)
	for scanner.Scan() {
		message := strings.SplitN(scanner.Text(), " ", 2)
		switch message[0] {
		case "/ready":
			mutex.Lock()
			allReady := true
			for i := range connections[socketID] {
				if connections[socketID][i].Conn == client.Conn {
					connections[socketID][i].Ready = true
					client.Conn.Write([]byte("Yeay, you're ready!\n"))
				}
				if !connections[socketID][i].Ready {
					allReady = false
				}
			}
			if allReady && len(connections[socketID]) == 4 {
				for i := range connections[socketID] {
					connections[socketID][i].Conn.Write([]byte("Yeay all the players are ready, let's get this game started\n"))
				}
			}
			mutex.Unlock()
		case "/check":
			checkPlayers(client, socketID)
		case "/chat":
			if len(message) > 1 {
				fullMessage := fmt.Sprintf("%s: %s", client.Username, message[1])
				fmt.Printf("Received message on socket ID %s: %s\n", socketID, fullMessage)
				broadcastMessage(socketID, client, fullMessage)
			}
		case "/help":
			client.Conn.Write([]byte("Enter '/ready' to signify readiness, '/check' to check the status of the other players, '/chat' followed by your message to chat, '/help' to see these instructions again.\n"))
		default:
			fullMessage := fmt.Sprintf("Unrecognized command: '%s'. For list of commands type /help", scanner.Text())
			client.Conn.Write([]byte(fullMessage + "\n"))
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
