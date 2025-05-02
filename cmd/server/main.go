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

var pvpQueue = make(chan gamev1.GameService_StartPVPGameServer, 1)

func (s *server) StartPVPGame(req *gamev1.StartPVPGameRequest, stream gamev1.GameService_StartPVPGameServer) error {
	userID, ok := stream.Context().Value("user_id").(string)
	if !ok {
		log.Println("StartPVPGame: user_id not found in context")
		return status.Error(codes.Unauthenticated, "invalid user")
	}
	log.Printf("StartPVPGame: called by user %s", userID)

	// Если в очереди уже есть игрок — запускаем игру
	select {
	case opponentStream := <-pvpQueue:
		opponentCtx := opponentStream.Context()
		opponentID, ok := opponentCtx.Value("user_id").(string)
		if !ok {
			log.Println("StartPVPGame: opponent user_id not found")
			return status.Error(codes.Internal, "opponent invalid")
		}

		// Создаём игру
		game := s.gameManager.CreatePvPGame(userID, opponentID, nil, nil)
		log.Printf("StartPVPGame: PvP game started between %s and %s (ID: %s)", userID, opponentID, game.ID)

		// Запускаем игровой цикл для обоих
		go s.runPVPGameLoop(game, stream, opponentStream, userID, opponentID)
		return nil

	default:
		// Если очереди нет — ставим игрока в очередь
		log.Printf("StartPVPGame: user %s waiting for opponent", userID)
		pvpQueue <- stream
		<-stream.Context().Done() // игрок ушел
		log.Printf("StartPVPGame: user %s disconnected while waiting", userID)
		return nil
	}
}

func (s *server) runPVPGameLoop(game *game.PvPGame, stream1, stream2 gamev1.GameService_StartPVPGameServer, player1ID, player2ID string) {
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stream1.Context().Done():
			s.gameManager.RemoveGame(game.ID)
			return
		case <-stream2.Context().Done():
			s.gameManager.RemoveGame(game.ID)
			return
		case <-ticker.C:
			game.Update()

			state1 := game.ToProto(player1ID)
			state2 := game.ToProto(player2ID)

			if err := stream1.Send(state1); err != nil {
				log.Println("Stream1 send error:", err)
				s.gameManager.RemoveGame(game.ID)
				return
			}
			if err := stream2.Send(state2); err != nil {
				log.Println("Stream2 send error:", err)
				s.gameManager.RemoveGame(game.ID)
				return
			}
		}
	}
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
