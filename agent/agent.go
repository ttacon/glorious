package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ttacon/glorious/config"
	"github.com/ttacon/glorious/context"
)

type Agent struct {
	conf    *config.GloriousConfig
	fileLoc string
	lgr     context.Logger
}

func NewAgent(conf *config.GloriousConfig, fileLoc string, lgr context.Logger) *Agent {
	return &Agent{
		conf:    conf,
		fileLoc: fileLoc,
		lgr:     lgr,
	}
}

func (a *Agent) Greet(params []string, reply *string) error {
	debugRemoteCallStart(a.lgr, "Greet")

	*reply = fmt.Sprintf("Hello, %s", strings.Join(params, " "))
	return nil
}

func (a *Agent) Config(_ struct{}, units *[]UnitConfig) error {
	debugRemoteCallStart(a.lgr, "Config")

	*units = make([]UnitConfig, len(a.conf.Units))
	for i, unit := range a.conf.Units {
		(*units)[i] = UnitConfig{
			Name:        unit.Name,
			NumSlots:    len(unit.Slots),
			Description: unit.Description,
		}
	}

	return nil
}

func (a *Agent) Status(_ struct{}, units *[]UnitStatus) error {
	debugRemoteCallStart(a.lgr, "Status")

	*units = make([]UnitStatus, len(a.conf.Units))
	for i, unit := range a.conf.Units {
		(*units)[i] = UnitStatus{
			Name:   unit.Name,
			Groups: unit.Groups,
			Status: unit.ProcessStatus(),
		}
	}

	return nil
}

func (a *Agent) Reload(_ struct{}, err *error) error {
	debugRemoteCallStart(a.lgr, "Reload")

	a.conf, *err = config.LoadConfig(a.fileLoc)
	return nil
}

func (a *Agent) StartUnit(unitName string, err *error) error {
	debugRemoteCallStart(a.lgr, "StartUnit")

	unit, exists := a.conf.GetUnit(unitName)
	if !exists {
		*err = errors.New("unknown unit")
		return nil
	} else if startErr := unit.Start(); startErr != nil {
		*err = startErr
		return nil
	}

	return nil
}

func (a *Agent) StorePutValue(req StorePutValueRequest, resp *ErrResponse) error {
	debugRemoteCallStart(a.lgr, "StorePutValue")

	store := a.conf.GetContext().InternalStore()
	resp.Err = store.PutInternalStoreVal(req.Key, req.Value)
	return nil
}

type StorePutValueRequest struct {
	Key   string
	Value string
}

type ErrResponse struct {
	Err error
}

func (a *Agent) StoreGetValues(req *StoreGetValuesRequest, resp *StoreGetValuesResponse) error {
	debugRemoteCallStart(a.lgr, "StoreGetValue")

	store := a.conf.GetContext().InternalStore()

	resp.Values = make(map[string]string)
	for _, key := range req.Keys {
		val, err := store.GetInternalStoreVal(key)
		if err != nil || len(val) == 0 {
			val = "(not found)"
		}
		resp.Values[key] = val
	}
	return nil
}

type StoreGetValuesRequest struct {
	Keys []string
}

type StoreGetValuesResponse struct {
	Values map[string]string
}

type UnitConfig struct {
	Name        string `json:"name"`
	NumSlots    int    `json:"numSlots"`
	Description string `json:"description"`
}

type UnitStatus struct {
	Name   string   `json:"name"`
	Groups []string `json:"groups"`
	Status string   `json:"status"`
}

func debugRemoteCallStart(lgr context.Logger, action string) {
	lgr.Debugf("Agent.%s called", action)
}
