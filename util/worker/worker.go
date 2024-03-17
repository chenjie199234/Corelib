package worker

import (
	"github.com/chenjie199234/Corelib/container/list"
)

type Worker struct {
	list *list.BlockList[func()]
}

func NewWorker(num uint16) *Worker {
	w := &Worker{
		list: list.NewBlockList[func()](),
	}
	for i := uint16(0); i < num; i++ {
		go func() {
			for {
				f, ok := w.list.Pop()
				if !ok {
					return
				}
				if f != nil {
					f()
				}
			}
		}()
	}
	return w
}
func (w *Worker) Do(f func()) {
	w.list.Push(f)
}
