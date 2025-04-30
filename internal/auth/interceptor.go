package auth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func AuthInterceptor(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "metadata missing")
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, "authorization token missing")
	}

	token := strings.TrimPrefix(authHeaders[0], "Bearer ")
	claims, err := ValidateToken(token)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}

	return context.WithValue(ctx, "user_id", claims.UserID), nil
}

func UnaryInterceptor() grpc.UnaryServerInterceptor {
	skipAuth := map[string]bool{
		"/pingpong.v1.GameService/Register": true,
		"/pingpong.v1.GameService/Login":    true,
	}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if skipAuth[info.FullMethod] {
			return handler(ctx, req)
		}

		newCtx, err := AuthInterceptor(ctx)
		if err != nil {
			return nil, err
		}
		return handler(newCtx, req)
	}
}
