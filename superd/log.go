package superd

type logformat struct {
	Time    string `json:"time"`
	Superd  string `json:"superd"`
	Group   string `json:"group"`
	Lpid    uint64 `json:"lpid"`
	Ppid    uint64 `json:"ppid"`
	Version string `json:"version"`
	Log     string `json:"log"`
}
