package main

import (
	"fmt"
	"os"

	"github.com/lestrrat-go/sketch/gen"
)

func main() {
	var app gen.App
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
}
