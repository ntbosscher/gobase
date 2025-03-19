package modelutil

import (
	"context"
	"net/http"
)

type cacheKeyType string

var cacheKey cacheKeyType = "modelutil-cache"

func CacheMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req = req.WithContext(context.WithValue(req.Context(), cacheKey, &outerCache{
			inner: map[string]interface{}{},
		}))

		next.ServeHTTP(w, req)
	})
}

type outerCache struct {
	inner map[string]interface{}
}

type cacheInner[T any] struct {
	cache map[int]T
}

func (t *cacheInner[T]) Get(key2 int) T {
	if t.cache == nil {
		var defaultT T
		return defaultT
	}

	return t.cache[key2]
}

func (t *cacheInner[T]) Set(key2 int, value T) {
	if t.cache == nil {
		t.cache = map[int]T{}
	}

	t.cache[key2] = value
}

type Cache[T any] struct {
	key string
}

func NewCache[T any](key string) *Cache[T] {
	return &Cache[T]{
		key: key,
	}
}

func (c *Cache[T]) Set(ctx context.Context, key2 int, value T) {

	outer, ok := ctx.Value(cacheKey).(*outerCache)
	if !ok {
		return
	}

	inner, ok := outer.inner[c.key].(*cacheInner[T])
	if !ok {
		inner = &cacheInner[T]{}
		outer.inner[c.key] = inner
	}

	inner.Set(key2, value)
}

func (c *Cache[T]) Clear(ctx context.Context, key2 int) {
	var defaultT T
	c.Set(ctx, key2, defaultT)
}

func (c *Cache[T]) Get(ctx context.Context, key2 int) T {
	var defaultT T

	outer, ok := ctx.Value(cacheKey).(*outerCache)
	if !ok {
		return defaultT
	}

	inner, ok := outer.inner[c.key].(*cacheInner[T])
	if !ok {
		return defaultT
	}

	return inner.Get(key2)
}
