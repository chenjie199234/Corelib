package bufpool

import (
	"reflect"
	"strconv"
	"sync"
	"time"
	"unsafe"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/util/common"
	ctime "github.com/chenjie199234/Corelib/util/time"
)

var pool *sync.Pool

func init() {
	pool = &sync.Pool{}
}

type Buffer []byte

func GetBuffer() *Buffer {
	b, ok := pool.Get().(*Buffer)
	if !ok {
		temp := Buffer(make([]byte, 0, 258))
		return &temp
	}
	*b = (*b)[:0]
	return b
}
func PutBuffer(b *Buffer) {
	pool.Put(b)
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
func (b *Buffer) Resize(n uint32) {
	if uint32(cap(*b)) >= n {
		(*[3]uintptr)(unsafe.Pointer(b))[1] = uintptr(n)
	} else {
		*b = make([]byte, n)
	}
}
func (b *Buffer) Bytes() []byte {
	return *b
}
func (b *Buffer) String() string {
	return common.Byte2str(*b)
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
		*b = strconv.AppendBool(*b, d)
	case *bool:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendBool(*b, *d)
		}
	case []bool:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendBool(*b, dd)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*bool:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendBool(*b, *dd)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case int:
		*b = strconv.AppendInt(*b, int64(d), 10)
	case int8:
		*b = strconv.AppendInt(*b, int64(d), 10)
	case int16:
		*b = strconv.AppendInt(*b, int64(d), 10)
	case int32:
		*b = strconv.AppendInt(*b, int64(d), 10)
	case int64:
		*b = strconv.AppendInt(*b, d, 10)
	case *int:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendInt(*b, int64(*d), 10)
		}
	case *int8:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendInt(*b, int64(*d), 10)
		}
	case *int16:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendInt(*b, int64(*d), 10)
		}
	case *int32:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendInt(*b, int64(*d), 10)
		}
	case *int64:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendInt(*b, *d, 10)
		}
	case []int:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendInt(*b, int64(dd), 10)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []int8:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendInt(*b, int64(dd), 10)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []int16:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendInt(*b, int64(dd), 10)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []int32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendInt(*b, int64(dd), 10)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []int64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendInt(*b, dd, 10)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*int:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendInt(*b, int64(*dd), 10)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*int8:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendInt(*b, int64(*dd), 10)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*int16:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendInt(*b, int64(*dd), 10)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*int32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendInt(*b, int64(*dd), 10)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*int64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendInt(*b, *dd, 10)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case uint:
		*b = strconv.AppendUint(*b, uint64(d), 10)
	case uint8:
		*b = strconv.AppendUint(*b, uint64(d), 10)
	case uint16:
		*b = strconv.AppendUint(*b, uint64(d), 10)
	case uint32:
		*b = strconv.AppendUint(*b, uint64(d), 10)
	case uint64:
		*b = strconv.AppendUint(*b, d, 10)
	case *uint:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendUint(*b, uint64(*d), 10)
		}
	case *uint8:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendUint(*b, uint64(*d), 10)
		}
	case *uint16:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendUint(*b, uint64(*d), 10)
		}
	case *uint32:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendUint(*b, uint64(*d), 10)
		}
	case *uint64:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendUint(*b, *d, 10)
		}
	case []uint:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendUint(*b, uint64(dd), 10)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []uint8:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendUint(*b, uint64(dd), 10)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []uint16:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendUint(*b, uint64(dd), 10)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []uint32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendUint(*b, uint64(dd), 10)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []uint64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendUint(*b, dd, 10)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uint:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendUint(*b, uint64(*dd), 10)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uint8:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendUint(*b, uint64(*dd), 10)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uint16:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendUint(*b, uint64(*dd), 10)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uint32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendUint(*b, uint64(*dd), 10)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uint64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendUint(*b, *dd, 10)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case float32:
		*b = strconv.AppendFloat(*b, float64(d), 'f', -1, 32)
	case float64:
		*b = strconv.AppendFloat(*b, d, 'f', -1, 64)
	case *float32:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendFloat(*b, float64(*d), 'f', -1, 32)
		}
	case *float64:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendFloat(*b, *d, 'f', -1, 64)
		}
	case []float32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendFloat(*b, float64(dd), 'f', -1, 32)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []float64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendFloat(*b, float64(dd), 'f', -1, 64)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*float32:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendFloat(*b, float64(*dd), 'f', -1, 32)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*float64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendFloat(*b, float64(*dd), 'f', -1, 64)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case complex64:
		*b = strconv.AppendFloat(*b, float64(real(d)), 'f', -1, 32)
		if imag(d) >= 0 {
			*b = append(*b, '+')
		}
		*b = strconv.AppendFloat(*b, float64(imag(d)), 'f', -1, 32)
		*b = append(*b, 'i')
	case complex128:
		*b = strconv.AppendFloat(*b, real(d), 'f', -1, 32)
		if imag(d) >= 0 {
			*b = append(*b, '+')
		}
		*b = strconv.AppendFloat(*b, imag(d), 'f', -1, 32)
		*b = append(*b, 'i')
	case *complex64:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendFloat(*b, float64(real(*d)), 'f', -1, 32)
			if imag(*d) >= 0 {
				*b = append(*b, '+')
			}
			*b = strconv.AppendFloat(*b, float64(imag(*d)), 'f', -1, 32)
			*b = append(*b, 'i')
		}
	case *complex128:
		if d == nil {
			b.appendnil()
		} else {
			*b = strconv.AppendFloat(*b, real(*d), 'f', -1, 32)
			if imag(*d) >= 0 {
				*b = append(*b, '+')
			}
			*b = strconv.AppendFloat(*b, imag(*d), 'f', -1, 32)
			*b = append(*b, 'i')
		}
	case []complex64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendFloat(*b, float64(real(dd)), 'f', -1, 32)
				if imag(dd) >= 0 {
					*b = append(*b, '+')
				}
				*b = strconv.AppendFloat(*b, float64(imag(dd)), 'f', -1, 32)
				*b = append(*b, 'i', ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []complex128:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = strconv.AppendFloat(*b, real(dd), 'f', -1, 32)
				if imag(dd) >= 0 {
					*b = append(*b, '+')
				}
				*b = strconv.AppendFloat(*b, imag(dd), 'f', -1, 32)
				*b = append(*b, 'i', ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*complex64:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendFloat(*b, float64(real(*dd)), 'f', -1, 32)
					if imag(*dd) >= 0 {
						*b = append(*b, '+')
					}
					*b = strconv.AppendFloat(*b, float64(imag(*dd)), 'f', -1, 32)
					*b = append(*b, 'i')
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*complex128:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = strconv.AppendFloat(*b, real(*dd), 'f', -1, 32)
					if imag(*dd) >= 0 {
						*b = append(*b, '+')
					}
					*b = strconv.AppendFloat(*b, imag(*dd), 'f', -1, 32)
					*b = append(*b, 'i')
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case string:
		*b = append(*b, d...)
	case *string:
		if d == nil {
			b.appendnil()
		} else {
			*b = append(*b, *d...)
		}
	case []string:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = append(*b, dd...)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*string:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = append(*b, *dd...)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case uintptr:
		*b = b.appendPointer(uint64(d))
	case unsafe.Pointer:
		*b = b.appendPointer(uint64(uintptr(d)))
	case *uintptr:
		if d == nil {
			b.appendnil()
		} else {
			*b = b.appendPointer(uint64(*d))
		}
	case *unsafe.Pointer:
		if d == nil {
			b.appendnil()
		} else {
			*b = b.appendPointer(uint64(uintptr(*d)))
		}
	case []uintptr:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = b.appendPointer(uint64(dd))
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []unsafe.Pointer:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = b.appendPointer(uint64(uintptr(dd)))
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*uintptr:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = b.appendPointer(uint64(*dd))
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*unsafe.Pointer:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = b.appendPointer(uint64(uintptr(*dd)))
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case time.Duration:
		*b = b.appendDuration(d)
	case ctime.Duration:
		*b = b.appendDuration(time.Duration(d))
	case *time.Duration:
		if d == nil {
			b.appendnil()
		} else {
			*b = b.appendDuration(*d)
		}
	case *ctime.Duration:
		if d == nil {
			b.appendnil()
		} else {
			*b = b.appendDuration(time.Duration(*d))
		}
	case []time.Duration:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = b.appendDuration(dd)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []ctime.Duration:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = b.appendDuration(time.Duration(dd))
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*time.Duration:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = b.appendDuration(*dd)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*ctime.Duration:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = b.appendDuration(time.Duration(*dd))
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case time.Time:
		*b = d.AppendFormat(*b, "2006-01-02 15:04:05.000000000 -07")
	case ctime.Time:
		*b = time.Time(d).AppendFormat(*b, "2006-01-02 15:04:05.000000000 -07")
	case *time.Time:
		if d == nil {
			b.appendnil()
		} else {
			*b = (*d).AppendFormat(*b, "2006-01-02 15:04:05.000000000 -07")
		}
	case *ctime.Time:
		if d == nil {
			b.appendnil()
		} else {
			*b = time.Time(*d).AppendFormat(*b, "2006-01-02 15:04:05.000000000 -07")
		}
	case []time.Time:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = dd.AppendFormat(*b, "2006-01-02 15:04:05.000000000 -07")
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []ctime.Time:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				*b = time.Time(dd).AppendFormat(*b, "2006-01-02 15:04:05.000000000 -07")
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*time.Time:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = (*dd).AppendFormat(*b, "2006-01-02 15:04:05.000000000 -07")
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*ctime.Time:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					*b = time.Time(*dd).AppendFormat(*b, "2006-01-02 15:04:05.000000000 -07")
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case error:
		switch dd := d.(type) {
		case *cerror.Error:
			if dd == nil {
				b.appendnil()
			} else {
				b.Append(*dd)
			}
		case cerror.Error:
			*b = append(*b, "{Code:"...)
			*b = strconv.AppendInt(*b, int64(dd.Code), 10)
			*b = append(*b, ",Msg:"...)
			*b = append(*b, dd.Msg...)
			*b = append(*b, '}')
		default:
			if d == nil {
				b.appendnil()
			} else {
				*b = append(*b, d.Error()...)
			}
		}
	case *error:
		if d == nil {
			b.appendnil()
		} else {
			b.Append(*d)
		}
	case []error:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				b.Append(dd)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []cerror.Error:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				b.Append(dd)
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*error:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.Append(*dd)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case []*cerror.Error:
		if d == nil {
			b.appendnil()
		} else if len(d) > 0 {
			*b = append(*b, '[')
			for _, dd := range d {
				if dd == nil {
					b.appendnil()
				} else {
					b.Append(*dd)
				}
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
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
		*b = strconv.AppendBool(*b, d.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if d.Type().Name() == "Duration" {
			*b = b.appendDuration(*(*time.Duration)(unsafe.Pointer(d.Addr().Pointer())))
		} else {
			*b = strconv.AppendInt(*b, d.Int(), 10)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		*b = strconv.AppendUint(*b, d.Uint(), 10)
	case reflect.Float32:
		*b = strconv.AppendFloat(*b, d.Float(), 'f', -1, 32)
	case reflect.Float64:
		*b = strconv.AppendFloat(*b, d.Float(), 'f', -1, 64)
	case reflect.Complex64:
		c := d.Complex()
		*b = strconv.AppendFloat(*b, float64(real(c)), 'f', -1, 32)
		if imag(c) >= 0 {
			*b = append(*b, '+')
		}
		*b = strconv.AppendFloat(*b, float64(imag(c)), 'f', -1, 32)
		*b = append(*b, 'i')
	case reflect.Complex128:
		c := d.Complex()
		*b = strconv.AppendFloat(*b, float64(real(c)), 'f', -1, 64)
		if imag(c) >= 0 {
			*b = append(*b, '+')
		}
		*b = strconv.AppendFloat(*b, float64(imag(c)), 'f', -1, 64)
		*b = append(*b, 'i')
	case reflect.String:
		*b = append(*b, d.String()...)
	case reflect.Interface:
		if d.IsNil() {
			b.appendnil()
		} else if d.Type().String() == "error" {
			b.appendbasic(d.Interface())
		} else {
			b.appendreflect(d.Elem())
		}
	case reflect.Array:
		if d.Len() > 0 {
			*b = append(*b, '[')
			for i := 0; i < d.Len(); i++ {
				b.appendreflect(d.Index(i))
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case reflect.Slice:
		if d.IsNil() {
			b.appendnil()
		} else if d.Len() > 0 {
			*b = append(*b, '[')
			for i := 0; i < d.Len(); i++ {
				b.appendreflect(d.Index(i))
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = ']'
		} else {
			b.appendemptyslice()
		}
	case reflect.Map:
		d.IsZero()
		if d.IsNil() {
			b.appendnil()
		} else if d.Len() > 0 {
			*b = append(*b, '{')
			iter := d.MapRange()
			for iter.Next() {
				b.appendreflect(iter.Key())
				*b = append(*b, ':')
				b.appendreflect(iter.Value())
				*b = append(*b, ',')
			}
			(*b)[len(*b)-1] = '}'
		} else {
			b.appendemptyobj()
		}
	case reflect.Struct:
		if d.Type().String() == "time.Time" {
			t := (*time.Time)(unsafe.Pointer(d.Addr().Pointer()))
			b.appendbasic(t)
		} else if d.Type().String() == "github.com/chenjie199234/Corelib/error.Error" {
			e := (*cerror.Error)(unsafe.Pointer(d.Addr().Pointer()))
			b.appendbasic(*e)
		} else if d.NumField() > 0 {
			has := false
			*b = append(*b, '{')
			t := d.Type()
			for i := 0; i < d.NumField(); i++ {
				if t.Field(i).Name[0] >= 65 && t.Field(i).Name[0] <= 90 {
					has = true
					*b = append(*b, t.Field(i).Name...)
					*b = append(*b, ':')
					b.appendreflect(d.Field(i))
					*b = append(*b, ',')
				}
			}
			if has {
				(*b)[len(*b)-1] = '}'
			} else {
				*b = append(*b, '}')
			}
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
		*b = b.appendPointer(d.Uint())
	case reflect.UnsafePointer:
		if d.IsNil() {
			b.appendnil()
		} else {
			*b = b.appendPointer(uint64(d.Pointer()))
		}
	case reflect.Func:
		if d.IsNil() {
			b.appendnil()
		} else {
			*b = b.appendPointer(uint64(d.Pointer()))
		}
	case reflect.Chan:
		if d.IsNil() {
			b.appendnil()
		} else {
			*b = b.appendPointer(uint64(d.Pointer()))
		}
	default:
		*b = append(*b, "unsupported type"...)
	}
}
func (b *Buffer) appendDuration(d time.Duration) []byte {
	if d == 0 {
		*b = append(*b, "0s"...)
	} else {
		if d >= time.Hour {
			*b = strconv.AppendInt(*b, int64(d)/int64(time.Hour), 10)
			*b = append(*b, 'h')
			if d = d % time.Hour; d == 0 {
				return *b
			}
		}
		if d >= time.Minute {
			*b = strconv.AppendInt(*b, int64(d)/int64(time.Minute), 10)
			*b = append(*b, 'm')
			if d = d % time.Minute; d == 0 {
				return *b
			}
		}
		if d >= time.Second {
			*b = strconv.AppendInt(*b, int64(d)/int64(time.Second), 10)
			*b = append(*b, 's')
			if d = d % time.Second; d == 0 {
				return *b
			}
		}
		if d >= time.Millisecond {
			*b = strconv.AppendInt(*b, int64(d)/int64(time.Millisecond), 10)
			*b = append(*b, "ms"...)
			if d = d % time.Millisecond; d == 0 {
				return *b
			}
		}
		if d >= time.Microsecond {
			*b = strconv.AppendInt(*b, int64(d)/int64(time.Microsecond), 10)
			*b = append(*b, "us"...)
			if d = d % time.Millisecond; d == 0 {
				return *b
			}
		}
		*b = strconv.AppendInt(*b, int64(d), 10)
		*b = append(*b, "ns"...)
	}
	return *b
}
func (b *Buffer) appendPointer(p uint64) []byte {
	if p == 0 {
		b.appendnil()
	} else {
		first := false
		*b = append(*b, "0x"...)
		for i := 0; i < 16; i++ {
			temp := (p << (i * 4)) >> 60
			if temp > 0 {
				first = true
			}
			switch temp {
			case 0:
				if first {
					*b = append(*b, '0')
				}
			case 1:
				*b = append(*b, '1')
			case 2:
				*b = append(*b, '2')
			case 3:
				*b = append(*b, '3')
			case 4:
				*b = append(*b, '4')
				*b = append(*b, 'f')
				*b = append(*b, '5')
			case 6:
				*b = append(*b, '6')
			case 7:
				*b = append(*b, '7')
			case 8:
				*b = append(*b, '8')
			case 9:
				*b = append(*b, '9')
			case 10:
				*b = append(*b, 'a')
			case 11:
				*b = append(*b, 'b')
			case 12:
				*b = append(*b, 'c')
			case 13:
				*b = append(*b, 'd')
			case 14:
				*b = append(*b, 'e')
			case 15:
				*b = append(*b, 'f')
			}
		}
	}
	return *b
}
func (b *Buffer) appendnil() {
	*b = append(*b, "nil"...)
}
func (b *Buffer) appendemptyslice() {
	*b = append(*b, "[]"...)
}
func (b *Buffer) appendemptyobj() {
	*b = append(*b, "{}"...)
}
