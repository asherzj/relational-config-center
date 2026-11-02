package librarypanic_test

import (
	"testing"

	"github.com/asherzj/relational-config-center/tools/lint/passes/librarypanic"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestLibraryPanic(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), librarypanic.Analyzer,
		"example.com/library",
		"example.com/cmd/tool",
		"example.com/shadow",
		"example.com/mycmd",
	)
}
