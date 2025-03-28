package server

import (
	"context"
)

type serverKey struct{}

func wait(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	w, ok := ctx.Value("wait").(bool)
	if !ok {
		return false
	}

	return w
}

// FromContext FromContext
func FromContext(ctx context.Context) (Server, bool) {
	c, ok := ctx.Value(serverKey{}).(Server)
	return c, ok
}

// NewContext new content
func NewContext(ctx context.Context, s Server) context.Context {
	return context.WithValue(ctx, serverKey{}, s)
}
