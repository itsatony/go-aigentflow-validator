package aifvalidate

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// exonsOpenDelim is the .exons engine's tag opener. A frontmatter that contains
// it is EXECUTED by the engine before it is decoded, so a static reader cannot
// know what it decodes to.
const exonsOpenDelim = "{~"

// exonsLegacyConfigOpen is the retired JSON config-block opener; the engine
// refuses a document that starts with it.
const exonsLegacyConfigOpen = "{~exons.config~}"

// frontmatterReader is the built-in ExonsInspector: it reads the YAML
// frontmatter the way the engine extracts and decodes it, and judges nothing it
// would need the engine for (see Options.Exons).
type frontmatterReader struct{}

// exonsSpecFields is the subset of the engine's Spec the save-door rules read.
// yaml.v3's non-strict decode, as the engine's ParseYAMLSpec uses, so a key
// this struct does not declare is ignored here exactly as it is there.
type exonsSpecFields struct {
	Execution *struct {
		Provider string `yaml:"provider"`
	} `yaml:"execution"`
	Tools *struct {
		Allow []string `yaml:"allow"`
	} `yaml:"tools"`
	Requirements *struct {
		Resources []ExonsResource `yaml:"resources"`
	} `yaml:"requirements"`
}

func (frontmatterReader) InspectExons(source string) ExonsReport {
	fm, hasFrontmatter, parseErr := extractExonsFrontmatter(source)
	if parseErr != "" {
		return ExonsReport{ParseError: parseErr}
	}
	if !hasFrontmatter || fm == "" {
		// No spec: the engine's Parse yields a nil spec.
		return ExonsReport{SpecRead: true}
	}
	if strings.Contains(fm, exonsOpenDelim) {
		return ExonsReport{}
	}
	var fields exonsSpecFields
	if err := yaml.Unmarshal([]byte(fm), &fields); err != nil {
		return ExonsReport{}
	}
	report := ExonsReport{SpecRead: true, HasSpec: true}
	if fields.Execution != nil {
		report.Provider = fields.Execution.Provider
	}
	if fields.Tools != nil && fields.Tools.Allow != nil {
		report.ToolsAllowDeclared = true
		report.ToolsAllow = fields.Tools.Allow
	}
	if fields.Requirements != nil {
		report.Resources = fields.Requirements.Resources
	}
	return report
}

// extractExonsFrontmatter mirrors the engine's ExtractYAMLFrontmatter: after an
// optional UTF-8 BOM and leading spaces or tabs (not newlines), the document
// must start with "---" and a line break; the frontmatter ends at the next line
// that starts with "---" followed by a line break or the end of the input, and
// is trimmed. parseErr is set where the engine's extraction refuses the
// document outright (the retired config block, an unclosed frontmatter): the
// only parse verdicts the built-in reader can make with certainty.
func extractExonsFrontmatter(source string) (frontmatter string, hasFrontmatter bool, parseErr string) {
	delim := spec.Exons.FrontmatterDelimiter
	start := strings.TrimPrefix(source, "\xef\xbb\xbf")
	start = strings.TrimLeft(start, " \t")
	if strings.HasPrefix(start, exonsLegacyConfigOpen) {
		return "", false, "legacy JSON config block detected; use YAML frontmatter"
	}
	rest, isDelim := strings.CutPrefix(start, delim)
	if !isDelim {
		return "", false, ""
	}
	switch {
	case strings.HasPrefix(rest, "\n"):
		rest = rest[1:]
	case strings.HasPrefix(rest, "\r\n"):
		rest = rest[2:]
	default:
		return "", false, ""
	}
	pos := 0
	for pos <= len(rest) {
		line := rest[pos:]
		if strings.HasPrefix(line, delim) {
			after := line[len(delim):]
			if after == "" || after[0] == '\n' || after[0] == '\r' {
				return strings.TrimSpace(rest[:pos]), true, ""
			}
		}
		nl := strings.IndexByte(line, '\n')
		if nl < 0 {
			break
		}
		pos += nl + 1
	}
	return "", true, "YAML frontmatter not properly closed"
}
