package editor

import (
	"strings"
	"unicode"

	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

type tokenKind int

const (
	tkPlain tokenKind = iota
	tkKeyword
	tkDML
	tkFunction
	tkOperator
	tkString
	tkNumber
	tkComment
)

type token struct {
	kind tokenKind
	text string
}

var sqlKeywords = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true, "JOIN": true,
	"LEFT": true, "RIGHT": true, "INNER": true, "OUTER": true, "FULL": true,
	"CROSS": true, "ON": true, "AS": true, "DISTINCT": true,
	"GROUP": true, "BY": true, "ORDER": true, "HAVING": true,
	"LIMIT": true, "OFFSET": true, "UNION": true, "ALL": true,
	"WITH": true, "RECURSIVE": true, "INTO": true,
	"SET": true, "VALUES": true, "RETURNING": true,
	"CASE": true, "WHEN": true, "THEN": true, "ELSE": true, "END": true,
	"EXISTS": true, "ANY": true, "SOME": true, "UNIQUE": true,
	"PRIMARY": true, "KEY": true, "FOREIGN": true, "REFERENCES": true,
	"DEFAULT": true, "NOT": true, "CONSTRAINT": true, "INDEX": true,
	"USING": true, "WINDOW": true, "OVER": true, "PARTITION": true,
	"ROWS": true, "RANGE": true, "BETWEEN": true, "UNBOUNDED": true,
	"PRECEDING": true, "FOLLOWING": true, "CURRENT": true, "ROW": true,
	"ASC": true, "DESC": true, "NULLS": true, "FIRST": true, "LAST": true,
	"LATERAL": true, "NATURAL": true, "EXCEPT": true, "INTERSECT": true,
	"FETCH": true, "NEXT": true, "ONLY": true,
}

var sqlDML = map[string]bool{
	"INSERT": true, "UPDATE": true, "DELETE": true,
	"CREATE": true, "DROP": true, "ALTER": true, "TRUNCATE": true,
	"REPLACE": true, "MERGE": true, "UPSERT": true,
	"TABLE": true, "VIEW": true, "DATABASE": true, "SCHEMA": true,
	"SEQUENCE": true, "FUNCTION": true, "PROCEDURE": true, "TRIGGER": true,
	"INDEX": true, "IF": true, "TEMP": true, "TEMPORARY": true,
	"ADD": true, "COLUMN": true, "RENAME": true, "MODIFY": true,
	"BEGIN": true, "COMMIT": true, "ROLLBACK": true, "TRANSACTION": true,
	"GRANT": true, "REVOKE": true, "PRIVILEGES": true,
}

var sqlFunctions = map[string]bool{
	"COUNT": true, "SUM": true, "AVG": true, "MIN": true, "MAX": true,
	"COALESCE": true, "NULLIF": true, "CAST": true, "CONVERT": true,
	"ISNULL": true, "IFNULL": true, "NVL": true,
	"NOW": true, "CURRENT_TIMESTAMP": true, "CURRENT_DATE": true,
	"CURRENT_TIME": true, "DATE": true, "TIME": true,
	"EXTRACT": true, "DATE_PART": true, "DATE_TRUNC": true,
	"UPPER": true, "LOWER": true, "TRIM": true, "LTRIM": true, "RTRIM": true,
	"LENGTH": true, "CHAR_LENGTH": true, "SUBSTRING": true, "SUBSTR": true,
	"REPLACE": true, "CONCAT": true, "STRING_AGG": true, "GROUP_CONCAT": true,
	"ARRAY_AGG": true, "JSON_AGG": true, "JSONB_AGG": true,
	"ROW_NUMBER": true, "RANK": true, "DENSE_RANK": true, "NTILE": true,
	"LAG": true, "LEAD": true, "FIRST_VALUE": true, "LAST_VALUE": true,
	"ABS": true, "CEIL": true, "FLOOR": true, "ROUND": true, "TRUNC": true,
	"MOD": true, "POWER": true, "SQRT": true, "RANDOM": true,
	"GENERATE_SERIES": true, "UNNEST": true, "TO_CHAR": true, "TO_DATE": true,
	"TO_TIMESTAMP": true, "TO_NUMBER": true,
	"ARRAY": true, "STRING": true, "BOOL": true, "INT": true, "FLOAT": true,
}

var sqlOperatorWords = map[string]bool{
	"AND": true, "OR": true, "NOT": true, "IN": true, "IS": true,
	"LIKE": true, "ILIKE": true, "BETWEEN": true, "SIMILAR": true,
	"NULL": true, "TRUE": true, "FALSE": true,
}

func tokenizeSQL(sql string) []token {
	runes := []rune(sql)
	n := len(runes)
	var tokens []token
	i := 0

	for i < n {
		switch {
		case runes[i] == '\'':
			j := i + 1
			for j < n {
				if runes[j] == '\'' {
					j++
					if j < n && runes[j] == '\'' {
						j++
						continue
					}
					break
				}
				j++
			}
			tokens = append(tokens, token{tkString, string(runes[i:j])})
			i = j

		case runes[i] == '"':
			j := i + 1
			for j < n && runes[j] != '"' {
				j++
			}
			if j < n {
				j++
			}
			tokens = append(tokens, token{tkPlain, string(runes[i:j])})
			i = j

		case runes[i] == '-' && i+1 < n && runes[i+1] == '-':
			j := i + 2
			for j < n && runes[j] != '\n' {
				j++
			}
			tokens = append(tokens, token{tkComment, string(runes[i:j])})
			i = j

		case runes[i] == '/' && i+1 < n && runes[i+1] == '*':
			j := i + 2
			for j+1 < n && !(runes[j] == '*' && runes[j+1] == '/') {
				j++
			}
			if j+1 < n {
				j += 2
			}
			tokens = append(tokens, token{tkComment, string(runes[i:j])})
			i = j

		case unicode.IsDigit(runes[i]):
			j := i + 1
			for j < n && (unicode.IsDigit(runes[j]) || runes[j] == '.' || runes[j] == '_') {
				j++
			}
			if j < n && (runes[j] == 'e' || runes[j] == 'E') {
				j++
				if j < n && (runes[j] == '+' || runes[j] == '-') {
					j++
				}
				for j < n && unicode.IsDigit(runes[j]) {
					j++
				}
			}
			tokens = append(tokens, token{tkNumber, string(runes[i:j])})
			i = j

		case isSymbolOp(runes[i]):
			j := i + 1
			if i+1 < n && isSymbolOp(runes[i+1]) {
				combined := string(runes[i : i+2])
				if combined == "<>" || combined == ">=" || combined == "<=" || combined == "!=" || combined == "::" {
					j = i + 2
				}
			}
			tokens = append(tokens, token{tkOperator, string(runes[i:j])})
			i = j

		case unicode.IsLetter(runes[i]) || runes[i] == '_':
			j := i + 1
			for j < n && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j]) || runes[j] == '_') {
				j++
			}
			word := string(runes[i:j])
			upper := strings.ToUpper(word)
			kind := classifyWord(upper)
			tokens = append(tokens, token{kind, word})
			i = j

		default:
			j := i + 1
			for j < n && !isTokenStart(runes[j]) {
				j++
			}
			tokens = append(tokens, token{tkPlain, string(runes[i:j])})
			i = j
		}
	}

	return tokens
}

func classifyWord(upper string) tokenKind {
	if sqlOperatorWords[upper] {
		return tkOperator
	}
	if sqlDML[upper] {
		return tkDML
	}
	if sqlKeywords[upper] {
		return tkKeyword
	}
	if sqlFunctions[upper] {
		return tkFunction
	}
	return tkPlain
}

func isSymbolOp(r rune) bool {
	switch r {
	case '=', '<', '>', '!', '+', '-', '*', '/', '%', '(', ')', ',', ';', '.', ':', '[', ']', '{', '}', '|', '&', '^', '~':
		return true
	}
	return false
}

func isTokenStart(r rune) bool {
	return r == '\'' || r == '"' || r == '-' || r == '/' ||
		unicode.IsDigit(r) || unicode.IsLetter(r) || r == '_' ||
		isSymbolOp(r)
}

// HighlightSQL tokenises sql and applies lipgloss syntax styles from s.
// The returned string contains ANSI escape codes and reconstructs the original
// text exactly (joining all tokens produces the input unchanged).
func HighlightSQL(sql string, s styles.Styles) string {
	if sql == "" {
		return ""
	}
	tokens := tokenizeSQL(sql)
	var sb strings.Builder
	sb.Grow(len(sql) * 2)
	for _, tok := range tokens {
		switch tok.kind {
		case tkKeyword:
			sb.WriteString(s.SynKeyword.Render(tok.text))
		case tkDML:
			sb.WriteString(s.SynDML.Render(tok.text))
		case tkFunction:
			sb.WriteString(s.SynFunction.Render(tok.text))
		case tkOperator:
			sb.WriteString(s.SynOperator.Render(tok.text))
		case tkString:
			sb.WriteString(s.SynString.Render(tok.text))
		case tkNumber:
			sb.WriteString(s.SynNumber.Render(tok.text))
		case tkComment:
			sb.WriteString(s.SynComment.Render(tok.text))
		default:
			sb.WriteString(tok.text)
		}
	}
	return sb.String()
}
