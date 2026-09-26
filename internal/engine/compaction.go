package engine

import (
	"bufio"
	"container/heap"
	"encoding/json"
	"io"
	"os"
)

type Iterator struct {
	Records []KV
	pos     int
}

type heapItem struct {
	record KV
	itrIdx int
}

func (it *Iterator) Peek() (KV, bool) {
	if it.pos >= len(it.Records) {
		return KV{}, false
	}

	return it.Records[it.pos], true
}

func (it *Iterator) Next() { it.pos++ }

type minHeap []heapItem

func (h minHeap) Len() int { return len(h) }
func (h minHeap) Less(i, j int) bool {
	if h[i].record.Key < h[j].record.Key {
		return true
	} else if h[i].record.Key > h[j].record.Key {
		return false
	}

	return h[i].itrIdx > h[j].itrIdx
}
func (h minHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)   { *h = append(*h, x.(heapItem)) }
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func (e *Engine) Compaction() error {
	file, err := os.Open(e.Manifest)
	if err != nil {
		return err
	}
	defer file.Close()

	iters := make([]*Iterator, 0, 100)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		records, err := parseSSTRecords(scanner.Text())
		if err != nil {
			return err
		}

		iters = append(iters, &Iterator{Records: records})
	}

	h := &minHeap{}

	for i, iter := range iters {
		if rec, ok := iter.Peek(); ok {
			heap.Push(h, heapItem{record: rec, itrIdx: i})
		}
	}

	tempStore := make(map[string]Value, 2000)

	for h.Len() > 0 {
		newestItem := heap.Pop(h).(heapItem)
		for h.Len() > 0 && (*h)[0].record.Key == newestItem.record.Key {
			dup := heap.Pop(h).(heapItem)
			it := iters[dup.itrIdx]
			it.Next()
			if rec, ok := it.Peek(); ok {
				heap.Push(h, heapItem{record: rec, itrIdx: dup.itrIdx})
			}
		}

		if newestItem.record.Deleted {
			continue
		}

		tempStore[newestItem.record.Key] = Value{value: newestItem.record.Value, op: OpPut}
		it := iters[newestItem.itrIdx]
		it.Next()
		if rec, ok := it.Peek(); ok {
			heap.Push(h, heapItem{record: rec, itrIdx: newestItem.itrIdx})
		}
	}

	return nil
}

func parseSSTRecords(filename string) ([]KV, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var records []KV
	err = json.Unmarshal(data, &records)
	if err != nil {
		return nil, err
	}

	return records, nil
}
