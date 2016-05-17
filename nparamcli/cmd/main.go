package main

import (
	"fmt"
	"os"

	"nparam/nparamcli"
)

func main() {
	err := nparamcli.Process(false, true)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	} else {
		fmt.Println("OK")
	}
}
