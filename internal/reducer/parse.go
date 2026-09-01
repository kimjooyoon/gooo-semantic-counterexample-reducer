package reducer

import (
	"fmt"
	"strconv"
	"strings"
)

func LoadMeta(path string) (MetaContract, []byte, error) {
	raw, err := loadBytes(path)
	if err != nil {
		return MetaContract{}, nil, err
	}
	meta, err := parseMeta(string(raw))
	if err != nil {
		return MetaContract{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := meta.Validate(); err != nil {
		return MetaContract{}, nil, err
	}
	return meta, raw, nil
}

func parseMeta(input string) (MetaContract, error) {
	var meta MetaContract
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	for lineNumber, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		tokens, err := tokenize(line)
		if err != nil {
			return MetaContract{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if len(tokens) == 0 {
			continue
		}
		switch tokens[0] {
		case "gooo":
			if len(tokens) != 3 || tokens[1] != "semantic_counterexample_reducer" || tokens[2] != "v1" {
				return MetaContract{}, fmt.Errorf("line %d: invalid header", lineNumber+1)
			}
			meta.Schema = MetaSchema
		case "authority":
			if len(tokens) != 2 {
				return MetaContract{}, fmt.Errorf("line %d: invalid authority", lineNumber+1)
			}
			meta.Authority = tokens[1]
		case "denominator":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return MetaContract{}, lineError(lineNumber, err)
			}
			meta.Denominator = DenominatorDecl{ID: values["id"], Scenarios: mustInt(values["scenarios"]), Unit: values["unit"]}
		case "decision":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return MetaContract{}, lineError(lineNumber, err)
			}
			meta.Statuses = splitCSV(values["statuses"])
		case "precedence":
			if len(tokens) != 2 {
				return MetaContract{}, fmt.Errorf("line %d: invalid precedence", lineNumber+1)
			}
			meta.Precedence = strings.Split(tokens[1], ">")
		case "unknown_fields":
			if len(tokens) != 2 {
				return MetaContract{}, fmt.Errorf("line %d: invalid unknown_fields", lineNumber+1)
			}
			meta.UnknownFields = strings.Split(tokens[1], ",")
		case "source_policy":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return MetaContract{}, lineError(lineNumber, err)
			}
			meta.SourcePolicy = values["mode"]
		case "toolchain":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return MetaContract{}, lineError(lineNumber, err)
			}
			meta.Toolchain = ToolchainDecl{Go: values["go"], Digest: values["digest"]}
		case "oracle_replays":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return MetaContract{}, lineError(lineNumber, err)
			}
			meta.OracleReplays = mustInt(values["count"])
		case "predicate":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return MetaContract{}, lineError(lineNumber, err)
			}
			meta.Predicates = append(meta.Predicates, PredicateDecl{Ordinal: mustInt(values["ordinal"]), ID: values["id"], Kind: values["kind"], Preserve: mustBool(values["preserve"])})
		case "operation":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return MetaContract{}, lineError(lineNumber, err)
			}
			meta.Operations = append(meta.Operations, OperationDecl{Ordinal: mustInt(values["ordinal"]), ID: values["id"], Kind: values["kind"], Target: values["target"], Granularity: mustInt(values["granularity"])})
		case "generation_plan":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return MetaContract{}, lineError(lineNumber, err)
			}
			meta.GenerationPlan = GenerationPlan{Order: splitCSV(values["order"]), Outputs: splitCSV(values["output"])}
		case "scenario":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return MetaContract{}, lineError(lineNumber, err)
			}
			meta.Scenarios = append(meta.Scenarios, ScenarioDecl{
				Ordinal: mustInt(values["ordinal"]), ID: values["id"], Fixture: values["fixture"], FixtureDigest: values["fixture_digest"],
				SourceDigest: values["source_digest"], ToolchainDigest: values["toolchain_digest"], OracleKind: values["oracle_kind"], OracleTarget: values["oracle_target"],
				OracleThreshold: mustInt(values["oracle_threshold"]), OracleKinds: splitCSV(values["oracle_kinds"]), ExpectedDecision: values["expected_decision"], Reason: values["reason"],
			})
		default:
			return MetaContract{}, fmt.Errorf("line %d: unknown declaration %q", lineNumber+1, tokens[0])
		}
	}
	return meta, nil
}

func LoadGraph(path string) (Graph, []byte, error) {
	raw, err := loadBytes(path)
	if err != nil {
		return Graph{}, nil, err
	}
	graph, err := parseGraph(string(raw))
	if err != nil {
		return Graph{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := graph.Validate(); err != nil {
		return Graph{}, nil, err
	}
	return graph, raw, nil
}

func parseGraph(input string) (Graph, error) {
	var graph Graph
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	for lineNumber, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		tokens, err := tokenize(line)
		if err != nil {
			return Graph{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if len(tokens) == 0 {
			continue
		}
		if strings.Contains(tokens[0], "=") {
			values, err := keyValues(tokens)
			if err != nil {
				return Graph{}, lineError(lineNumber, err)
			}
			if len(values) != 1 {
				return Graph{}, fmt.Errorf("line %d: expected one graph assignment", lineNumber+1)
			}
			for key, value := range values {
				switch key {
				case "scenario":
					graph.Scenario = value
				case "source_digest":
					graph.SourceDigest = value
				case "toolchain_digest":
					graph.ToolchainDigest = value
				case "payload":
					graph.Payload = value
				default:
					return Graph{}, fmt.Errorf("line %d: unknown graph assignment %q", lineNumber+1, key)
				}
			}
			continue
		}
		switch tokens[0] {
		case "gooo":
			if len(tokens) != 3 || tokens[1] != "semantic_graph" || tokens[2] != "v1" {
				return Graph{}, fmt.Errorf("line %d: invalid graph header", lineNumber+1)
			}
			graph.Schema, graph.Version = GraphSchema, "v1"
		case "scenario", "source_digest", "toolchain_digest", "payload":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return Graph{}, lineError(lineNumber, err)
			}
			if len(values) != 1 {
				return Graph{}, fmt.Errorf("line %d: expected one graph assignment", lineNumber+1)
			}
			for key, value := range values {
				switch key {
				case "scenario":
					graph.Scenario = value
				case "source_digest":
					graph.SourceDigest = value
				case "toolchain_digest":
					graph.ToolchainDigest = value
				case "payload":
					graph.Payload = value
				}
			}
		case "node":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return Graph{}, lineError(lineNumber, err)
			}
			graph.Nodes = append(graph.Nodes, GraphNode{ID: values["id"], Kind: values["kind"], Label: values["label"], Payload: values["payload"]})
		case "edge":
			values, err := keyValues(tokens[1:])
			if err != nil {
				return Graph{}, lineError(lineNumber, err)
			}
			graph.Edges = append(graph.Edges, GraphEdge{ID: values["id"], From: values["from"], To: values["to"], Kind: values["kind"], Payload: values["payload"]})
		default:
			return Graph{}, fmt.Errorf("line %d: unknown graph declaration %q", lineNumber+1, tokens[0])
		}
	}
	return graph, nil
}

func tokenize(line string) ([]string, error) {
	var tokens []string
	for index := 0; index < len(line); {
		for index < len(line) && (line[index] == ' ' || line[index] == '\t') {
			index++
		}
		if index == len(line) {
			break
		}
		var token strings.Builder
		for index < len(line) && line[index] != ' ' && line[index] != '\t' {
			if line[index] != '"' {
				token.WriteByte(line[index])
				index++
				continue
			}
			start := index
			index++
			for index < len(line) {
				if line[index] == '\\' {
					index += 2
					continue
				}
				if line[index] == '"' {
					index++
					break
				}
				index++
			}
			if index > len(line) || line[index-1] != '"' {
				return nil, fmt.Errorf("unterminated quoted value")
			}
			value, err := strconv.Unquote(line[start:index])
			if err != nil {
				return nil, fmt.Errorf("invalid quoted value: %w", err)
			}
			token.WriteString(value)
		}
		tokens = append(tokens, token.String())
	}
	return tokens, nil
}

func keyValues(tokens []string) (map[string]string, error) {
	values := make(map[string]string, len(tokens))
	for _, token := range tokens {
		parts := strings.SplitN(token, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("expected key=value, got %q", token)
		}
		if _, exists := values[parts[0]]; exists {
			return nil, fmt.Errorf("duplicate key %q", parts[0])
		}
		values[parts[0]] = parts[1]
	}
	return values, nil
}

func lineError(lineNumber int, err error) error { return fmt.Errorf("line %d: %w", lineNumber+1, err) }

func mustInt(value string) int { result, _ := strconv.Atoi(value); return result }

func mustBool(value string) bool { result, _ := strconv.ParseBool(value); return result }

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}
