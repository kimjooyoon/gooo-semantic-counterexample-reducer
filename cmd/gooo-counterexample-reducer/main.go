package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kimjooyoon/gooo-semantic-counterexample-reducer/internal/reducer"
)

func main() {
	if len(os.Args) < 2 {
		fail(errors.New("command is required: reduce or manifest"))
	}
	switch os.Args[1] {
	case "reduce":
		runReduce(os.Args[2:])
	case "manifest":
		runManifest(os.Args[2:])
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func runManifest(args []string) {
	flags := flag.NewFlagSet("manifest", flag.ExitOnError)
	contractPath := flags.String("contract", "meta/counterexample-reducer.gooo", "authoritative reducer .gooo metacode")
	outPath := flags.String("output", "", "caller-owned JSON output")
	repoRoot := flags.String("repo-root", ".", "input repository root")
	if err := flags.Parse(args); err != nil {
		fail(err)
	}
	meta, raw, err := reducer.LoadMeta(*contractPath)
	if err != nil {
		fail(err)
	}
	runner, err := reducer.NewRunner(meta, raw)
	if err != nil {
		fail(err)
	}
	if err := reducer.EnsureCallerOutput(*outPath, *repoRoot); err != nil {
		fail(err)
	}
	if err := reducer.WriteJSON(*outPath, runner.Manifest()); err != nil {
		fail(err)
	}
}

func runReduce(args []string) {
	flags := flag.NewFlagSet("reduce", flag.ExitOnError)
	contractPath := flags.String("contract", "meta/counterexample-reducer.gooo", "authoritative reducer .gooo metacode")
	repoRoot := flags.String("repo-root", ".", "input repository root")
	outPath := flags.String("out", "", "empty caller-owned output directory")
	scenarioID := flags.String("scenario", "all", "declared scenario id or all")
	beforePath := flags.String("before-report", "", "optional matched prior report, or a directory containing per-scenario reports")
	if err := flags.Parse(args); err != nil {
		fail(err)
	}
	meta, raw, err := reducer.LoadMeta(*contractPath)
	if err != nil {
		fail(err)
	}
	runner, err := reducer.NewRunner(meta, raw)
	if err != nil {
		fail(err)
	}
	if err := reducer.PrepareOutput(*outPath, *repoRoot); err != nil {
		fail(err)
	}
	if err := reducer.WriteJSON(filepath.Join(*outPath, "runner-manifest.json"), runner.Manifest()); err != nil {
		fail(err)
	}

	var scenarios []reducer.ScenarioDecl
	if *scenarioID == "all" {
		scenarios = append(scenarios, meta.Scenarios...)
	} else {
		scenario, err := meta.Scenario(*scenarioID)
		if err != nil {
			fail(err)
		}
		scenarios = []reducer.ScenarioDecl{scenario}
	}
	results := make([]reducer.ReductionReport, 0, len(scenarios))
	for _, scenario := range scenarios {
		fixturePath := filepath.Join(*repoRoot, filepath.FromSlash(scenario.Fixture))
		before, err := loadBefore(*beforePath, *scenarioID, scenario.ID)
		if err != nil {
			fail(err)
		}
		result, err := runner.ReduceScenario(scenario, fixturePath, before)
		if err != nil {
			fail(err)
		}
		results = append(results, result)
		if err := reducer.WriteJSON(filepath.Join(*outPath, "reduction-reports", scenario.ID+".json"), result); err != nil {
			fail(err)
		}
		if result.ReducedGraph.Schema != "" {
			if err := reducer.WriteJSON(filepath.Join(*outPath, "reduced-graphs", scenario.ID+".json"), result.ReducedGraph); err != nil {
				fail(err)
			}
		}
	}
	batch := reducer.BuildBatchReport(runner, results)
	if err := reducer.WriteJSON(filepath.Join(*outPath, "reduction-report.json"), batch); err != nil {
		fail(err)
	}
	if batch.Status != reducer.StatusClosed {
		fail(fmt.Errorf("reduction status is %s", batch.Status))
	}
}

func loadBefore(path, selected, scenario string) (*reducer.ReductionReport, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	if selected == "all" {
		path = filepath.Join(path, "reduction-reports", scenario+".json")
	} else if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, "reduction-reports", scenario+".json")
	}
	report, err := reducer.LoadReductionReport(path)
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
