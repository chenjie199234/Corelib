package bitfilter

import "github.com/chenjie199234/Corelib/common"

//thread unsafe

type MemoryBitFilter struct {
	bytelen  uint64
	bitlen   uint64
	data     [][]byte
	addednum uint64
}

func NewMemoryBitFilter(lengths uint64) *MemoryBitFilter {
	f := &MemoryBitFilter{
		bytelen:  lengths,
		bitlen:   lengths * 8,
		data:     make([][]byte, lengths),
		addednum: 0,
	}
	for i := uint64(0); i < lengths; i++ {
		f.data[i] = make([]byte, 6)
	}
	return f
}

// Export 导出所有的数据
func (f *MemoryBitFilter) Export() ([][]byte, uint64) {
	return f.data, f.addednum
}

// Rebuild 重建
func Rebuild(datas [][]byte, addednum uint64) *MemoryBitFilter {
	if len(datas) == 0 {
		return nil
	}
	for _, data := range datas {
		if len(data) != 6 {
			return nil
		}
	}
	return &MemoryBitFilter{
		bytelen:  uint64(len(datas)),
		bitlen:   uint64(len(datas) * 8),
		data:     datas,
		addednum: addednum,
	}
}

// Clear 重置该filter
func (f *MemoryBitFilter) Clear() {
	f.data = make([][]byte, f.bytelen)
	for i := uint64(0); i < f.bytelen; i++ {
		f.data[i] = make([]byte, 6)
	}
	f.addednum = 0
}

// Check 该方法不会将data加入filter中,只会返回key是否不在filter中
// 返回true,key确定100%不在filter中
// 返回false,key不确定是否存在,需要其他额外逻辑辅助,非filter的逻辑
func (f *MemoryBitFilter) Check(data []byte) bool {
	for i := 0; i < 6; i++ {
		bitnum := uint64(0)
		switch i {
		case 0:
			bitnum = common.BkdrhashByte(data, f.bitlen)
		case 1:
			bitnum = common.DjbhashByte(data, f.bitlen)
		case 2:
			bitnum = common.FnvhashByte(data, f.bitlen)
		case 3:
			bitnum = common.DekhashByte(data, f.bitlen)
		case 4:
			bitnum = common.RshashByte(data, f.bitlen)
		case 5:
			bitnum = common.SdbmhashByte(data, f.bitlen)
		}
		index := bitnum / 8
		offset := bitnum % 8
		set := (f.data[index][i] | (1 << offset)) == f.data[index][i]
		if !set {
			return true
		}
	}
	return false
}

// Set 该方法会将data加入filter中,并判断key之前是否加入过
//error不为nil时表示出错
//error为nil时执行成功
//返回true,布隆过滤器中并没有该key,加入成功,返回的第二个参数为改变的index的最新值
//返回false,布隆过滤器中可能已经存在该key,无法100%保证,请进行其他额外逻辑处理,返回的第二个参数肯定为空
func (f *MemoryBitFilter) Set(data []byte) (bool, map[uint64][]byte) {
	temp := make(map[uint64][]byte, 6)
	add := false
	for i := 0; i < 6; i++ {
		bitnum := uint64(0)
		switch i {
		case 0:
			bitnum = common.BkdrhashByte(data, f.bitlen)
		case 1:
			bitnum = common.DjbhashByte(data, f.bitlen)
		case 2:
			bitnum = common.FnvhashByte(data, f.bitlen)
		case 3:
			bitnum = common.DekhashByte(data, f.bitlen)
		case 4:
			bitnum = common.RshashByte(data, f.bitlen)
		case 5:
			bitnum = common.SdbmhashByte(data, f.bitlen)
		}
		index := bitnum / 8
		offset := bitnum % 8
		set := (f.data[index][i] | (1 << offset)) == f.data[index][i]
		if !set {
			add = true
			f.data[index][i] |= (1 << offset)
			temp[index] = f.data[index]
		}
	}
	f.addednum++
	return add, temp
}

func (f *MemoryBitFilter) GetFilterDataByIndex(index uint64) []byte {
	if index >= f.bytelen {
		return nil
	}
	return f.data[index]
}
