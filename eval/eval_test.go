package eval

import (
	"fmt"
	"testing"
	"time"
)

// baseCtx provides a generic map context for evaluator tests.
func baseCtx() map[string]interface{} {
	return map[string]interface{}{
		"Used":          float64(80),
		"Capacity":      float64(100),
		"Status":        "available",
		"Class":         "large",
		"Encrypted":     true,
		"Configs": []interface{}{
			map[string]interface{}{
				"Name":   "default",
				"Status": "in-sync",
			},
		},
		"Network": map[string]interface{}{
			"Id": "vpc-12345",
			"Subnets": []interface{}{
				map[string]interface{}{"Identifier": "subnet-aaa", "Status": "Active"},
				map[string]interface{}{"Identifier": "subnet-bbb", "Status": "Inactive"},
			},
		},
	}
}

func TestEvaluate(t *testing.T) {
	ctx := baseCtx()

	tests := []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		// ── Literals ──────────────────────────────────────────────────────────
		{"integer literal", "42", float64(42), false},
		{"float literal", "3.14", float64(3.14), false},
		{"string double-quoted", `"hello"`, "hello", false},
		{"string single-quoted", `'hello'`, "hello", false},
		{"bool true", "true", true, false},
		{"bool false", "false", false, false},

		// ── Arithmetic ────────────────────────────────────────────────────────
		{"add integers", "1 + 2", float64(3), false},
		{"subtract", "10 - 3", float64(7), false},
		{"multiply", "4 * 3", float64(12), false},
		{"divide", "10 / 4", float64(2.5), false},
		{"unary minus literal", "-5", float64(-5), false},
		{"unary minus variable", "-Used", float64(-80), false},
		{"mul before add", "2 + 3 * 4", float64(14), false},
		{"paren overrides precedence", "(2 + 3) * 4", float64(20), false},
		{"string concatenation", `"foo" + "bar"`, "foobar", false},
		{"variable in arithmetic", "Capacity * 0.8", float64(80), false},
		{"multi-variable arithmetic", "Capacity - Used", float64(20), false},

		// ── Comparison ────────────────────────────────────────────────────────
		{"eq true", "1 + 2 == 3", true, false},
		{"eq false", "1 + 2 == 4", false, false},
		{"neq strings", `"ala" != "kota"`, true, false},
		{"neq false", `"same" != "same"`, false, false},
		{"diamond neq true", `"ala" <> "kota"`, true, false},
		{"diamond neq false", `"same" <> "same"`, false, false},
		{"lt numbers", "Used < Capacity", true, false},
		{"lt false", "Capacity < Used", false, false},
		{"gt", "Capacity > Used", true, false},
		{"gt false", "Capacity * 0.8 > Used", false, false},
		{"gte equal", "Capacity * 0.8 >= Used", true, false},
		{"lte", "Used <= Capacity", true, false},
		{"string comparison lt", `"apple" < "banana"`, true, false},
		{"string comparison gt", `"zebra" > "apple"`, true, false},
		{"string eq", `Status == "available"`, true, false},
		{"string neq", `Status != "modifying"`, true, false},

		// ── Logical operators ─────────────────────────────────────────────────
		{"and both true", "true && true", true, false},
		{"and left false", "false && true", false, false},
		{"and right false", "true && false", false, false},
		{"or both false", "false || false", false, false},
		{"or left true", "true || false", true, false},
		{"or right true", "false || true", true, false},
		{"not true", "!true", false, false},
		{"not false", "!false", true, false},
		{"not comparison", `!(Status == "available")`, false, false},
		// Short-circuit: right side is never evaluated
		{"and short-circuits on false", "false && UnknownFn()", false, false},
		{"or short-circuits on true", "true || UnknownFn()", true, false},

		// ── Variable / field access ───────────────────────────────────────────
		{"top-level field", "Used", float64(80), false},
		{"bool field", "Encrypted", true, false},
		{"missing field returns undefined", "NoSuchField", undefinedType{}, false},
		{"nested field", "Network.Id", "vpc-12345", false},
		{"double-nested field", "Network.Subnets[0].Status", "Active", false},
		{"array index 0", "Network.Subnets[0].Identifier", "subnet-aaa", false},
		{"array index 1", "Network.Subnets[1].Status", "Inactive", false},
		{"array element of param group", "Configs[0].Status", "in-sync", false},
		{"out-of-bounds index", "Network.Subnets[5]", undefinedType{}, false},
		{"missing nested key", "Network.NoSuchKey", undefinedType{}, false},
		{"chain from missing", "NoSuchField.Child", undefinedType{}, false},

		// ── exists() ─────────────────────────────────────────────────────────
		{"exists present", "exists(Network.Subnets[0])", true, false},
		{"exists present index 1", "exists(Network.Subnets[1])", true, false},
		{"exists out of bounds", "exists(Network.Subnets[5])", false, false},
		{"exists missing nested key", "exists(Network.Missing)", false, false},
		{"exists missing top-level", "exists(NoSuchField)", false, false},
		{"exists in logical", "exists(Encrypted) && Encrypted == true", true, false},
		{"not exists", "!exists(Network.Subnets[5])", true, false},

		// ── len() ─────────────────────────────────────────────────────────────
		{"len array 2", "len(Network.Subnets)", float64(2), false},
		{"len array 1", "len(Configs)", float64(1), false},
		{"len missing field", "len(NoSuchArray)", float64(0), false},
		{"len string", `len("hello")`, float64(5), false},
		{"len empty string", `len("")`, float64(0), false},
		{"len in comparison", "len(Network.Subnets) >= 2", true, false},
		{"len too few subnets", "len(Network.Subnets) < 2", false, false},

		// ── Complex / combined expressions ───────────────────────────────────
		{"storage ratio threshold true", "Used / Capacity > 0.7", true, false},
		{"storage ratio threshold false", "Used / Capacity > 0.9", false, false},
		{"status not available", `Status != "available"`, false, false},
		{"param in-sync check", `Configs[0].Status != "in-sync"`, false, false},
		{"param out-of-sync check", `Configs[0].Status == "pending-reboot"`, false, false},
		{"multi-az disabled", "Encrypted == false", false, false},
		{"subnet inactive check", `!exists(Network.Subnets[1]) || Network.Subnets[1].Status != "Active"`, true, false},
		{"subnet all active", `exists(Network.Subnets[0]) && Network.Subnets[0].Status == "Active"`, true, false},
		{"compound and/or", "Encrypted == true && Used < Capacity", true, false},

		// ── Parse / evaluation errors ─────────────────────────────────────────
		{"div by zero", "1 / 0", nil, true},
		{"unknown function", "unknownFn()", nil, true},
		{"exists wrong arity", "exists()", nil, true},
		{"exists too many args", "exists(Encrypted, Used)", nil, true},
		{"len wrong arity", "len()", nil, true},
		{"add type mismatch", `Used + "str"`, nil, true},
		{"sub type mismatch", `"str" - 1`, nil, true},
		{"compare mixed types", `"abc" < 1`, nil, true},
		{"index non-array", "Used[0]", nil, true},
		{"field on scalar", "Used.X", nil, true},
		{"unterminated string", `"unterminated`, nil, true},
		{"unexpected token", "1 + * 2", nil, true},
		{"unclosed paren", "(1 + 2", nil, true},
		{"trailing token", "1 + 2 3", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.expr, ctx)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Evaluate(%q) = %v; want error", tt.expr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) unexpected error: %v", tt.expr, err)
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tt.want) {
				t.Errorf("Evaluate(%q) = %v (%T); want %v (%T)", tt.expr, got, got, tt.want, tt.want)
			}
		})
	}
}

// TestEvaluateValueInjection explicitly tests the -e / Value pattern.
func TestEvaluateValueInjection(t *testing.T) {
	ctx := baseCtx()

	val, err := Evaluate("Used / Capacity", ctx)
	if err != nil {
		t.Fatalf("global expression error: %v", err)
	}
	if val != float64(0.8) {
		t.Fatalf("global expression = %v; want 0.8", val)
	}

	ctx["Value"] = val

	tests := []struct {
		expr string
		want bool
	}{
		{"Value > 0.9", false},
		{"Value > 0.8", false}, // 0.8 is not > 0.8
		{"Value >= 0.8", true},
		{"Value > 0.7", true},
		{"Value < 0.5", false},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			got, err := Evaluate(tt.expr, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Evaluate(%q) = %v; want %v", tt.expr, got, tt.want)
			}
		})
	}
}

// TestNullHandling tests null literals, null comparisons, and ifnull().
func TestNullHandling(t *testing.T) {
	ctx := map[string]interface{}{
		"PresentField": "hello",
		"NullField":    nil,
		// MissingField is intentionally absent (undefined)
	}

	tests := []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		// ── null literal ─────────────────────────────────────────────────────
		{"null eq null", `null == null`, true, false},
		{"null neq null", `null != null`, false, false},
		{"null eq string", `null == "x"`, false, false},
		{"string neq null", `"x" != null`, true, false},
		{"null eq zero", `null == 0`, false, false},

		// ── nil field (JSON null) ─────────────────────────────────────────────
		{"nil field eq null", `NullField == null`, true, false},
		{"nil field neq null", `NullField != null`, false, false},
		{"nil field neq string", `NullField != "x"`, true, false},

		// ── undefined field treated as null ──────────────────────────────────
		{"undefined eq null", `MissingField == null`, true, false},
		{"undefined neq null", `MissingField != null`, false, false},
		{"present field neq null", `PresentField != null`, true, false},
		{"present field eq null", `PresentField == null`, false, false},

		// ── ifnull() ─────────────────────────────────────────────────────────
		{"ifnull non-null", `ifnull(PresentField, "default")`, "hello", false},
		{"ifnull nil field", `ifnull(NullField, "fallback")`, "fallback", false},
		{"ifnull undefined", `ifnull(MissingField, "fallback")`, "fallback", false},
		{"ifnull null literal", `ifnull(null, 42)`, float64(42), false},
		{"ifnull in expression", `ifnull(NullField, 0) == 0`, true, false},

		// ── ordering null is an error ─────────────────────────────────────────
		{"null lt 1", `null < 1`, nil, true},
		{"null gt 1", `null > 1`, nil, true},
		{"1 lt null", `1 < null`, nil, true},

		// ── ifnull arity errors ───────────────────────────────────────────────
		{"ifnull no args", `ifnull()`, nil, true},
		{"ifnull one arg", `ifnull(null)`, nil, true},
		{"ifnull three args", `ifnull(null, 1, 2)`, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.expr, ctx)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Evaluate(%q) = %v; want error", tt.expr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) unexpected error: %v", tt.expr, err)
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tt.want) {
				t.Errorf("Evaluate(%q) = %v (%T); want %v (%T)", tt.expr, got, got, tt.want, tt.want)
			}
		})
	}
}

// TestContains tests the contains() built-in.
func TestContains(t *testing.T) {
	ctx := map[string]interface{}{
		"Logs": []interface{}{"error", "iam-db-auth-error", "slowquery"},
		"Nums": []interface{}{float64(1), float64(2), float64(3)},
	}

	tests := []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		{"present string", `contains(Logs, "error")`, true, false},
		{"present string middle", `contains(Logs, "iam-db-auth-error")`, true, false},
		{"present string last", `contains(Logs, "slowquery")`, true, false},
		{"absent string", `contains(Logs, "audit")`, false, false},
		{"present number", `contains(Nums, 2)`, true, false},
		{"absent number", `contains(Nums, 5)`, false, false},
		{"all required present", `contains(Logs, "error") && contains(Logs, "slowquery")`, true, false},
		{"one required missing", `contains(Logs, "error") && contains(Logs, "audit")`, false, false},
		{"either present", `contains(Logs, "audit") || contains(Logs, "error")`, true, false},
		{"negation", `!contains(Logs, "audit")`, true, false},
		// error cases
		{"wrong arity", `contains(Logs)`, nil, true},
		{"non-array first arg", `contains("str", "s")`, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.expr, ctx)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Evaluate(%q) = %v; want error", tt.expr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) unexpected error: %v", tt.expr, err)
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tt.want) {
				t.Errorf("Evaluate(%q) = %v (%T); want %v (%T)", tt.expr, got, got, tt.want, tt.want)
			}
		})
	}
}

// TestArrayFunctions tests all(), any(), and count() with the it variable.
func TestArrayFunctions(t *testing.T) {
	ctx := baseCtx()

	tests := []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		// ── all() ────────────────────────────────────────────────────────────
		{"all match", `all(Configs, it.Status == "in-sync")`, true, false},
		{"all no match", `all(Network.Subnets, it.Status == "Active")`, false, false},
		{"all empty array", `all(Configs, it.Status == "x")`, false, false}, // one element, doesn't match
		{"all string truthy", `all(Configs, it.Status)`, true, false},

		// ── any() ────────────────────────────────────────────────────────────
		{"any one match", `any(Network.Subnets, it.Status == "Active")`, true, false},
		{"any no match", `any(Network.Subnets, it.Status == "Unknown")`, false, false},
		{"any all match", `any(Configs, it.Status == "in-sync")`, true, false},

		// ── count() ──────────────────────────────────────────────────────────
		{"count all match", `count(Configs, it.Status == "in-sync")`, float64(1), false},
		{"count partial match", `count(Network.Subnets, it.Status == "Active")`, float64(1), false},
		{"count no match", `count(Network.Subnets, it.Status == "Unknown")`, float64(0), false},
		{"count in comparison", `count(Network.Subnets, it.Status != "Active") > 0`, true, false},

		// ── it does not leak outside the call ────────────────────────────────
		{"it not set before call", `all(Configs, it.Status == "in-sync")`, true, false},

		// ── error cases ──────────────────────────────────────────────────────
		{"all wrong arity", `all(Configs)`, nil, true},
		{"any wrong arity", `any()`, nil, true},
		{"count wrong arity", `count(Configs, it.Status == "x", 3)`, nil, true},
		{"all non-array", `all(Used, it > 0)`, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.expr, ctx)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Evaluate(%q) = %v; want error", tt.expr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) unexpected error: %v", tt.expr, err)
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tt.want) {
				t.Errorf("Evaluate(%q) = %v (%T); want %v (%T)", tt.expr, got, got, tt.want, tt.want)
			}
		})
	}
}

// TestTimeFunctions tests now(), parseTime(), and time comparisons.
func TestTimeFunctions(t *testing.T) {
	// Fix the clock so all "now()" calls return a deterministic value.
	fixed := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	orig := clockNow
	clockNow = func() time.Time { return fixed }
	defer func() { clockNow = orig }()

	ctx := map[string]interface{}{
		// future relative to fixed clock
		"ValidTill": "2027-04-17T11:20:01Z",
		// past relative to fixed clock
		"ExpiredOn": "2025-03-01T00:00:00Z",
		// exactly equal to fixed clock
		"ExactNow": "2026-01-15T12:00:00Z",
	}

	tests := []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		// ── now() ────────────────────────────────────────────────────────────────
		{"now returns time", `now() == now()`, true, false},
		{"now after past date", `now() > parseTime("2020-01-01T00:00:00Z")`, true, false},
		{"now before future date", `now() < parseTime("2030-01-01T00:00:00Z")`, true, false},

		// ── parseTime() ──────────────────────────────────────────────────────────
		{"parseTime future gt now", `parseTime("2027-01-01T00:00:00Z") > now()`, true, false},
		{"parseTime past lt now", `parseTime("2024-01-01T00:00:00Z") < now()`, true, false},
		{"parseTime equality", `parseTime("2026-01-15T12:00:00Z") == now()`, true, false},
		{"parseTime inequality", `parseTime("2026-01-15T12:00:00Z") != parseTime("2026-01-15T13:00:00Z")`, true, false},

		// ── auto-coerce string fields via now() ───────────────────────────────
		{"field future gt now", `ValidTill > now()`, true, false},
		{"field past lt now", `ExpiredOn < now()`, true, false},
		{"field exact eq now", `ExactNow == now()`, true, false},
		{"field expired check", `ExpiredOn > now()`, false, false},

		// ── now() combined with field and logical ops ─────────────────────────
		{"not expired", `!(ExpiredOn > now())`, true, false},
		{"valid and not expired", `ValidTill > now() && !(ExpiredOn > now())`, true, false},

		// ── INTERVAL arithmetic ───────────────────────────────────────────────
		// fixed now = 2026-01-15T12:00:00Z
		{"interval day adds", `now() + INTERVAL 1 DAY > now()`, true, false},
		{"interval day subtracts", `now() - INTERVAL 1 DAY < now()`, true, false},
		{"interval plural days", `now() + INTERVAL 30 DAYS > now()`, true, false},
		{"interval week", `now() + INTERVAL 1 WEEK > now() + INTERVAL 6 DAYS`, true, false},
		{"interval hours", `now() + INTERVAL 24 HOURS > now() + INTERVAL 23 HOURS`, true, false},
		{"interval minutes", `now() + INTERVAL 60 MINUTES > now() + INTERVAL 59 MINUTES`, true, false},
		{"interval seconds", `now() + INTERVAL 3600 SECONDS > now() + INTERVAL 3599 SECONDS`, true, false},
		{"interval month", `now() + INTERVAL 1 MONTH > now()`, true, false},
		{"interval year", `now() + INTERVAL 1 YEAR > now()`, true, false},
		// field vs shifted now: ValidTill="2027-04-17T11:20:01Z", now+1year=2027-01-15
		{"field beyond one year", `ValidTill > now() + INTERVAL 1 YEAR`, true, false},
		// now+2years = 2028-01-15; ValidTill < that
		{"field within two years", `ValidTill < now() + INTERVAL 2 YEARS`, true, false},
		// ExpiredOn="2025-03-01": was it expired more than 10 months ago?
		// now - 10 months = 2025-03-15; ExpiredOn 2025-03-01 < 2025-03-15 → true
		{"field expired over 10 months ago", `ExpiredOn < now() - INTERVAL 10 MONTHS`, true, false},
		// amount can be an additive expression
		{"interval computed amount", `now() + INTERVAL 7 + 1 DAYS > now() + INTERVAL 7 DAYS`, true, false},
		// lowercase unit
		{"interval lowercase unit", `now() + INTERVAL 1 day > now()`, true, false},

		// ── INTERVAL error cases ──────────────────────────────────────────────
		{"interval unknown unit", `now() + INTERVAL 1 FORTNIGHT`, nil, true},
		{"interval non-numeric amount", `now() + INTERVAL "x" DAYS`, nil, true},
		{"interval missing unit", `now() + INTERVAL 1`, nil, true},

		// ── other error cases ─────────────────────────────────────────────────
		{"now wrong arity", `now(1)`, nil, true},
		{"parseTime wrong arity", `parseTime()`, nil, true},
		{"parseTime too many args", `parseTime("a", "b")`, nil, true},
		{"parseTime non-string", `parseTime(42)`, nil, true},
		{"parseTime invalid format", `parseTime("not-a-date")`, nil, true},
		{"compare string non-rfc3339 with now", `"hello" > now()`, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.expr, ctx)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Evaluate(%q) = %v; want error", tt.expr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) unexpected error: %v", tt.expr, err)
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tt.want) {
				t.Errorf("Evaluate(%q) = %v (%T); want %v (%T)", tt.expr, got, got, tt.want, tt.want)
			}
		})
	}
}

// TestOperatorPrecedence checks that precedence rules are applied correctly.
func TestOperatorPrecedence(t *testing.T) {
	ctx := map[string]interface{}{}
	cases := []struct {
		expr string
		want float64
	}{
		{"1 + 2 * 3", 7},
		{"(1 + 2) * 3", 9},
		{"10 - 2 - 3", 5},      // left-associative
		{"10 / 2 * 5", 25},     // left-associative
		{"2 * 3 + 4 * 5", 26},
		{"-2 * 3", -6},
		{"-(2 + 3)", -5},
	}
	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			got, err := Evaluate(c.expr, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("= %v; want %v", got, c.want)
			}
		})
	}
}
