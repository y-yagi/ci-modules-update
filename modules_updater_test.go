package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratePullRequestBody(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	updater := NewModulesUpdater(nil)
	before, err := updater.readModules(filepath.Join(wd, "testdata", "before"))
	if err != nil {
		t.Fatal(err)
	}

	after, err := updater.readModules(filepath.Join(wd, "testdata", "after"))
	if err != nil {
		t.Fatal(err)
	}

	got := updater.generatePullRequestBody(before, after)
	want := `**Changed:**

* [cloud.google.com/go](https://github.com/GoogleCloudPlatform/google-cloud-go) [v0.28.0...v0.31.0](https://github.com/GoogleCloudPlatform/google-cloud-go/compare/v0.28.0...v0.31.0)
* [github.com/go-pg/pg](https://github.com/go-pg/pg) [v6.14.0...v6.15.1](https://github.com/go-pg/pg/compare/v6.14.0...v6.15.1)
* [golang.org/x/crypto](https://github.com/golang/crypto) [74cb1d3d52f4...e84da0312774](https://github.com/golang/crypto/compare/74cb1d3d52f4...e84da0312774)
* [golang.org/x/sys](https://github.com/golang/sys) [5cd93ef61a7c...d989b31c8746](https://github.com/golang/sys/compare/5cd93ef61a7c...d989b31c8746)
* [google.golang.org/api](https://github.com/googleapis/google-api-go-client) [920bb1beccf7...511bab8e55de](https://github.com/googleapis/google-api-go-client/compare/920bb1beccf7...511bab8e55de)
`

	if got != want {
		t.Fatalf("want\n%v\nbut got \n\n%v", want, got)
	}
}
