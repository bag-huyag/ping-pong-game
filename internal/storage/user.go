package storage

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserExists   = errors.New("user already exists")
	ErrUserNotFound = errors.New("user not found")
)

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Rating       int
}

type UserStorage struct {
	mu     sync.RWMutex
	users  map[string]*User // key: username
	nextID uint64           // добавили счетчик ID
}

func NewUserStorage() *UserStorage {
	return &UserStorage{
		users:  make(map[string]*User),
		nextID: 1, // начальное значение ID
	}
}

// Добавляем метод генерации ID
func (s *UserStorage) generateID() string {
	id := s.nextID
	s.nextID++
	fmt.Println("Generated ID:", id)
	return fmt.Sprintf("user-%d", id)
}

func (s *UserStorage) CreateUser(username, password string) (*User, error) {
	fmt.Println("CreateUser: start")

	if username == "" || password == "" {
		fmt.Println("CreateUser: empty username or password")
		return nil, errors.New("username and password required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; exists {
		fmt.Println("CreateUser: user already exists")
		return nil, ErrUserExists
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 4) // минимальный cost
	if err != nil {
		fmt.Println("CreateUser: bcrypt error", err)
		return nil, err
	}

	id := s.generateID()

	fmt.Println("CreateUser: generated ID:", id)

	user := &User{
		ID:           id,
		Username:     username,
		PasswordHash: string(hashedPassword),
		Rating:       1000,
	}

	s.users[username] = user
	fmt.Println("CreateUser: user saved")
	return user, nil
}

func (s *UserStorage) GetUser(username string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[username]
	if !exists {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (s *UserStorage) UpdateRating(userID string, delta int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, user := range s.users {
		if user.ID == userID {
			user.Rating += delta
			if user.Rating < 0 {
				user.Rating = 0
			}
			return nil
		}
	}
	return ErrUserNotFound
}
