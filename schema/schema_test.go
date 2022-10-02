package schema_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/lestrrat-go/sketch/schema"
	"github.com/stretchr/testify/require"
)

type StringList struct {
	mu      sync.RWMutex
	storage []string
}

func (sl *StringList) AcceptValue(v interface{}) error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	switch v := v.(type) {
	case []string:
		sl.storage = make([]string, len(v))
		copy(sl.storage, v)
	default:
		return fmt.Errorf(`invalid value passed to StringList.AcceptValue (got %T, expected []string)`, v)
	}
	return nil
}

func (sl *StringList) GetValue() []string {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	ret := make([]string, len(sl.storage))
	copy(ret, sl.storage)
	return ret
}

func TestType(t *testing.T) {
	ti := schema.Type(&StringList{})
	require.Equal(t, `GetValue`, ti.GetGetValueMethodName())
	require.Equal(t, `AcceptValue`, ti.GetAcceptValueMethodName())
	require.Equal(t, ti.GetApparentType(), `[]string`)
}
