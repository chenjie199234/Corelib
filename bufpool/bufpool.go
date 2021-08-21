package bufpool

import (
	"io"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/util/common"
	ctime "github.com/chenjie199234/Corelib/util/time"
)

var pool *sync.Pool

func init() {
	pool = &sync.Pool{}
}

type Buffer struct {
	buf    []byte
	offset int
}

func GetBuffer() *Buffer {
	buf, ok := pool.Get().(*Buffer)
	if !ok {
		return &Buffer{
			buf: make([]byte, 0, 512),
		}
	}
	buf.buf = buf.buf[:0]
	buf.offset = 0
	return buf
}
func PutBuffer(buf *Buffer) {
	pool.Put(buf)
}

//byte will type assert to uint8,so it will be treated as number
//rune will type assert to int32,so it will be treated as number
func (b *Buffer) Append(data interface{}) {
	switch d := data.(type) {
	case reflect.Value:
		b.appendreflect(d)
	default:
		b.appendbasic(data)
	}
}

func (b *Buffer) appendbasic(data interface{}) {
	switch d := data.(type) {
	case bool:
		b.buf = strconv.AppendBool(b.buf, d)
	case *bool:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendBool(b.buf, *d)
		}
	case []bool:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendBool(b.buf, dd)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*bool:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendBool(b.buf, *dd)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case int:
		b.buf = strconv.AppendInt(b.buf, int64(d), 10)
	case int8:
		b.buf = strconv.AppendInt(b.buf, int64(d), 10)
	case int16:
		b.buf = strconv.AppendInt(b.buf, int64(d), 10)
	case int32:
		b.buf = strconv.AppendInt(b.buf, int64(d), 10)
	case int64:
		b.buf = strconv.AppendInt(b.buf, d, 10)
	case *int:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendInt(b.buf, int64(*d), 10)
		}
	case *int8:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendInt(b.buf, int64(*d), 10)
		}
	case *int16:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendInt(b.buf, int64(*d), 10)
		}
	case *int32:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendInt(b.buf, int64(*d), 10)
		}
	case *int64:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendInt(b.buf, *d, 10)
		}
	case []int:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendInt(b.buf, int64(dd), 10)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []int8:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendInt(b.buf, int64(dd), 10)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []int16:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendInt(b.buf, int64(dd), 10)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []int32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendInt(b.buf, int64(dd), 10)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []int64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendInt(b.buf, dd, 10)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*int:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendInt(b.buf, int64(*dd), 10)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*int8:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendInt(b.buf, int64(*dd), 10)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*int16:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendInt(b.buf, int64(*dd), 10)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*int32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendInt(b.buf, int64(*dd), 10)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*int64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendInt(b.buf, *dd, 10)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case uint:
		b.buf = strconv.AppendUint(b.buf, uint64(d), 10)
	case uint8:
		b.buf = strconv.AppendUint(b.buf, uint64(d), 10)
	case uint16:
		b.buf = strconv.AppendUint(b.buf, uint64(d), 10)
	case uint32:
		b.buf = strconv.AppendUint(b.buf, uint64(d), 10)
	case uint64:
		b.buf = strconv.AppendUint(b.buf, d, 10)
	case *uint:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendUint(b.buf, uint64(*d), 10)
		}
	case *uint8:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendUint(b.buf, uint64(*d), 10)
		}
	case *uint16:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendUint(b.buf, uint64(*d), 10)
		}
	case *uint32:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendUint(b.buf, uint64(*d), 10)
		}
	case *uint64:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendUint(b.buf, *d, 10)
		}
	case []uint:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendUint(b.buf, uint64(dd), 10)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []uint8:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendUint(b.buf, uint64(dd), 10)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []uint16:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendUint(b.buf, uint64(dd), 10)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []uint32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendUint(b.buf, uint64(dd), 10)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []uint64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendUint(b.buf, dd, 10)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uint:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendUint(b.buf, uint64(*dd), 10)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uint8:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendUint(b.buf, uint64(*dd), 10)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uint16:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendUint(b.buf, uint64(*dd), 10)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uint32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendUint(b.buf, uint64(*dd), 10)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uint64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendUint(b.buf, *dd, 10)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case float32:
		b.buf = strconv.AppendFloat(b.buf, float64(d), 'f', -1, 32)
	case float64:
		b.buf = strconv.AppendFloat(b.buf, d, 'f', -1, 64)
	case *float32:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendFloat(b.buf, float64(*d), 'f', -1, 32)
		}
	case *float64:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendFloat(b.buf, *d, 'f', -1, 64)
		}
	case []float32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendFloat(b.buf, float64(dd), 'f', -1, 32)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []float64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendFloat(b.buf, float64(dd), 'f', -1, 64)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*float32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendFloat(b.buf, float64(*dd), 'f', -1, 32)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*float64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendFloat(b.buf, float64(*dd), 'f', -1, 64)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case complex64:
		b.buf = strconv.AppendFloat(b.buf, float64(real(d)), 'f', -1, 32)
		if imag(d) >= 0 {
			b.buf = append(b.buf, '+')
		}
		b.buf = strconv.AppendFloat(b.buf, float64(imag(d)), 'f', -1, 32)
		b.buf = append(b.buf, 'i')
	case complex128:
		b.buf = strconv.AppendFloat(b.buf, real(d), 'f', -1, 32)
		if imag(d) >= 0 {
			b.buf = append(b.buf, '+')
		}
		b.buf = strconv.AppendFloat(b.buf, imag(d), 'f', -1, 32)
		b.buf = append(b.buf, 'i')
	case *complex64:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendFloat(b.buf, float64(real(*d)), 'f', -1, 32)
			if imag(*d) >= 0 {
				b.buf = append(b.buf, '+')
			}
			b.buf = strconv.AppendFloat(b.buf, float64(imag(*d)), 'f', -1, 32)
			b.buf = append(b.buf, 'i')
		}
	case *complex128:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = strconv.AppendFloat(b.buf, real(*d), 'f', -1, 32)
			if imag(*d) >= 0 {
				b.buf = append(b.buf, '+')
			}
			b.buf = strconv.AppendFloat(b.buf, imag(*d), 'f', -1, 32)
			b.buf = append(b.buf, 'i')
		}
	case []complex64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendFloat(b.buf, float64(real(dd)), 'f', -1, 32)
				if imag(dd) >= 0 {
					b.buf = append(b.buf, '+')
				}
				b.buf = strconv.AppendFloat(b.buf, float64(imag(dd)), 'f', -1, 32)
				b.buf = append(b.buf, 'i', ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []complex128:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = strconv.AppendFloat(b.buf, real(dd), 'f', -1, 32)
				if imag(dd) >= 0 {
					b.buf = append(b.buf, '+')
				}
				b.buf = strconv.AppendFloat(b.buf, imag(dd), 'f', -1, 32)
				b.buf = append(b.buf, 'i', ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*complex64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendFloat(b.buf, float64(real(*dd)), 'f', -1, 32)
					if imag(*dd) >= 0 {
						b.buf = append(b.buf, '+')
					}
					b.buf = strconv.AppendFloat(b.buf, float64(imag(*dd)), 'f', -1, 32)
					b.buf = append(b.buf, 'i')
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*complex128:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = strconv.AppendFloat(b.buf, real(*dd), 'f', -1, 32)
					if imag(*dd) >= 0 {
						b.buf = append(b.buf, '+')
					}
					b.buf = strconv.AppendFloat(b.buf, imag(*dd), 'f', -1, 32)
					b.buf = append(b.buf, 'i')
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case string:
		b.buf = append(b.buf, d...)
	case *string:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = append(b.buf, *d...)
		}
	case []string:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = append(b.buf, dd...)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*string:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = append(b.buf, *dd...)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case uintptr:
		b.buf = b.appendPointer(uint64(d))
	case unsafe.Pointer:
		b.buf = b.appendPointer(uint64(uintptr(d)))
	case *uintptr:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = b.appendPointer(uint64(*d))
		}
	case *unsafe.Pointer:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = b.appendPointer(uint64(uintptr(*d)))
		}
	case []uintptr:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = b.appendPointer(uint64(dd))
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []unsafe.Pointer:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = b.appendPointer(uint64(uintptr(dd)))
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uintptr:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = b.appendPointer(uint64(*dd))
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*unsafe.Pointer:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = b.appendPointer(uint64(uintptr(*dd)))
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case time.Duration:
		b.buf = b.appendDuration(d)
	case ctime.Duration:
		b.buf = b.appendDuration(time.Duration(d))
	case *time.Duration:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = b.appendDuration(*d)
		}
	case *ctime.Duration:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = b.appendDuration(time.Duration(*d))
		}
	case []time.Duration:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = b.appendDuration(dd)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []ctime.Duration:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = b.appendDuration(time.Duration(dd))
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*time.Duration:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = b.appendDuration(*dd)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*ctime.Duration:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = b.appendDuration(time.Duration(*dd))
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case time.Time:
		b.buf = d.AppendFormat(b.buf, "2006-01-02 15:04:05.000000000 -07")
	case ctime.Time:
		b.buf = time.Time(d).AppendFormat(b.buf, "2006-01-02 15:04:05.000000000 -07")
	case *time.Time:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = (*d).AppendFormat(b.buf, "2006-01-02 15:04:05.000000000 -07")
		}
	case *ctime.Time:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = time.Time(*d).AppendFormat(b.buf, "2006-01-02 15:04:05.000000000 -07")
		}
	case []time.Time:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = dd.AppendFormat(b.buf, "2006-01-02 15:04:05.000000000 -07")
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []ctime.Time:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = time.Time(dd).AppendFormat(b.buf, "2006-01-02 15:04:05.000000000 -07")
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*time.Time:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = (*dd).AppendFormat(b.buf, "2006-01-02 15:04:05.000000000 -07")
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*ctime.Time:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = time.Time(*dd).AppendFormat(b.buf, "2006-01-02 15:04:05.000000000 -07")
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case error:
		b.buf = append(b.buf, d.Error()...)
	case *error:
		if d == nil {
			b.appendnil()
		} else {
			b.buf = append(b.buf, (*d).Error()...)
		}
	case []error:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				b.buf = append(b.buf, dd.Error()...)
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*error:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			b.buf = append(b.buf, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.buf = append(b.buf, (*dd).Error()...)
				}
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	default:
		b.appendreflect(reflect.ValueOf(d))
	}
}
func (b *Buffer) appendreflect(d reflect.Value) {
	switch d.Kind() {
	case reflect.Bool:
		b.buf = strconv.AppendBool(b.buf, d.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if d.Type().Name() == "Duration" {
			b.buf = b.appendDuration(*(*time.Duration)(unsafe.Pointer(d.Addr().Pointer())))
		} else {
			b.buf = strconv.AppendInt(b.buf, d.Int(), 10)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b.buf = strconv.AppendUint(b.buf, d.Uint(), 10)
	case reflect.Float32:
		b.buf = strconv.AppendFloat(b.buf, d.Float(), 'f', -1, 32)
	case reflect.Float64:
		b.buf = strconv.AppendFloat(b.buf, d.Float(), 'f', -1, 64)
	case reflect.Complex64:
		c := d.Complex()
		b.buf = strconv.AppendFloat(b.buf, float64(real(c)), 'f', -1, 32)
		if imag(c) >= 0 {
			b.buf = append(b.buf, '+')
		}
		b.buf = strconv.AppendFloat(b.buf, float64(imag(c)), 'f', -1, 32)
		b.buf = append(b.buf, 'i')
	case reflect.Complex128:
		c := d.Complex()
		b.buf = strconv.AppendFloat(b.buf, float64(real(c)), 'f', -1, 64)
		if imag(c) >= 0 {
			b.buf = append(b.buf, '+')
		}
		b.buf = strconv.AppendFloat(b.buf, float64(imag(c)), 'f', -1, 64)
		b.buf = append(b.buf, 'i')
	case reflect.String:
		b.buf = append(b.buf, d.String()...)
	case reflect.Interface:
		if d.IsNil() {
			b.appendnil()
		} else {
			b.appendreflect(d.Elem())
		}
	case reflect.Array:
		if d.Len() > 0 {
			b.buf = append(b.buf, '[')
			for i := 0; i < d.Len(); i++ {
				b.appendreflect(d.Index(i))
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case reflect.Slice:
		if d.IsNil() {
			b.appendnil()
		} else if d.Len() > 0 {
			b.buf = append(b.buf, '[')
			for i := 0; i < d.Len(); i++ {
				b.appendreflect(d.Index(i))
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case reflect.Map:
		if d.IsNil() {
			b.appendnil()
		} else if d.Len() > 0 {
			b.buf = append(b.buf, '{')
			iter := d.MapRange()
			for iter.Next() {
				b.appendreflect(iter.Key())
				b.buf = append(b.buf, ':')
				b.appendreflect(iter.Value())
				b.buf = append(b.buf, ',')
			}
			b.buf[len(b.buf)-1] = '}'
		} else {
			b.appendemptyobj()
		}
	case reflect.Struct:
		if d.Type().Name() == "Time" {
			t := (*time.Time)(unsafe.Pointer(d.Addr().Pointer()))
			b.appendbasic(t)
		} else if d.NumField() > 0 {
			b.buf = append(b.buf, '{')
			t := d.Type()
			for i := 0; i < d.NumField(); i++ {
				if t.Field(i).Name[0] >= 65 && t.Field(i).Name[0] <= 90 {
					b.buf = append(b.buf, t.Field(i).Name...)
					b.buf = append(b.buf, ':')
					b.appendreflect(d.Field(i))
					b.buf = append(b.buf, ',')
				}
			}
			b.buf[len(b.buf)-1] = '}'
		} else {
			b.appendemptyobj()
		}
	case reflect.Ptr:
		if d.IsNil() {
			b.appendnil()
		} else {
			b.appendreflect(d.Elem())
		}
	case reflect.Uintptr:
		b.buf = b.appendPointer(d.Uint())
	case reflect.UnsafePointer:
		if d.IsNil() {
			b.appendnil()
		} else {
			b.buf = b.appendPointer(uint64(d.Pointer()))
		}
	case reflect.Func:
		if d.IsNil() {
			b.appendnil()
		} else {
			b.buf = b.appendPointer(uint64(d.Pointer()))
		}
	case reflect.Chan:
		if d.IsNil() {
			b.appendnil()
		} else {
			b.buf = b.appendPointer(uint64(d.Pointer()))
		}
	default:
		b.buf = append(b.buf, "unsupported type"...)
	}
}
func (b *Buffer) appendDuration(d time.Duration) []byte {
	if d == 0 {
		b.buf = append(b.buf, "0s"...)
	} else {
		if d >= time.Hour {
			b.buf = strconv.AppendInt(b.buf, int64(d)/int64(time.Hour), 10)
			b.buf = append(b.buf, 'h')
			if d = d % time.Hour; d == 0 {
				return b.buf
			}
		}
		if d >= time.Minute {
			b.buf = strconv.AppendInt(b.buf, int64(d)/int64(time.Minute), 10)
			b.buf = append(b.buf, 'm')
			if d = d % time.Minute; d == 0 {
				return b.buf
			}
		}
		if d >= time.Second {
			b.buf = strconv.AppendInt(b.buf, int64(d)/int64(time.Second), 10)
			b.buf = append(b.buf, 's')
			if d = d % time.Second; d == 0 {
				return b.buf
			}
		}
		if d >= time.Millisecond {
			b.buf = strconv.AppendInt(b.buf, int64(d)/int64(time.Millisecond), 10)
			b.buf = append(b.buf, "ms"...)
			if d = d % time.Millisecond; d == 0 {
				return b.buf
			}
		}
		if d >= time.Microsecond {
			b.buf = strconv.AppendInt(b.buf, int64(d)/int64(time.Microsecond), 10)
			b.buf = append(b.buf, "us"...)
			if d = d % time.Millisecond; d == 0 {
				return b.buf
			}
		}
		b.buf = strconv.AppendInt(b.buf, int64(d), 10)
		b.buf = append(b.buf, "ns"...)
	}
	return b.buf
}
func (b *Buffer) appendPointer(p uint64) []byte {
	if p == 0 {
		b.appendnil()
	} else {
		first := false
		b.buf = append(b.buf, "0x"...)
		for i := 0; i < 16; i++ {
			temp := (p << (i * 4)) >> 60
			if temp > 0 {
				first = true
			}
			switch temp {
			case 0:
				if first {
					b.buf = append(b.buf, '0')
				}
			case 1:
				b.buf = append(b.buf, '1')
			case 2:
				b.buf = append(b.buf, '2')
			case 3:
				b.buf = append(b.buf, '3')
			case 4:
				b.buf = append(b.buf, '4')
			case 5:
				b.buf = append(b.buf, '5')
			case 6:
				b.buf = append(b.buf, '6')
			case 7:
				b.buf = append(b.buf, '7')
			case 8:
				b.buf = append(b.buf, '8')
			case 9:
				b.buf = append(b.buf, '9')
			case 10:
				b.buf = append(b.buf, 'a')
			case 11:
				b.buf = append(b.buf, 'b')
			case 12:
				b.buf = append(b.buf, 'c')
			case 13:
				b.buf = append(b.buf, 'd')
			case 14:
				b.buf = append(b.buf, 'e')
			case 15:
				b.buf = append(b.buf, 'f')
			}
		}
	}
	return b.buf
}
func (b *Buffer) appendnil() {
	b.buf = append(b.buf, "nil"...)
}
func (b *Buffer) appendemptyslice() {
	b.buf = append(b.buf, "[]"...)
}
func (b *Buffer) appendemptyobj() {
	b.buf = append(b.buf, "{}"...)
}
func (b *Buffer) Len() int {
	return len(b.buf)
}
func (b *Buffer) Cap() int {
	return cap(b.buf)
}
func (b *Buffer) Grow(n uint64) {
	if uint64(cap(b.buf)) >= n {
		(*[3]uintptr)(unsafe.Pointer(&(b.buf)))[1] = uintptr(n)
	} else {
		b.buf = make([]byte, n)
	}
}
func (b *Buffer) Reset() {
	b.buf = b.buf[:0]
	b.offset = 0
}
func (b *Buffer) Bytes() []byte {
	return b.buf
}
func (b *Buffer) String() string {
	return common.Byte2str(b.buf)
}
func (b *Buffer) Read(p []byte) (n int, err error) {
	if b.Len() == 0 {
		if len(p) == 0 {
			return 0, nil
		}
		return 0, io.EOF
	}
	n = copy(p, b.buf[b.offset:])
	b.offset += n
	return n, nil
}
