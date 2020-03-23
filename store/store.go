package store

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
)

type Store struct {
	readFile      func(location string) ([]byte, error)
	writeFile     func(location string, data []byte) error
	isNotExistErr func(err error) bool
	osHome        func() string

	values map[string]string
}

func NewStore() *Store {
	return &Store{
		readFile:      readFile,
		writeFile:     writeFile,
		isNotExistErr: isNotExistErr,
		osHome:        osHome,

		values: make(map[string]string),
	}
}

func readFile(location string) ([]byte, error) {
	data, err := ioutil.ReadFile(location)
	return data, err
}

func writeFile(location string, data []byte) error {
	return ioutil.WriteFile(
		location,
		data,
		0666,
	)
}

func isNotExistErr(err error) bool {
	return os.IsNotExist(err)
}

func osHome() string {
	return os.Getenv("HOME")
}

func (s *Store) LoadInternalStore() error {
	data, err := s.readFile(s.internalStoreFileLocation())
	if err != nil {
		if s.isNotExistErr(err) {
			// If the file doesn't exist, return an empty map.
			return nil
		}
		return err

	}

	if err := json.Unmarshal(data, &s.values); err != nil {
		return err
	}

	return nil
}

func (s *Store) internalStoreFileLocation() string {
	return filepath.Join(s.osHome(), ".glorious", "store.internal")
}

func (s *Store) PutInternalStoreVal(key, val string) error {
	s.values[key] = val

	return s.persistInternalStore()
}

func (s *Store) persistInternalStore() error {
	data, err := json.Marshal(s.values)
	if err != nil {
		return err
	}

	return s.writeFile(
		s.internalStoreFileLocation(),
		data,
	)
}

func (s *Store) GetInternalStoreVal(key string) (string, error) {
	val := s.values[key]
	return val, nil
}
