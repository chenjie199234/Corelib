package bitfilter

import "github.com/chenjie199234/Corelib/common"

//thread unsafe
type BitFilter struct {
	//length of data,each element of data has hash function nums byte
	bytelen  uint64
	bitlen   uint64
	data     [][]byte
	addednum uint64
}

//create new filter
func New(lengths uint64) *BitFilter {
	f := new(BitFilter)
	f.bytelen = lengths
	f.bitlen = lengths * 8
	f.data = make([][]byte, lengths)
	for i := uint64(0); i < lengths; i++ {
		f.data[i] = make([]byte, 6)
	}
	f.addednum = 0
	return f
}

//rebuild this filter
func Rebuild(datas [][]byte, addednum uint64) *BitFilter {
	return &BitFilter{
		bytelen:  uint64(len(datas)),
		bitlen:   uint64(len(datas) * 8),
		data:     datas,
		addednum: addednum,
	}
}

//clear this filter
func (f *BitFilter) Clear() {
	f.data = make([][]byte, f.bytelen)
	for i := uint64(0); i < f.bytelen; i++ {
		f.data[i] = make([]byte, 6)
	}
	f.addednum = 0
}

//check is in this filter
func (f *BitFilter) IsAdd(txhash []byte) bool {
	for i := 0; i < len(f.data[0]); i++ {
		bitnum := uint64(0)
		switch i {
		case 0:
			bitnum = common.BkdrhashByte(txhash, f.bitlen)
		case 1:
			bitnum = common.DjbhashByte(txhash, f.bitlen)
		case 2:
			bitnum = common.FnvhashByte(txhash, f.bitlen)
		case 3:
			bitnum = common.DekhashByte(txhash, f.bitlen)
		case 4:
			bitnum = common.RshashByte(txhash, f.bitlen)
		case 5:
			bitnum = common.SdbmhashByte(txhash, f.bitlen)
		}
		index := bitnum / 8
		offset := bitnum % 8
		set := (f.data[index][i] | (1 << offset)) == f.data[index][i]
		if !set {
			return false
		}
	}
	return true
}

//add data into this filter
//return result
//key:changed index
//value:current value
func (f *BitFilter) Add(txhashs [][]byte) map[uint64][]byte {
	temp := make(map[uint64][]byte)
	for _, txhash := range txhashs {
		for i := 0; i < len(f.data[0]); i++ {
			bitnum := uint64(0)
			switch i {
			case 0:
				bitnum = common.BkdrhashByte(txhash, f.bitlen)
			case 1:
				bitnum = common.DjbhashByte(txhash, f.bitlen)
			case 2:
				bitnum = common.FnvhashByte(txhash, f.bitlen)
			case 3:
				bitnum = common.DekhashByte(txhash, f.bitlen)
			case 4:
				bitnum = common.RshashByte(txhash, f.bitlen)
			case 5:
				bitnum = common.SdbmhashByte(txhash, f.bitlen)
			}
			index := bitnum / 8
			offset := bitnum % 8
			f.data[index][i] |= (1 << offset)
			temp[index] = f.data[index]
		}
		f.addednum++
	}
	return temp
}

//how many data in this filter
func (f *BitFilter) GetAddedNum() uint64 {
	return f.addednum
}
func (f *BitFilter) GetByteLength() uint64 {
	return f.bytelen
}
func (f *BitFilter) GetBitLength() uint64 {
	return f.bitlen
}

//return
//first:all data
//second:added num
func (f *BitFilter) GetAllFilterData() ([][]byte, uint64) {
	return f.data, f.addednum
}
func (f *BitFilter) GetFilterDataByIndex(index uint64) []byte {
	if index >= f.bytelen {
		return nil
	}
	return f.data[index]
}
