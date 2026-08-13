package jsonx

import (
	"encoding/json"
	"strings"
	"testing"
)

// The file this package exists for is the user's own settings.json. The thing
// that must never happen is a rewrite that moves their keys around.
func TestKeyOrderSurvivesRoundTrip(t *testing.T) {
	input := `{
  "theme": "dark",
  "model": "opus",
  "permissions": {
    "allow": [
      "Bash(git:*)",
      "Read"
    ],
    "deny": []
  },
  "aaa_alphabetically_first": true,
  "zzz_last": 3
}`
	obj, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	if out != input {
		t.Errorf("round trip changed the file:\n--- got ---\n%s\n--- want ---\n%s", out, input)
	}
}

func TestSetKeepsExistingPositionAndAppendsNew(t *testing.T) {
	obj, err := Parse([]byte(`{"a": 1, "b": 2}`))
	if err != nil {
		t.Fatal(err)
	}
	obj.Set("a", "degisti") // existing key: stays first
	obj.Set("c", "yeni")    // new key: goes last

	out, _ := Marshal(obj)
	want := "{\n  \"a\": \"degisti\",\n  \"b\": 2,\n  \"c\": \"yeni\"\n}"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

// Empty containers, numbers and unicode all have to render exactly the way
// Python's json.dumps(..., ensure_ascii=False, indent=2) renders them, because
// the two implementations are compared byte for byte.
func TestPythonCompatibleFormatting(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"bos nesne", `{"a":{}}`, "{\n  \"a\": {}\n}"},
		{"bos dizi", `{"a":[]}`, "{\n  \"a\": []\n}"},
		{"sayi bicimi korunur", `{"t":20}`, "{\n  \"t\": 20\n}"},
		{"bool ve null", `{"a":true,"b":null}`, "{\n  \"a\": true,\n  \"b\": null\n}"},
		{"unicode kacilmaz", `{"m":"ölçüldü"}`, "{\n  \"m\": \"ölçüldü\"\n}"},
		{"HTML kacilmaz", `{"m":"a & b < c"}`, "{\n  \"m\": \"a & b < c\"\n}"},
	} {
		obj, err := Parse([]byte(tc.input))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got, err := Marshal(obj)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s:\ngot:  %q\nwant: %q", tc.name, got, tc.want)
		}
	}
}

func TestNestedHelpersCreateOnDemand(t *testing.T) {
	obj := NewObject()
	list := obj.Object("hooks").Array("SessionStart")
	if len(list) != 0 {
		t.Fatalf("yeni dizi bos olmali, %d oge var", len(list))
	}
	entry := NewObject()
	entry.Set("type", "command")
	entry.Set("timeout", json.Number("20"))
	obj.Object("hooks").Set("SessionStart", append(list, entry))

	out, _ := Marshal(obj)
	for _, parca := range []string{`"hooks"`, `"SessionStart"`, `"type": "command"`, `"timeout": 20`} {
		if !strings.Contains(out, parca) {
			t.Errorf("cikti %q icermiyor:\n%s", parca, out)
		}
	}
}

func TestBadInputIsAnError(t *testing.T) {
	for _, bad := range []string{`{ bu json degil`, `[1, 2]`, ``, `"sadece string"`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("%q icin hata bekleniyordu", bad)
		}
	}
}
