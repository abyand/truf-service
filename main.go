package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Card struct {
	ID       int
	Suit     string
	Value    string
	SuitNum  int // 0 => "♥", 1 => "♦", 2 => "♣", 3 => "♠"
	ValueNum int // 2-10 => "2"-"10", 11 => "J", 12 => "Q", 13 => "K", 14 => "A"
}

type Client struct {
	Conn        net.Conn
	Username    string
	Ready       bool
	Hand        []Card
	BiddingCard Card
}

type Room struct {
	id         string
	Clients    []Client
	State      string
	InnerState string
	Round      int
	MaxBid     Card
	Truf       string
}

var (
	rooms = make(map[string]*Room)
	mutex = &sync.Mutex{}
)

var data struct {
	Username string `json:"username"`
	SocketID string `json:"socketId"`
}

var socketCommand struct {
	SocketID string `json:"socketId"`
	Command  string `json:"command"`
	MetaData string `json:"metadata"`
}

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
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		payload := scanner.Text()
		json.Unmarshal([]byte(payload), &data)
		username := data.Username
		socketID := data.SocketID
		client := Client{Conn: conn, Username: username, Ready: false}
		mutex.Lock()
		if rooms[socketID] == nil {
			rooms[socketID] = &Room{State: "pregame", id: socketID}
		}
		if len(rooms[socketID].Clients) >= 4 {
			mutex.Unlock()
			conn.Write([]byte("Sorry, the maximum number of players in a game has been reached.\n"))
			return
		}
		rooms[socketID].Clients = append(rooms[socketID].Clients, client)
		mutex.Unlock()
		conn.Write([]byte("Welcome to room " + socketID + ", " + username + "!\n"))
		go handleConnection(client.Username, socketID)
	}
}

func handleConnection(username string, socketID string) {
	client, err := findClient(username, socketID)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(client.Conn)
	for scanner.Scan() {
		mutex.Lock()
		payload := scanner.Text()
		json.Unmarshal([]byte(payload), &socketCommand)
		command := socketCommand.Command
		socketID := socketCommand.SocketID
		metadata := socketCommand.MetaData
		socketRoom := rooms[socketID]

		switch socketRoom.State {
		case "pregame":
			switch command {
			case "/ready":
				// Find the client in the room, to update their ready status
				for i, roomClient := range socketRoom.Clients {
					if roomClient.Username == client.Username {
						socketRoom.Clients[i].Ready = true
						broadCastToClient("Yeay, you're ready!\n", socketID, client)
						broadCastToOthers(roomClient.Username+" is ready\n", socketID, client)
					}
				}
				if canStartTheGame(socketID) {
					startTheGame(socketRoom, client)
				}
			case "/check":
				checkPlayers(client, socketID)
			case "/chat":
				fullMessage := fmt.Sprintf("%s: %s", client.Username, metadata)
				fmt.Printf("Received message on socket ID %s: %s\n", socketID, fullMessage)
				broadCastToOthers(fullMessage, socketID, client)
			case "/help":
				broadCastToClient(
					"Enter \n'/ready' to signify readiness \n'/check' to check the status of the other players \n'/chat' followed by your message to chat \n'/help' to see these instructions again.\n",
					socketID,
					client)
			default:
				fullMessage := fmt.Sprintf("Unrecognized socketCommand: '%s'. For list of commands type /help", scanner.Text())
				broadCastToClient(fullMessage, socketID, client)
			}
		case "ingame":
			switch socketRoom.InnerState {
			case "bid":
				cardId, err := strconv.Atoi(metadata)
				if err != nil {
					fmt.Println("Could not convert string to int")
					broadCastToClient("Could not convert string to int: "+err.Error(), socketID, client)
				} else {
					card, err := getCardByID(client, cardId)
					if err != nil {
						broadCastToClient("error: "+err.Error(), socketID, client)
					} else {
						client.BiddingCard = card
						client.Ready = true
						broadCastToClient("successfully bid card "+getCardFormat(card), socketID, client)
						broadCastToOthers(client.Username+" has submit bid", socketID, client)

						if allBid(socketRoom) {

						}
					}
				}
			case "play":
				broadCastToAll("play", socketID)
			case "score":
				broadCastToAll("score", socketID)
			}
			broadCastToAll("ingame", socketID)
		case "postgame":
			broadCastToAll("postgame", socketID)
		}

		mutex.Unlock()
	}
	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading from connection:", err)
	}
	mutex.Lock()
	rooms[socketID].Clients = removeConnection(rooms[socketID].Clients, client)
	mutex.Unlock()
	client.Conn.Close()
}

func findClient(username string, id string) (Client, error) {
	for i := range rooms[id].Clients {
		if rooms[id].Clients[i].Username == username {
			return rooms[id].Clients[i], nil
		}
	}
	return Client{}, fmt.Errorf("client with username %d not found", username)
}

func allBid(room *Room) bool {
	for i := range room.Clients {
		if !room.Clients[i].Ready {
			return false
		}
	}
	return true
}

func startTheGame(socketRoom *Room, client Client) {
	socketRoom.State = "ingame"
	socketRoom.Round = 1
	dealCards(socketRoom.Clients)
	for i := range socketRoom.Clients {
		socketRoom.Clients[i].Ready = false
		cardsData, _ := json.Marshal(socketRoom.Clients[i].Hand)
		msg := fmt.Sprintf(
			"All players are ready, game has started. Here are your cards: %s\n",
			string(cardsData),
		)
		for j, otherClient := range socketRoom.Clients {
			if j != i {
				msg += fmt.Sprintf("Total cards %s has: %d\n", otherClient.Username, len(otherClient.Hand))
			}
		}
		broadCastToClient(msg, socketRoom.id, socketRoom.Clients[i])
	}
	socketRoom.InnerState = "bid"
}

func canStartTheGame(id string) bool {
	allReady := true
	for _, roomClient := range rooms[id].Clients {
		if !roomClient.Ready {
			allReady = false
		}
	}
	return allReady
}

func checkPlayers(requester Client, socketID string) {
	var response string
	response += "Checking status of members in the socket...\n"
	for _, client := range rooms[socketID].Clients {
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

func removeConnection(slice []Client, client Client) []Client {
	for i, v := range slice {
		if v.Conn == client.Conn {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func createDeck() []Card {
	suits := []string{"♥", "♦", "♣", "♠"}
	values := []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
	valueNums := []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
	var deck []Card

	id := 0
	for i, suit := range suits {
		for j, value := range values {
			deck = append(deck, Card{ID: id, Suit: suit, Value: value, SuitNum: i, ValueNum: valueNums[j]})
			id++
		}
	}
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	return deck
}

func dealCards(clients []Client) []Card {
	deck := createDeck()
	for i := range clients {
		clients[i].Hand = deck[:13] // Deal 13 cards to each player
		sort.Slice(clients[i].Hand, func(j, k int) bool {
			if clients[i].Hand[j].SuitNum == clients[i].Hand[k].SuitNum {
				return clients[i].Hand[j].ValueNum < clients[i].Hand[k].ValueNum
			}
			return clients[i].Hand[j].SuitNum < clients[i].Hand[k].SuitNum
		}) // Sort the cards
		if !validateHand(clients[i].Hand) { // Validate the hand
			fmt.Println("Invalid hand, dealing again...")
			// If the hand is invalid, deal again.
			// Note: This can potentially lead to an infinite loop if the deck can't produce a valid hand.
			return dealCards(clients)
		}
		deck = deck[13:] // Remove the dealt cards from the deck
	}
	return deck // Return remaining deck
}

func validateHand(hand []Card) bool {
	suitCount := make(map[int]bool)
	hasCardBiggerThan10 := false
	for _, card := range hand {
		suitCount[card.SuitNum] = true
		if card.ValueNum > 10 {
			hasCardBiggerThan10 = true
		}
	}
	return len(suitCount) == 4 && hasCardBiggerThan10
}

func broadCastToAll(message string, socketID string) {
	for _, client := range rooms[socketID].Clients {
		_, err := client.Conn.Write([]byte(message + "\n"))
		if err != nil {
			fmt.Println("Error writing to connection:", err)
		}
	}
}

func broadCastToOthers(message string, socketID string, sender Client) {
	for _, client := range rooms[socketID].Clients {
		if client.Conn != sender.Conn {
			_, err := client.Conn.Write([]byte(message + "\n"))
			if err != nil {
				fmt.Println("Error writing to connection:", err)
			}
		}
	}
}

func broadCastToClient(message string, socketID string, sender Client) {
	_, err := sender.Conn.Write([]byte(message + "\n"))
	if err != nil {
		fmt.Println("Error writing to connection:", err)
	}
}

func getCardByID(client Client, cardID int) (Card, error) {
	for _, card := range client.Hand {
		if card.ID == cardID {
			return card, nil
		}
	}
	return Card{}, fmt.Errorf("card with ID %d not found", cardID)
}

func getCardFormat(card Card) string {
	return card.Value + card.Suit
}
