package pan115open

import (
	"encoding/json"
	"testing"
)

func TestFlexibleProgressUnmarshalJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want int
	}{
		{name: "int", raw: `{"percentDone":39}`, want: 39},
		{name: "float", raw: `{"percentDone":39.18}`, want: 39},
		{name: "string float", raw: `{"percentDone":"39.18"}`, want: 39},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var task offlineTask
			if err := json.Unmarshal([]byte(tc.raw), &task); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if got := int(task.PercentDone); got != tc.want {
				t.Fatalf("percentDone = %d, want %d", got, tc.want)
			}
		})
	}
}
