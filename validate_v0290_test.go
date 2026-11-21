package aifvalidate

import "testing"

// v0.29.0 (AIgentFlow v2.917.0): the EUROPE residency code (the EU-27 plus the
// GDPR-safe European countries) is EU-bound for compliance_residency_unenforced.

func v0290Flow(residency, query string) string {
	return "aigentflow_version: '2.0.0'\nname: t\nversion: '1.0.0'\ncompliance:\n  residency: " + residency +
		"\nstart: work\nsteps:\n  work:\n    executor: 'ai://mistral/chat'\n    query:\n      model: m\n      prompt: hi\n" +
		query + "    next:\n      default: 'null'\n"
}

func v0290Unenforced(src string) int {
	r := ValidateFlow(src, Options{})
	n := 0
	for _, w := range r.Warnings {
		if w.Code == codeComplianceResidencyUnenforced {
			n++
		}
	}
	return n
}

func TestV0290EuropeIsEUBound(t *testing.T) {
	for _, residency := range []string{"['EUROPE']", "['EU', 'EUROPE']", "[' europe ']", "['DE', 'Europe']"} {
		if got := v0290Unenforced(v0290Flow(residency, "")); got != 1 {
			t.Errorf("%s beside an unpinned region-lane call: want 1 warning, got %d", residency, got)
		}
	}
	for _, residency := range []string{"['EUROPE', 'US']", "['EUROPEAN']", "['EEA']"} {
		if got := v0290Unenforced(v0290Flow(residency, "")); got != 0 {
			t.Errorf("%s is not EU-bound: want no warning, got %d", residency, got)
		}
	}
	if got := v0290Unenforced(v0290Flow("['EUROPE']", "      region: eu\n")); got != 0 {
		t.Errorf("a pinned call under EUROPE: want no warning, got %d", got)
	}
}
