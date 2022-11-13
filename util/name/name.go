package name

import (
	"errors"
	"strings"
)

// [a-z][0-9],first character must in [a-z]
func SingleCheck(name string) error {
	if len(name) == 0 {
		return errors.New("[name.SingleCheck] empty")
	}
	if len(name) > 63 {
		return errors.New("[name.SingleCheck] too long")
	}
	if name[0] < 'a' || name[0] > 'z' {
		return errors.New("[name.SingleCheck] first character must in [a-z]")
	}
	for _, v := range name {
		if v < '0' || (v > '9' && v < 'a') || v > 'z' {
			return errors.New("[name.SingleCheck] character must in [a-z][0-9]")
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
	if e := SingleCheck(strs[0]); e != nil {
		return e
	}
	if e := SingleCheck(strs[1]); e != nil {
		return e
	}
	return nil
}
