package eval

import (
	"fmt"
	"testing"
	"time"
)

func baseCtx() map[string]interface{} {
	return map[string]interface{}{
		"Used":          float64(80),
		"Capacity":      float64(100),
		"Status":        "available",
		"Encrypted":     true,
		"Tags":          []interface{}{"aaa", "bbb", "ccc"},
		"Counts":        []interface{}{float64(1), float64(2), float64(3)},
		"Configs": []interface{}{
			map[string]interface{}{
				"Name":   "default",
				"Status": "present",
			},
		},
		"Network": map[string]interface{}{
			"Id": "production",
			"Subnets": []interface{}{
				map[string]interface{}{"Identifier": "subnet-1", "Status": "Active"},
				map[string]interface{}{"Identifier": "subnet-2", "Status": "Inactive"},
			},
		},
	}
}

func runCases(t *testing.T, ctx map[string]interface{}, tests []struct {
	name    string
	expr    string
	want    interface{}
	wantErr bool
}) {
	t.Helper()
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

func TestLiterals(t *testing.T) {
	runCases(t, map[string]interface{}{}, []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		{"integer literal", "42", float64(42), false},
		{"float literal", "3.14", float64(3.14), false},
		{"string double-quoted", `"hello"`, "hello", false},
		{"string single-quoted", `'hello'`, "hello", false},
		{"bool true", "true", true, false},
		{"bool false", "false", false, false},
	})
}

func TestArithmetic(t *testing.T) {
	runCases(t, baseCtx(), []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
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
	})
}

func TestComparison(t *testing.T) {
	runCases(t, baseCtx(), []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
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
	})
}

func TestLogical(t *testing.T) {
	runCases(t, baseCtx(), []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
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
	})
}

func TestFieldAccess(t *testing.T) {
	runCases(t, baseCtx(), []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		{"top-level field", "Used", float64(80), false},
		{"bool field", "Encrypted", true, false},
		{"missing field returns undefined", "NoSuchField", undefinedType{}, false},
		{"nested field", "Network.Id", "production", false},
		{"double-nested field", "Network.Subnets[0].Status", "Active", false},
		{"array index 0", "Network.Subnets[0].Identifier", "subnet-1", false},
		{"array index 1", "Network.Subnets[1].Status", "Inactive", false},
		{"array element of param group", "Configs[0].Status", "present", false},
		{"out-of-bounds index", "Network.Subnets[5]", undefinedType{}, false},
		{"missing nested key", "Network.NoSuchKey", undefinedType{}, false},
		{"chain from missing", "NoSuchField.Child", undefinedType{}, false},
	})
}

func TestExists(t *testing.T) {
	runCases(t, baseCtx(), []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		{"exists present", "exists(Network.Subnets[0])", true, false},
		{"exists present index 1", "exists(Network.Subnets[1])", true, false},
		{"exists out of bounds", "exists(Network.Subnets[5])", false, false},
		{"exists missing nested key", "exists(Network.Missing)", false, false},
		{"exists missing top-level", "exists(NoSuchField)", false, false},
		{"exists in logical", "exists(Encrypted) && Encrypted == true", true, false},
		{"not exists", "!exists(Network.Subnets[5])", true, false},
		{"exists wrong arity", "exists()", nil, true},
		{"exists too many args", "exists(Encrypted, Used)", nil, true},
	})
}

func TestLen(t *testing.T) {
	runCases(t, baseCtx(), []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		{"len array 2", "len(Network.Subnets)", float64(2), false},
		{"len array 1", "len(Configs)", float64(1), false},
		{"len missing field", "len(NoSuchArray)", float64(0), false},
		{"len string", `len("hello")`, float64(5), false},
		{"len empty string", `len("")`, float64(0), false},
		{"len in comparison", "len(Network.Subnets) >= 2", true, false},
		{"len too few subnets", "len(Network.Subnets) < 2", false, false},
		{"len wrong arity", "len()", nil, true},
	})
}

func TestCombinedExprs(t *testing.T) {
	runCases(t, baseCtx(), []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		{"storage ratio threshold true", "Used / Capacity > 0.7", true, false},
		{"storage ratio threshold false", "Used / Capacity > 0.9", false, false},
		{"status not available", `Status != "available"`, false, false},
		{"param loaded check", `Configs[0].Status != "present"`, false, false},
		{"param out-of-sync check", `Configs[0].Status == "not-present"`, false, false},
		{"multi-az disabled", "Encrypted == false", false, false},
		{"subnet inactive check", `!exists(Network.Subnets[1]) || Network.Subnets[1].Status != "Active"`, true, false},
		{"subnet all active", `exists(Network.Subnets[0]) && Network.Subnets[0].Status == "Active"`, true, false},
		{"compound and/or", "Encrypted == true && Used < Capacity", true, false},
	})
}

func TestEvalErrors(t *testing.T) {
	runCases(t, baseCtx(), []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		{"div by zero", "1 / 0", nil, true},
		{"unknown function", "unknownFn()", nil, true},
		{"add type mismatch", `Used + "str"`, nil, true},
		{"sub type mismatch", `"str" - 1`, nil, true},
		{"compare mixed types", `"abc" < 1`, nil, true},
		{"index non-array", "Used[0]", nil, true},
		{"field on scalar", "Used.X", nil, true},
		{"unterminated string", `"unterminated`, nil, true},
		{"unexpected token", "1 + * 2", nil, true},
		{"unclosed paren", "(1 + 2", nil, true},
		{"trailing token", "1 + 2 3", nil, true},
	})
}

// TestNullHandling tests null literals, null comparisons, and ifnull().
func TestNullHandling(t *testing.T) {
	ctx := map[string]interface{}{
		"PresentField": "hello",
		"NullField":    nil,
		// MissingField is intentionally absent (undefined)
	}
	runCases(t, ctx, []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		// null literal
		{"null eq null", `null == null`, true, false},
		{"null neq null", `null != null`, false, false},
		{"null eq string", `null == "x"`, false, false},
		{"string neq null", `"x" != null`, true, false},
		{"null eq zero", `null == 0`, false, false},

		// nil field (JSON null)
		{"nil field eq null", `NullField == null`, true, false},
		{"nil field neq null", `NullField != null`, false, false},
		{"nil field neq string", `NullField != "x"`, true, false},

		// undefined field treated as null
		{"undefined eq null", `MissingField == null`, true, false},
		{"undefined neq null", `MissingField != null`, false, false},
		{"present field neq null", `PresentField != null`, true, false},
		{"present field eq null", `PresentField == null`, false, false},

		// ifnull()
		{"ifnull non-null", `ifnull(PresentField, "default")`, "hello", false},
		{"ifnull nil field", `ifnull(NullField, "fallback")`, "fallback", false},
		{"ifnull undefined", `ifnull(MissingField, "fallback")`, "fallback", false},
		{"ifnull null literal", `ifnull(null, 42)`, float64(42), false},
		{"ifnull in expression", `ifnull(NullField, 0) == 0`, true, false},

		// ordering null is an error
		{"null lt 1", `null < 1`, nil, true},
		{"null gt 1", `null > 1`, nil, true},
		{"1 lt null", `1 < null`, nil, true},

		// ifnull arity errors
		{"ifnull no args", `ifnull()`, nil, true},
		{"ifnull one arg", `ifnull(null)`, nil, true},
		{"ifnull three args", `ifnull(null, 1, 2)`, nil, true},
	})
}

// TestContains tests the contains() built-in.
func TestContains(t *testing.T) {
	runCases(t, baseCtx(), []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		{"present string", `contains(Tags, "aaa")`, true, false},
		{"present string middle", `contains(Tags, "bbb")`, true, false},
		{"present string last", `contains(Tags, "ccc")`, true, false},
		{"absent string", `contains(Tags, "ddd")`, false, false},
		{"present number", `contains(Counts, 2)`, true, false},
		{"absent number", `contains(Counts, 5)`, false, false},
		{"all required present", `contains(Tags, "aaa") && contains(Tags, "ccc")`, true, false},
		{"one required missing", `contains(Tags, "aaa") && contains(Tags, "ddd")`, false, false},
		{"either present", `contains(Tags, "ddd") || contains(Tags, "aaa")`, true, false},
		{"negation", `!contains(Tags, "ddd")`, true, false},
		{"wrong arity", `contains(Tags)`, nil, true},
		{"non-array first arg", `contains("str", "s")`, nil, true},
	})
}

// TestArrayFunctions tests all(), any(), and count() with the it variable.
func TestArrayFunctions(t *testing.T) {
	runCases(t, baseCtx(), []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		// all()
		{"all match", `all(Configs, it.Status == "present")`, true, false},
		{"all no match", `all(Network.Subnets, it.Status == "Active")`, false, false},
		{"all empty array", `all(Configs, it.Status == "x")`, false, false}, // one element, doesn't match
		{"all string truthy", `all(Configs, it.Status)`, true, false},

		// any()
		{"any one match", `any(Network.Subnets, it.Status == "Active")`, true, false},
		{"any no match", `any(Network.Subnets, it.Status == "Unknown")`, false, false},
		{"any all match", `any(Configs, it.Status == "present")`, true, false},

		// count()
		{"count all match", `count(Configs, it.Status == "present")`, float64(1), false},
		{"count partial match", `count(Network.Subnets, it.Status == "Active")`, float64(1), false},
		{"count no match", `count(Network.Subnets, it.Status == "Unknown")`, float64(0), false},
		{"count in comparison", `count(Network.Subnets, it.Status != "Active") > 0`, true, false},

		// it does not leak outside the call
		{"it not set before call", `all(Configs, it.Status == "present")`, true, false},

		// error cases
		{"all wrong arity", `all(Configs)`, nil, true},
		{"any wrong arity", `any()`, nil, true},
		{"count wrong arity", `count(Configs, it.Status == "x", 3)`, nil, true},
		{"all non-array", `all(Used, it > 0)`, nil, true},
	})
}

// TestTimeFunctions tests now(), parseTime(), and time comparisons.
func TestTimeFunctions(t *testing.T) {
	// Fix the clock so all "now()" calls return a deterministic value.
	fixed := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	orig := clockNow
	clockNow = func() time.Time { return fixed }
	defer func() { clockNow = orig }()

	runCases(t, map[string]interface{}{
		"ValidTill": "2027-04-17T11:20:01Z",
		"ExpiredOn": "2025-03-01T00:00:00Z",
		"ExactNow":  fixed.Format(time.RFC3339),
	}, []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		// now()
		{"now returns time", `now() == now()`, true, false},
		{"now after past date", `now() > parseTime("2020-01-01T00:00:00Z")`, true, false},
		{"now before future date", `now() < parseTime("2030-01-01T00:00:00Z")`, true, false},

		// parseTime()
		{"parseTime future gt now", `parseTime("2027-01-01T00:00:00Z") > now()`, true, false},
		{"parseTime past lt now", `parseTime("2024-01-01T00:00:00Z") < now()`, true, false},
		{"parseTime equality", `parseTime("2026-01-15T12:00:00Z") == now()`, true, false},
		{"parseTime inequality", `parseTime("2026-01-15T12:00:00Z") != parseTime("2026-01-15T13:00:00Z")`, true, false},

		// string fields auto-coerced when compared with now()
		{"field future gt now", `ValidTill > now()`, true, false},
		{"field past lt now", `ExpiredOn < now()`, true, false},
		{"field exact eq now", `ExactNow == now()`, true, false},
		{"field expired check", `ExpiredOn > now()`, false, false},
		{"not expired", `!(ExpiredOn > now())`, true, false},
		{"valid and not expired", `ValidTill > now() && !(ExpiredOn > now())`, true, false},

		// INTERVAL arithmetic
		{"interval day adds", `now() + INTERVAL 1 DAY > now()`, true, false},
		{"interval day subtracts", `now() - INTERVAL 1 DAY < now()`, true, false},
		{"interval plural days", `now() + INTERVAL 30 DAYS > now()`, true, false},
		{"interval week", `now() + INTERVAL 1 WEEK > now() + INTERVAL 6 DAYS`, true, false},
		{"interval hours", `now() + INTERVAL 24 HOURS > now() + INTERVAL 23 HOURS`, true, false},
		{"interval minutes", `now() + INTERVAL 60 MINUTES > now() + INTERVAL 59 MINUTES`, true, false},
		{"interval seconds", `now() + INTERVAL 3600 SECONDS > now() + INTERVAL 3599 SECONDS`, true, false},
		{"interval month", `now() + INTERVAL 1 MONTH > now()`, true, false},
		{"interval year", `now() + INTERVAL 1 YEAR > now()`, true, false},
		{"field beyond one year", `ValidTill > now() + INTERVAL 1 YEAR`, true, false},
		{"field within two years", `ValidTill < now() + INTERVAL 2 YEARS`, true, false},
		{"field expired over 10 months ago", `ExpiredOn < now() - INTERVAL 10 MONTHS`, true, false},
		{"interval computed amount", `now() + INTERVAL 7 + 1 DAYS > now() + INTERVAL 7 DAYS`, true, false},
		{"interval lowercase unit", `now() + INTERVAL 1 day > now()`, true, false},

		// INTERVAL error cases
		{"interval unknown unit", `now() + INTERVAL 1 FORTNIGHT`, nil, true},
		{"interval non-numeric amount", `now() + INTERVAL "x" DAYS`, nil, true},
		{"interval missing unit", `now() + INTERVAL 1`, nil, true},

		// other error cases
		{"now wrong arity", `now(1)`, nil, true},
		{"parseTime wrong arity", `parseTime()`, nil, true},
		{"parseTime too many args", `parseTime("a", "b")`, nil, true},
		{"parseTime non-string", `parseTime(42)`, nil, true},
		{"parseTime invalid format", `parseTime("not-a-date")`, nil, true},
		{"compare string non-rfc3339 with now", `"hello" > now()`, nil, true},
	})
}

// TestOperatorPrecedence checks that precedence rules are applied correctly.
func TestOperatorPrecedence(t *testing.T) {
	runCases(t, map[string]interface{}{}, []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		{"1 + 2 * 3", "1 + 2 * 3", float64(7), false},
		{"(1 + 2) * 3", "(1 + 2) * 3", float64(9), false},
		{"10 - 2 - 3", "10 - 2 - 3", float64(5), false},   // left-associative
		{"10 / 2 * 5", "10 / 2 * 5", float64(25), false},  // left-associative
		{"2 * 3 + 4 * 5", "2 * 3 + 4 * 5", float64(26), false},
		{"-2 * 3", "-2 * 3", float64(-6), false},
		{"-(2 + 3)", "-(2 + 3)", float64(-5), false},
	})
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

	runCases(t, ctx, []struct {
		name    string
		expr    string
		want    interface{}
		wantErr bool
	}{
		{"greater than 0.9", "Value > 0.9", false, false},
		{"not greater than 0.8", "Value > 0.8", false, false}, // 0.8 is not > 0.8
		{"greater than or equal 0.8", "Value >= 0.8", true, false},
		{"greater than 0.7", "Value > 0.7", true, false},
		{"less than 0.5", "Value < 0.5", false, false},
	})
}
