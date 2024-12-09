package main

import (
	"os"
	"fmt"
	"github.com/deekshith-elear/perf-tests/clusterloader2/clusterloaderrunner"
)

func main() {
	if err := clusterloaderrunner.RunClusterLoader(os.Args[1:]); err != nil {
        fmt.Println("Error:", err)
        os.Exit(1)
    }
}