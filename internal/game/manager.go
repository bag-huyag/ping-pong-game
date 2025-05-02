package game

import (
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type GameManager struct {
	mu         sync.RWMutex
	games      map[string]*PvEGame // gameID -> game
	pvpManager *PvPManager         // manager for PvP games
}

func (m *GameManager) CreatePvPGame(p1ID, p2ID string, conn1, conn2 *websocket.Conn) *PvPGame {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Создание уникального ID игры
	gameID := uuid.New().String()

	// Создание новой PvP игры
	game := NewPvPGame(gameID, p1ID, p2ID, conn1, conn2)

	game.Start()

	// Сохраняем игру в карту PvP-игр (предполагаем, что GameManager знает о PvPManager)
	if m.pvpManager == nil {
		m.pvpManager = NewPvPManager()
	}
	m.pvpManager.mu.Lock()
	m.pvpManager.games[game.ID] = game
	m.pvpManager.mu.Unlock()

	return game
}

type PvPManager struct {
	mu    sync.RWMutex
	games map[string]*PvPGame
}

func NewGameManager() *GameManager {
	return &GameManager{
		games: make(map[string]*PvEGame),
	}
}

func (m *GameManager) CreatePvEGame(playerID string) *PvEGame {
	m.mu.Lock()
	defer m.mu.Unlock()

	game := NewPvEGame(playerID)
	m.games[game.ID] = game
	return game
}

func (m *GameManager) GetGame(gameID string) (*PvEGame, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	game, exists := m.games[gameID]
	return game, exists
}

func (m *PvPManager) GetGame(gameID string) (*PvPGame, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	game, exists := m.games[gameID]
	return game, exists
}

func (m *PvPManager) RemoveGame(gameID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.games, gameID)
}

func (m *GameManager) RemoveGame(gameID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.games, gameID)
}

func NewPvPManager() *PvPManager {
	return &PvPManager{
		games: make(map[string]*PvPGame),
	}
}

func (m *PvPManager) CreatePvPGame(p1ID, p2ID string, conn1, conn2 *websocket.Conn) *PvPGame {
	m.mu.Lock()
	defer m.mu.Unlock()

	game := NewPvPGame(uuid.New().String(), p1ID, p2ID, conn1, conn2)

	game.Start()

	m.games[game.ID] = game
	return game
}
