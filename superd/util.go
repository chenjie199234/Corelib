package superd

import (
	"encoding/json"
	"runtime"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
)

func log(rf *rotatefile.RotateFile, summary string, kvs map[string]interface{}) {
	buf := pool.GetBuffer()
	buf.AppendString("{\"_summary\":\"")
	buf.AppendString(summary)
	buf.AppendString("\",\"_time\":\"")
	buf.AppendStdTime(time.Now().UTC())
	buf.AppendString("\",\"_fileline\":\"")
	_, file, line, _ := runtime.Caller(2)
	buf.AppendString(file)
	buf.AppendByte(':')
	buf.AppendInt(line)
	buf.AppendByte('"')
	if len(kvs) != 0 {
		buf.AppendString(",\"_kvs\":{")
		first := true
		for k, v := range kvs {
			if !first {
				buf.AppendString(",")
			}
			buf.AppendByte('"')
			buf.AppendString(k)
			buf.AppendString("\":")
			writeany(buf, v)
			first = false
		}
		buf.AppendString("}}\n")
	} else {
		buf.AppendString("}\n")
	}
	rf.WriteBuf(buf)
}
func writeany(buf *pool.Buffer, data interface{}) {
	switch d := data.(type) {
	case string:
		special := false
		for _, v := range d {
			if v == '\\' || v == '"' {
				special = true
				break
			}
		}
		if special {
			dd, _ := json.Marshal(d)
			buf.AppendBytes(dd)
		} else {
			buf.AppendByte('"')
			buf.AppendString(d)
			buf.AppendByte('"')
		}
	case []string:
		for i, v := range d {
			special := false
			for _, vv := range v {
				if vv == '\\' || vv == '"' {
					special = true
					break
				}
			}
			if special {
				vv, _ := json.Marshal(v)
				d[i] = common.Byte2str(vv)
			}
		}
		buf.AppendStrings(d)

	case int64:
		buf.AppendInt64(d)
	case []int64:
		buf.AppendInt64s(d)
	case uint64:
		buf.AppendUint64(d)
	case []uint64:
		buf.AppendUint64s(d)
	case float64:
		buf.AppendFloat64(d)
	case []float64:
		buf.AppendFloat64s(d)

	case int32:
		buf.AppendInt32(d)
	case []int32:
		buf.AppendInt32s(d)
	case uint32:
		buf.AppendUint32(d)
	case []uint32:
		buf.AppendUint32s(d)
	case float32:
		buf.AppendFloat32(d)
	case []float32:
		buf.AppendFloat32s(d)

	case int:
		buf.AppendInt(d)
	case []int:
		buf.AppendInts(d)
	case uint:
		buf.AppendUint(d)
	case []uint:
		buf.AppendUints(d)

	case int8:
		buf.AppendInt8(d)
	case []int8:
		buf.AppendInt8s(d)
	case uint8:
		buf.AppendUint8(d)
	case []uint8:
		buf.AppendUint8s(d)

	case int16:
		buf.AppendInt16(d)
	case []int16:
		buf.AppendInt16s(d)
	case uint16:
		buf.AppendUint16(d)
	case []uint16:
		buf.AppendUint16s(d)

	case ctime.Duration:
		buf.AppendByte('"')
		buf.AppendDuration(d)
		buf.AppendByte('"')
	case *ctime.Duration:
		buf.AppendByte('"')
		buf.AppendDuration(*d)
		buf.AppendByte('"')
	case []ctime.Duration:
		buf.AppendDurations(d)
	case []*ctime.Duration:
		buf.AppendDurationPointers(d)

	case time.Duration:
		buf.AppendStdDuration(d)
	case *time.Duration:
		buf.AppendStdDuration(*d)
	case []time.Duration:
		buf.AppendStdDurations(d)
	case []*time.Duration:
		buf.AppendStdDurationPointers(d)

	case time.Time:
		buf.AppendByte('"')
		buf.AppendStdTime(d)
		buf.AppendByte('"')
	case *time.Time:
		buf.AppendByte('"')
		buf.AppendStdTime(*d)
		buf.AppendByte('"')
	case []time.Time:
		buf.AppendStdTimes(d)
	case []*time.Time:
		buf.AppendStdTimePointers(d)

	case *cerror.Error:
		buf.AppendError(d)
	case []*cerror.Error:
		buf.AppendErrors(d)
	case error:
		estr := d.Error()
		special := false
		for _, v := range estr {
			if v == '\\' || v == '"' {
				special = true
				break
			}
		}
		if special {
			dd, _ := json.Marshal(estr)
			buf.AppendBytes(dd)
		} else {
			buf.AppendByte('"')
			buf.AppendString(estr)
			buf.AppendByte('"')
		}
	case []error:
		buf.AppendByte('[')
		for i, v := range d {
			if i != 0 {
				buf.AppendByte(',')
			}
			if v == nil {
				buf.AppendString("null")
			} else if vv, ok := v.(*cerror.Error); ok {
				buf.AppendError(vv)
			} else {
				estr := v.Error()
				special := false
				for _, vvv := range estr {
					if vvv == '\\' || vvv == '"' {
						special = true
						break
					}
				}
				if special {
					dd, _ := json.Marshal(estr)
					buf.AppendBytes(dd)
				} else {
					buf.AppendByte('"')
					buf.AppendString(estr)
					buf.AppendByte('"')
				}
			}
		}
		buf.AppendByte(']')

	default:
		tmp, e := json.Marshal(data)
		if e != nil {
			buf.AppendString("unsupported type")
		} else {
			buf.AppendBytes(tmp)
		}
	}
}
