package name

import (
	"errors"
	"sync"
)

// [a-z][0-9][-],first character must in [a-z],last character must in [a-z][0-9]
func SingleCheck(name string, dash bool) error {
	if len(name) == 0 {
		return errors.New("[name] empty")
	}
	if len(name) > 63 {
		return errors.New("[name] too long")
	}
	if name[0] < 'a' || name[0] > 'z' {
		return errors.New("[name] first character must in [a-z]")
	}
	if name[len(name)-1] < '0' || (name[len(name)-1] > '9' && name[len(name)-1] < 'a') || name[len(name)-1] > 'z' {
		return errors.New("[name] last character must in [a-z][0-9]")
	}
	for _, v := range name {
		if (!dash && v < '0') || (dash && v < '0' && v != '-') || (v > '9' && v < 'a') || v > 'z' {
			if dash {
				return errors.New("[name] character must in [a-z][0-9][-]")
			} else {
				return errors.New("[name] character must in [a-z][0-9]")
			}
		}
	}
	return nil
}
func MakeFullName(project, group, app string) (string, error) {
	if e := SingleCheck(project, false); e != nil {
		return "", e
	}
	if e := SingleCheck(group, false); e != nil {
		return "", e
	}
	if e := SingleCheck(app, false); e != nil {
		return "", e
	}
	return project + "-" + group + "." + app, nil
}

var lker sync.RWMutex
var fullname string
var project string
var group string
var app string

func SetSelfFullName(p, g, a string) error {
	lker.Lock()
	defer lker.Unlock()
	if fullname != "" {
		return errors.New("[name] self full name already setted")
	}
	str, e := MakeFullName(p, g, a)
	if e != nil {
		return e
	}
	fullname = str
	project = p
	group = g
	app = a
	return nil
}
func GetSelfFullName() string {
	lker.RLock()
	defer lker.RUnlock()
	return fullname
}
func HasSelfFullName() error {
	lker.RLock()
	defer lker.RUnlock()
	if fullname == "" {
		return errors.New("[name] missing self full name")
	}
	return nil
}
func GetSelfProject() string {
	lker.RLock()
	defer lker.RUnlock()
	return project
}
func GetSelfGroup() string {
	lker.RLock()
	defer lker.RUnlock()
	return group
}
func GetSelfApp() string {
	lker.RLock()
	defer lker.RUnlock()
	return app
}
