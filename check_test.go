package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile("testdata/instance.json")
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return m
}

func assertCode(t *testing.T, want, got int, msg string) {
	t.Helper()
	names := map[int]string{OK: "OK", WARNING: "WARNING", CRITICAL: "CRITICAL", UNKNOWN: "UNKNOWN"}
	if got != want {
		t.Errorf("exit code = %s; want %s — msg: %s", names[got], names[want], msg)
	}
}

// ── Exit-code routing ─────────────────────────────────────────────────────────

func TestExitOK(t *testing.T) {
	// warn: 85 > 90 = false; crit: 85 > 92 = false → OK
	code, msg := runCheck(fixture(t), "", "AllocatedStorage > 90", "AllocatedStorage > 92")
	assertCode(t, OK, code, msg)
}

func TestExitWarning(t *testing.T) {
	// warn: 85 > 80 = true; crit: 85 > 90 = false → WARNING
	code, msg := runCheck(fixture(t), "", "AllocatedStorage > 80", "AllocatedStorage > 90")
	assertCode(t, WARNING, code, msg)
}

func TestExitCritical(t *testing.T) {
	// crit: 85 > 80 = true → CRITICAL
	code, msg := runCheck(fixture(t), "", "", "AllocatedStorage > 80")
	assertCode(t, CRITICAL, code, msg)
}

func TestCriticalBeatsWarning(t *testing.T) {
	// warn: 85 > 70 = true; crit: 85 > 80 = true → CRITICAL wins
	code, msg := runCheck(fixture(t), "", "AllocatedStorage > 70", "AllocatedStorage > 80")
	assertCode(t, CRITICAL, code, msg)
}

func TestOnlyWarnSpecified(t *testing.T) {
	// only -w set, triggers → must not exceed WARNING
	code, msg := runCheck(fixture(t), "", "AllocatedStorage > 80", "")
	assertCode(t, WARNING, code, msg)
}

func TestOnlyWarnNotTriggered(t *testing.T) {
	// 85 > 90 = false → OK
	code, msg := runCheck(fixture(t), "", "AllocatedStorage > 90", "")
	assertCode(t, OK, code, msg)
}

// ── Global expression (-e / Value) ───────────────────────────────────────────

func TestGlobalExprOK(t *testing.T) {
	// ratio = 0.85; > 0.9 = false → OK
	code, msg := runCheck(fixture(t), "AllocatedStorage / MaxAllocatedStorage", "Value > 0.9", "")
	assertCode(t, OK, code, msg)
}

func TestGlobalExprWarning(t *testing.T) {
	// ratio = 0.85; warn > 0.8 = true; crit > 0.9 = false → WARNING
	code, msg := runCheck(fixture(t),
		"AllocatedStorage / MaxAllocatedStorage", "Value > 0.8", "Value > 0.9")
	assertCode(t, WARNING, code, msg)
}

func TestGlobalExprCritical(t *testing.T) {
	// ratio = 0.85; warn > 0.7 = true; crit > 0.8 = true → CRITICAL
	code, msg := runCheck(fixture(t),
		"AllocatedStorage / MaxAllocatedStorage", "Value > 0.7", "Value > 0.8")
	assertCode(t, CRITICAL, code, msg)
}

func TestGlobalExprValueSuffix(t *testing.T) {
	// ratio = 85/100 = 0.85 → message must contain (Value=0.85)
	_, msg := runCheck(fixture(t), "AllocatedStorage / MaxAllocatedStorage", "Value > 0.9", "")
	if !strings.Contains(msg, "(Value=0.85)") {
		t.Errorf("msg = %q; want (Value=0.85)", msg)
	}
}

// ── Direct threshold (no global expression) ──────────────────────────────────

func TestDirectThresholdOK(t *testing.T) {
	// 90 < 85 = false → OK
	code, msg := runCheck(fixture(t), "", "", "MaxAllocatedStorage * 0.9 < AllocatedStorage")
	assertCode(t, OK, code, msg)
}

func TestDirectThresholdWarning(t *testing.T) {
	// warn: 80 < 85 = true; crit: 90 < 85 = false → WARNING
	code, msg := runCheck(fixture(t), "",
		"MaxAllocatedStorage * 0.8 < AllocatedStorage",
		"MaxAllocatedStorage * 0.9 < AllocatedStorage")
	assertCode(t, WARNING, code, msg)
}

// ── Parameter group expressions ───────────────────────────────────────────────

func TestParamGroupInSync(t *testing.T) {
	// "in-sync" != "in-sync" = false → OK
	code, msg := runCheck(fixture(t), "", "",
		`DBParameterGroups[0].ParameterApplyStatus != "in-sync"`)
	assertCode(t, OK, code, msg)
}

func TestParamGroupStatusTriggers(t *testing.T) {
	// "in-sync" == "in-sync" = true → CRITICAL (confirms array index + string match works)
	code, msg := runCheck(fixture(t), "", "",
		`DBParameterGroups[0].ParameterApplyStatus == "in-sync"`)
	assertCode(t, CRITICAL, code, msg)
}

// ── Subnet expressions ────────────────────────────────────────────────────────

func TestSubnetFirstSlotActive(t *testing.T) {
	// Subnets[0]="Active" → "Active" != "Active" = false → OK
	code, msg := runCheck(fixture(t), "",
		`DBSubnetGroup.Subnets[0].SubnetStatus != "Active"`, "")
	assertCode(t, OK, code, msg)
}

func TestSubnetSecondSlotInactive(t *testing.T) {
	// Subnets[1]="Inactive" → "Inactive" != "Active" = true → WARNING
	code, msg := runCheck(fixture(t), "",
		`DBSubnetGroup.Subnets[1].SubnetStatus != "Active"`, "")
	assertCode(t, WARNING, code, msg)
}

func TestSubnetExistsAndInactive(t *testing.T) {
	// exists([1])=true AND "Inactive"!="Active"=true → WARNING
	code, msg := runCheck(fixture(t), "",
		`!exists(DBSubnetGroup.Subnets[1]) || DBSubnetGroup.Subnets[1].SubnetStatus != "Active"`, "")
	assertCode(t, WARNING, code, msg)
}

func TestSubnetMissingSlotWarns(t *testing.T) {
	// exists(Subnets[5])=false → !false=true → WARNING
	code, msg := runCheck(fixture(t), "", "!exists(DBSubnetGroup.Subnets[5])", "")
	assertCode(t, WARNING, code, msg)
}

func TestSubnetCountOK(t *testing.T) {
	// len=2; 2 < 2 = false → OK
	code, msg := runCheck(fixture(t), "", "len(DBSubnetGroup.Subnets) < 2", "")
	assertCode(t, OK, code, msg)
}

func TestSubnetCountTooLow(t *testing.T) {
	// len=2; 2 < 3 = true → WARNING
	code, msg := runCheck(fixture(t), "", "len(DBSubnetGroup.Subnets) < 3", "")
	assertCode(t, WARNING, code, msg)
}

// ── Boolean and numeric field expressions ────────────────────────────────────

func TestMultiAZEnabled(t *testing.T) {
	// true == false = false → OK
	code, msg := runCheck(fixture(t), "", "", "MultiAZ == false")
	assertCode(t, OK, code, msg)
}

func TestMultiAZEnabledTriggers(t *testing.T) {
	// !true = false... test inverse: MultiAZ == true = true → CRITICAL
	code, msg := runCheck(fixture(t), "", "", "MultiAZ == true")
	assertCode(t, CRITICAL, code, msg)
}

func TestStorageEncryptedOK(t *testing.T) {
	// !true = false → OK
	code, msg := runCheck(fixture(t), "", "", "!StorageEncrypted")
	assertCode(t, OK, code, msg)
}

func TestBackupRetentionOK(t *testing.T) {
	// 7 < 7 = false → OK
	code, msg := runCheck(fixture(t), "", "", "BackupRetentionPeriod < 7")
	assertCode(t, OK, code, msg)
}

func TestBackupRetentionWarning(t *testing.T) {
	// 7 < 8 = true → WARNING
	code, msg := runCheck(fixture(t), "", "BackupRetentionPeriod < 8", "")
	assertCode(t, WARNING, code, msg)
}

func TestInstanceStatusOK(t *testing.T) {
	// "available" != "available" = false → OK
	code, msg := runCheck(fixture(t), "", "", `DBInstanceStatus != "available"`)
	assertCode(t, OK, code, msg)
}

func TestInstanceStatusTriggers(t *testing.T) {
	// "available" != "modifying" = true → CRITICAL
	code, msg := runCheck(fixture(t), "", "", `DBInstanceStatus != "modifying"`)
	assertCode(t, CRITICAL, code, msg)
}

// ── Error handling ────────────────────────────────────────────────────────────

func TestGlobalExprError(t *testing.T) {
	code, msg := runCheck(fixture(t), "1 / 0", "Value > 0", "")
	assertCode(t, UNKNOWN, code, msg)
	if !strings.Contains(msg, "-e expression error") {
		t.Errorf("msg = %q; want -e expression error", msg)
	}
}

func TestCritExprError(t *testing.T) {
	// Returns a number, not a bool
	code, msg := runCheck(fixture(t), "", "", "AllocatedStorage + 1")
	assertCode(t, UNKNOWN, code, msg)
	if !strings.Contains(msg, "-c expression error") {
		t.Errorf("msg = %q; want -c expression error", msg)
	}
}

func TestWarnExprError(t *testing.T) {
	code, msg := runCheck(fixture(t), "", "AllocatedStorage + 1", "")
	assertCode(t, UNKNOWN, code, msg)
	if !strings.Contains(msg, "-w expression error") {
		t.Errorf("msg = %q; want -w expression error", msg)
	}
}

// ── Output message format ─────────────────────────────────────────────────────

func TestOKMessage(t *testing.T) {
	// 85 > 90 = false → OK with plain message
	_, msg := runCheck(fixture(t), "", "AllocatedStorage > 90", "")
	if msg != "check passed" {
		t.Errorf("msg = %q; want %q", msg, "check passed")
	}
}

func TestOKMessageWithValueSuffix(t *testing.T) {
	// ratio = 0.85; > 0.9 = false → OK (Value=0.85)
	_, msg := runCheck(fixture(t), "AllocatedStorage / MaxAllocatedStorage", "Value > 0.9", "")
	if !strings.HasPrefix(msg, "check passed") || !strings.Contains(msg, "(Value=0.85)") {
		t.Errorf("msg = %q; want \"check passed (Value=0.85)\"", msg)
	}
}

func TestCritMessageContainsExpression(t *testing.T) {
	critExpr := "AllocatedStorage > 80"
	_, msg := runCheck(fixture(t), "", "", critExpr)
	if !strings.Contains(msg, critExpr) {
		t.Errorf("msg = %q; want to contain %q", msg, critExpr)
	}
}

func TestWarnMessageContainsExpression(t *testing.T) {
	warnExpr := "AllocatedStorage > 80"
	_, msg := runCheck(fixture(t), "", warnExpr, "")
	if !strings.Contains(msg, warnExpr) {
		t.Errorf("msg = %q; want to contain %q", msg, warnExpr)
	}
}
