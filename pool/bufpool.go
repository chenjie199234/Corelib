package pool

import (
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
)

var bufpool *sync.Pool

func init() {
	bufpool = &sync.Pool{}
}

type Buffer []byte

func GetBuffer() *Buffer {
	b, ok := bufpool.Get().(*Buffer)
	if !ok {
		temp := Buffer(make([]byte, 0, 512))
		return &temp
	}
	*b = (*b)[:0]
	return b
}
func PutBuffer(b *Buffer) {
	if b == nil {
		return
	}
	bufpool.Put(b)
}
func (b *Buffer) Len() int {
	return len(*b)
}
func (b *Buffer) Cap() int {
	return cap(*b)
}
func (b *Buffer) Reset() {
	*b = (*b)[:0]
}

var m128 = uint64(1024 * 1024 * 128)
var m972 = uint64(float64(m128) * 1.5 * 1.5 * 1.5 * 1.5 * 1.5)

func nextcap(reqsize, nowcap uint64) uint64 {
	for nowcap < reqsize {
		if nowcap >= m972 {
			nowcap = uint64(float64(nowcap) * 1.25)
		} else if nowcap >= m128 {
			nowcap = uint64(float64(nowcap) * 1.5)
		} else {
			nowcap *= 2
		}
	}
	return nowcap
}

// old data unsafe
func (b *Buffer) Resize(n uint32) {
	nowcap := uint32(cap(*b))
	if nowcap >= n {
		(*[3]uintptr)(unsafe.Pointer(b))[1] = uintptr(n)
	} else {
		olddata := *b
		*b = make([]byte, n, nextcap(uint64(n), uint64(nowcap)))
		PutBuffer((*Buffer)(&olddata))
	}
}

// old data safe
func (b *Buffer) Growth(n uint32) {
	nowcap := uint32(cap(*b))
	if nowcap >= n {
		(*[3]uintptr)(unsafe.Pointer(b))[1] = uintptr(n)
	} else {
		olddata := *b
		*b = make([]byte, n, nextcap(uint64(n), uint64(nowcap)))
		copy(*b, olddata)
		PutBuffer((*Buffer)(&olddata))
	}
}

// return data is unsafe
func (b *Buffer) Bytes() []byte {
	return *b
}

// return data is safe
func (b *Buffer) CopyBytes() []byte {
	r := make([]byte, len(*b))
	copy(r, *b)
	return r
}

// return data is unsafe
func (b *Buffer) String() string {
	return common.Byte2str(*b)
}

// return data is safe
func (b *Buffer) CopyString() string {
	r := make([]byte, len(*b))
	copy(r, *b)
	return common.Byte2str(r)
}
func (b *Buffer) AppendBool(data bool) {
	*b = strconv.AppendBool(*b, data)
}
func (b *Buffer) AppendBools(data []bool) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendBool(*b, d)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendInt(data int) {
	*b = strconv.AppendInt(*b, int64(data), 10)
}
func (b *Buffer) AppendInts(data []int) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendInt(*b, int64(d), 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendUint(data uint) {
	*b = strconv.AppendUint(*b, uint64(data), 10)
}
func (b *Buffer) AppendUints(data []uint) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendUint(*b, uint64(d), 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendInt8(data int8) {
	*b = strconv.AppendInt(*b, int64(data), 10)
}
func (b *Buffer) AppendInt8s(data []int8) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendInt(*b, int64(d), 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendUint8(data uint8) {
	*b = strconv.AppendUint(*b, uint64(data), 10)
}
func (b *Buffer) AppendUint8s(data []uint8) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendUint(*b, uint64(d), 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendInt16(data int16) {
	*b = strconv.AppendInt(*b, int64(data), 10)
}
func (b *Buffer) AppendInt16s(data []int16) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendInt(*b, int64(d), 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendUint16(data uint16) {
	*b = strconv.AppendUint(*b, uint64(data), 10)
}
func (b *Buffer) AppendUint16s(data []uint16) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendUint(*b, uint64(d), 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendInt32(data int32) {
	*b = strconv.AppendInt(*b, int64(data), 10)
}
func (b *Buffer) AppendInt32s(data []int32) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendInt(*b, int64(d), 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendUint32(data uint32) {
	*b = strconv.AppendUint(*b, uint64(data), 10)
}
func (b *Buffer) AppendUint32s(data []uint32) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendUint(*b, uint64(d), 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendInt64(data int64) {
	*b = strconv.AppendInt(*b, data, 10)
}
func (b *Buffer) AppendInt64s(data []int64) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendInt(*b, d, 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendUint64(data uint64) {
	*b = strconv.AppendUint(*b, data, 10)
}
func (b *Buffer) AppendUint64s(data []uint64) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendUint(*b, d, 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendByte(data byte) {
	*b = append(*b, data)
}
func (b *Buffer) AppendByteSlice(data []byte) {
	*b = append(*b, data...)
}
func (b *Buffer) AppendByteSlices(data [][]byte) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = append(*b, '"')
			*b = append(*b, d...)
			*b = append(*b, '"')
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendString(data string) {
	*b = append(*b, data...)
}
func (b *Buffer) AppendStrings(data []string) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = append(*b, '"')
			*b = append(*b, d...)
			*b = append(*b, '"')
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendFloat32(data float32) {
	*b = strconv.AppendFloat(*b, float64(data), 'f', -1, 32)
}
func (b *Buffer) AppendFloat32s(data []float32) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendFloat(*b, float64(d), 'f', -1, 32)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendFloat64(data float64) {
	*b = strconv.AppendFloat(*b, float64(data), 'f', -1, 64)
}
func (b *Buffer) AppendFloat64s(data []float64) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendFloat(*b, float64(d), 'f', -1, 64)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendStdDuration(data time.Duration) {
	*b = strconv.AppendInt(*b, int64(data), 10)
}
func (b *Buffer) AppendStdDurations(data []time.Duration) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = strconv.AppendInt(*b, int64(d), 10)
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendStdDurationPointers(data []*time.Duration) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			if d == nil {
				b.AppendNil()
			} else {
				*b = strconv.AppendInt(*b, int64(*d), 10)
			}
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendDuration(data ctime.Duration) {
	if data == 0 {
		*b = append(*b, "0s"...)
		return
	}
	d := time.Duration(data)
	if d >= time.Hour {
		*b = strconv.AppendInt(*b, int64(d/time.Hour), 10)
		*b = append(*b, 'h')
		if d = d % time.Hour; d == 0 {
			return
		}
	}
	if d >= time.Minute {
		*b = strconv.AppendInt(*b, int64(d/time.Minute), 10)
		*b = append(*b, 'm')
		if d = d % time.Minute; d == 0 {
			return
		}
	}
	if d >= time.Second {
		*b = strconv.AppendInt(*b, int64(d/time.Second), 10)
		*b = append(*b, 's')
		if d = d % time.Second; d == 0 {
			return
		}
	}
	if d >= time.Millisecond {
		*b = strconv.AppendInt(*b, int64(d/time.Millisecond), 10)
		*b = append(*b, "ms"...)
		if d = d % time.Millisecond; d == 0 {
			return
		}
	}
	if d >= time.Microsecond {
		*b = strconv.AppendInt(*b, int64(d/time.Microsecond), 10)
		*b = append(*b, "us"...)
		if d = d % time.Millisecond; d == 0 {
			return
		}
	}
	*b = strconv.AppendInt(*b, int64(d), 10)
	*b = append(*b, "ns"...)
}
func (b *Buffer) AppendDurations(data []ctime.Duration) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = append(*b, '"')
			b.AppendDuration(d)
			*b = append(*b, '"')
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendDurationPointers(data []*ctime.Duration) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			if d == nil {
				b.AppendNil()
			} else {
				*b = append(*b, '"')
				b.AppendDuration(*d)
				*b = append(*b, '"')
			}
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendStdTime(data time.Time) {
	*b = data.AppendFormat(*b, time.RFC3339Nano)
}
func (b *Buffer) AppendStdTimes(data []time.Time) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			*b = append(*b, '"')
			*b = d.AppendFormat(*b, time.RFC3339Nano)
			*b = append(*b, '"')
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendStdTimePointers(data []*time.Time) {
	if data == nil {
		b.AppendNil()
	} else if len(data) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, d := range data {
			if d == nil {
				b.AppendNil()
			} else {
				*b = append(*b, '"')
				*b = d.AppendFormat(*b, time.RFC3339Nano)
				*b = append(*b, '"')
			}
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendError(e error) {
	if e == nil {
		b.AppendNil()
	} else if ee, ok := e.(*cerror.Error); ok {
		if ee == nil {
			b.AppendNil()
		} else {
			*b = append(*b, "{\"code\":"...)
			*b = strconv.AppendInt(*b, int64(ee.Code), 10)
			*b = append(*b, ",\"msg\":\""...)
			*b = append(*b, ee.Msg...)
			*b = append(*b, "\"}"...)
		}
	} else {
		*b = append(*b, e.Error()...)
	}
}
func (b *Buffer) AppendErrors(e []error) {
	if e == nil {
		b.AppendNil()
	} else if len(e) == 0 {
		b.AppendEmptySlice()
	} else {
		*b = append(*b, '[')
		for _, ee := range e {
			if ee == nil {
				b.AppendNil()
			} else if eee, ok := ee.(*cerror.Error); ok {
				if eee == nil {
					b.AppendNil()
				} else {
					*b = append(*b, "{\"code\":"...)
					*b = strconv.AppendInt(*b, int64(eee.Code), 10)
					*b = append(*b, ",\"msg\":\""...)
					*b = append(*b, eee.Msg...)
					*b = append(*b, "\"}"...)
				}
			} else {
				*b = append(*b, '"')
				*b = append(*b, ee.Error()...)
				*b = append(*b, '"')
			}
			*b = append(*b, ',')
		}
		(*b)[len(*b)-1] = ']'
	}
}
func (b *Buffer) AppendNil() {
	*b = append(*b, "null"...)
}
func (b *Buffer) AppendEmptySlice() {
	*b = append(*b, "[]"...)
}
func (b *Buffer) AppendEmptyObj() {
	*b = append(*b, "{}"...)
}
