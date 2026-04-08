package persist

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
			key:  "country",
			in:   `{"code":"morocco","name":"Morocco"}`,
			want: "{\"country\":{\"code\":\"morocco\",\"name\":\"Morocco\"}}\n",
		},
		{
			name: "wrap array",
			key:  "countries",
			in:   `[{"code":"morocco"},{"code":"canada"}]`,
			want: "{\"countries\":[{\"code\":\"morocco\"},{\"code\":\"canada\"}]}\n",
		},
		{
			name: "trims whitespace",
			key:  "data",
			in:   "  {\"a\":1}  \n",
			want: "{\"data\":{\"a\":1}}\n",
		},
		{
			name: "sanitizes key with double quote",
			key:  `foo"bar`,
			in:   `{"x":1}`,
			want: "{\"foobar\":{\"x\":1}}\n",
		},
		{
			name: "sanitizes key with backslash",
			key:  `foo\bar`,
			in:   `{"x":1}`,
			want: "{\"foobar\":{\"x\":1}}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(WrapJSON(tt.key, []byte(tt.in)))
			if got != tt.want {
				t.Errorf("WrapJSON(%q, %q) = %q, want %q", tt.key, tt.in, got, tt.want)
			}
		})
	}
}
