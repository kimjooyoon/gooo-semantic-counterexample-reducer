package reducer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MetaSchema      = "gooo.semantic_counterexample_reducer/v1"
	GraphSchema     = "gooo.semantic_graph/v1"
	ReportSchema    = "gooo.semantic_counterexample_reduction_report/v1"
	ManifestSchema  = "gooo.semantic_counterexample_runner_manifest/v1"
	StatusClosed    = "CLOSED"
	StatusUnknown   = "UNKNOWN"
	StatusRefuted   = "REFUTED"
	OracleReplays   = 2
	UnknownStage    = "reduction"
	UnknownNextStep = "retry_with_stable_oracle"
	UnknownClass    = "ORACLE_UNSTABLE"
)

var requiredUnknownFields = []string{"stage", "step", "reason", "unknown_class", "next_operation", "blocked_by"}

type MetaContract struct {
	Schema         string
	Authority      string
	Denominator    DenominatorDecl
	Statuses       []string
	Precedence     []string
	UnknownFields  []string
	SourcePolicy   string
	Toolchain      ToolchainDecl
	OracleReplays  int
	Predicates     []PredicateDecl
	Operations     []OperationDecl
	GenerationPlan GenerationPlan
	Scenarios      []ScenarioDecl
}

type DenominatorDecl struct {
	ID        string `json:"id"`
	Scenarios int    `json:"scenarios"`
	Unit      string `json:"unit"`
}

type ToolchainDecl struct {
	Go     string `json:"go"`
	Digest string `json:"digest"`
}

type PredicateDecl struct {
	Ordinal  int    `json:"ordinal"`
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Preserve bool   `json:"preserve"`
}

type OperationDecl struct {
	Ordinal     int    `json:"ordinal"`
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	Granularity int    `json:"granularity"`
}

type GenerationPlan struct {
	Order   []string `json:"order"`
	Outputs []string `json:"outputs"`
}

type ScenarioDecl struct {
	Ordinal          int
	ID               string
	Fixture          string
	FixtureDigest    string
	SourceDigest     string
	ToolchainDigest  string
	OracleKind       string
	OracleTarget     string
	OracleThreshold  int
	OracleKinds      []string
	ExpectedDecision string
	Reason           string
}

type Graph struct {
	Schema          string      `json:"schema"`
	Version         string      `json:"version"`
	Scenario        string      `json:"scenario"`
	SourceDigest    string      `json:"source_digest"`
	ToolchainDigest string      `json:"toolchain_digest"`
	Payload         string      `json:"payload"`
	Nodes           []GraphNode `json:"nodes"`
	Edges           []GraphEdge `json:"edges"`
}

type GraphNode struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Payload string `json:"payload"`
}

type GraphEdge struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Kind    string `json:"kind"`
	Payload string `json:"payload"`
}

type OracleResult struct {
	Decision     string `json:"decision"`
	Reason       string `json:"reason"`
	ReasonDigest string `json:"reason_digest"`
	Stable       bool   `json:"stable"`
	Invocations  int    `json:"invocations"`
}

type Counts struct {
	Nodes int `json:"nodes"`
	Edges int `json:"edges"`
	Bytes int `json:"bytes"`
}

type OperationMeasurement struct {
	OperationID string `json:"operation_id"`
	Kind        string `json:"kind"`
	Attempts    int    `json:"attempts"`
	Accepted    int    `json:"accepted"`
	Before      Counts `json:"before"`
	After       Counts `json:"after"`
}

type UnknownRecord struct {
	Stage         string `json:"stage"`
	Step          string `json:"step"`
	Reason        string `json:"reason"`
	UnknownClass  string `json:"unknown_class"`
	NextOperation string `json:"next_operation"`
	BlockedBy     string `json:"blocked_by"`
}

type Improvement struct {
	Status          string         `json:"status"`
	Scenario        string         `json:"scenario"`
	SourceDigest    string         `json:"source_digest"`
	ContractDigest  string         `json:"contract_digest"`
	ToolchainDigest string         `json:"toolchain_digest"`
	MatchedPair     bool           `json:"matched_pair"`
	BeforeBytes     int            `json:"before_bytes"`
	AfterBytes      int            `json:"after_bytes"`
	Unknown         *UnknownRecord `json:"unknown,omitempty"`
}

type ReductionReport struct {
	Schema              string                 `json:"schema"`
	Authority           string                 `json:"authority"`
	Scenario            string                 `json:"scenario"`
	Fixture             string                 `json:"fixture"`
	FixtureDigest       string                 `json:"fixture_digest"`
	SourceDigest        string                 `json:"source_digest"`
	ContractDigest      string                 `json:"contract_digest"`
	ToolchainDigest     string                 `json:"toolchain_digest"`
	Status              string                 `json:"status"`
	Baseline            OracleResult           `json:"baseline"`
	Final               OracleResult           `json:"final"`
	Before              Counts                 `json:"before"`
	After               Counts                 `json:"after"`
	OracleInvocations   int                    `json:"oracle_invocations"`
	CandidateAttempts   int                    `json:"candidate_attempts"`
	DecisionChanges     int                    `json:"decision_changes"`
	Operations          []OperationMeasurement `json:"operations"`
	PreservedPredicates []string               `json:"preserved_predicates"`
	ReducedGraph        Graph                  `json:"reduced_graph"`
	Improvement         Improvement            `json:"improvement"`
	Unknown             *UnknownRecord         `json:"unknown,omitempty"`
}

type RunnerManifest struct {
	Schema         string          `json:"schema"`
	Authority      string          `json:"authority"`
	ContractDigest string          `json:"contract_digest"`
	Denominator    DenominatorDecl `json:"denominator"`
	Precedence     []string        `json:"precedence"`
	UnknownFields  []string        `json:"unknown_fields"`
	Predicates     []PredicateDecl `json:"predicates"`
	Operations     []OperationDecl `json:"operations"`
	GenerationPlan GenerationPlan  `json:"generation_plan"`
	ScenarioIDs    []string        `json:"scenario_ids"`
	SourcePolicy   string          `json:"source_policy"`
	Toolchain      ToolchainDecl   `json:"toolchain"`
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func DigestString(value string) string { return Digest([]byte(value)) }

func (m MetaContract) Scenario(id string) (ScenarioDecl, error) {
	for _, scenario := range m.Scenarios {
		if scenario.ID == id {
			return scenario, nil
		}
	}
	return ScenarioDecl{}, fmt.Errorf("scenario %q is not declared in .gooo", id)
}

func (m MetaContract) Validate() error {
	if m.Schema != MetaSchema || m.Authority != "metacode" {
		return errors.New(".gooo must declare the reducer schema and metacode authority")
	}
	if m.Denominator.ID == "" || m.Denominator.Scenarios != len(m.Scenarios) || m.Denominator.Unit != "semantic_graph" {
		return errors.New("denominator must match the declared semantic graph scenarios")
	}
	if !sameStrings(m.Statuses, []string{StatusClosed, StatusUnknown, StatusRefuted}) || !sameStrings(m.Precedence, []string{StatusRefuted, StatusUnknown, StatusClosed}) {
		return errors.New("decision statuses or precedence are not declared exactly")
	}
	if !sameStrings(m.UnknownFields, requiredUnknownFields) {
		return errors.New("UNKNOWN must declare the required six fields in order")
	}
	if m.SourcePolicy != "immutable_digest_or_self_pinned_fixture" || m.Toolchain.Go != "1.27" || m.Toolchain.Digest == "" || m.OracleReplays < 2 {
		return errors.New("source, Go toolchain, or oracle replay policy is incomplete")
	}
	if !sameStrings(predicateIDs(m.Predicates), []string{"decision", "reason-digest", "source-digest", "contract-digest", "toolchain-digest"}) {
		return errors.New("preserved semantic predicates are incomplete or out of order")
	}
	for _, predicate := range m.Predicates {
		if !predicate.Preserve || predicate.Kind == "" {
			return fmt.Errorf("predicate %q is not marked preserve", predicate.ID)
		}
	}
	if !sameStrings(operationIDs(m.Operations), []string{"remove-nodes", "remove-edges", "trim-payload"}) || !sameStrings(m.GenerationPlan.Order, operationIDs(m.Operations)) {
		return errors.New("deterministic operation order is not declared exactly")
	}
	for _, operation := range m.Operations {
		if operation.Granularity < 2 || operation.Kind == "" || operation.Target == "" {
			return fmt.Errorf("operation %q is incomplete", operation.ID)
		}
	}
	if len(m.GenerationPlan.Outputs) == 0 {
		return errors.New("generation plan must declare outputs")
	}
	seen := map[string]bool{}
	for _, scenario := range m.Scenarios {
		if seen[scenario.ID] || scenario.ID == "" || scenario.Fixture == "" || scenario.FixtureDigest == "" || strings.Contains(scenario.FixtureDigest, "PLACEHOLDER") || scenario.SourceDigest == "" || scenario.ToolchainDigest == "" || scenario.ExpectedDecision == "" || scenario.Reason == "" {
			return fmt.Errorf("scenario %q is incomplete or duplicated", scenario.ID)
		}
		seen[scenario.ID] = true
		if scenario.ExpectedDecision != StatusRefuted {
			return fmt.Errorf("scenario %q must begin as REFUTED", scenario.ID)
		}
		if scenario.OracleKind != "activity_count_at_least" && scenario.OracleKind != "edge_kinds_present" {
			return fmt.Errorf("scenario %q has unsupported declared oracle kind %q", scenario.ID, scenario.OracleKind)
		}
	}
	return nil
}

func predicateIDs(values []PredicateDecl) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}

func operationIDs(values []OperationDecl) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}

func sameStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}

func (g Graph) Validate() error {
	if g.Schema != GraphSchema || g.Version != "v1" || g.Scenario == "" || g.SourceDigest == "" || g.ToolchainDigest == "" {
		return errors.New("semantic graph header is incomplete")
	}
	seen := map[string]bool{}
	for _, node := range g.Nodes {
		if node.ID == "" || node.Kind == "" || seen[node.ID] {
			return fmt.Errorf("invalid or duplicate node %q", node.ID)
		}
		seen[node.ID] = true
	}
	seenEdges := map[string]bool{}
	for _, edge := range g.Edges {
		if edge.ID == "" || edge.Kind == "" || seenEdges[edge.ID] || !seen[edge.From] || !seen[edge.To] {
			return fmt.Errorf("invalid or dangling edge %q", edge.ID)
		}
		seenEdges[edge.ID] = true
	}
	return nil
}

func (g Graph) Clone() Graph {
	copyGraph := g
	copyGraph.Nodes = append([]GraphNode(nil), g.Nodes...)
	copyGraph.Edges = append([]GraphEdge(nil), g.Edges...)
	return copyGraph
}

func (g Graph) CanonicalBytes() ([]byte, error) {
	copyGraph := g.Clone()
	sort.Slice(copyGraph.Nodes, func(i, j int) bool { return copyGraph.Nodes[i].ID < copyGraph.Nodes[j].ID })
	sort.Slice(copyGraph.Edges, func(i, j int) bool { return copyGraph.Edges[i].ID < copyGraph.Edges[j].ID })
	return json.Marshal(copyGraph)
}

func (g Graph) Digest() (string, error) {
	encoded, err := g.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return Digest(encoded), nil
}

func (g Graph) Counts() (Counts, error) {
	encoded, err := g.CanonicalBytes()
	if err != nil {
		return Counts{}, err
	}
	return Counts{Nodes: len(g.Nodes), Edges: len(g.Edges), Bytes: len(encoded)}, nil
}

func loadBytes(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}
	return os.ReadFile(filepath.Clean(path))
}

func validateUnknown(record *UnknownRecord) error {
	if record == nil || record.Stage == "" || record.Step == "" || record.Reason == "" || record.UnknownClass == "" || record.NextOperation == "" || record.BlockedBy == "" {
		return errors.New("UNKNOWN must preserve stage, step, reason, unknown_class, next_operation, and blocked_by")
	}
	return nil
}
