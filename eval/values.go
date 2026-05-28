package eval

import (
	"fmt"
	"time"
)

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case bool:
		if val {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func asBool(v interface{}) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case float64:
		return val != 0, nil
	case string:
		return val != "", nil
	case nil, undefinedType:
		return false, nil
	}
	return false, fmt.Errorf("cannot coerce %T to bool", v)
}

// isNull reports whether v is nil or the undefined sentinel.
func isNull(v interface{}) bool {
	if v == nil {
		return true
	}
	_, undef := v.(undefinedType)
	return undef
}

func valEqual(a, b interface{}) bool {
	if isNull(a) && isNull(b) {
		return true
	}
	if isNull(a) || isNull(b) {
		return false
	}
	if af, aok := toFloat64(a); aok {
		if bf, bok := toFloat64(b); bok {
			return af == bf
		}
	}
	at, atime := a.(time.Time)
	bt, btime := b.(time.Time)
	if atime || btime {
		if !atime {
			var err error
			at, err = tryParseTime(a)
			if err != nil {
				return false
			}
		}
		if !btime {
			var err error
			bt, err = tryParseTime(b)
			if err != nil {
				return false
			}
		}
		return at.Equal(bt)
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func valCompare(a, b interface{}) (int, error) {
	if isNull(a) || isNull(b) {
		return 0, fmt.Errorf("cannot order null values; use == or != for null checks")
	}
	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)
	if aok && bok {
		switch {
		case af < bf:
			return -1, nil
		case af > bf:
			return 1, nil
		default:
			return 0, nil
		}
	}
	at, atime := a.(time.Time)
	bt, btime := b.(time.Time)
	if atime || btime {
		var err error
		if !atime {
			at, err = tryParseTime(a)
			if err != nil {
				return 0, fmt.Errorf("cannot compare %T with time: %v", a, err)
			}
		}
		if !btime {
			bt, err = tryParseTime(b)
			if err != nil {
				return 0, fmt.Errorf("cannot compare %T with time: %v", b, err)
			}
		}
		switch {
		case at.Before(bt):
			return -1, nil
		case at.After(bt):
			return 1, nil
		default:
			return 0, nil
		}
	}
	as, astr := a.(string)
	bs, bstr := b.(string)
	if astr && bstr {
		switch {
		case as < bs:
			return -1, nil
		case as > bs:
			return 1, nil
		default:
			return 0, nil
		}
	}
	return 0, fmt.Errorf("cannot compare %T and %T", a, b)
}

func tryParseTime(v interface{}) (time.Time, error) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("expected string or time.Time, got %T", v)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot parse %q as RFC3339: %w", s, err)
	}
	return t, nil
}
