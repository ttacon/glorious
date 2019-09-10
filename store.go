package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
)

var (
	internalStore = loadInternalStore()
)

func loadInternalStore() map[string]string {
	data, err := ioutil.ReadFile(internalStoreFileLocation())
	if err != nil {
		panic(err)
	}
	var m = make(map[string]string)
	if err := json.Unmarshal(data, &m); err != nil {
		panic(err)
	}
	return m
}

func internalStoreFileLocation() string {
	return filepath.Join(os.Getenv("HOME"), ".glorious", "store.internal")
}

func putInternalStoreVal(key, val string) error {
	internalStore[key] = val

	return persistInternalStore()
}

func persistInternalStore() error {
	data, err := json.Marshal(internalStore)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(
		internalStoreFileLocation(),
		data,
		0666,
	)
}

func getInternalStoreVal(key string) (string, error) {
	val := internalStore[key]
	return val, nil
}
