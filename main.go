package main

import (
	"github.com/harvester/hperf/cmd"
	_ "github.com/harvester/hperf/internal/suites" // register built-in suites
)

func main() {
	cmd.Execute()
}
