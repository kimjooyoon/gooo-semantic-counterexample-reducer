package reducer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type BatchReport struct {
	Schema             string            `json:"schema"`
	Authority          string            `json:"authority"`
	ContractDigest     string            `json:"contract_digest"`
	Precedence         []string          `json:"precedence"`
	Status             string            `json:"status"`
	Scenarios          int               `json:"scenarios"`
	Closed             int               `json:"closed"`
	Unknown            int               `json:"unknown"`
	Refuted            int               `json:"refuted"`
	ImprovementUnknown int               `json:"improvement_unknown"`
	Results            []ReductionReport `json:"results"`
}

func PrepareOutput(path, repoRoot string) error {
	if err := EnsureCallerOutput(path, repoRoot); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0o755)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("output must be a directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("caller-owned output directory must be empty")
	}
	return nil
}

func WriteJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

func LoadReductionReport(path string) (ReductionReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ReductionReport{}, err
	}
	var report ReductionReport
	if err := json.Unmarshal(data, &report); err != nil {
		return ReductionReport{}, err
	}
	if report.Schema != ReportSchema || report.Scenario == "" {
		return ReductionReport{}, fmt.Errorf("invalid reduction report %s", path)
	}
	return report, nil
}

func BuildBatchReport(runner Runner, results []ReductionReport) BatchReport {
	batch := BatchReport{Schema: "gooo.semantic_counterexample_batch/v1", Authority: runner.Meta.Authority, ContractDigest: runner.ContractDigest, Precedence: append([]string(nil), runner.Meta.Precedence...), Status: StatusClosed, Scenarios: len(results), Results: results}
	for _, result := range results {
		switch result.Status {
		case StatusClosed:
			batch.Closed++
		case StatusUnknown:
			batch.Unknown++
		case StatusRefuted:
			batch.Refuted++
		}
		if result.Improvement.Status == StatusUnknown {
			batch.ImprovementUnknown++
		}
		if result.Status == StatusRefuted {
			batch.Status = StatusRefuted
		} else if result.Status == StatusUnknown && batch.Status != StatusRefuted {
			batch.Status = StatusUnknown
		}
	}
	return batch
}
