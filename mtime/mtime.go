package mtime

import (
	"bytes"
	"fmt"
	"time"

	"github.com/chenjie199234/Corelib/common"
)

type MDuration time.Duration
type MTime time.Time

func (d *MDuration) UnmarshalText(data []byte) error {
	if data[0] == '"' && data[len(data)-1] == '"' {
		if len(data) == 2 {
			*d = MDuration(0)
			return nil
		}
		data = data[1 : len(data)-1]
	}
	if data[0] != '"' && data[len(data)-1] != '"' {
		temp, e := time.ParseDuration(common.Byte2str(data))
		if e != nil {
			return fmt.Errorf("format wrong for MDuration,supported:\"1h2m3s4ms5us6ns\"")
		}
		*d = MDuration(temp)
	} else {
		return fmt.Errorf("format wrong for MDuration,supported: \"1h2m3s4ms5us6ns\"")
	}
	return nil
}

func (t *MTime) UnmarshalText(data []byte) error {
	if data[0] == '"' && data[len(data)-1] == '"' {
		if len(data) == 2 {
			*t = MTime(time.Unix(0, 0))
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
			return fmt.Errorf("format wrong for MTime,supported:\"2006-01-02 15:04:05\"(time zone is utc)/\"2006-01-02 15:04:05 +08(time zone is +08)\"")
		}
		*t = MTime(temp)
	} else {
		return fmt.Errorf("format wrong for MTime,supported: \"2006-01-02 15:04:05\"(time zone is utc)/\"2006-01-02 15:04:05 +01(time zone is +01)\"")
	}
	return nil
}
