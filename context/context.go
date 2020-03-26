package context

import (
	"github.com/sirupsen/logrus"
	"github.com/ttacon/glorious/store"
)

type Context interface {
	InternalStore() *store.Store
	Logger() Logger
}

type context struct {
	internalStore *store.Store
	logger        Logger
}

func (c *context) InternalStore() *store.Store {
	return c.internalStore
}

func (c *context) Logger() Logger {
	return c.logger
}

func NewContext() Context {
	c := &context{
		internalStore: store.NewStore(),
		logger:        logrus.New(),
	}

	// This will panic if it doesn't succeed.
	c.internalStore.LoadInternalStore()

	return c
}

type Logger interface {
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Trace(args ...interface{})
	Tracef(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})

	SetLevel(level logrus.Level)
}
