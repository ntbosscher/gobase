package modelutil

import (
	"context"
	"fmt"
	"sync"
)

type Cache2[T any] struct {
	name string
}

// prevent name conflicts
var muCache2Counter sync.Mutex
var cache2Counter = 0

func NewCache2[T any](name string) *Cache2[T] {

	muCache2Counter.Lock()
	cache2Counter++
	counter := cache2Counter
	muCache2Counter.Unlock()

	return &Cache2[T]{
		name: fmt.Sprintf("%s-%d", name, counter),
	}
}

func (c *Cache2[T]) NewScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, c.name, &cache2Inner[T]{
		values: map[int]T{},
	})
}

func (c *Cache2[T]) Set(ctx context.Context, key int, value T) {

	outer, ok := ctx.Value(c.name).(*cache2Inner[T])
	if !ok {
		return
	}

	outer.Set(key, value)
}

func (c *Cache2[T]) Clear(ctx context.Context, key2 int) {
	outer, ok := ctx.Value(c.name).(*cache2Inner[T])
	if !ok {
		return
	}

	var defaultT T
	outer.Set(key2, defaultT)
}

func (c *Cache2[T]) Get(ctx context.Context, key2 int) T {
	var defaultT T

	outer, ok := ctx.Value(c.name).(*cache2Inner[T])
	if !ok {
		return defaultT
	}

	return outer.Get(key2)
}

type cache2Inner[T any] struct {
	values map[int]T
}

func (t *cache2Inner[T]) Set(key int, value T) {
	t.values[key] = value
}

func (t *cache2Inner[T]) Get(key int) T {
	return t.values[key]
}
