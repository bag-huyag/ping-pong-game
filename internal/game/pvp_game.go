package game

import (
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	gamev1 "ping-pong-game/api/v1"

	"github.com/gorilla/websocket"
)

type PvPGame struct {
	ID         string
	Player1    *Player
	Player2    *Player
	BallX      float64
	BallY      float64
	BallSpeedX float64
	BallSpeedY float64
	Score1     int
	Score2     int
	Running    bool
	Mutex      sync.Mutex
}

type Player struct {
	ID      string
	PaddleY float64
	Conn    *websocket.Conn // интерфейс, для отправки сообщений (например, через Gorilla WebSocket)
	mu      sync.Mutex
}

type GameState struct {
	GameId      string  `json:"game_id"`
	BallX       float64 `json:"ball_x"`
	BallY       float64 `json:"ball_y"`
	PaddleY     float64 `json:"paddle_y"`
	AiPaddleY   float64 `json:"ai_paddle_y"` // соперник
	PlayerScore int     `json:"player_score"`
	AiScore     int     `json:"ai_score"`
}

func (p *Player) SendJSON(message interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Conn.WriteJSON(message)
}

func NewPvPGame(id, player1ID, player2ID string, conn1, conn2 *websocket.Conn) *PvPGame {
	game := &PvPGame{
		ID: id,
		Player1: &Player{
			ID:      player1ID,
			PaddleY: float64(FieldHeight/2 - PaddleHeight/2),
			Conn:    conn1,
		},
		Player2: &Player{
			ID:      player2ID,
			PaddleY: float64(FieldHeight/2 - PaddleHeight/2),
			Conn:    conn2,
		},
		Running: true,
	}

	// Инициализация мяча
	game.resetBall()

	log.Printf("New PvP game created: %s (Player1: %s, Player2: %s)",
		id, player1ID, player2ID)
	log.Printf("Initial state: Ball=(%.2f,%.2f), P1=%.2f, P2=%.2f",
		game.BallX, game.BallY, game.Player1.PaddleY, game.Player2.PaddleY)

	return game
}

func (g *PvPGame) resetBall() {

	rand.Seed(time.Now().UnixNano())

	g.BallX = float64(FieldWidth / 2)
	g.BallY = float64(FieldHeight / 2)

	angle := (rand.Float64()*math.Pi/2 - math.Pi/4) // -45° до +45°
	if rand.Intn(2) == 0 {
		angle += math.Pi
	}

	g.BallSpeedX = math.Cos(angle) * BallSpeed
	g.BallSpeedY = math.Sin(angle) * BallSpeed
}

func (g *PvPGame) Update() {
	g.Mutex.Lock()
	defer g.Mutex.Unlock()

	if !g.Running {
		return
	}

	g.BallX += g.BallSpeedX
	g.BallY += g.BallSpeedY

	// Столкновение с верхом/низом
	if g.BallY < 0 || g.BallY > FieldHeight-BallSize {
		g.BallSpeedY *= -1
		g.BallY = math.Max(0, math.Min(g.BallY, float64(FieldHeight-BallSize)))
	}

	// Столкновение с ракеткой Player1 (слева)
	if g.BallX <= PaddleWidth &&
		g.BallY+BallSize >= g.Player1.PaddleY &&
		g.BallY <= g.Player1.PaddleY+PaddleHeight {
		g.BallSpeedX = math.Abs(g.BallSpeedX)
		hitPos := (g.BallY - g.Player1.PaddleY) / PaddleHeight
		g.BallSpeedY = (hitPos - 0.5) * 2 * BallSpeed
	}

	// Столкновение с ракеткой Player2 (справа)
	if g.BallX >= FieldWidth-PaddleWidth-BallSize &&
		g.BallY+BallSize >= g.Player2.PaddleY &&
		g.BallY <= g.Player2.PaddleY+PaddleHeight {
		g.BallSpeedX = -math.Abs(g.BallSpeedX)
		hitPos := (g.BallY - g.Player2.PaddleY) / PaddleHeight
		g.BallSpeedY = (hitPos - 0.5) * 2 * BallSpeed
	}

	// Проверка голов
	if g.BallX < 0 {
		g.Score2++ // Игрок 2 забил гол
		log.Printf("Player 2 scored! Score: %d-%d", g.Score1, g.Score2)
		g.resetBall()
	} else if g.BallX > FieldWidth-BallSize {
		g.Score1++ // Игрок 1 забил гол
		log.Printf("Player 1 scored! Score: %d-%d", g.Score1, g.Score2)
		g.resetBall()
	}
}

func (g *PvPGame) MovePaddle(playerID string, direction int) {
	g.Mutex.Lock()
	defer g.Mutex.Unlock()

	log.Printf("MovePaddle: player=%s, direction=%d", playerID, direction)

	if playerID == g.Player1.ID {
		if direction == 1 { // UP
			g.Player1.PaddleY = math.Max(0, g.Player1.PaddleY-PaddleSpeed)
			log.Printf("Player1 paddle moved to %f", g.Player1.PaddleY)
		} else if direction == -1 { // DOWN
			g.Player1.PaddleY = math.Min(float64(FieldHeight-PaddleHeight), g.Player1.PaddleY+PaddleSpeed)
			log.Printf("Player1 paddle moved to %f", g.Player1.PaddleY)
		}
	} else if playerID == g.Player2.ID {
		if direction == 1 { // UP
			g.Player2.PaddleY = math.Max(0, g.Player2.PaddleY-PaddleSpeed)
			log.Printf("Player2 paddle moved to %f", g.Player2.PaddleY)
		} else if direction == -1 { // DOWN
			g.Player2.PaddleY = math.Min(float64(FieldHeight-PaddleHeight), g.Player2.PaddleY+PaddleSpeed)
			log.Printf("Player2 paddle moved to %f", g.Player2.PaddleY)
		}
	} else {
		log.Printf("Unknown player ID: %s", playerID)
	}
}

func (g *PvPGame) ToProto(playerID string) *gamev1.GameState {
	var myPaddle, otherPaddle float64
	var myScore, otherScore int32

	if playerID == g.Player1.ID {
		myPaddle = g.Player1.PaddleY
		otherPaddle = g.Player2.PaddleY
		myScore = int32(g.Score1)
		otherScore = int32(g.Score2)
	} else {
		myPaddle = g.Player2.PaddleY
		otherPaddle = g.Player1.PaddleY
		myScore = int32(g.Score2)
		otherScore = int32(g.Score1)
	}

	return &gamev1.GameState{
		GameId:      g.ID,
		BallX:       int32(g.BallX),
		BallY:       int32(g.BallY),
		PaddleY:     int32(myPaddle),
		AiPaddleY:   int32(otherPaddle),
		PlayerScore: myScore,
		AiScore:     otherScore,
	}
}

func (g *PvPGame) Start() {
	log.Printf("Starting PvP game: %s", g.ID)
	g.resetBall() // Убедимся, что мяч инициализирован

	go func() {
		ticker := time.NewTicker(16 * time.Millisecond) // ~60 FPS
		defer ticker.Stop()

		for range ticker.C {
			g.Mutex.Lock()
			if !g.Running {
				g.Mutex.Unlock()
				return
			}
			g.Mutex.Unlock()
		}
	}()
}
