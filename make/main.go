package main

import (
	. "github.com/anchore/go-make"
	"github.com/anchore/go-make/tasks/golint"
	"github.com/anchore/go-make/tasks/gotest"
	"github.com/anchore/go-make/tasks/release"
)

func main() {
	Makefile(
		golint.Tasks(),
		Task{
			Name: "install:oras",
			Run: func() {
				oras := BinnyInstall("oras")
				Run("oras ")
			},
		},
		release.Tasks(),
		gotest.Test("unit"),
	)
}
