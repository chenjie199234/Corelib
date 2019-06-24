package bitfilter

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
func Rebuild(length uint64, datas [][]byte, addednum uint64) *BitFilter {
	if uint64(len(datas)) != length {
		return nil
	}
	return &BitFilter{
		bytelen:  length,
		bitlen:   length * 8,
		data:     datas,
		addednum: addednum,
	}
}

//check is in this filter
func (f *BitFilter) IsAdd(txhash []byte) bool {
	for i := 0; i < len(f.data[0]); i++ {
		bitnum := uint64(0)
		switch i {
		case 0:
			bitnum = f.bkdrhash_byte(txhash)
		case 1:
			bitnum = f.djbhash_byte(txhash)
		case 2:
			bitnum = f.fnvhash_byte(txhash)
		case 3:
			bitnum = f.dekhash_byte(txhash)
		case 4:
			bitnum = f.rshash_byte(txhash)
		case 5:
			bitnum = f.sdbmhash_byte(txhash)
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
				bitnum = f.bkdrhash_byte(txhash)
			case 1:
				bitnum = f.djbhash_byte(txhash)
			case 2:
				bitnum = f.fnvhash_byte(txhash)
			case 3:
				bitnum = f.dekhash_byte(txhash)
			case 4:
				bitnum = f.rshash_byte(txhash)
			case 5:
				bitnum = f.sdbmhash_byte(txhash)
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
func (f *BitFilter) GetByIndex(index uint64) []byte {
	if index >= f.bytelen {
		return nil
	}
	return f.data[index]
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

//clear this filter
func (f *BitFilter) Clear() {
	f.data = make([][]byte, f.bytelen)
	for i := uint64(0); i < f.bytelen; i++ {
		f.data[i] = make([]byte, 6)
	}
	f.addednum = 0
}

//return
//first:all data
//second:added num
func (f *BitFilter) Export() ([][]byte, uint64) {
	return f.data, f.addednum
}

//hash
func (f *BitFilter) bkdrhash_byte(data []byte) uint64 {
	seed := uint64(131)
	hash := uint64(0)
	for _, v := range data {
		hash = hash*seed + uint64(v)
	}
	return hash % f.bitlen
}
func (f *BitFilter) djbhash_byte(data []byte) uint64 {
	hash := uint64(5381)
	for _, v := range data {
		hash = (hash << 5) + uint64(v)
	}
	return hash % f.bitlen
}
func (f *BitFilter) fnvhash_byte(data []byte) uint64 {
	hash := uint64(2166136261)
	for _, v := range data {
		hash *= uint64(16777619)
		hash ^= uint64(v)
	}
	return hash % f.bitlen
}
func (f *BitFilter) dekhash_byte(data []byte) uint64 {
	hash := uint64(len(data))
	for _, v := range data {
		hash = ((hash << 5) ^ (hash >> 27)) ^ uint64(v)
	}
	return hash % f.bitlen
}
func (f *BitFilter) rshash_byte(data []byte) uint64 {
	seed := uint64(63689)
	hash := uint64(0)
	for _, v := range data {
		hash = hash*seed + uint64(v)
		seed *= uint64(378551)
	}
	return hash % f.bitlen
}
func (f *BitFilter) sdbmhash_byte(data []byte) uint64 {
	hash := uint64(0)
	for _, v := range data {
		hash = uint64(v) + (hash << 6) + (hash << 16) - hash
	}
	return hash % f.bitlen
}
