package userctx

import (
	"context"

	"github.com/gin-gonic/gin"
)

type User struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Jwt    string `json:"-"`
	Cookie string `json:"-"`
}

type contextKey struct{}

func WithContext(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, contextKey{}, user)
}

func FromContext(ctx context.Context) (User, bool) {
	if ctx == nil {
		return User{}, false
	}
	user, ok := ctx.Value(contextKey{}).(User)
	return user, ok
}

func MustFromContext(c *gin.Context) User {
	if c == nil || c.Request == nil {
		panic("user context is unavailable")
	}
	user, ok := FromContext(c.Request.Context())
	if !ok {
		panic("authenticated user is unavailable")
	}
	return user
}
