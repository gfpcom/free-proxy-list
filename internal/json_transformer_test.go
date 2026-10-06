package internal

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestFromJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		options string
		want    string
	}{
		{
			name:    "array of existing proxy URIs",
			input:   `[{"proxy":"http://1.2.3.4:8080"},{"proxy":"socks5://5.6.7.8:1080"}]`,
			options: "path=$[*];uri=proxy",
			want:    "http://1.2.3.4:8080\nsocks5://5.6.7.8:1080\n",
		},
		{
			name:    "nested records and mapped fields",
			input:   `{"data":{"proxies":[{"protocol":"socks5","ip":"2001:db8::1","port":1080},{"protocol":"http","ip":"1.2.3.4","port":"8080"}]}}`,
			options: "path=$.data.proxies[*];scheme=protocol;host=ip;port=port",
			want:    "socks5://[2001:db8::1]:1080\nhttp://1.2.3.4:8080\n",
		},
		{
			name:    "object wildcard selects grouped records in key order",
			input:   `{"countries":{"US":[{"proto":"http","ip":"1.2.3.4","port":8080}],"JP":[{"proto":"socks5","ip":"5.6.7.8","port":1080}]}}`,
			options: "path=$.countries.*[*];scheme=proto;host=ip;port=port",
			want:    "socks5://5.6.7.8:1080\nhttp://1.2.3.4:8080\n",
		},
		{
			name:    "source protocol supplies scheme",
			input:   `[{"ip":"1.2.3.4","port":8080}]`,
			options: "path=$[*];host=ip;port=port",
			want:    "1.2.3.4:8080\n",
		},
		{
			name:    "single record object",
			input:   `{"proxy":"http://1.2.3.4:8080"}`,
			options: "path=$;uri=proxy",
			want:    "http://1.2.3.4:8080\n",
		},
		{
			name:    "array of endpoints within a protocol group",
			input:   `{"http":["1.2.3.4:8080","[2001:db8::1]:3128"]}`,
			options: "path=$.http[*];endpoint=$",
			want:    "1.2.3.4:8080\n[2001:db8::1]:3128\n",
		},
		{
			name:    "array index selector",
			input:   `{"proxies":[{"proxy":"http://1.2.3.4:8080"},{"proxy":"socks5://5.6.7.8:1080"}]}`,
			options: "path=$.proxies[1];uri=proxy",
			want:    "socks5://5.6.7.8:1080\n",
		},
		{
			name:    "invalid records are skipped",
			input:   `[{"ip":"1.2.3.4","port":8080},{"ip":"bad host","port":80},{"ip":"5.6.7.8","port":70000},{"ip":"9.10.11.12","port":80.5}]`,
			options: "path=$[*];host=ip;port=port",
			want:    "1.2.3.4:8080\n",
		},
		{
			name:    "missing selected path",
			input:   `{"other":[]}`,
			options: "path=$.proxies[*];uri=proxy",
			want:    "",
		},
		{
			name:    "malformed or trailing JSON rejected",
			input:   `[{"proxy":"http://1.2.3.4:8080"}] trailing`,
			options: "path=$[*];uri=proxy",
			want:    "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := string(FromJSON([]byte(test.input), test.options))
			if got != test.want {
				t.Fatalf("FromJSON() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFromJSONEnforcesInputAndRecordLimits(t *testing.T) {
	options := "path=$[*];uri=proxy"
	valid := ` [{"proxy":"http://1.2.3.4:8080"}]`
	exactLimitInput := valid + strings.Repeat(" ", maxJSONSize-len(valid))
	if got := string(FromJSON([]byte(exactLimitInput), options)); got != "http://1.2.3.4:8080\n" {
		t.Fatalf("expected input at the size limit to be processed, got %q", got)
	}

	if got := FromJSON(make([]byte, maxJSONSize+1), options); len(got) != 0 {
		t.Fatalf("expected oversized input to produce no output, got %d bytes", len(got))
	}

	input := `[` + strings.Repeat(`{},`, maxJSONRecords) + `{}]`
	if got := FromJSON([]byte(input), options); len(got) != 0 {
		t.Fatalf("expected excess records to produce no output, got %d bytes", len(got))
	}
}

func TestResolveJSONPathEnforcesWildcardRecordLimit(t *testing.T) {
	object := make(map[string]any, maxJSONRecords+1)
	for i := 0; i <= maxJSONRecords; i++ {
		object[strconv.Itoa(i)] = i
	}
	if _, ok := resolveJSONPath(object, []string{"*"}); ok {
		t.Fatal("expected object wildcard exceeding the record limit to fail")
	}
}

func TestParseJSONTransformerOptions(t *testing.T) {
	tests := []struct {
		name    string
		options string
	}{
		{name: "missing path", options: "uri=proxy"},
		{name: "missing mapping", options: "path=$[*]"},
		{name: "incomplete field mapping", options: "path=$[*];host=ip"},
		{name: "uri with conflicting fields", options: "path=$[*];uri=proxy;host=ip"},
		{name: "endpoint with conflicting URI", options: "path=$[*];uri=proxy;endpoint=$"},
		{name: "endpoint with conflicting fields", options: "path=$[*];endpoint=$;host=ip"},
		{name: "duplicate option", options: "path=$[*];uri=proxy;uri=link"},
		{name: "unknown option", options: "path=$[*];uri=proxy;filter=active"},
		{name: "unsupported path filter", options: "path=$[?(@.active)];uri=proxy"},
		{name: "empty array index", options: "path=$[];uri=proxy"},
		{name: "object wildcard must be a complete selector", options: "path=$.proxies.*name;uri=proxy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateJSONTransformerOptions(test.options); err == nil {
				t.Fatalf("expected invalid options %q to fail", test.options)
			}
		})
	}
}

func TestValidateSourceRejectsInvalidJSONTransformerOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"proxy":"http://1.2.3.4:8080"}]`))
	}))
	defer server.Close()

	err := ValidateSource("http", []byte(server.URL+",json:path=$[*];uri=proxy;unknown=value"))
	if err == nil {
		t.Fatal("expected invalid JSON transformer options to fail validation")
	}
}
