package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
)

var (
	Version    = "dev"
	CommitHash = "unknown"
	BuildTime  = "unknown"
)

const (
	OK       = 0
	WARNING  = 1
	CRITICAL = 2
	UNKNOWN  = 3
)

func main() {
	version    := flag.Bool("v", false, "Print version information and exit")
	ident      := flag.String("i", "", "Instance identifier (required)")
	exprGlobal := flag.String("e", "", "Global expression; result is injected as 'Value' into -w/-c")
	warnExpr   := flag.String("w", "", "Warning expression - must evaluate to bool")
	critExpr   := flag.String("c", "", "Critical expression - must evaluate to bool")

	flag.Parse()

	if *version {
		fmt.Printf("%s %s (%s) date=%s\n",
			filepath.Base(os.Args[0]), Version, CommitHash, BuildTime)
		os.Exit(0)
	}

	if *ident == "" {
		exitf(UNKNOWN, "-i is required")
	}
	if *warnExpr == "" && *critExpr == "" {
		exitf(UNKNOWN, "at least one of -w or -c must be specified")
	}

	sess, err := session.NewSession()
	if err != nil {
		exitf(UNKNOWN, "AWS session: %v", err)
	}

	result, err := rds.New(sess).DescribeDBInstances(&rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(*ident),
	})
	if err != nil {
		exitf(UNKNOWN, "DescribeDBInstances: %v", err)
	}
	if len(result.DBInstances) == 0 {
		exitf(UNKNOWN, "instance %q not found", *ident)
	}

	ctx, err := instanceToContext(result.DBInstances[0])
	if err != nil {
		exitf(UNKNOWN, "internal serialization error: %v", err)
	}

	code, msg := runCheck(ctx, *exprGlobal, *warnExpr, *critExpr)
	exitf(code, "%s", msg)
}

func runCheck(ctx map[string]interface{}, globalExpr, warnExpr, critExpr string) (int, string) {
	var valueSuffix string
	if globalExpr != "" {
		val, err := Evaluate(globalExpr, ctx)
		if err != nil {
			return UNKNOWN, fmt.Sprintf("-e expression error: %v", err)
		}
		ctx["Value"] = val
		valueSuffix = fmt.Sprintf(" (Value=%s)", fmtVal(val))
	}

	if critExpr != "" {
		triggered, err := evalBool(critExpr, ctx)
		if err != nil {
			return UNKNOWN, fmt.Sprintf("-c expression error: %v", err)
		}
		if triggered {
			return CRITICAL, critExpr + valueSuffix
		}
	}

	if warnExpr != "" {
		triggered, err := evalBool(warnExpr, ctx)
		if err != nil {
			return UNKNOWN, fmt.Sprintf("-w expression error: %v", err)
		}
		if triggered {
			return WARNING, warnExpr + valueSuffix
		}
	}

	return OK, "check passed" + valueSuffix
}

func evalBool(expr string, ctx map[string]interface{}) (bool, error) {
	val, err := Evaluate(expr, ctx)
	if err != nil {
		return false, err
	}
	b, ok := val.(bool)
	if !ok {
		return false, fmt.Errorf("expression must evaluate to bool, got %T (%v)", val, val)
	}
	return b, nil
}

func instanceToContext(instance interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(instance)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	return m, json.Unmarshal(b, &m)
}

func fmtVal(v interface{}) string {
	if f, ok := v.(float64); ok {
		if f == float64(int64(f)) {
			return fmt.Sprintf("%.0f", f)
		}
		return fmt.Sprintf("%g", f)
	}
	return fmt.Sprintf("%v", v)
}

func exitf(code int, msg string, args ...interface{}) {
	var prefix string
	switch code {
	case OK:
		prefix = "OK"
	case WARNING:
		prefix = "WARNING"
	case CRITICAL:
		prefix = "CRITICAL"
	default:
		prefix = "UNKNOWN"
	}
	fmt.Printf("%s: %s\n", prefix, fmt.Sprintf(msg, args...))
	os.Exit(code)
}
