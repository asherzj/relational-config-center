// Command rccvet runs Relational Configuration Center analyzers.
//
// go vet -vettool replaces the standard vet suite with the analyzers
// registered here. Add a pass by importing it and passing its Analyzer
// to multichecker.Main. make lint builds this command and runs it on
// every workspace module.
package main

import (
	"github.com/asherzj/relational-config-center/tools/lint/passes/librarypanic"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(librarypanic.Analyzer)
}
