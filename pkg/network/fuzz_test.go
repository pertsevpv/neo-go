package network

import (
	"github.com/nspcc-dev/neo-go/internal/testserdes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzMessageDecode(f *testing.F) {
	f.Fuzz(func(t *testing.T, bytes []byte) {
		if len(bytes) < (1 << 8) {
			return
		}
		actual := Message{}
		generator := NewGenerator(bytes)
		err := generator.GenerateStruct(&actual)

		if err != nil {
			t.Error("Err: ", err)
		}
		expected := &Message{}

		data, err := testserdes.Encode(expected)
		require.NoError(t, err)
		err = testserdes.Decode(data, &actual)
		if err != nil && !strings.HasPrefix(err.Error(), "unexpected empty payload: ") {
			t.Error("Err: ", err)
		}
		require.Equal(t, expected, &actual)
	})
}
