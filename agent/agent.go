package agent

import (
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

func (a *Agent) StartUnit(unitName string, err *string) error {
	debugRemoteCallStart(a.lgr, "StartUnit")

	unit, exists := a.conf.GetUnit(unitName)
	if !exists {
		*err = "unknown unit"
		return nil
	} else if startErr := unit.Start(); startErr != nil {
		*err = startErr.Error()
		return nil
	}

	return nil
}

func (a *Agent) StopUnit(unitName string, err *string) error {
	debugRemoteCallStart(a.lgr, "StopUnit")

	unit, exists := a.conf.GetUnit(unitName)
	if !exists {
		*err = "unknown unit"
		return nil
	} else if stopErr := unit.Stop(); stopErr != nil {
		*err = stopErr.Error()
		return nil
	}

	return nil
}

func (a *Agent) StorePutValue(req StorePutValueRequest, resp *ErrResponse) error {
	debugRemoteCallStart(a.lgr, "StorePutValue")

	store := a.conf.GetContext().InternalStore()
	if err := store.PutInternalStoreVal(req.Key, req.Value); err != nil {
		resp.Err = err.Error()
	}
	return nil
}

func (a *Agent) Logger() context.Logger {
	return a.lgr
}

func (a *Agent) Conf() *config.GloriousConfig {
	return a.conf
}

func (a *Agent) ExchangeTailToken(token string) ([]string, bool) {
	names, ok := a.conf.ExchangeTailToken(token)
	return names, ok
}

type StorePutValueRequest struct {
	Key   string
	Value string
}

type ErrResponse struct {
	Err string
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

func (a *Agent) TailProcesses(req *TailProcessesRequest, resp *TailProcessesResponse) error {
	debugRemoteCallStart(a.lgr, "TailProcesses")

	if len(req.Names) == 0 {
		resp.Err = "no names provided"
		return nil
	}

	// Validate all names are valid units
	var invalidNames []string
	for _, name := range req.Names {
		_, exists := a.conf.GetUnit(name)
		if !exists {
			invalidNames = append(invalidNames, name)
		}
	}
	if len(invalidNames) > 0 {
		resp.Err = fmt.Sprintf(
			"invalid names: %s",
			strings.Join(invalidNames, ", "),
		)
		return nil
	}

	resp.Token = a.conf.CreateTailProcessToken(req.Names)
	return nil
}

type TailProcessesRequest struct {
	Names []string
}

type TailProcessesResponse struct {
	Token string
	Err   string
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
