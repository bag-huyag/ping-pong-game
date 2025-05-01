package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"ping-pong-game/internal/auth"
	"ping-pong-game/internal/game"
	"ping-pong-game/internal/storage"
	"syscall"
	"time"

	gamev1 "ping-pong-game/api/v1"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type server struct {
	gamev1.UnimplementedGameServiceServer
	userStorage *storage.UserStorage
	gameManager *game.GameManager
}

func NewServer() *server {
	return &server{
		userStorage: storage.NewUserStorage(),
		gameManager: game.NewGameManager(),
	}
}

// Реализация Register
func (s *server) Register(ctx context.Context, req *gamev1.RegisterRequest) (*gamev1.RegisterResponse, error) {
	fmt.Println("Register called with:", req.Username)

	user, err := s.userStorage.CreateUser(req.Username, req.Password)
	if err != nil {
		fmt.Println("Register failed:", err)
		return nil, status.Errorf(codes.AlreadyExists, "registration failed: %v", err)
	}

	fmt.Println("User created:", user.ID)

	resp := &gamev1.RegisterResponse{UserId: user.ID}
	fmt.Printf("Register: about to return response: %+v\n", resp)

	return resp, nil
}

// Реализация Login
func (s *server) Login(ctx context.Context, req *gamev1.LoginRequest) (*gamev1.LoginResponse, error) {
	fmt.Println("Login called with:", req.Username)

	user, err := s.userStorage.GetUser(req.Username)
	if err != nil {
		fmt.Println("User not found:", err)
		return nil, status.Error(codes.NotFound, "user not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		fmt.Println("Invalid password")
		return nil, status.Error(codes.Unauthenticated, "invalid password")
	}

	token, err := auth.GenerateToken(user.ID)
	if err != nil {
		fmt.Println("Token generation failed:", err)
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	fmt.Println("Login: returning token")
	fmt.Println("Login successful")
	return &gamev1.LoginResponse{Token: token}, nil
}

// Реализация StartPVEGame
func (s *server) StartPVEGame(req *gamev1.StartPVEGameRequest, stream gamev1.GameService_StartPVEGameServer) error {

	// Получаем пользователя из контекста
	userID, ok := stream.Context().Value("user_id").(string)
	if !ok {
		log.Println("StartPVEGame: user_id not found in context")
		return status.Error(codes.Unauthenticated, "invalid user")
	}
	log.Printf("StartPVEGame: starting game for user %s", userID)

	// Создаем новую игру
	game := s.gameManager.CreatePvEGame(userID)

	// Игровой цикл
	ticker := time.NewTicker(16 * time.Millisecond) // ~60 FPS
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			game.Update()

			// Отправляем состояние
			if err := stream.Send(game.ToProto()); err != nil {
				s.gameManager.RemoveGame(game.ID)
				return err
			}

			// Проверка окончания игры
			if game.PlayerScore >= 10 || game.AIScore >= 10 {

				ratingDelta := 20
				if game.AIScore >= 10 {
					ratingDelta = -20
				}

				s.userStorage.UpdateRating(userID, ratingDelta)
				s.gameManager.RemoveGame(game.ID)
				return nil
			}

		case <-stream.Context().Done():
			s.gameManager.RemoveGame(game.ID)
			return nil
		}
	}
}

func (s *server) StartPVPGame(req *gamev1.StartPVPGameRequest, stream gamev1.GameService_StartPVPGameServer) error {
	// TODO: Реализовать PVP логику
	return nil
}

func main() {
	lis, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer(
		grpc.UnaryInterceptor(auth.UnaryInterceptor()),
		grpc.StreamInterceptor(auth.StreamInterceptor()),
	)

	gamev1.RegisterGameServiceServer(s, NewServer())

	reflection.Register(s)

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		s.GracefulStop()
	}()

	fmt.Println("Server started on :8080")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func (s *server) PlayerAction(ctx context.Context, req *gamev1.PlayerActionRequest) (*gamev1.PlayerActionResponse, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "invalid user")
	}

	game, exists := s.gameManager.GetGame(req.GameId)
	if !exists {
		return nil, status.Error(codes.NotFound, "game not found")
	}

	// Проверяем что игрок участвует в этой игре
	if game.PlayerID != userID {
		return nil, status.Error(codes.PermissionDenied, "not your game")
	}

	var direction int
	switch req.Action {
	case gamev1.PlayerActionRequest_UP:
		direction = 1
	case gamev1.PlayerActionRequest_DOWN:
		direction = -1
	default:
		direction = 0
	}

	game.MovePlayerPaddle(direction)

	return &gamev1.PlayerActionResponse{Success: true}, nil
}
