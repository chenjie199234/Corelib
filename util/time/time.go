package time

import (
	"errors"
	"strconv"
	stdtime "time"

	"github.com/chenjie199234/Corelib/util/common"
)

var ErrFormatDuration = errors.New("format wrong for Duration,supported:\"1h2m3s4ms5us6ns\"")

type Duration stdtime.Duration

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
		temp, e := stdtime.ParseDuration(common.Byte2str(data))
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
	if d == nil {
		return common.Str2byte("\"null\""), nil
	}
	if *d == 0 {
		return common.Str2byte("\"0s\""), nil
	}
	sd := d.StdDuration()
	r := make([]byte, 0, 10)
	r = append(r, '"')
	if sd >= stdtime.Hour {
		r = strconv.AppendInt(r, int64(sd/stdtime.Hour), 10)
		r = append(r, 'h')
		if sd = sd % stdtime.Hour; sd == 0 {
			r = append(r, '"')
			return r, nil
		}
	}
	if sd >= stdtime.Minute {
		r = strconv.AppendInt(r, int64(sd/stdtime.Minute), 10)
		r = append(r, 'm')
		if sd = sd % stdtime.Minute; sd == 0 {
			r = append(r, '"')
			return r, nil
		}
	}
	if sd >= stdtime.Second {
		r = strconv.AppendInt(r, int64(sd/stdtime.Second), 10)
		r = append(r, 's')
		if sd = sd % stdtime.Second; sd == 0 {
			r = append(r, '"')
			return r, nil
		}
	}
	if sd >= stdtime.Millisecond {
		r = strconv.AppendInt(r, int64(sd/stdtime.Millisecond), 10)
		r = append(r, "ms"...)
		if sd = sd % stdtime.Millisecond; sd == 0 {
			r = append(r, '"')
			return r, nil
		}
	}
	if sd >= stdtime.Microsecond {
		r = strconv.AppendInt(r, int64(sd/stdtime.Microsecond), 10)
		r = append(r, "us"...)
		if sd = sd % stdtime.Microsecond; sd == 0 {
			r = append(r, '"')
			return r, nil
		}
	}
	r = strconv.AppendInt(r, int64(sd), 10)
	r = append(r, "ns\""...)
	return r, nil
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
		temp, e := stdtime.ParseDuration(common.Byte2str(data))
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
	if d == nil {
		return common.Str2byte("null"), nil
	}
	if *d == 0 {
		return common.Str2byte("0s"), nil
	}
	sd := d.StdDuration()
	r := make([]byte, 0, 10)
	r = append(r, '"')
	if sd >= stdtime.Hour {
		r = strconv.AppendInt(r, int64(sd/stdtime.Hour), 10)
		r = append(r, 'h')
		if sd = sd % stdtime.Hour; sd == 0 {
			r = append(r, '"')
			return r, nil
		}
	}
	if sd >= stdtime.Minute {
		r = strconv.AppendInt(r, int64(sd/stdtime.Minute), 10)
		r = append(r, 'm')
		if sd = sd % stdtime.Minute; sd == 0 {
			r = append(r, '"')
			return r, nil
		}
	}
	if sd >= stdtime.Second {
		r = strconv.AppendInt(r, int64(sd/stdtime.Second), 10)
		r = append(r, 's')
		if sd = sd % stdtime.Second; sd == 0 {
			r = append(r, '"')
			return r, nil
		}
	}
	if sd >= stdtime.Millisecond {
		r = strconv.AppendInt(r, int64(sd/stdtime.Millisecond), 10)
		r = append(r, "ms"...)
		if sd = sd % stdtime.Millisecond; sd == 0 {
			r = append(r, '"')
			return r, nil
		}
	}
	if sd >= stdtime.Microsecond {
		r = strconv.AppendInt(r, int64(sd/stdtime.Microsecond), 10)
		r = append(r, "us"...)
		if sd = sd % stdtime.Microsecond; sd == 0 {
			r = append(r, '"')
			return r, nil
		}
	}
	r = strconv.AppendInt(r, int64(sd), 10)
	r = append(r, "ns\""...)
	return r, nil
}
func (d *Duration) StdDuration() stdtime.Duration {
	return stdtime.Duration(*d)
}
