package main

import (
	"os"

	"nparam/nparamserver"
)

func main() {
	err := nparamserver.StartServer(configFile)
	if err != nil {
		os.Exit(1)
	}

}
const configFile = "config.json"
