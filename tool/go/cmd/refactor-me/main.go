package main

import (
	"os"

	"github.com/soom-kang/refactor-me/tool/go/internal/controller"
	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
)

// version is set only for an approved release build. A local build reports dev.
var version = "dev"
var commit = "unknown"

func main() {
	surface.Version = version
	surface.Commit = commit
	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(surface.ExitAborted)
	}
	callbacks := surface.Callbacks{
		Run: controller.Run, Doctor: controller.Doctor, Clean: controller.Clean,
	}
	if surface.InteractiveTerminal(os.Stdin, os.Stderr) {
		callbacks.SelectMaxMinutes = func(defaultMinutes int, language string) (int, error) {
			return surface.SelectMaxMinutes(os.Stdin, os.Stderr, defaultMinutes, language)
		}
	}
	os.Exit(surface.Execute(os.Args[1:], cwd, os.Stdout, os.Stderr, callbacks))
}
