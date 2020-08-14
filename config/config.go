package config

import (
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"github.com/hashicorp/hcl"
	"github.com/satori/go.uuid"
	gcontext "github.com/ttacon/glorious/context"
	gerrors "github.com/ttacon/glorious/errors"
	"github.com/ttacon/glorious/unit"
)

func LoadConfig(configFileLocation string) (*GloriousConfig, error) {
	data, err := ioutil.ReadFile(configFileLocation)
	if err != nil {
		return nil, err
	}

	return ParseConfigRaw(data)
}

func ParseConfig(str string) (*GloriousConfig, error) {
	return ParseConfigRaw([]byte(str))
}

func ParseConfigRaw(data []byte) (*GloriousConfig, error) {
	var m GloriousConfig
	if err := hcl.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	// We need to identify if any previous processes are running.
	//
	// For now, we'll solely support docker. Next will be PIDfile based
	// support.
	m.Groups = make(map[string][]string)
	var dependenciesToProcess []*unit.Unit

	for _, unit := range m.Units {
		if len(unit.Groups) > 0 {
			for _, group := range unit.Groups {
				if m.Groups[group] == nil {
					m.Groups[group] = make([]string, 0)
				}
				m.Groups[group] = append(m.Groups[group], unit.Name)
			}
		}

		if len(unit.DependsOnRaw) > 0 {
			dependenciesToProcess = append(dependenciesToProcess, unit)
		}
	}

	for _, unit := range dependenciesToProcess {
		for _, dep := range unit.DependsOnRaw {
			identifiedUnit, dependencyExists := (&m).GetUnit(dep)
			if !dependencyExists {
				return nil, fmt.Errorf(
					"invalid dependency %q for unit %q",
					dep,
					unit.Name,
				)
			}
			unit.DependsOn = append(unit.DependsOn, identifiedUnit)
		}
	}

	(&m).initTailGroupProcessing()

	return &m, nil
}

type GloriousConfig struct {
	Units  []*unit.Unit `hcl:"unit"`
	Groups map[string][]string

	contxt gcontext.Context

	tailGroupMux *sync.Mutex
	tailGroups   map[string]*tailProcessState
}

func (g *GloriousConfig) initTailGroupProcessing() {
	g.tailGroupMux = new(sync.Mutex)
	g.tailGroups = make(map[string]*tailProcessState)

	go g.cleanupOldTailGroups()
}

func (g *GloriousConfig) ExchangeTailToken(token string) ([]string, bool) {
	g.tailGroupMux.Lock()

	state, ok := g.tailGroups[token]
	if !ok {
		g.tailGroupMux.Unlock()
		return nil, ok
	}

	names := state.names
	delete(g.tailGroups, token)

	g.tailGroupMux.Unlock()

	return names, ok
}

func (g *GloriousConfig) cleanupOldTailGroups() {
	ticker := time.NewTimer(time.Minute * 5)
	for {
		<-ticker.C

		g.tailGroupMux.Lock()
		for token, state := range g.tailGroups {
			if state.createdAt.Add(time.Hour).After(time.Now()) {
				delete(g.tailGroups, token)
			}
		}
		g.tailGroupMux.Unlock()
	}
}

type tailProcessState struct {
	names     []string
	createdAt time.Time
}

func (g *GloriousConfig) CreateTailProcessToken(names []string) string {
	g.tailGroupMux.Lock()
	token := uuid.NewV4().String()
	g.tailGroups[token] = &tailProcessState{
		names:     names,
		createdAt: time.Now(),
	}
	g.tailGroupMux.Unlock()
	return token
}

func (g *GloriousConfig) Validate() []*gerrors.ErrWithPath {
	var configErrs []*gerrors.ErrWithPath
	for _, unit := range g.Units {
		if errs := unit.Validate(); len(errs) > 0 {
			configErrs = append(configErrs, errs...)
		}
	}

	return configErrs
}

func (g *GloriousConfig) GetContext() gcontext.Context {
	return g.contxt
}

func (g *GloriousConfig) SetContext(c gcontext.Context) {
	g.contxt = c

	for _, unit := range g.Units {
		unit.SetContext(c)
	}
}

func (g *GloriousConfig) Init() error {
	for _, unit := range g.Units {
		if err := unit.Init(); err != nil {
			return err
		}
	}
	return nil
}

func (g *GloriousConfig) GetUnit(name string) (*unit.Unit, bool) {
	for _, unit := range g.Units {
		if unit.Name == name {
			return unit, true
		}

	}
	return nil, false
}

func (g *GloriousConfig) GetGroup(name string) ([]*unit.Unit, bool) {
	if g.Groups[name] == nil {
		return nil, false
	}

	var units []*unit.Unit
	for _, name := range g.Groups[name] {
		unit, ok := g.GetUnit(name)
		if !ok {
			return nil, false
		}

		units = append(units, unit)
	}

	return units, true
}

func (g *GloriousConfig) GetUnits(args []string) ([]*unit.Unit, error) {
	var unitsToStart []*unit.Unit
	for _, name := range args {
		group, ok := g.GetGroup(name)
		if ok {
			return group, nil
		}

		unit, ok := g.GetUnit(name)
		if !ok {
			return nil, fmt.Errorf("unknown unit %q, aborting", name)
		}

		unitsToStart = append(unitsToStart, unit)
	}

	return unitsToStart, nil
}

func (g *GloriousConfig) AssertKeyChange(key string) error {
	for _, unit := range g.Units {
		for _, slot := range unit.Slots {
			if slot.Resolver["type"] != "keyword/value" {
				continue
			} else if slot.Resolver["keyword"] == key {
				if err := unit.Restart(); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
