package main

import (
	"github.com/harvester/hvperf/cmd"
	_ "github.com/harvester/hvperf/internal/suites" // register built-in suites
)

func main() {
	cmd.Execute()
}
