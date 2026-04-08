package resources

import "testing"

func TestIsValidIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple name", "MY_USER", true},
		{"lowercase", "my_user", true},
		{"with digits", "USER123", true},
		{"with dots", "SCHEMA.TABLE", true},
		{"with spaces", "my user", true},    // quoted identifiers allow spaces
		{"with special chars", "user@domain.com", true}, // quoted identifiers allow special chars
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("isValidIdentifier(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeStringLiteral(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no quotes", "hello", "hello"},
		{"single quote", "it's", "it''s"},
		{"multiple quotes", "it's a 'test'", "it''s a ''test''"},
		{"empty string", "", ""},
		{"double quotes unchanged", `he said "hello"`, `he said "hello"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeStringLiteral(tt.input)
			if got != tt.want {
				t.Errorf("escapeStringLiteral(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeIdentifierLiteral(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no quotes", "MY_TABLE", "MY_TABLE"},
		{"double quote", `my"table`, `my""table`},
		{"multiple double quotes", `a"b"c`, `a""b""c`},
		{"empty string", "", ""},
		{"single quotes unchanged", "it's", "it's"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeIdentifierLiteral(tt.input)
			if got != tt.want {
				t.Errorf("escapeIdentifierLiteral(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeLogSQL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"password with double quotes",
			`CREATE USER "MYUSER" IDENTIFIED BY "secret123"`,
			`CREATE USER "MYUSER" IDENTIFIED BY "***REDACTED***"`,
		},
		{
			"password with single quotes",
			`CREATE USER "MYUSER" IDENTIFIED BY 'secret123'`,
			`CREATE USER "MYUSER" IDENTIFIED BY "***REDACTED***"`,
		},
		{
			"alter user password",
			`ALTER USER "MYUSER" IDENTIFIED BY "newpass"`,
			`ALTER USER "MYUSER" IDENTIFIED BY "***REDACTED***"`,
		},
		{
			"no password",
			`CREATE ROLE "MYROLE"`,
			`CREATE ROLE "MYROLE"`,
		},
		{
			"case insensitive",
			`create user "u" identified by "p"`,
			`create user "u" identified by "***REDACTED***"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeLogSQL(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeLogSQL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
