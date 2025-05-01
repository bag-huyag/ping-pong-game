package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	gamev1 "ping-pong-game/api/v1"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // временно разрешаем все источники
	},
}

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

	// Подключаемся к gRPC
	grpcConn, err := grpc.Dial("localhost:8080", grpc.WithInsecure())
	if err != nil {
		log.Println("gRPC connect error:", err)
		return
	}
	defer grpcConn.Close()

	client := gamev1.NewGameServiceClient(grpcConn)

	var token string
	// var userID string
	var gameID string

	for {
		// Читаем входящее сообщение от клиента
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}

		// Парсим сообщение и обрабатываем
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

			// Сохраняем gameID из первого состояния
			firstState, err := stream.Recv()
			if err != nil {
				log.Println("Start stream error:", err)
				conn.WriteJSON(map[string]string{"type": "error", "message": err.Error()})
				continue
			}

			gameID = firstState.GameId // <-- добавь поле GameId в GameState proto!

			conn.WriteJSON(map[string]interface{}{
				"type":         "game_state",
				"ball_x":       firstState.BallX,
				"ball_y":       firstState.BallY,
				"player_score": firstState.PlayerScore,
				"ai_score":     firstState.AiScore,
				"paddle_y":     firstState.PaddleY,
				"ai_paddle_y":  firstState.AiPaddleY,
			})

			// Запускаем горутину, чтобы слушать обновления от сервера
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

		case "move":
			if token == "" || gameID == "" {
				conn.WriteJSON(map[string]string{"type": "error", "message": "Game not started"})
				continue
			}

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

		default:
			conn.WriteJSON(map[string]string{"type": "error", "message": "unknown type"})
		}
	}
}
