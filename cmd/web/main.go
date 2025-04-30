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
			conn.WriteJSON(map[string]string{"type": "login_ok", "token": resp.Token})

		// TODO: добавим start_pve, move, pvp позже
		default:
			conn.WriteJSON(map[string]string{"type": "error", "message": "unknown type"})
		}
	}
}
