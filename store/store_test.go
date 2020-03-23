package store

import (
	"errors"
	"testing"
)

func TestStore_LoadInternalStore(t *testing.T) {
	var tests = []struct {
		readFile      func(location string) ([]byte, error)
		writeFile     func(location string, data []byte) error
		isNotExistErr func(err error) bool
		osHome        func() string
		key           string
		expectedVal   string
	}{
		{
			readFile: func(_ string) ([]byte, error) {
				return nil, errors.New("our error")
			},
			isNotExistErr: func(_ error) bool {
				return true
			},
			key:         "foo",
			expectedVal: "",
		}, {
			readFile: func(_ string) ([]byte, error) {
				return []byte("{\"foo\":\"bar\"}"), nil
			},
			key:         "foo",
			expectedVal: "bar",
		},
	}

	for i, test := range tests {
		s := NewStore()
		s.readFile = test.readFile
		s.isNotExistErr = test.isNotExistErr

		_ = s.LoadInternalStore()
		got, _ := s.GetInternalStoreVal(test.key)
		if got != test.expectedVal {
			t.Errorf("[test %d] expected %v, got %v\n", i, test.expectedVal, got)
		}
	}
}

func TestStore_PutInternalStoreVal(t *testing.T) {

	var writes []string

	s := NewStore()
	s.writeFile = func(loc string, data []byte) error {
		writes = append(writes, string(data))
		return nil
	}

	if err := s.PutInternalStoreVal("foo", "bar"); err != nil {
		t.Error("unexpected error on first write")
	} else if err := s.PutInternalStoreVal("foo", "baz"); err != nil {
		t.Error("unexpected error on second write")
	}

	if len(writes) != 2 {
		t.Errorf("expected two writes, found %d\n", len(writes))
	}

	if writes[0] != `{"foo":"bar"}` {
		t.Error("unexpected write: ", writes[0])
	} else if writes[1] != `{"foo":"baz"}` {
		t.Error("unexpected write: ", writes[1])
	}

}
