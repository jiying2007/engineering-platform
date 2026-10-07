package productionassets

import (
	"bytes"
	"os"
	"testing"
)

func TestEmbeddedTemplatesAreOriginalSourceFiles(t *testing.T) {
	all, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 25 {
		t.Fatal("explicit deployment template inventory changed", len(all))
	}
	for path, raw := range all {
		original, e := os.ReadFile(path)
		if e != nil || !bytes.Equal(original, raw) {
			t.Fatal("embedded template drift", path, e)
		}
		if len(raw) == 0 {
			t.Fatal("empty template", path)
		}
	}
	all["control.env.example"][0] ^= 1
	again, e := Files()
	if e != nil || bytes.Equal(all["control.env.example"], again["control.env.example"]) {
		t.Fatal("mutable template escaped", e)
	}
}
