package main

import (
	"strings"

	"golang.org/x/tools/go/analysis"
	"honnef.co/go/tools/staticcheck"
)

func staticcheckAnalyzers() []*analysis.Analyzer {
	var checks []*analysis.Analyzer

	for _, v := range staticcheck.Analyzers {
		if strings.HasPrefix(v.Analyzer.Name, "SA") || v.Analyzer.Name == "ST1000" || v.Analyzer.Name == "S1000" || v.Analyzer.Name == "QF1001" {
			checks = append(checks, v.Analyzer)
		}
	}

	return checks
}
