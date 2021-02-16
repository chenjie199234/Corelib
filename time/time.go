package time

import (
	"bytes"
	"errors"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/common"
)

type Duration time.Duration
type Time time.Time

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
			return errors.New("format wrong for MDuration,supported:\"1h2m3s4ms5us6ns\"")
		}
		*d = Duration(temp)
	} else {
		return errors.New("format wrong for MDuration,supported: \"1h2m3s4ms5us6ns\"")
	}
	return nil
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
			return errors.New("format wrong for MDuration,supported:\"1h2m3s4ms5us6ns\"")
		}
		*d = Duration(temp)
	} else {
		return errors.New("format wrong for MDuration,supported: \"1h2m3s4ms5us6ns\"")
	}
	return nil
}
func (t *Time) UnmarshalJSON(data []byte) error {
	if data[0] == '"' && data[len(data)-1] == '"' {
		if len(data) == 2 {
			*t = Time(time.Unix(0, 0))
			return nil
		}
		data = data[1 : len(data)-1]
	}
	if data[0] != '"' && data[len(data)-1] != '"' {
		var temp time.Time
		var e error
		if bytes.Count(data, []byte{' '}) == 1 {
			temp, e = time.ParseInLocation("2006-01-02 15:04:05", common.Byte2str(data), time.UTC)
		} else {
			temp, e = time.Parse("2006-01-02 15:04:05 -07", common.Byte2str(data))
		}
		if e != nil {
			return errors.New("format wrong for MTime,supported:\"2006-01-02 15:04:05\"(time zone is utc)/\"2006-01-02 15:04:05 +08(time zone is +08)\"")
		}
		*t = Time(temp)
	} else {
		return errors.New("format wrong for MTime,supported: \"2006-01-02 15:04:05\"(time zone is utc)/\"2006-01-02 15:04:05 +01(time zone is +01)\"")
	}
	return nil
}
func (t *Time) UnmarshalText(data []byte) error {
	if data[0] == '"' && data[len(data)-1] == '"' {
		if len(data) == 2 {
			*t = Time(time.Unix(0, 0))
			return nil
		}
		data = data[1 : len(data)-1]
	}
	if data[0] != '"' && data[len(data)-1] != '"' {
		var temp time.Time
		var e error
		if bytes.Count(data, []byte{' '}) == 1 {
			temp, e = time.ParseInLocation("2006-01-02 15:04:05", common.Byte2str(data), time.UTC)
		} else {
			temp, e = time.Parse("2006-01-02 15:04:05 -07", common.Byte2str(data))
		}
		if e != nil {
			return errors.New("format wrong for MTime,supported:\"2006-01-02 15:04:05\"(time zone is utc)/\"2006-01-02 15:04:05 +08(time zone is +08)\"")
		}
		*t = Time(temp)
	} else {
		return errors.New("format wrong for MTime,supported: \"2006-01-02 15:04:05\"(time zone is utc)/\"2006-01-02 15:04:05 +01(time zone is +01)\"")
	}
	return nil
}
