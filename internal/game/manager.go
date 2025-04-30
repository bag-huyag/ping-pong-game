package game

import (
	"sync"
)

type GameManager struct {
	mu    sync.RWMutex
	games map[string]*PvEGame // gameID -> game
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

func (m *GameManager) RemoveGame(gameID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.games, gameID)
}
