package resources

import "testing"

func TestQualify(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple name", "MYSCHEMA", `"MYSCHEMA"`},
		{"two parts", "MYSCHEMA.MYTABLE", `"MYSCHEMA"."MYTABLE"`},
		{"already quoted", `"MYSCHEMA"."MYTABLE"`, `"MYSCHEMA"."MYTABLE"`},
		{"mixed quoting", `"MYSCHEMA".MYTABLE`, `"MYSCHEMA"."MYTABLE"`},
		{"three parts", "A.B.C", `"A"."B"."C"`},
		// NOTE: qualify() does not escape double quotes inside identifiers
		// because isValidIdentifier() passes all non-empty strings and
		// escapeIdentifierLiteral is only called on validation failure.
		// This is a known limitation - see qualify() implementation.
		{"with double quote in name (not escaped)", `MY"TABLE`, `"MY"TABLE"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := qualify(tt.input)
			if got != tt.want {
				t.Errorf("qualify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
