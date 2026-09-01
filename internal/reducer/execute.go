package reducer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Runner struct {
	Meta           MetaContract
	ContractDigest string
}

func NewRunner(meta MetaContract, raw []byte) (Runner, error) {
	if err := meta.Validate(); err != nil {
		return Runner{}, err
	}
	return Runner{Meta: meta, ContractDigest: Digest(raw)}, nil
}

func (r Runner) Manifest() RunnerManifest {
	ids := make([]string, 0, len(r.Meta.Scenarios))
	for _, scenario := range r.Meta.Scenarios {
		ids = append(ids, scenario.ID)
	}
	return RunnerManifest{
		Schema: ManifestSchema, Authority: r.Meta.Authority, ContractDigest: r.ContractDigest,
		Denominator: r.Meta.Denominator, Precedence: append([]string(nil), r.Meta.Precedence...),
		UnknownFields: append([]string(nil), r.Meta.UnknownFields...), Predicates: append([]PredicateDecl(nil), r.Meta.Predicates...),
		Operations: append([]OperationDecl(nil), r.Meta.Operations...), GenerationPlan: r.Meta.GenerationPlan,
		ScenarioIDs: ids, SourcePolicy: r.Meta.SourcePolicy, Toolchain: r.Meta.Toolchain,
	}
}

func (r Runner) ReduceScenario(scenario ScenarioDecl, fixturePath string, before *ReductionReport) (ReductionReport, error) {
	graph, fixtureRaw, err := LoadGraph(fixturePath)
	if err != nil {
		return ReductionReport{}, err
	}
	fixtureDigest := Digest(fixtureRaw)
	report := ReductionReport{
		Schema: ReportSchema, Authority: r.Meta.Authority, Scenario: scenario.ID, Fixture: scenario.Fixture,
		FixtureDigest: fixtureDigest, SourceDigest: graph.SourceDigest, ContractDigest: r.ContractDigest,
		ToolchainDigest: graph.ToolchainDigest, PreservedPredicates: predicateIDs(r.Meta.Predicates),
		Operations: []OperationMeasurement{},
	}
	if fixtureDigest != scenario.FixtureDigest {
		return unknownReport(report, "verify_fixture_digest", "fixture digest does not match the immutable .gooo declaration", "FIXTURE_DIGEST_MISMATCH", "re-pin_fixture_digest", "fixture-digest-mismatch", before), nil
	}
	if graph.Scenario != scenario.ID || graph.SourceDigest != scenario.SourceDigest || graph.ToolchainDigest != scenario.ToolchainDigest {
		return unknownReport(report, "verify_source_identity", "fixture identity does not match the declared scenario or toolchain digest", "SOURCE_IDENTITY_MISMATCH", "verify_immutable_fixture", "source-or-toolchain-identity-mismatch", before), nil
	}
	baseline, err := r.runOracle(scenario, graph)
	if err != nil {
		return unknownReport(report, "baseline_oracle", err.Error(), "ORACLE_UNSTABLE", "retry_with_stable_oracle", "baseline-oracle-failed", before), nil
	}
	report.Baseline = baseline
	beforeCounts, err := graph.Counts()
	if err != nil {
		return report, err
	}
	report.Before = beforeCounts
	if baseline.Decision != scenario.ExpectedDecision || baseline.Reason != scenario.Reason {
		return unknownReport(report, "baseline_identity", "fixture baseline does not match the declared failed decision and reason", "BASELINE_MISMATCH", "pin_the_original_oracle_observation", "baseline-decision-or-reason-mismatch", before), nil
	}

	current := graph.Clone()
	totalInvocations := baseline.Invocations
	totalAttempts := 0
	decisionChanges := 0
	for _, operation := range r.Meta.Operations {
		operationBefore, err := current.Counts()
		if err != nil {
			return report, err
		}
		var attempts, accepted, operationDecisionChanges, invocations int
		var operationErr error
		switch operation.Kind {
		case "remove_nodes":
			current, attempts, accepted, operationDecisionChanges, invocations, operationErr = r.reduceNodes(scenario, current, operation, baseline)
		case "remove_edges":
			current, attempts, accepted, operationDecisionChanges, invocations, operationErr = r.reduceEdges(scenario, current, operation, baseline)
		case "trim_bytes":
			current, attempts, accepted, operationDecisionChanges, invocations, operationErr = r.reducePayload(scenario, current, operation, baseline)
		default:
			operationErr = fmt.Errorf("unsupported operation kind %q", operation.Kind)
		}
		totalInvocations += invocations
		totalAttempts += attempts
		decisionChanges += operationDecisionChanges
		operationAfter, countErr := current.Counts()
		if countErr != nil {
			return report, countErr
		}
		report.Operations = append(report.Operations, OperationMeasurement{OperationID: operation.ID, Kind: operation.Kind, Attempts: attempts, Accepted: accepted, Before: operationBefore, After: operationAfter})
		if operationErr != nil {
			report.OracleInvocations = totalInvocations
			report.CandidateAttempts = totalAttempts
			report.DecisionChanges = decisionChanges
			return unknownReport(report, operation.ID, operationErr.Error(), "ORACLE_UNSTABLE", "retry_with_stable_oracle", "candidate-oracle-instability", before), nil
		}
	}
	final, err := r.runOracle(scenario, current)
	totalInvocations += final.Invocations
	if err != nil {
		report.OracleInvocations = totalInvocations
		report.CandidateAttempts = totalAttempts
		report.DecisionChanges = decisionChanges
		return unknownReport(report, "final_oracle", err.Error(), "ORACLE_UNSTABLE", "retry_with_stable_oracle", "final-oracle-failed", before), nil
	}
	afterCounts, err := current.Counts()
	if err != nil {
		return report, err
	}
	report.Final = final
	report.After = afterCounts
	report.OracleInvocations = totalInvocations
	report.CandidateAttempts = totalAttempts
	report.DecisionChanges = decisionChanges
	if final.Decision == baseline.Decision && final.ReasonDigest == baseline.ReasonDigest {
		report.Status = StatusClosed
	} else {
		report.Status = StatusRefuted
	}
	report.ReducedGraph = current
	report.Improvement = evaluateImprovement(report, before)
	return report, nil
}

func (r Runner) runOracle(scenario ScenarioDecl, graph Graph) (OracleResult, error) {
	var first OracleResult
	for invocation := 0; invocation < r.Meta.OracleReplays; invocation++ {
		result, err := evaluateDeclaredOracle(scenario, graph)
		if err != nil {
			return OracleResult{Invocations: invocation + 1}, err
		}
		if invocation == 0 {
			first = result
			continue
		}
		if result.Decision != first.Decision || result.ReasonDigest != first.ReasonDigest {
			return OracleResult{Decision: result.Decision, Reason: result.Reason, ReasonDigest: result.ReasonDigest, Stable: false, Invocations: invocation + 1}, errors.New("oracle replay produced different decision or reason digest")
		}
	}
	first.Stable = true
	first.Invocations = r.Meta.OracleReplays
	return first, nil
}

func evaluateDeclaredOracle(scenario ScenarioDecl, graph Graph) (OracleResult, error) {
	if err := graph.Validate(); err != nil {
		return OracleResult{}, err
	}
	decision := StatusClosed
	reason := "declared rejection predicate not satisfied"
	switch scenario.OracleKind {
	case "activity_count_at_least":
		if scenario.OracleTarget == "" || scenario.OracleThreshold < 1 {
			return OracleResult{}, errors.New("activity oracle declaration is incomplete")
		}
		count := 0
		for _, node := range graph.Nodes {
			if node.Kind == scenario.OracleTarget {
				count++
			}
		}
		if count >= scenario.OracleThreshold {
			decision, reason = scenario.ExpectedDecision, scenario.Reason
		}
	case "edge_kinds_present":
		if len(scenario.OracleKinds) == 0 {
			return OracleResult{}, errors.New("edge oracle declaration is incomplete")
		}
		present := map[string]bool{}
		for _, edge := range graph.Edges {
			present[edge.Kind] = true
		}
		matches := true
		for _, kind := range scenario.OracleKinds {
			if !present[kind] {
				matches = false
				break
			}
		}
		if matches {
			decision, reason = scenario.ExpectedDecision, scenario.Reason
		}
	default:
		return OracleResult{}, fmt.Errorf("unsupported declared oracle kind %q", scenario.OracleKind)
	}
	return OracleResult{Decision: decision, Reason: reason, ReasonDigest: DigestString(reason)}, nil
}

func (r Runner) reduceNodes(scenario ScenarioDecl, current Graph, operation OperationDecl, baseline OracleResult) (Graph, int, int, int, int, error) {
	return r.reduceIDs(scenario, current, operation, baseline, func(graph Graph, ids []string) Graph {
		remove := map[string]bool{}
		for _, id := range ids {
			remove[id] = true
		}
		result := graph.Clone()
		result.Nodes = result.Nodes[:0]
		for _, node := range graph.Nodes {
			if !remove[node.ID] {
				result.Nodes = append(result.Nodes, node)
			}
		}
		result.Edges = result.Edges[:0]
		for _, edge := range graph.Edges {
			if !remove[edge.From] && !remove[edge.To] {
				result.Edges = append(result.Edges, edge)
			}
		}
		return result
	}, func(graph Graph) []string {
		ids := make([]string, 0, len(graph.Nodes))
		for _, node := range graph.Nodes {
			ids = append(ids, node.ID)
		}
		sort.Strings(ids)
		return ids
	})
}

func (r Runner) reduceEdges(scenario ScenarioDecl, current Graph, operation OperationDecl, baseline OracleResult) (Graph, int, int, int, int, error) {
	return r.reduceIDs(scenario, current, operation, baseline, func(graph Graph, ids []string) Graph {
		remove := map[string]bool{}
		for _, id := range ids {
			remove[id] = true
		}
		result := graph.Clone()
		result.Edges = result.Edges[:0]
		for _, edge := range graph.Edges {
			if !remove[edge.ID] {
				result.Edges = append(result.Edges, edge)
			}
		}
		return result
	}, func(graph Graph) []string {
		ids := make([]string, 0, len(graph.Edges))
		for _, edge := range graph.Edges {
			ids = append(ids, edge.ID)
		}
		sort.Strings(ids)
		return ids
	})
}

func (r Runner) reduceIDs(scenario ScenarioDecl, current Graph, operation OperationDecl, baseline OracleResult, apply func(Graph, []string) Graph, ids func(Graph) []string) (Graph, int, int, int, int, error) {
	attempts, accepted, decisionChanges, invocations := 0, 0, 0, 0
	granularity := operation.Granularity
	for len(ids(current)) > 0 {
		items := ids(current)
		if granularity > len(items) {
			granularity = len(items)
		}
		chunks := splitChunks(items, granularity)
		acceptedThisRound := false
		for _, chunk := range chunks {
			candidate := apply(current, chunk)
			result, err := r.runOracle(scenario, candidate)
			attempts++
			invocations += result.Invocations
			if err != nil {
				return current, attempts, accepted, decisionChanges, invocations, err
			}
			if result.Decision != baseline.Decision {
				decisionChanges++
			}
			if preserves(baseline, result) {
				current = candidate
				accepted++
				acceptedThisRound = true
				granularity = operation.Granularity
				break
			}
		}
		if acceptedThisRound {
			continue
		}
		if granularity >= len(items) {
			break
		}
		granularity *= 2
		if granularity > len(items) {
			granularity = len(items)
		}
	}
	return current, attempts, accepted, decisionChanges, invocations, nil
}

type payloadRef struct{ kind, id string }

func (r Runner) reducePayload(scenario ScenarioDecl, current Graph, operation OperationDecl, baseline OracleResult) (Graph, int, int, int, int, error) {
	attempts, accepted, decisionChanges, invocations := 0, 0, 0, 0
	granularity := operation.Granularity
	for {
		refs := nonEmptyPayloads(current)
		if len(refs) == 0 {
			break
		}
		if granularity > len(refs) {
			granularity = len(refs)
		}
		acceptedThisRound := false
		for _, chunk := range splitPayloadChunks(refs, granularity) {
			candidate := clearPayloads(current, chunk)
			result, err := r.runOracle(scenario, candidate)
			attempts++
			invocations += result.Invocations
			if err != nil {
				return current, attempts, accepted, decisionChanges, invocations, err
			}
			if result.Decision != baseline.Decision {
				decisionChanges++
			}
			if preserves(baseline, result) {
				current = candidate
				accepted++
				acceptedThisRound = true
				granularity = operation.Granularity
				break
			}
		}
		if acceptedThisRound {
			continue
		}
		if granularity >= len(refs) {
			break
		}
		granularity *= 2
		if granularity > len(refs) {
			granularity = len(refs)
		}
	}
	return current, attempts, accepted, decisionChanges, invocations, nil
}

func preserves(baseline, candidate OracleResult) bool {
	return candidate.Stable && candidate.Decision == baseline.Decision && candidate.ReasonDigest == baseline.ReasonDigest
}

func splitChunks(items []string, count int) [][]string {
	if count < 1 {
		count = 1
	}
	chunkSize := (len(items) + count - 1) / count
	chunks := make([][]string, 0, count)
	for start := 0; start < len(items); start += chunkSize {
		end := start + chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, append([]string(nil), items[start:end]...))
	}
	return chunks
}

func nonEmptyPayloads(graph Graph) []payloadRef {
	refs := []payloadRef{}
	if graph.Payload != "" {
		refs = append(refs, payloadRef{kind: "graph"})
	}
	nodes := append([]GraphNode(nil), graph.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	for _, node := range nodes {
		if node.Payload != "" {
			refs = append(refs, payloadRef{kind: "node", id: node.ID})
		}
	}
	edges := append([]GraphEdge(nil), graph.Edges...)
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	for _, edge := range edges {
		if edge.Payload != "" {
			refs = append(refs, payloadRef{kind: "edge", id: edge.ID})
		}
	}
	return refs
}

func splitPayloadChunks(items []payloadRef, count int) [][]payloadRef {
	if count < 1 {
		count = 1
	}
	chunkSize := (len(items) + count - 1) / count
	chunks := make([][]payloadRef, 0, count)
	for start := 0; start < len(items); start += chunkSize {
		end := start + chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, append([]payloadRef(nil), items[start:end]...))
	}
	return chunks
}

func clearPayloads(graph Graph, refs []payloadRef) Graph {
	result := graph.Clone()
	remove := map[payloadRef]bool{}
	for _, ref := range refs {
		remove[ref] = true
	}
	if remove[payloadRef{kind: "graph"}] {
		result.Payload = ""
	}
	for index := range result.Nodes {
		if remove[payloadRef{kind: "node", id: result.Nodes[index].ID}] {
			result.Nodes[index].Payload = ""
		}
	}
	for index := range result.Edges {
		if remove[payloadRef{kind: "edge", id: result.Edges[index].ID}] {
			result.Edges[index].Payload = ""
		}
	}
	return result
}

func evaluateImprovement(report ReductionReport, before *ReductionReport) Improvement {
	result := Improvement{Status: StatusUnknown, Scenario: report.Scenario, SourceDigest: report.SourceDigest, ContractDigest: report.ContractDigest, ToolchainDigest: report.ToolchainDigest, BeforeBytes: report.Before.Bytes, AfterBytes: report.After.Bytes}
	if before == nil {
		result.Unknown = &UnknownRecord{Stage: "improvement", Step: "match_before_after", Reason: "same scenario, source, contract, and toolchain before/after evidence is missing", UnknownClass: "MISSING_MATCHED_PAIR", NextOperation: "provide_a_same_digest_prior_report", BlockedBy: "no_matched_before_after_pair"}
		return result
	}
	if before.Scenario != report.Scenario || before.SourceDigest != report.SourceDigest || before.ContractDigest != report.ContractDigest || before.ToolchainDigest != report.ToolchainDigest {
		result.Unknown = &UnknownRecord{Stage: "improvement", Step: "match_before_after", Reason: "before report does not share scenario, source, contract, and toolchain digests", UnknownClass: "DIGEST_MISMATCH", NextOperation: "collect_a_same_digest_pair", BlockedBy: "unmatched-before-after-evidence"}
		return result
	}
	result.MatchedPair = true
	result.BeforeBytes = before.After.Bytes
	if before.After.Bytes > report.After.Bytes {
		result.Status = StatusClosed
		return result
	}
	result.Status = StatusRefuted
	result.Unknown = nil
	return result
}

func unknownReport(report ReductionReport, step, reason, unknownClass, nextOperation, blockedBy string, before *ReductionReport) ReductionReport {
	report.Status = StatusUnknown
	report.Unknown = &UnknownRecord{Stage: UnknownStage, Step: step, Reason: reason, UnknownClass: unknownClass, NextOperation: nextOperation, BlockedBy: blockedBy}
	report.Improvement = evaluateImprovement(report, before)
	return report
}

func EnsureCallerOutput(path, repoRoot string) error {
	if path == "" || repoRoot == "" {
		return errors.New("caller-owned output and repository root are required")
	}
	out, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, out)
	if err != nil {
		return err
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return errors.New("caller-owned output must be outside the input repository")
	}
	if info, err := os.Stat(out); err == nil && !info.IsDir() {
		return errors.New("caller-owned output path must be a directory")
	}
	return nil
}
