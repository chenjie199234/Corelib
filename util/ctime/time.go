package ctime

import (
	"errors"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/util/common"
)

var ErrFormatDuration = errors.New("format wrong for Duration,supported:\"1h2m3s4ms5us6ns\"")

type Duration time.Duration

func (d *Duration) UnmarshalJSON(data []byte) error {
	if data[0] == '"' && data[len(data)-1] == '"' {
		if len(data) == 2 {
			*d = Duration(0)
			return nil
		}
		data = data[1 : len(data)-1]
	}
	if data[0] != '"' && data[len(data)-1] != '"' {
		if num, e := strconv.ParseInt(common.Byte2str(data), 10, 64); e == nil {
			*d = Duration(num)
			return nil
		}
		temp, e := time.ParseDuration(common.Byte2str(data))
		if e != nil {
			return ErrFormatDuration
		}
		*d = Duration(temp)
	} else {
		return ErrFormatDuration
	}
	return nil
}
func (d *Duration) MarshalJSON() ([]byte, error) {
	dd := d.StdDuration()
	b := make([]byte, 0, 50)
	b = append(b, '"')
	//hour
	b = strconv.AppendInt(b, int64(dd/time.Hour), 10)
	b = append(b, 'h')
	dd = dd % time.Hour
	//minute
	b = strconv.AppendInt(b, int64(dd/time.Minute), 10)
	b = append(b, 'm')
	dd = dd % time.Minute
	//second
	b = strconv.AppendInt(b, int64(dd/time.Second), 10)
	b = append(b, 's')
	dd = dd % time.Second
	//millisecond
	b = strconv.AppendInt(b, int64(dd/time.Millisecond), 10)
	b = append(b, "ms"...)
	dd = dd % time.Millisecond
	//microsecond
	b = strconv.AppendInt(b, int64(dd/time.Microsecond), 10)
	b = append(b, "us"...)
	dd = dd % time.Microsecond
	//nanosecond
	b = strconv.AppendInt(b, int64(dd), 10)
	b = append(b, "ns"...)
	b = append(b, '"')
	return b, nil
}
func (d *Duration) UnmarshalText(data []byte) error {
	if data[0] == '"' && data[len(data)-1] == '"' {
		if len(data) == 2 {
			*d = Duration(0)
			return nil
		}
		data = data[1 : len(data)-1]
	}
	if data[0] != '"' && data[len(data)-1] != '"' {
		if num, e := strconv.ParseInt(common.Byte2str(data), 10, 64); e == nil {
			*d = Duration(num)
			return nil
		}
		temp, e := time.ParseDuration(common.Byte2str(data))
		if e != nil {
			return ErrFormatDuration
		}
		*d = Duration(temp)
	} else {
		return ErrFormatDuration
	}
	return nil
}
func (d *Duration) MarshalText() ([]byte, error) {
	dd := d.StdDuration()
	b := make([]byte, 0, 50)
	b = append(b, '"')
	//hour
	b = strconv.AppendInt(b, int64(dd/time.Hour), 10)
	b = append(b, 'h')
	dd = dd % time.Hour
	//minute
	b = strconv.AppendInt(b, int64(dd/time.Minute), 10)
	b = append(b, 'm')
	dd = dd % time.Minute
	//second
	b = strconv.AppendInt(b, int64(dd/time.Second), 10)
	b = append(b, 's')
	dd = dd % time.Second
	//millisecond
	b = strconv.AppendInt(b, int64(dd/time.Millisecond), 10)
	b = append(b, "ms"...)
	dd = dd % time.Millisecond
	//microsecond
	b = strconv.AppendInt(b, int64(dd/time.Microsecond), 10)
	b = append(b, "us"...)
	dd = dd % time.Microsecond
	//nanosecond
	b = strconv.AppendInt(b, int64(dd), 10)
	b = append(b, "ns"...)
	b = append(b, '"')
	return b, nil
}
func (d *Duration) StdDuration() time.Duration {
	return time.Duration(*d)
}
