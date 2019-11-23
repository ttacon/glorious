package context

import "github.com/ttacon/glorious/store"

type Context interface {
	InternalStore() *store.Store
}

type context struct {
	internalStore *store.Store
}

func (c *context) InternalStore() *store.Store {
	return c.internalStore
}

func NewContext() Context {
	c := &context{
		internalStore: store.NewStore(),
	}

	// This will panic if it doesn't succeed.
	c.internalStore.LoadInternalStore()

	return c
}
