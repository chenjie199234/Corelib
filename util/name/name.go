package name

import (
	"errors"
	"strings"
)

// [a-z][0-9][-],first character must in [a-z],last character must in [a-z][0-9]
func SingleCheck(name string, dash bool) error {
	if len(name) == 0 {
		return errors.New("[name.SingleCheck] empty")
	}
	if len(name) > 63 {
		return errors.New("[name.SingleCheck] too long")
	}
	if name[0] < 'a' || name[0] > 'z' {
		return errors.New("[name.SingleCheck] first character must in [a-z]")
	}
	if name[len(name)-1] < '0' || (name[len(name)-1] > '9' && name[len(name)-1] < 'a') || name[len(name)-1] > 'z' {
		return errors.New("[name.SingleCheck] last character must in [a-z][0-9]")
	}
	for _, v := range name {
		if (!dash && v < '0') || (dash && v < '0' && v != '-') || (v > '9' && v < 'a') || v > 'z' {
			if dash {
				return errors.New("[name.SingleCheck] character must in [a-z][0-9][-]")
			} else {
				return errors.New("[name.SingleCheck] character must in [a-z][0-9]")
			}
		}
	}
	return nil
}

// full = group.name
func FullCheck(full string) error {
	if len(full) == 0 {
		return errors.New("[name.FullCheck] empty")
	}
	if len(full) > 253 {
		return errors.New("[name.FullCheck] too long")
	}
	if strings.Count(full, ".") != 1 {
		return errors.New("[name.FullCheck] fullname's format must be group.name")
	}
	strs := strings.Split(full, ".")
	if e := SingleCheck(strs[0], true); e != nil {
		//group can use dash
		return e
	}
	if e := SingleCheck(strs[1], false); e != nil {
		//name can't use dash
		return e
	}
	return nil
}
