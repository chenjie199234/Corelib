package name

import (
	"errors"
	"strings"
)

//[a-z][A-Z][0-9][_],first character must in [a-z][A-Z]
func NameCheck(name string) error {
	if len(name) == 0 {
		return errors.New("[name.NameCheck] empty")
	}
	if name[0] < 'A' || (name[0] > 'Z' && name[0] < 'a') || name[0] > 'z' {
		return errors.New("[name.NameCheck] first character must in [A-Z][a-z]")
	}
	for _, v := range name {
		if v < '0' || (v > '9' && v < 'A') || (v > 'Z' && v < 'a' && v != '_') || v > 'z' {
			return errors.New("[name.NameCheck] character must in [A-Z][a-z][0-9][_]")
		}
	}
	return nil
}

//[a-z][A-Z][0-9][-_|()[]{}<>],first character must in [a-z][A-Z]
func GroupCheck(group string) error {
	if len(group) == 0 {
		return errors.New("[name.GroupCheck] empty")
	}
	if group[0] < 'A' || (group[0] > 'Z' && group[0] < 'a') || group[0] > 'z' {
		return errors.New("[name.GroupCheck] first character must in [A-Z][a-z]")
	}
	for _, v := range group {
		if (v < '0' && v != '(' && v != ')' && v != '-') ||
			(v > '9' && v < 'A' && v != '<' && v != '>') ||
			(v > 'Z' && v < 'a' && v != '[' && v != ']' && v != '_') ||
			v > '}' {
			return errors.New("[name.GroupCheck] character must in [A-Z][a-z][0-9][-_|()[]{}<>]")
		}
	}
	return nil
}

//full = group.name
func FullCheck(full string) error {
	if len(full) == 0 {
		return errors.New("[name.FullCheck] empty")
	}
	if len(full) > 32 {
		return errors.New("[name.FullCheck] too long")
	}
	if strings.Count(full, ".") != 1 {
		return errors.New("[name.FullCheck] fullname's format must be group.name")
	}
	strs := strings.Split(full, ".")
	if e := NameCheck(strs[1]); e != nil {
		return e
	}
	if e := GroupCheck(strs[0]); e != nil {
		return e
	}
	return nil
}
