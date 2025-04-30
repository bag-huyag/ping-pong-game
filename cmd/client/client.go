package main

import (
	"context"
	"fmt"
	"log"
	"time"

	gamev1 "ping-pong-game/api/v1"

	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("could not connect: %v", err)
	}
	defer conn.Close()

	client := gamev1.NewGameServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// === Регистрация ===
	registerReq := &gamev1.RegisterRequest{
		Username: "testuser1",
		Password: "pass123",
	}

	fmt.Println("➡️ Calling Register...")

	registerResp, err := client.Register(ctx, registerReq)
	if err != nil {
		log.Fatalf("❌ Register failed: %v", err)
	}

	fmt.Printf("✅ Register success! UserID: %s\n", registerResp.UserId)

	// === Логин ===
	loginReq := &gamev1.LoginRequest{
		Username: "testuser1",
		Password: "pass123",
	}

	fmt.Println("➡️ Calling Login...")

	loginResp, err := client.Login(ctx, loginReq)
	if err != nil {
		log.Fatalf("❌ Login failed: %v", err)
	}

	fmt.Printf("✅ Login success! Token: %s\n", loginResp.Token)
}
