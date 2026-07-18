package ctime

import (
	"errors"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/util/common"
)

var ErrDurationFormatWrong = errors.New("Duration's format wrong,should be number(unit nanosecond) or string(format: 1h2m3s4ms5us6ns)")

type Duration time.Duration

func (d Duration) StdDuration() time.Duration {
	return time.Duration(d)
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		data = data[1 : len(data)-1]
	}
	if len(data) == 0 || (len(data) == 4 && data[0] == 'n' && data[1] == 'u' && data[2] == 'l' && data[3] == 'l') {
		*d = Duration(0)
		return nil
	}
	if data[0] != '"' && data[len(data)-1] != '"' {
		if temp, e := time.ParseDuration(common.BTS(data)); e == nil {
			*d = Duration(temp)
			return nil
		}
		if num, e := strconv.ParseInt(common.BTS(data), 10, 64); e == nil {
			*d = Duration(num)
			return nil
		}
	}
	return ErrDurationFormatWrong
}
func (d Duration) MarshalJSON() ([]byte, error) {
	return d.format(true), nil
}
func (d *Duration) UnmarshalText(data []byte) error {
	if len(data) == 0 {
		*d = Duration(0)
		return nil
	}
	if temp, e := time.ParseDuration(common.BTS(data)); e == nil {
		*d = Duration(temp)
		return nil
	}
	if num, e := strconv.ParseInt(common.BTS(data), 10, 64); e == nil {
		*d = Duration(num)
		return nil
	}
	return ErrDurationFormatWrong
}
func (d Duration) MarshalText() ([]byte, error) {
	return d.format(false), nil
}
func (d Duration) String() string {
	return common.BTS(d.format(false))
}
func (d Duration) format(quoted bool) []byte {
	if d == 0 {
		if quoted {
			return []byte{'"', '0', 's', '"'}
		}
		return []byte{'0', 's'}
	}
	var dd uint64
	b := make([]byte, 0, 50)
	if quoted {
		b = append(b, '"')
	}
	if d.StdDuration() < 0 {
		b = append(b, '-')
		//prevent overflow when d.StdDuration() == math.MinInt64
		dd = uint64(-(d.StdDuration() + 1)) + 1
	} else {
		dd = uint64(d.StdDuration())
	}
	//hour
	if tmp := dd / uint64(time.Hour); tmp > 0 {
		b = strconv.AppendUint(b, tmp, 10)
		b = append(b, 'h')
		dd = dd % uint64(time.Hour)
	}
	//minute
	if tmp := dd / uint64(time.Minute); tmp > 0 {
		b = strconv.AppendUint(b, tmp, 10)
		b = append(b, 'm')
		dd = dd % uint64(time.Minute)
	}
	//second
	if tmp := dd / uint64(time.Second); tmp > 0 {
		b = strconv.AppendUint(b, tmp, 10)
		b = append(b, 's')
		dd = dd % uint64(time.Second)
	}
	//millisecond
	if tmp := dd / uint64(time.Millisecond); tmp > 0 {
		b = strconv.AppendUint(b, tmp, 10)
		b = append(b, "ms"...)
		dd = dd % uint64(time.Millisecond)
	}
	//microsecond
	if tmp := dd / uint64(time.Microsecond); tmp > 0 {
		b = strconv.AppendUint(b, tmp, 10)
		b = append(b, "us"...)
		dd = dd % uint64(time.Microsecond)
	}
	//nanosecond
	if dd > 0 {
		b = strconv.AppendUint(b, dd, 10)
		b = append(b, "ns"...)
	}
	if quoted {
		b = append(b, '"')
	}
	return b
}
