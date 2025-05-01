package auth

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
)

const (
	secretKey     = "your-secret-key" // в production используйте env переменные!
	tokenDuration = 15 * time.Minute
)

type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

func GenerateToken(userID string) (string, error) {
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenDuration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

func ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrSignatureInvalid
}

func StreamInterceptor() grpc.StreamServerInterceptor {
	skipAuth := map[string]bool{
		"/pingpong.v1.GameService/Register": true,
		"/pingpong.v1.GameService/Login":    true,
	}

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if skipAuth[info.FullMethod] {
			return handler(srv, ss)
		}

		newCtx, err := AuthInterceptor(ss.Context())
		if err != nil {
			return err
		}

		wrappedStream := &wrappedStream{ss, newCtx}
		return handler(srv, wrappedStream)
	}
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}
