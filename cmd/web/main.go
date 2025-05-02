package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	gamev1 "ping-pong-game/api/v1"
	"ping-pong-game/internal/game"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// PvP глобальные переменные
var (
	pvpManager    = game.NewPvPManager()
	waitingPlayer string
	waitingConn   *websocket.Conn

	connMutexes = make(map[*websocket.Conn]*sync.Mutex)
	mutexLock   sync.Mutex
)

// // Функция для безопасной отправки сообщений через WebSocket
// func safeWriteJSON(conn *websocket.Conn, message interface{}) error {
// 	// Получаем или создаем мьютекс для этого соединения
// 	mutexLock.Lock()
// 	mutex, exists := connMutexes[conn]
// 	if !exists {
// 		mutex = &sync.Mutex{}
// 		connMutexes[conn] = mutex
// 	}
// 	mutexLock.Unlock()

// 	// Блокируем доступ к соединению на время записи
// 	mutex.Lock()
// 	defer mutex.Unlock()
// 	return conn.WriteJSON(message)
// }

func main() {
	http.HandleFunc("/ws", handleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("./web")))

	fmt.Println("WebSocket server started on :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	playerID := uuid.New().String()
	var currentGame *game.PvPGame
	var token, gameID string

	// Подключаемся к gRPC
	grpcConn, err := grpc.Dial("localhost:8080", grpc.WithInsecure())
	if err != nil {
		log.Println("gRPC connect error:", err)
		return
	}
	defer grpcConn.Close()

	client := gamev1.NewGameServiceClient(grpcConn)

	// Добавляем мьютекс для защиты соединения
	mutexLock.Lock()
	connMutexes[conn] = &sync.Mutex{}
	mutexLock.Unlock()
	defer func() {
		mutexLock.Lock()
		delete(connMutexes, conn)
		mutexLock.Unlock()
	}()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			if currentGame != nil {
				currentGame.Update()

				// отправляем каждому игроку его версию состояния
				state1 := currentGame.ToProto(currentGame.Player1.ID)
				state2 := currentGame.ToProto(currentGame.Player2.ID)

				// Безопасно отправляем сообщения игрокам
				if currentGame.Player1 != nil && currentGame.Player1.Conn != nil {
					currentGame.Player1.SendJSON(map[string]interface{}{
						"type":         "game_state",
						"game_id":      state1.GameId,
						"ball_x":       state1.BallX,
						"ball_y":       state1.BallY,
						"paddle_y":     state1.PaddleY,
						"ai_paddle_y":  state1.AiPaddleY,
						"player_score": state1.PlayerScore,
						"ai_score":     state1.AiScore,
					})
				}

				if currentGame.Player2 != nil && currentGame.Player2.Conn != nil {
					currentGame.Player2.SendJSON(map[string]interface{}{
						"type":         "game_state",
						"game_id":      state2.GameId,
						"ball_x":       state2.BallX,
						"ball_y":       state2.BallY,
						"paddle_y":     state2.PaddleY,
						"ai_paddle_y":  state2.AiPaddleY,
						"player_score": state2.PlayerScore,
						"ai_score":     state2.AiScore,
					})
				}
			}
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}

		var req map[string]interface{}
		if err := json.Unmarshal(msg, &req); err != nil {
			log.Println("Invalid JSON:", err)
			continue
		}

		switch req["type"] {
		case "register":
			username := req["username"].(string)
			password := req["password"].(string)

			resp, err := client.Register(context.Background(), &gamev1.RegisterRequest{
				Username: username,
				Password: password,
			})
			if err != nil {
				conn.WriteJSON(map[string]string{"type": "error", "message": err.Error()})
				continue
			}
			conn.WriteJSON(map[string]string{"type": "register_ok", "user_id": resp.UserId})

		case "login":
			username := req["username"].(string)
			password := req["password"].(string)

			resp, err := client.Login(context.Background(), &gamev1.LoginRequest{
				Username: username,
				Password: password,
			})
			if err != nil {
				conn.WriteJSON(map[string]string{"type": "error", "message": err.Error()})
				continue
			}
			token = resp.Token
			conn.WriteJSON(map[string]string{"type": "login_ok", "token": resp.Token})

		case "start_pve":
			if token == "" {
				conn.WriteJSON(map[string]string{"type": "error", "message": "Not authenticated"})
				continue
			}

			md := metadata.New(map[string]string{"authorization": "Bearer " + token})
			ctx := metadata.NewOutgoingContext(context.Background(), md)

			stream, err := client.StartPVEGame(ctx, &gamev1.StartPVEGameRequest{})
			if err != nil {
				conn.WriteJSON(map[string]string{"type": "error", "message": err.Error()})
				continue
			}

			firstState, err := stream.Recv()
			if err != nil {
				log.Println("Start stream error:", err)
				conn.WriteJSON(map[string]string{"type": "error", "message": err.Error()})
				continue
			}

			gameID = firstState.GameId

			conn.WriteJSON(map[string]interface{}{
				"type":         "game_state",
				"ball_x":       firstState.BallX,
				"ball_y":       firstState.BallY,
				"player_score": firstState.PlayerScore,
				"ai_score":     firstState.AiScore,
				"paddle_y":     firstState.PaddleY,
				"ai_paddle_y":  firstState.AiPaddleY,
			})

			go func() {
				for {
					state, err := stream.Recv()
					if err != nil {
						log.Println("Stream error:", err)
						conn.WriteJSON(map[string]string{"type": "error", "message": "Game ended"})
						return
					}

					conn.WriteJSON(map[string]interface{}{
						"type":         "game_state",
						"ball_x":       state.BallX,
						"ball_y":       state.BallY,
						"player_score": state.PlayerScore,
						"ai_score":     state.AiScore,
						"paddle_y":     state.PaddleY,
						"ai_paddle_y":  state.AiPaddleY,
					})
				}
			}()

		case "start_pvp":
			log.Println("PvP request from player:", playerID)

			if waitingPlayer == "" {
				waitingPlayer = playerID
				waitingConn = conn
				log.Println("Player added to waiting queue:", playerID)

				conn.WriteJSON(map[string]string{
					"type": "waiting",
				})
			} else {
				log.Println("Starting PvP game between", waitingPlayer, "and", playerID)

				// Создаем уникальный ID для игры
				// gameInstanceID := uuid.New().String()

				gameInstance := pvpManager.CreatePvPGame(waitingPlayer, playerID, waitingConn, conn)
				currentGame = gameInstance

				log.Printf("PvP game created with ID: %s", gameInstance.ID)

				// Подготавливаем начальное состояние для обоих игроков
				state1 := gameInstance.ToProto(waitingPlayer)
				state2 := gameInstance.ToProto(playerID)

				startMsg1 := map[string]interface{}{
					"type":         "game_start",
					"game_id":      state1.GameId,
					"ball_x":       state1.BallX,
					"ball_y":       state1.BallY,
					"paddle_y":     state1.PaddleY,
					"ai_paddle_y":  state1.AiPaddleY,
					"player_score": state1.PlayerScore,
					"ai_score":     state1.AiScore,
				}

				startMsg2 := map[string]interface{}{
					"type":         "game_start",
					"game_id":      state2.GameId,
					"ball_x":       state2.BallX,
					"ball_y":       state2.BallY,
					"paddle_y":     state2.PaddleY,
					"ai_paddle_y":  state2.AiPaddleY,
					"player_score": state2.PlayerScore,
					"ai_score":     state2.AiScore,
				}

				// Отправляем начальное состояние игрокам
				if err := gameInstance.Player1.SendJSON(startMsg1); err != nil {
					log.Println("Error sending start message to player 1:", err)
				}

				if err := gameInstance.Player2.SendJSON(startMsg2); err != nil {
					log.Println("Error sending start message to player 2:", err)
				}

				// Сбрасываем очередь ожидания
				waitingPlayer = ""
				waitingConn = nil
			}

		case "move":
			// Обработка движения для PvP
			if req["game_id"] != nil {
				gameIDStr, ok := req["game_id"].(string)
				if !ok {
					log.Println("Invalid game_id format")
					continue
				}

				directionF, ok := req["direction"].(float64)
				if !ok {
					log.Println("Invalid direction format")
					continue
				}

				direction := int(directionF)

				log.Printf("PvP move: player=%s, game=%s, direction=%d", playerID, gameIDStr, direction)

				gameInstance, ok := pvpManager.GetGame(gameIDStr)
				if !ok {
					log.Printf("Game not found: %s", gameIDStr)
					continue
				}

				gameInstance.MovePaddle(playerID, direction)
			} else if token != "" && gameID != "" {
				// PVE
				actionStr, ok := req["action"].(string)
				if !ok {
					conn.WriteJSON(map[string]string{"type": "error", "message": "Invalid action"})
					continue
				}

				var action gamev1.PlayerActionRequest_Action
				switch actionStr {
				case "UP":
					action = gamev1.PlayerActionRequest_UP
				case "DOWN":
					action = gamev1.PlayerActionRequest_DOWN
				default:
					action = gamev1.PlayerActionRequest_NONE
				}

				ctx := metadata.NewOutgoingContext(context.Background(), metadata.New(map[string]string{
					"authorization": "Bearer " + token,
				}))

				_, err := client.PlayerAction(ctx, &gamev1.PlayerActionRequest{
					GameId: gameID,
					Action: action,
				})
				if err != nil {
					log.Println("Move error:", err)
				}
			}

		default:
			conn.WriteJSON(map[string]string{"type": "error", "message": "unknown type"})
		}
	}
}
