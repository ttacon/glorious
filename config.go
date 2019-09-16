package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/abiosoft/ishell"
	"github.com/hashicorp/hcl"
)

func loadConfig(configFileLocation string) (*GloriousConfig, error) {
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
	for _, unit := range m.Units {
		if len(unit.Groups) > 0 {
			for _, group := range unit.Groups {
				if m.Groups[group] == nil {
					m.Groups[group] = make([]string, 0)
				}
				m.Groups[group] = append(m.Groups[group], unit.Name)
			}
		}

		slot, err := unit.identifySlot()
		if err != nil {
			// TODO(ttacon): wrap this error
			return nil, err
		}

		if slot == nil || slot.Provider == nil {
			return nil, fmt.Errorf("invalid provider for unit %q", unit.Name)
		}

		if strings.HasPrefix(slot.Provider.Type, "docker") {
			if err := unit.populateDockerStatus(slot); err != nil {
				return nil, err
			}
		}
	}

	return &m, nil
}

type GloriousConfig struct {
	Units  []*Unit `hcl:"unit"`
	Groups map[string][]string
}

func (g *GloriousConfig) Validate() []*ErrWithPath {
	var configErrs []*ErrWithPath
	for _, unit := range g.Units {
		if errs := unit.Validate(); len(errs) > 0 {
			configErrs = append(configErrs, errs...)
		}
	}

	return configErrs
}

func (g *GloriousConfig) GetUnit(name string) (*Unit, bool) {
	for _, unit := range g.Units {
		if unit.Name == name {
			return unit, true
		}

	}
	return nil, false
}

func (g *GloriousConfig) GetGroup(name string) ([]*Unit, bool) {
	if g.Groups[name] == nil {
		return nil, false
	}

	var units []*Unit
	for _, name := range g.Groups[name] {
		unit, ok := g.GetUnit(name)
		if !ok {
			return nil, false
		}

		units = append(units, unit)
	}

	return units, true
}

func (g *GloriousConfig) GetUnits(args []string) ([]*Unit, error) {
	var unitsToStart []*Unit
	for _, name := range args {
		group, ok := g.GetGroup(name)
		if ok {
			return group, nil
		}

		unit, ok := g.GetUnit(name)
		if !ok {
			return nil, errors.New(fmt.Sprintf("unknown unit %q, aborting", name))
		}

		unitsToStart = append(unitsToStart, unit)
	}

	return unitsToStart, nil
}

func (g *GloriousConfig) assertKeyChange(key string, ctxt *ishell.Context) error {
	for _, unit := range g.Units {
		for _, slot := range unit.Slots {
			if slot.Resolver["type"] != "keyword/value" {
				continue
			} else if slot.Resolver["keyword"] == key {
				if err := unit.Restart(ctxt); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
