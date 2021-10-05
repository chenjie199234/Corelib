package superd

import (
	"encoding/json"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	ctime "github.com/chenjie199234/Corelib/util/time"
)

func writeany(buf *bufpool.Buffer, data interface{}) {
	switch d := data.(type) {
	case string:
		buf.AppendString(d)
	case []string:
		buf.AppendStrings(d)
	//case []uint8:
	case []byte:
		buf.AppendByteSlice(d)
	case [][]byte:
		buf.AppendByteSlices(d)
	//case uint8:
	case byte:
		buf.AppendByte(d)

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

	case int16:
		buf.AppendInt16(d)
	case []int16:
		buf.AppendInt16s(d)
	case uint16:
		buf.AppendUint16(d)
	case []uint16:
		buf.AppendUint16s(d)

	case ctime.Duration:
		buf.AppendDuration(d)
	case *ctime.Duration:
		buf.AppendDuration(*d)
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
		buf.AppendStdTime(d)
	case *time.Time:
		buf.AppendStdTime(*d)
	case []time.Time:
		buf.AppendStdTimes(d)
	case []*time.Time:
		buf.AppendStdTimePointers(d)

	case error:
		buf.AppendError(d)
	case []error:
		buf.AppendErrors(d)

	default:
		tmp, e := json.Marshal(data)
		if e != nil {
			buf.AppendString("unsupported type")
		} else {
			buf.AppendByteSlice(tmp)
		}
	}
}
