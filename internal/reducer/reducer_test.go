package reducer

import (
	"os"
	"path/filepath"
	"testing"
)

func testMeta(fixtureDigest string) MetaContract {
	return MetaContract{
		Schema: MetaSchema, Authority: "metacode",
		Denominator:   DenominatorDecl{ID: "test", Scenarios: 1, Unit: "semantic_graph"},
		Statuses:      []string{StatusClosed, StatusUnknown, StatusRefuted},
		Precedence:    []string{StatusRefuted, StatusUnknown, StatusClosed},
		UnknownFields: append([]string(nil), requiredUnknownFields...),
		SourcePolicy:  "immutable_digest_or_self_pinned_fixture", Toolchain: ToolchainDecl{Go: "1.27", Digest: "sha256:go1.27"}, OracleReplays: OracleReplays,
		Predicates: []PredicateDecl{
			{Ordinal: 1, ID: "decision", Kind: "decision", Preserve: true}, {Ordinal: 2, ID: "reason-digest", Kind: "reason_digest", Preserve: true},
			{Ordinal: 3, ID: "source-digest", Kind: "source_digest", Preserve: true}, {Ordinal: 4, ID: "contract-digest", Kind: "contract_digest", Preserve: true},
			{Ordinal: 5, ID: "toolchain-digest", Kind: "toolchain_digest", Preserve: true},
		},
		Operations:     []OperationDecl{{Ordinal: 1, ID: "remove-nodes", Kind: "remove_nodes", Target: "nodes", Granularity: 2}, {Ordinal: 2, ID: "remove-edges", Kind: "remove_edges", Target: "edges", Granularity: 2}, {Ordinal: 3, ID: "trim-payload", Kind: "trim_bytes", Target: "payload", Granularity: 2}},
		GenerationPlan: GenerationPlan{Order: []string{"remove-nodes", "remove-edges", "trim-payload"}, Outputs: []string{"runner-manifest.json"}},
		Scenarios:      []ScenarioDecl{{Ordinal: 1, ID: "test-scenario", Fixture: "fixture.gooo", FixtureDigest: fixtureDigest, SourceDigest: "sha256:test-source", ToolchainDigest: "sha256:go1.27", OracleKind: "activity_count_at_least", OracleTarget: "activity", OracleThreshold: 2, ExpectedDecision: StatusRefuted, Reason: "two activities are rejected"}},
	}
}

func TestParserPreservesQuotedReason(t *testing.T) {
	meta, err := parseMeta(`gooo semantic_counterexample_reducer v1
authority metacode
denominator id=test scenarios=1 unit=semantic_graph
decision statuses=CLOSED,UNKNOWN,REFUTED
precedence REFUTED>UNKNOWN>CLOSED
unknown_fields stage,step,reason,unknown_class,next_operation,blocked_by
source_policy mode=immutable_digest_or_self_pinned_fixture
toolchain go=1.27 digest=sha256:go1.27
oracle_replays count=2
predicate ordinal=1 id=decision kind=decision preserve=true
predicate ordinal=2 id=reason-digest kind=reason_digest preserve=true
predicate ordinal=3 id=source-digest kind=source_digest preserve=true
predicate ordinal=4 id=contract-digest kind=contract_digest preserve=true
predicate ordinal=5 id=toolchain-digest kind=toolchain_digest preserve=true
operation ordinal=1 id=remove-nodes kind=remove_nodes target=nodes granularity=2
operation ordinal=2 id=remove-edges kind=remove_edges target=edges granularity=2
operation ordinal=3 id=trim-payload kind=trim_bytes target=payload granularity=2
generation_plan order=remove-nodes,remove-edges,trim-payload output=runner-manifest.json
scenario ordinal=1 id=test-scenario fixture=fixture.gooo fixture_digest=sha256:fixture source_digest=sha256:test-source toolchain_digest=sha256:go1.27 oracle_kind=activity_count_at_least oracle_target=activity oracle_threshold=2 expected_decision=REFUTED reason="two activities are rejected"`)
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Validate(); err != nil {
		t.Fatal(err)
	}
	if meta.Scenarios[0].Reason != "two activities are rejected" {
		t.Fatalf("reason was %q", meta.Scenarios[0].Reason)
	}
}

func TestReductionPreservesFailedOracle(t *testing.T) {
	root := t.TempDir()
	fixturePath := filepath.Join(root, "fixture.gooo")
	fixture := `gooo semantic_graph v1
scenario=test-scenario
source_digest=sha256:test-source
toolchain_digest=sha256:go1.27
payload="removable graph explanation"
node id=a kind=activity label="a" payload="a payload"
node id=b kind=activity label="b" payload="b payload"
node id=note kind=annotation label="note" payload="note payload"
edge id=e from=a to=b kind=depends_on payload="edge payload"
`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := testMeta(Digest([]byte(fixture)))
	runner, err := NewRunner(meta, []byte("test contract"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.ReduceScenario(meta.Scenarios[0], fixturePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != StatusClosed {
		t.Fatalf("status=%s, unknown=%#v", report.Status, report.Unknown)
	}
	if report.Baseline.Decision != StatusRefuted || report.Final.Decision != StatusRefuted || report.Baseline.ReasonDigest != report.Final.ReasonDigest {
		t.Fatalf("oracle was not preserved: %#v", report)
	}
	if report.After.Nodes != 2 || report.After.Edges != 0 || report.After.Bytes >= report.Before.Bytes {
		t.Fatalf("unexpected reduction: before=%#v after=%#v", report.Before, report.After)
	}
	if report.OracleInvocations < OracleReplays || report.CandidateAttempts == 0 {
		t.Fatalf("missing exact oracle accounting: %#v", report)
	}
	if report.Improvement.Status != StatusUnknown || report.Improvement.Unknown == nil {
		t.Fatalf("missing unmatched-pair UNKNOWN: %#v", report.Improvement)
	}
}

func TestUnknownRecordHasSixFields(t *testing.T) {
	record := &UnknownRecord{Stage: "reduction", Step: "oracle", Reason: "unstable", UnknownClass: "ORACLE_UNSTABLE", NextOperation: "retry", BlockedBy: "oracle"}
	if err := validateUnknown(record); err != nil {
		t.Fatal(err)
	}
}
