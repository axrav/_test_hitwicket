package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// Game represents the game state
type Game struct {
	Board         [5][5]*Character
	Players       [2]*Player
	CurrentPlayer int
	GameOver      bool
	Winner        int
}

// Player represents a player in the game
type Player struct {
	ID         int
	Characters []*Character
}

// Character represents a game piece
type Character struct {
	Type  string
	Name  string
	X     int
	Y     int
	Owner int
}

// Move represents a move command
type Move struct {
	CharacterName string `json:"character_name"`
	Direction     string `json:"direction"`
}

// GameState represents the current state of the game
type GameState struct {
	Board         [5][5]*Character `json:"board"`
	CurrentPlayer int              `json:"current_player"`
	GameOver      bool             `json:"game_over"`
	Winner        int              `json:"winner"`
}

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
	game    Game
	clients = make(map[*websocket.Conn]int)
)

func main() {
	http.HandleFunc("/ws", handleConnections)

	initGame()

	log.Println("Server starting on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	// Assign player to the game
	playerID := len(clients)
	if playerID >= 2 {
		log.Println("Game is full")
		return
	}
	clients[ws] = playerID

	// Send initial game state
	sendGameState(ws)

	for {
		var move Move
		err := ws.ReadJSON(&move)
		if err != nil {
			log.Printf("error: %v", err)
			delete(clients, ws)
			break
		}

		if game.CurrentPlayer == playerID && !game.GameOver {
			processMove(move, playerID)
			broadcastGameState()
		}
	}
}

func processMove(move Move, playerID int) {
	character := findCharacter(move.CharacterName, playerID)
	if character == nil {
		log.Printf("Invalid character: %s", move.CharacterName)
		return
	}

	if !isValidMove(character, move.Direction) {
		log.Printf("Invalid move: %s %s", move.CharacterName, move.Direction)
		return
	}

	moveCharacter(character, move.Direction)
	game.CurrentPlayer = (game.CurrentPlayer + 1) % 2

	if checkGameOver() {
		game.GameOver = true
		game.Winner = playerID
	}
}

func findCharacter(name string, playerID int) *Character {
	for _, char := range game.Players[playerID].Characters {
		if char.Name == name {
			return char
		}
	}
	return nil
}

func isValidMove(character *Character, direction string) bool {
	newX, newY := calculateNewPosition(character, direction)

	// Check if the move is within bounds
	if newX < 0 || newX >= 5 || newY < 0 || newY >= 5 {
		return false
	}

	// Check if the move is valid for the character type
	switch character.Type {
	case "Pawn":
		return isPawnMoveValid(direction)
	case "Hero1":
		return isHero1MoveValid(character, direction, newX, newY)
	case "Hero2":
		return isHero2MoveValid(direction)
	}

	return false
}

func isPawnMoveValid(direction string) bool {
	return direction == "L" || direction == "R" || direction == "F" || direction == "B"
}

func isHero1MoveValid(character *Character, direction string, newX, newY int) bool {
	if direction != "L" && direction != "R" && direction != "F" && direction != "B" {
		return false
	}

	// Check if there's a friendly character in the path
	midX, midY := (character.X+newX)/2, (character.Y+newY)/2
	if game.Board[midY][midX] != nil && game.Board[midY][midX].Owner == character.Owner {
		return false
	}

	return true
}

func isHero2MoveValid(direction string) bool {
	return direction == "FL" || direction == "FR" || direction == "BL" || direction == "BR"
}

func calculateNewPosition(character *Character, direction string) (int, int) {
	x, y := character.X, character.Y

	switch character.Type {
	case "Pawn":
		switch direction {
		case "L":
			x--
		case "R":
			x++
		case "F":
			y--
		case "B":
			y++
		}
	case "Hero1":
		switch direction {
		case "L":
			x -= 2
		case "R":
			x += 2
		case "F":
			y -= 2
		case "B":
			y += 2
		}
	case "Hero2":
		switch direction {
		case "FL":
			x--
			y -= 2
		case "FR":
			x++
			y -= 2
		case "BL":
			x--
			y += 2
		case "BR":
			x++
			y += 2
		}
	}

	return x, y
}

func moveCharacter(character *Character, direction string) {
	newX, newY := calculateNewPosition(character, direction)

	// Remove character from old position
	game.Board[character.Y][character.X] = nil

	// Handle character elimination
	if game.Board[newY][newX] != nil && game.Board[newY][newX].Owner != character.Owner {
		eliminateCharacter(game.Board[newY][newX])
	}

	// Update character position
	character.X, character.Y = newX, newY
	game.Board[newY][newX] = character

	// Handle Hero1 and Hero2 path elimination
	if character.Type == "Hero1" || character.Type == "Hero2" {
		midX, midY := (character.X+newX)/2, (character.Y+newY)/2
		if game.Board[midY][midX] != nil && game.Board[midY][midX].Owner != character.Owner {
			eliminateCharacter(game.Board[midY][midX])
			game.Board[midY][midX] = nil
		}
	}
}

func eliminateCharacter(character *Character) {
	player := game.Players[character.Owner]
	for i, char := range player.Characters {
		if char == character {
			player.Characters = append(player.Characters[:i], player.Characters[i+1:]...)
			break
		}
	}
}

func checkGameOver() bool {
	for _, player := range game.Players {
		if len(player.Characters) == 0 {
			return true
		}
	}
	return false
}

func broadcastGameState() {
	for client := range clients {
		sendGameState(client)
	}
}

func sendGameState(client *websocket.Conn) {
	state := GameState{
		Board:         game.Board,
		CurrentPlayer: game.CurrentPlayer,
		GameOver:      game.GameOver,
		Winner:        game.Winner,
	}
	err := client.WriteJSON(state)
	if err != nil {
		log.Printf("error: %v", err)
		client.Close()
		delete(clients, client)
	}
}

func initGame() {
	game = Game{
		Board:         [5][5]*Character{},
		Players:       [2]*Player{},
		CurrentPlayer: 0,
		GameOver:      false,
	}

	// Initialize players
	for i := 0; i < 2; i++ {
		game.Players[i] = &Player{
			ID:         i,
			Characters: make([]*Character, 0),
		}
	}

	// Set up initial board state (example setup)
	setupCharacters := []string{"Pawn", "Hero1", "Pawn", "Hero2", "Pawn"}
	for i, charType := range setupCharacters {
		for playerID := 0; playerID < 2; playerID++ {
			y := 0
			if playerID == 1 {
				y = 4
			}
			char := &Character{
				Type:  charType,
				Name:  fmt.Sprintf("%s%d", charType[:1], i+1),
				X:     i,
				Y:     y,
				Owner: playerID,
			}
			game.Players[playerID].Characters = append(game.Players[playerID].Characters, char)
			game.Board[y][i] = char
		}
	}
}

// Helper functions for move validation, character movement, etc.
