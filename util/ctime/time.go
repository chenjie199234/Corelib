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
	if data[0] == '"' && data[len(data)-1] == '"' {
		data = data[1 : len(data)-1]
	}
	if len(data) == 0 {
		*d = Duration(0)
		return nil
	}
	if data[0] != '"' && data[len(data)-1] != '"' {
		if temp, e := time.ParseDuration(common.Byte2str(data)); e == nil {
			*d = Duration(temp)
			return nil
		}
		if num, e := strconv.ParseInt(common.Byte2str(data), 10, 64); e == nil {
			*d = Duration(num)
			return nil
		}
	}
	return ErrDurationFormatWrong
}
func (d Duration) MarshalJSON() ([]byte, error) {
	if d == 0 {
		return []byte{'"', '0', 's', '"'}, nil
	}
	dd := d.StdDuration()
	b := make([]byte, 0, 50)
	b = append(b, '"')
	//hour
	if dd/time.Hour > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Hour), 10)
		b = append(b, 'h')
		dd = dd % time.Hour
	}
	//minute
	if dd/time.Minute > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Minute), 10)
		b = append(b, 'm')
		dd = dd % time.Minute
	}
	//second
	if dd/time.Second > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Second), 10)
		b = append(b, 's')
		dd = dd % time.Second
	}
	//millisecond
	if dd/time.Millisecond > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Millisecond), 10)
		b = append(b, "ms"...)
		dd = dd % time.Millisecond
	}
	//microsecond
	if dd/time.Microsecond > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Microsecond), 10)
		b = append(b, "us"...)
		dd = dd % time.Microsecond
	}
	//nanosecond
	if dd > 0 {
		b = strconv.AppendInt(b, int64(dd), 10)
		b = append(b, "ns"...)
	}
	b = append(b, '"')
	return b, nil
}
func (d *Duration) UnmarshalText(data []byte) error {
	if data[0] == '"' && data[len(data)-1] == '"' {
		data = data[1 : len(data)-1]
	}
	if len(data) == 0 {
		*d = Duration(0)
		return nil
	}
	if data[0] != '"' && data[len(data)-1] != '"' {
		if temp, e := time.ParseDuration(common.Byte2str(data)); e == nil {
			*d = Duration(temp)
			return nil
		}
		if num, e := strconv.ParseInt(common.Byte2str(data), 10, 64); e == nil {
			*d = Duration(num)
			return nil
		}
	}
	return ErrDurationFormatWrong
}
func (d Duration) MarshalText() ([]byte, error) {
	if d == 0 {
		return []byte{'"', '0', 's', '"'}, nil
	}
	dd := d.StdDuration()
	b := make([]byte, 0, 50)
	b = append(b, '"')
	//hour
	if dd/time.Hour > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Hour), 10)
		b = append(b, 'h')
		dd = dd % time.Hour
	}
	//minute
	if dd/time.Minute > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Minute), 10)
		b = append(b, 'm')
		dd = dd % time.Minute
	}
	//second
	if dd/time.Second > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Second), 10)
		b = append(b, 's')
		dd = dd % time.Second
	}
	//millisecond
	if dd/time.Millisecond > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Millisecond), 10)
		b = append(b, "ms"...)
		dd = dd % time.Millisecond
	}
	//microsecond
	if dd/time.Microsecond > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Microsecond), 10)
		b = append(b, "us"...)
		dd = dd % time.Microsecond
	}
	//nanosecond
	if dd > 0 {
		b = strconv.AppendInt(b, int64(dd), 10)
		b = append(b, "ns"...)
	}
	b = append(b, '"')
	return b, nil
}
func (d Duration) String() string {
	if d == 0 {
		return "0s"
	}
	dd := d.StdDuration()
	b := make([]byte, 0, 50)
	//hour
	if dd/time.Hour > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Hour), 10)
		b = append(b, 'h')
		dd = dd % time.Hour
	}
	//minute
	if dd/time.Minute > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Minute), 10)
		b = append(b, 'm')
		dd = dd % time.Minute
	}
	//second
	if dd/time.Second > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Second), 10)
		b = append(b, 's')
		dd = dd % time.Second
	}
	//millisecond
	if dd/time.Millisecond > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Millisecond), 10)
		b = append(b, "ms"...)
		dd = dd % time.Millisecond
	}
	//microsecond
	if dd/time.Microsecond > 0 {
		b = strconv.AppendInt(b, int64(dd/time.Microsecond), 10)
		b = append(b, "us"...)
		dd = dd % time.Microsecond
	}
	//nanosecond
	if dd > 0 {
		b = strconv.AppendInt(b, int64(dd), 10)
		b = append(b, "ns"...)
	}
	return common.Byte2str(b)
}
