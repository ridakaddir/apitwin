package grpc

import "testing"

func TestWrapJSON(t *testing.T) {
	tests := []struct {
		name string
		key  string
		in   string
		want string
	}{
		{
			name: "wrap object",
			key:  "user",
			in:   `{"name":"myname"}`,
			want: "{\"user\":{\"name\":\"myname\"}}\n",
		},
		{
			name: "wrap array",
			key:  "items",
			in:   `[{"id":1},{"id":2}]`,
			want: "{\"items\":[{\"id\":1},{\"id\":2}]}\n",
		},
		{
			name: "trims whitespace",
			key:  "data",
			in:   "  {\"a\":1}  \n",
			want: "{\"data\":{\"a\":1}}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(wrapJSON(tt.key, []byte(tt.in)))
			if got != tt.want {
				t.Errorf("wrapJSON(%q, %q) = %q, want %q", tt.key, tt.in, got, tt.want)
			}
		})
	}
}
