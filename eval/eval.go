package eval

import (
	"fmt"
	"time"
)

var clockNow = time.Now

type undefinedType struct{}

type intervalVal struct {
	years, months, days int
	dur                 time.Duration
}

type evaluator struct{ ctx map[string]interface{} }

func Evaluate(expr string, ctx map[string]interface{}) (interface{}, error) {
	n, err := parseExpr(expr)
	if err != nil {
		return nil, err
	}
	return (&evaluator{ctx: ctx}).eval(n)
}

func (e *evaluator) eval(n exprNode) (interface{}, error) {
	switch v := n.(type) {
	case *numLit:
		return v.val, nil
	case *strLit:
		return v.val, nil
	case *boolLit:
		return v.val, nil
	case *nullLit:
		return nil, nil

	case *identExpr:
		val, ok := e.ctx[v.name]
		if !ok {
			return undefinedType{}, nil
		}
		return val, nil

	case *fieldExpr:
		obj, err := e.eval(v.obj)
		if err != nil {
			return nil, err
		}
		if _, undef := obj.(undefinedType); undef || obj == nil {
			return undefinedType{}, nil
		}
		m, ok := obj.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot access field %q on %T", v.field, obj)
		}
		val, exists := m[v.field]
		if !exists {
			return undefinedType{}, nil
		}
		return val, nil

	case *indexExpr:
		obj, err := e.eval(v.obj)
		if err != nil {
			return nil, err
		}
		if _, undef := obj.(undefinedType); undef || obj == nil {
			return undefinedType{}, nil
		}
		arr, ok := obj.([]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot index %T with []", obj)
		}
		idxVal, err := e.eval(v.idx)
		if err != nil {
			return nil, err
		}
		f, ok := toFloat64(idxVal)
		if !ok {
			return nil, fmt.Errorf("array index must be numeric, got %T", idxVal)
		}
		i := int(f)
		if i < 0 || i >= len(arr) {
			return undefinedType{}, nil
		}
		return arr[i], nil

	case *unaryExpr:
		return e.evalUnary(v)
	case *binExpr:
		return e.evalBin(v)
	case *callExpr:
		return e.evalCall(v)
	case *intervalExpr:
		return e.evalInterval(v)
	}
	return nil, fmt.Errorf("unknown node type %T", n)
}

func (e *evaluator) evalInterval(n *intervalExpr) (interface{}, error) {
	amtVal, err := e.eval(n.amount)
	if err != nil {
		return nil, err
	}
	f, ok := toFloat64(amtVal)
	if !ok {
		return nil, fmt.Errorf("INTERVAL amount must be numeric, got %T", amtVal)
	}
	whole := int(f)
	switch n.unit {
	case "SECOND", "SECONDS":
		return intervalVal{dur: time.Duration(f * float64(time.Second))}, nil
	case "MINUTE", "MINUTES":
		return intervalVal{dur: time.Duration(f * float64(time.Minute))}, nil
	case "HOUR", "HOURS":
		return intervalVal{dur: time.Duration(f * float64(time.Hour))}, nil
	case "DAY", "DAYS":
		return intervalVal{days: whole}, nil
	case "WEEK", "WEEKS":
		return intervalVal{days: whole * 7}, nil
	case "MONTH", "MONTHS":
		return intervalVal{months: whole}, nil
	case "YEAR", "YEARS":
		return intervalVal{years: whole}, nil
	}
	return nil, fmt.Errorf("INTERVAL: unknown unit %q", n.unit)
}

func (e *evaluator) evalUnary(n *unaryExpr) (interface{}, error) {
	switch n.op {
	case "!":
		val, err := e.eval(n.operand)
		if err != nil {
			return nil, err
		}
		b, err := asBool(val)
		if err != nil {
			return nil, fmt.Errorf("!: %w", err)
		}
		return !b, nil

	case "-":
		val, err := e.eval(n.operand)
		if err != nil {
			return nil, err
		}
		f, ok := toFloat64(val)
		if !ok {
			return nil, fmt.Errorf("unary - requires numeric operand, got %T", val)
		}
		return -f, nil
	}
	return nil, fmt.Errorf("unknown unary op %q", n.op)
}

func (e *evaluator) evalBin(n *binExpr) (interface{}, error) {
	// Short-circuit logical operators
	switch n.op {
	case "&&":
		lv, err := e.eval(n.left)
		if err != nil {
			return nil, err
		}
		lb, err := asBool(lv)
		if err != nil {
			return nil, fmt.Errorf("&&: %w", err)
		}
		if !lb {
			return false, nil
		}
		rv, err := e.eval(n.right)
		if err != nil {
			return nil, err
		}
		rb, err := asBool(rv)
		if err != nil {
			return nil, fmt.Errorf("&&: %w", err)
		}
		return rb, nil

	case "||":
		lv, err := e.eval(n.left)
		if err != nil {
			return nil, err
		}
		lb, err := asBool(lv)
		if err != nil {
			return nil, fmt.Errorf("||: %w", err)
		}
		if lb {
			return true, nil
		}
		rv, err := e.eval(n.right)
		if err != nil {
			return nil, err
		}
		rb, err := asBool(rv)
		if err != nil {
			return nil, fmt.Errorf("||: %w", err)
		}
		return rb, nil
	}

	lv, err := e.eval(n.left)
	if err != nil {
		return nil, err
	}
	rv, err := e.eval(n.right)
	if err != nil {
		return nil, err
	}

	switch n.op {
	case "+":
		// time.Time + INTERVAL
		if t, isTime := lv.(time.Time); isTime {
			if iv, isIV := rv.(intervalVal); isIV {
				return t.AddDate(iv.years, iv.months, iv.days).Add(iv.dur), nil
			}
		}
		// String concatenation takes precedence over numeric addition.
		if ls, ok := lv.(string); ok {
			if rs, ok := rv.(string); ok {
				return ls + rs, nil
			}
		}
		lf, lok := toFloat64(lv)
		rf, rok := toFloat64(rv)
		if !lok || !rok {
			return nil, fmt.Errorf("+: requires numeric or string operands, got %T and %T", lv, rv)
		}
		return lf + rf, nil

	case "-":
		// time.Time - INTERVAL
		if t, isTime := lv.(time.Time); isTime {
			if iv, isIV := rv.(intervalVal); isIV {
				return t.AddDate(-iv.years, -iv.months, -iv.days).Add(-iv.dur), nil
			}
		}
		lf, lok := toFloat64(lv)
		rf, rok := toFloat64(rv)
		if !lok || !rok {
			return nil, fmt.Errorf("-: requires numeric operands, got %T and %T", lv, rv)
		}
		return lf - rf, nil

	case "*":
		lf, lok := toFloat64(lv)
		rf, rok := toFloat64(rv)
		if !lok || !rok {
			return nil, fmt.Errorf("*: requires numeric operands, got %T and %T", lv, rv)
		}
		return lf * rf, nil

	case "/":
		lf, lok := toFloat64(lv)
		rf, rok := toFloat64(rv)
		if !lok || !rok {
			return nil, fmt.Errorf("/: requires numeric operands, got %T and %T", lv, rv)
		}
		if rf == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return lf / rf, nil

	case "==":
		return valEqual(lv, rv), nil
	case "!=", "<>":
		return !valEqual(lv, rv), nil

	case "<":
		c, err := valCompare(lv, rv)
		if err != nil {
			return nil, err
		}
		return c < 0, nil
	case ">":
		c, err := valCompare(lv, rv)
		if err != nil {
			return nil, err
		}
		return c > 0, nil
	case "<=":
		c, err := valCompare(lv, rv)
		if err != nil {
			return nil, err
		}
		return c <= 0, nil
	case ">=":
		c, err := valCompare(lv, rv)
		if err != nil {
			return nil, err
		}
		return c >= 0, nil
	}
	return nil, fmt.Errorf("unknown binary op %q", n.op)
}

func (e *evaluator) evalCall(n *callExpr) (interface{}, error) {
	switch n.fn {
	case "exists":
		if len(n.args) != 1 {
			return nil, fmt.Errorf("exists() takes exactly 1 argument")
		}
		val, err := e.eval(n.args[0])
		if err != nil {
			// A hard evaluation error (e.g. type mismatch) still means it doesn't exist.
			return false, nil
		}
		_, isUndef := val.(undefinedType)
		return !isUndef && val != nil, nil

	case "len":
		if len(n.args) != 1 {
			return nil, fmt.Errorf("len() takes exactly 1 argument")
		}
		val, err := e.eval(n.args[0])
		if err != nil {
			return nil, err
		}
		switch v := val.(type) {
		case []interface{}:
			return float64(len(v)), nil
		case string:
			return float64(len(v)), nil
		case undefinedType, nil:
			return float64(0), nil
		default:
			return nil, fmt.Errorf("len() requires array or string, got %T", v)
		}

	case "ifnull":
		if len(n.args) != 2 {
			return nil, fmt.Errorf("ifnull() takes exactly 2 arguments")
		}
		val, err := e.eval(n.args[0])
		if err != nil {
			return nil, err
		}
		if isNull(val) {
			return e.eval(n.args[1])
		}
		return val, nil

	case "contains":
		if len(n.args) != 2 {
			return nil, fmt.Errorf("contains() takes exactly 2 arguments")
		}
		arrVal, err := e.eval(n.args[0])
		if err != nil {
			return nil, err
		}
		arr, ok := arrVal.([]interface{})
		if !ok {
			return nil, fmt.Errorf("contains() first argument must be an array, got %T", arrVal)
		}
		needle, err := e.eval(n.args[1])
		if err != nil {
			return nil, err
		}
		for _, elem := range arr {
			if valEqual(elem, needle) {
				return true, nil
			}
		}
		return false, nil

	case "all", "any", "count":
		if len(n.args) != 2 {
			return nil, fmt.Errorf("%s() takes exactly 2 arguments", n.fn)
		}
		arrVal, err := e.eval(n.args[0])
		if err != nil {
			return nil, err
		}
		arr, ok := arrVal.([]interface{})
		if !ok {
			return nil, fmt.Errorf("%s() first argument must be an array, got %T", n.fn, arrVal)
		}
		prev, hadPrev := e.ctx["it"]
		var matched float64
		for _, elem := range arr {
			e.ctx["it"] = elem
			res, err := e.eval(n.args[1])
			if err != nil {
				return nil, err
			}
			b, err := asBool(res)
			if err != nil {
				return nil, fmt.Errorf("%s() expression must evaluate to bool: %v", n.fn, err)
			}
			if b {
				matched++
				if n.fn == "any" {
					if hadPrev {
						e.ctx["it"] = prev
					} else {
						delete(e.ctx, "it")
					}
					return true, nil
				}
			} else if n.fn == "all" {
				if hadPrev {
					e.ctx["it"] = prev
				} else {
					delete(e.ctx, "it")
				}
				return false, nil
			}
		}
		if hadPrev {
			e.ctx["it"] = prev
		} else {
			delete(e.ctx, "it")
		}
		switch n.fn {
		case "all":
			return true, nil
		case "any":
			return false, nil
		default: // count
			return matched, nil
		}

	case "now":
		if len(n.args) != 0 {
			return nil, fmt.Errorf("now() takes no arguments")
		}
		return clockNow(), nil

	case "parseTime":
		if len(n.args) != 1 {
			return nil, fmt.Errorf("parseTime() takes exactly 1 argument")
		}
		val, err := e.eval(n.args[0])
		if err != nil {
			return nil, err
		}
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("parseTime() requires a string, got %T", val)
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("parseTime(): %w", err)
		}
		return t, nil
	}
	return nil, fmt.Errorf("unknown function %q", n.fn)
}
