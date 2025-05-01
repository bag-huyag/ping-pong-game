package game

import (
	"math"
	"math/rand"
	"sync"
	"time"

	gamev1 "ping-pong-game/api/v1"

	"github.com/google/uuid"
)

const (
	FieldWidth   = 800
	FieldHeight  = 600
	PaddleWidth  = 20
	PaddleHeight = 100
	BallSize     = 20
	BallSpeed    = 8
	AIReaction   = 0.1 // Скорость реакции ИИ (0-1)
	PaddleSpeed  = 10  // Константа скорости движения
)

type PvEGame struct {
	ID            string
	PlayerID      string
	Mutex         sync.Mutex
	BallX         float64
	BallY         float64
	BallSpeedX    float64
	BallSpeedY    float64
	PlayerPaddleY float64
	AIPaddleY     float64
	PlayerScore   int32
	AIScore       int32
	Running       bool
}

func NewPvEGame(playerID string) *PvEGame {
	rand.Seed(time.Now().UnixNano())

	game := &PvEGame{
		ID:            uuid.New().String(),
		PlayerID:      playerID,
		PlayerPaddleY: float64(FieldHeight/2 - PaddleHeight/2),
		AIPaddleY:     float64(FieldHeight/2 - PaddleHeight/2),
		Running:       true,
	}
	game.resetBall()
	return game
}

func (g *PvEGame) resetBall() {
	g.BallX = float64(FieldWidth / 2)
	g.BallY = float64(FieldHeight / 2)

	angle := (rand.Float64()*math.Pi/2 - math.Pi/4) // -45° до +45°
	if rand.Intn(2) == 0 {
		angle += math.Pi
	}

	g.BallSpeedX = math.Cos(angle) * BallSpeed
	g.BallSpeedY = math.Sin(angle) * BallSpeed
}

func (g *PvEGame) Update() {
	g.Mutex.Lock()
	defer g.Mutex.Unlock()

	if !g.Running {
		return
	}

	g.PlayerPaddleY = math.Max(0, math.Min(g.PlayerPaddleY, float64(FieldHeight-PaddleHeight)))

	// Движение мяча
	g.BallX += g.BallSpeedX
	g.BallY += g.BallSpeedY

	// Столкновение с верхом/низом
	if g.BallY < 0 || g.BallY > FieldHeight-BallSize {
		g.BallSpeedY *= -1
		g.BallY = math.Max(0, math.Min(g.BallY, float64(FieldHeight-BallSize)))
	}

	// Столкновение с ракеткой игрока
	if g.BallX <= PaddleWidth &&
		g.BallY >= g.PlayerPaddleY &&
		g.BallY <= g.PlayerPaddleY+PaddleHeight {
		g.BallSpeedX = math.Abs(g.BallSpeedX)
		// Добавляем угол в зависимости от места удара
		hitPos := (g.BallY - g.PlayerPaddleY) / PaddleHeight
		g.BallSpeedY = (hitPos - 0.5) * 2 * BallSpeed
	}

	// Столкновение с ракеткой AI
	if g.BallX >= FieldWidth-PaddleWidth-BallSize &&
		g.BallY >= g.AIPaddleY &&
		g.BallY <= g.AIPaddleY+PaddleHeight {
		g.BallSpeedX = -math.Abs(g.BallSpeedX)
		hitPos := (g.BallY - g.AIPaddleY) / PaddleHeight
		g.BallSpeedY = (hitPos - 0.5) * 2 * BallSpeed
	}

	// Обновление AI
	targetY := g.BallY - PaddleHeight/2
	g.AIPaddleY += (targetY - g.AIPaddleY) * AIReaction
	g.AIPaddleY = math.Max(0, math.Min(g.AIPaddleY, float64(FieldHeight-PaddleHeight)))

	// Проверка голов
	if g.BallX < 0 {
		g.AIScore++
		g.resetBall()
	} else if g.BallX > FieldWidth-BallSize {
		g.PlayerScore++
		g.resetBall()
	}
}

func (g *PvEGame) ToProto() *gamev1.GameState {
	g.Mutex.Lock()
	defer g.Mutex.Unlock()

	return &gamev1.GameState{
		BallX:       int32(g.BallX),
		BallY:       int32(g.BallY),
		PlayerScore: g.PlayerScore,
		AiScore:     g.AIScore,
		PaddleY:     int32(g.PlayerPaddleY),
		AiPaddleY:   int32(g.AIPaddleY),
		GameId:      g.ID,
	}
}

func (g *PvEGame) MovePlayerPaddle(direction int) {
	g.Mutex.Lock()
	defer g.Mutex.Unlock()

	switch direction {
	case 1: // UP
		g.PlayerPaddleY = math.Max(0, g.PlayerPaddleY-PaddleSpeed)
	case -1: // DOWN
		g.PlayerPaddleY = math.Min(float64(FieldHeight-PaddleHeight), g.PlayerPaddleY+PaddleSpeed)
	}
}
